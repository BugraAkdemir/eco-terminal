package rag

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// IndexResult describes what an Index* call did.
type IndexResult struct {
	DocumentID string
	Chunks     int
	Skipped    bool // content unchanged (content_hash) - nothing rewritten
	Removed    bool // document is not indexable (draft/rejected/deleted/disabled) and was removed
	Embedded   bool // vectors were stored too
	EmbedErr   error
}

type docSpec struct {
	SourceType string
	SourceID   string
	Title      string
	URL        string
	RegionID   string
	Lat, Lon   *float64
	DocDate    string
	Visibility string
	OwnerID    string
	Text       string
}

func (d docSpec) hash() string {
	h := sha256.New()
	for _, p := range []string{d.SourceType, d.SourceID, d.Title, d.URL, d.RegionID, d.DocDate, d.Visibility, d.OwnerID, d.Text,
		fmt.Sprint(ptrVal(d.Lat)), fmt.Sprint(ptrVal(d.Lon))} {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func ptrVal(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// ─── observations ──────────────────────────────────────────────────

var categoryTR = map[string]string{
	"animal": "hayvan", "plant": "bitki", "water": "su", "forest": "orman", "pollution": "kirlilik",
	"injured_wildlife": "yaralı yaban hayvanı", "other": "diğer",
}

var statusTR = map[string]string{
	"submitted": "gönderildi", "ai_processed": "yapay zekâ ön incelemesi yapıldı (doğrulanmadı)", "under_review": "incelemede",
	"needs_information": "ek bilgi bekleniyor", "verified": "doğrulandı", "resolved": "çözüldü",
}

// observationVisibility maps a lifecycle status to (visibility, indexable).
//
//	verified|resolved                                   -> public
//	submitted|ai_processed|under_review|needs_information -> staff (+ owner_user_id, so the owner sees own records)
//	draft|rejected, soft-deleted                         -> not indexed
func observationVisibility(status string, deleted bool) (string, bool) {
	if deleted {
		return "", false
	}
	switch status {
	case "verified", "resolved":
		return "public", true
	case "submitted", "ai_processed", "under_review", "needs_information":
		return "staff", true
	}
	return "", false
}

// IndexObservation (re)indexes one observation according to its status/visibility, or removes it from the index.
// Only PUBLIC coordinates are ever stored; for sensitive records the place is generalized to district level.
func (e *Engine) IndexObservation(ctx context.Context, id string) (IndexResult, error) {
	var (
		userID, recType, category, title, desc, guess, verified, observedAt, status, primaryRegion string
		count                                                                                      sql.NullInt64
		pubLat, pubLon                                                                             float64
		sensitive, demo                                                                            int
		deletedAt                                                                                  sql.NullString
	)
	err := e.DB.QueryRowContext(ctx, `SELECT user_id, record_type, category, title, description, species_guess, species_verified,
		count_observed, public_lat, public_lon, observed_at, status, is_sensitive, is_demo, COALESCE(primary_region_id,''), deleted_at
		FROM observations WHERE id = ?`, id).Scan(&userID, &recType, &category, &title, &desc, &guess, &verified,
		&count, &pubLat, &pubLon, &observedAt, &status, &sensitive, &demo, &primaryRegion, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return e.RemoveDocument(ctx, "observation", id)
	}
	if err != nil {
		return IndexResult{}, err
	}
	vis, ok := observationVisibility(status, deletedAt.Valid)
	if !ok {
		return e.RemoveDocument(ctx, "observation", id)
	}

	// place names: village-level detail only for non-sensitive records
	regionID, placeNames, err := e.regionChain(ctx, primaryRegion, sensitive == 1)
	if err != nil {
		return IndexResult{}, err
	}

	var sb strings.Builder
	line := func(label, v string) {
		if strings.TrimSpace(v) != "" {
			sb.WriteString(label + ": " + strings.TrimSpace(v) + "\n")
		}
	}
	kind := "Gözlem"
	if recType == "issue" {
		kind = "Sorun bildirimi"
	}
	line("Başlık", title)
	line("Kayıt türü", kind)
	line("Kategori", categoryTR[category]+" ("+category+")")
	if verified != "" {
		line("Tür (uzman doğrulaması)", verified)
	} else if guess != "" {
		line("Tür (kullanıcı tahmini, doğrulanmadı)", guess)
	}
	if count.Valid {
		line("Gözlenen sayı", fmt.Sprint(count.Int64))
	}
	line("Açıklama", desc)
	line("Bölge", strings.Join(placeNames, ", "))
	line("Gözlem tarihi", observedAt[:min(10, len(observedAt))])
	line("Durum", statusTR[status])
	if demo == 1 {
		sb.WriteString("Not: demo (örnek) veridir, gerçek gözlem değildir.\n")
	}
	spec := docSpec{
		SourceType: "observation", SourceID: id, Title: firstNonEmpty(title, verified, guess, "Gözlem"),
		RegionID: regionID, Lat: &pubLat, Lon: &pubLon, DocDate: observedAt, Visibility: vis, Text: sb.String(),
	}
	if vis != "public" {
		spec.OwnerID = userID
	}
	return e.upsertDocument(ctx, spec, false)
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// regionChain returns the region id to attach to the document and the human-readable place names (most specific
// first). With generalize=true village/grid/custom regions are skipped.
func (e *Engine) regionChain(ctx context.Context, regionID string, generalize bool) (string, []string, error) {
	if regionID == "" {
		return "", nil, nil
	}
	rows, err := e.DB.QueryContext(ctx, `WITH RECURSIVE up(id,name,kind,parent_id,depth) AS (
		SELECT id,name,kind,COALESCE(parent_id,''),0 FROM geographic_regions WHERE id = ?
		UNION ALL SELECT g.id,g.name,g.kind,COALESCE(g.parent_id,''),up.depth+1 FROM geographic_regions g JOIN up ON g.id = up.parent_id)
		SELECT id,name,kind FROM up ORDER BY depth`, regionID)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var chosen string
	var names []string
	for rows.Next() {
		var id, name, kind string
		if err := rows.Scan(&id, &name, &kind); err != nil {
			return "", nil, err
		}
		if generalize && kind != "district" && kind != "province" {
			continue
		}
		if chosen == "" {
			chosen = id
		}
		names = append(names, name)
	}
	return chosen, names, rows.Err()
}

// ─── articles ──────────────────────────────────────────────────────

// IndexArticle indexes an rss_articles row as a public document (or removes it when its source is disabled).
func (e *Engine) IndexArticle(ctx context.Context, id string) error {
	_, err := e.IndexArticleResult(ctx, id)
	return err
}

// IndexArticleResult is IndexArticle with details.
func (e *Engine) IndexArticleResult(ctx context.Context, id string) (IndexResult, error) {
	var title, summary, url, ingested, note, srcName, org string
	var published sql.NullString
	var enabled int
	err := e.DB.QueryRowContext(ctx, `SELECT a.title, a.summary, a.url, a.published_at, a.ingested_at, a.location_note, s.name, s.organization, s.enabled
		FROM rss_articles a JOIN rss_sources s ON s.id = a.source_id WHERE a.id = ?`, id).
		Scan(&title, &summary, &url, &published, &ingested, &note, &srcName, &org, &enabled)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && enabled == 0) {
		return e.RemoveDocument(ctx, "rss_article", id)
	}
	if err != nil {
		return IndexResult{}, err
	}
	rows, err := e.DB.QueryContext(ctx, `SELECT g.id, g.name, g.kind FROM rss_article_regions ar JOIN geographic_regions g ON g.id = ar.region_id WHERE ar.article_id = ?`, id)
	if err != nil {
		return IndexResult{}, err
	}
	var names []string
	best, bestRank := "", 99
	rank := map[string]int{"village": 0, "district": 1, "province": 2}
	for rows.Next() {
		var rid, name, kind string
		if err := rows.Scan(&rid, &name, &kind); err != nil {
			rows.Close()
			return IndexResult{}, err
		}
		names = append(names, name)
		if r, ok := rank[kind]; ok && r < bestRank {
			best, bestRank = rid, r
		}
	}
	rows.Close()
	var sb strings.Builder
	sb.WriteString("Başlık: " + title + "\n")
	if summary != "" {
		sb.WriteString(summary + "\n")
	}
	sb.WriteString("Kaynak: " + srcName)
	if org != "" {
		sb.WriteString(" (" + org + ")")
	}
	sb.WriteString("\n")
	if len(names) > 0 {
		sb.WriteString("Konum: " + strings.Join(names, ", ") + "\n")
	} else if note != "" {
		sb.WriteString(note + "\n")
	}
	date := ingested
	if published.Valid {
		date = published.String
	}
	return e.upsertDocument(ctx, docSpec{SourceType: "rss_article", SourceID: id, Title: title, URL: url, RegionID: best,
		DocDate: date, Visibility: "public", Text: sb.String()}, false)
}

// ─── manual documents ──────────────────────────────────────────────

// ManualDoc is an admin-provided reference text (e.g. a scientific report, an MGM bulletin transcribed by hand).
type ManualDoc struct {
	SourceType string // manual (default) | scientific | region_report
	Source     string // stable identifier; re-indexing the same Source replaces the document
	Title      string
	URL        string
	Text       string
	RegionID   string
	Date       string // optional YYYY-MM-DD
}

// IndexManualDocument indexes a public reference document.
func (e *Engine) IndexManualDocument(ctx context.Context, d ManualDoc) (IndexResult, error) {
	if strings.TrimSpace(d.Source) == "" || strings.TrimSpace(d.Text) == "" {
		return IndexResult{}, errors.New("rag: manual document needs a source id and text")
	}
	st := d.SourceType
	if st == "" {
		st = "manual"
	}
	if st != "manual" && st != "scientific" && st != "region_report" {
		return IndexResult{}, fmt.Errorf("rag: invalid manual source_type %q", st)
	}
	return e.upsertDocument(ctx, docSpec{SourceType: st, SourceID: d.Source, Title: d.Title, URL: d.URL, RegionID: d.RegionID,
		DocDate: d.Date, Visibility: "public", Text: d.Title + "\n" + d.Text}, false)
}

// ─── core upsert / remove ──────────────────────────────────────────

// RemoveDocument deletes a document with its chunks, FTS rows (via triggers) and vectors.
func (e *Engine) RemoveDocument(ctx context.Context, sourceType, sourceID string) (IndexResult, error) {
	var docID string
	err := e.DB.QueryRowContext(ctx, `SELECT id FROM rag_documents WHERE source_type=? AND source_id=?`, sourceType, sourceID).Scan(&docID)
	if errors.Is(err, sql.ErrNoRows) {
		return IndexResult{Removed: true}, nil
	}
	if err != nil {
		return IndexResult{}, err
	}
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return IndexResult{}, err
	}
	defer tx.Rollback()
	if err := deleteChunks(ctx, tx, docID, e.vecTableExists(ctx)); err != nil {
		return IndexResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM rag_documents WHERE id=?`, docID); err != nil {
		return IndexResult{}, err
	}
	return IndexResult{DocumentID: docID, Removed: true}, tx.Commit()
}

// deleteChunks removes a document's chunks. FTS5 external-content rows are removed by trg_chunks_ad; chunks are
// only ever replaced wholesale, so no UPDATE trigger is needed.
func deleteChunks(ctx context.Context, tx *sql.Tx, docID string, hasVec bool) error {
	if hasVec {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+vecTable+` WHERE rowid IN (SELECT id FROM rag_document_chunks WHERE document_id = ?)`, docID); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM rag_document_chunks WHERE document_id = ?`, docID)
	return err
}

func (e *Engine) upsertDocument(ctx context.Context, d docSpec, force bool) (IndexResult, error) {
	hash := d.hash()
	var docID, oldHash string
	err := e.DB.QueryRowContext(ctx, `SELECT id, content_hash FROM rag_documents WHERE source_type=? AND source_id=?`, d.SourceType, d.SourceID).Scan(&docID, &oldHash)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IndexResult{}, err
	}
	if exists && oldHash == hash && !force {
		return IndexResult{DocumentID: docID, Skipped: true}, nil
	}

	chunks := Chunk(d.Text)
	for i := 1; i < len(chunks); i++ {
		if d.Title != "" {
			chunks[i] = d.Title + "\n" + chunks[i]
		}
	}
	res := IndexResult{Chunks: len(chunks)}

	// Embed BEFORE opening the write transaction: network latency must not hold the SQLite write lock.
	var vectors [][]float32
	if len(chunks) > 0 && e.AI.HasEmbedding() {
		if ok, err := e.EnsureVectorIndex(ctx); err != nil {
			res.EmbedErr = err
		} else if ok {
			er, err := e.AI.Embedding.Embed(ctx, chunks)
			if err != nil {
				res.EmbedErr = err // lexical indexing still succeeds; EmbedBackfill will retry
			} else {
				vectors = er.Vectors
			}
		}
	}

	var lat, lon any
	if d.Lat != nil && d.Lon != nil {
		lat, lon = *d.Lat, *d.Lon
	}
	var region, date, owner any
	if d.RegionID != "" {
		region = d.RegionID
	}
	if d.DocDate != "" {
		date = d.DocDate
	}
	if d.OwnerID != "" {
		owner = d.OwnerID
	}
	now := storage.Time(e.now())

	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return IndexResult{}, err
	}
	defer tx.Rollback()
	// Embedding ran outside the write transaction. Another worker may have inserted
	// or replaced this document meanwhile. BEGIN IMMEDIATE serializes writers;
	// re-read the identity/hash under that lock before deciding INSERT vs UPDATE.
	err = tx.QueryRowContext(ctx, `SELECT id, content_hash FROM rag_documents WHERE source_type=? AND source_id=?`, d.SourceType, d.SourceID).Scan(&docID, &oldHash)
	exists = err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IndexResult{}, err
	}
	if exists && oldHash == hash && !force {
		return IndexResult{DocumentID: docID, Skipped: true}, nil
	}
	hasVec := e.vecTableExists(ctx)
	if exists {
		if err := deleteChunks(ctx, tx, docID, hasVec); err != nil {
			return IndexResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE rag_documents SET title=?, url=?, region_id=?, lat=?, lon=?, doc_date=?, visibility=?, owner_user_id=?, content_hash=?, updated_at=? WHERE id=?`,
			d.Title, d.URL, region, lat, lon, date, d.Visibility, owner, hash, now, docID); err != nil {
			return IndexResult{}, err
		}
	} else {
		docID = storage.NewID()
		if _, err := tx.ExecContext(ctx, `INSERT INTO rag_documents(id,source_type,source_id,title,url,region_id,lat,lon,doc_date,visibility,owner_user_id,content_hash,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			docID, d.SourceType, d.SourceID, d.Title, d.URL, region, lat, lon, date, d.Visibility, owner, hash, now, now); err != nil {
			return IndexResult{}, err
		}
	}
	for i, c := range chunks {
		r, err := tx.ExecContext(ctx, `INSERT INTO rag_document_chunks(document_id,chunk_index,text,token_estimate,created_at) VALUES (?,?,?,?,?)`,
			docID, i, c, TokenEstimate(c), now)
		if err != nil {
			return IndexResult{}, err
		}
		if vectors != nil && i < len(vectors) {
			cid, err := r.LastInsertId()
			if err != nil {
				return IndexResult{}, err
			}
			blob, err := vecBlob(vectors[i])
			if err != nil {
				return IndexResult{}, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO `+vecTable+`(rowid, embedding) VALUES (?, ?)`, cid, blob); err != nil {
				return IndexResult{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return IndexResult{}, err
	}
	res.DocumentID = docID
	res.Embedded = vectors != nil
	return res, nil
}

// ─── bulk ──────────────────────────────────────────────────────────

// ReindexStats summarizes ReindexAll.
type ReindexStats struct {
	Observations, Articles, Removed, Errors int
}

// ReindexAll re-indexes every observation and RSS article. force=true rewrites documents even when unchanged
// (use after changing the chunker or text templates). Documents whose source row disappeared are purged.
func (e *Engine) ReindexAll(ctx context.Context, force bool) (ReindexStats, error) {
	var st ReindexStats
	run := func(query string, fn func(id string) (IndexResult, error), counter *int) error {
		rows, err := e.DB.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			r, err := fn(id)
			switch {
			case err != nil:
				st.Errors++
				e.Log.Warn("rag: reindex failed", "id", id, "err", err)
			case r.Removed:
				st.Removed++
			default:
				*counter++
			}
		}
		return nil
	}
	obs := func(id string) (IndexResult, error) {
		if force {
			return e.indexObservationForce(ctx, id)
		}
		return e.IndexObservation(ctx, id)
	}
	if err := run(`SELECT id FROM observations`, obs, &st.Observations); err != nil {
		return st, err
	}
	if err := run(`SELECT id FROM rss_articles`, func(id string) (IndexResult, error) { return e.IndexArticleResult(ctx, id) }, &st.Articles); err != nil {
		return st, err
	}
	// purge orphans (source row hard-deleted)
	for _, q := range []struct{ typ, tbl string }{{"observation", "observations"}, {"rss_article", "rss_articles"}} {
		rows, err := e.DB.QueryContext(ctx, `SELECT source_id FROM rag_documents WHERE source_type = ? AND source_id NOT IN (SELECT id FROM `+q.tbl+`)`, q.typ)
		if err != nil {
			return st, err
		}
		var orphans []string
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			orphans = append(orphans, id)
		}
		rows.Close()
		for _, id := range orphans {
			if _, err := e.RemoveDocument(ctx, q.typ, id); err == nil {
				st.Removed++
			}
		}
	}
	if force {
		if _, err := e.DB.ExecContext(ctx, `INSERT INTO rag_chunks_fts(rag_chunks_fts) VALUES('rebuild')`); err != nil {
			return st, fmt.Errorf("rag: rebuilding fts index: %w", err)
		}
	}
	return st, nil
}

// indexObservationForce is IndexObservation bypassing the unchanged-hash shortcut.
func (e *Engine) indexObservationForce(ctx context.Context, id string) (IndexResult, error) {
	// clearing the stored hash makes upsertDocument rewrite the document
	if _, err := e.DB.ExecContext(ctx, `UPDATE rag_documents SET content_hash='' WHERE source_type='observation' AND source_id=?`, id); err != nil {
		return IndexResult{}, err
	}
	return e.IndexObservation(ctx, id)
}

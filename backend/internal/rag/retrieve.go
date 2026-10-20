package rag

import (
	"context"
	"sort"
	"strings"
)

// SearchRequest is a retrieval request. Region/date filters are optional.
type SearchRequest struct {
	Query     string
	RegionID  string // includes child regions
	From, To  string // YYYY-MM-DD, inclusive; documents without a date always pass
	Limit     int    // max hits returned (default 8)
	MaxPerDoc int    // max chunks per document (default 2)
}

// Hit is one retrieved chunk with its document metadata.
type Hit struct {
	ChunkID    int64
	DocumentID string
	SourceType string
	SourceID   string
	Title      string
	URL        string
	DocDate    string
	RegionID   string
	Text       string
	Score      float64 // reciprocal-rank-fusion score
	FTSRank    int     // 1-based rank in the lexical list, 0 if absent
	VecRank    int     // 1-based rank in the vector list, 0 if absent
}

const rrfK = 60

// filter builds the shared WHERE fragment (on alias d = rag_documents) plus its args, and the optional recursive
// CTE that expands the region subtree. The access clause is the security boundary: it encodes who may see what.
//
//	public        -> everyone
//	verified_only -> any signed-in user
//	staff         -> reviewer/admin, or the owner of the record
//	owner         -> only owner_user_id
func filterSQL(c Caller, req SearchRequest) (cte string, cteArgs []any, where string, args []any) {
	staff := 0
	if c.IsStaff() {
		staff = 1
	}
	conds := []string{`(d.visibility = 'public'
		OR (d.visibility = 'verified_only' AND ? <> '')
		OR (d.visibility = 'staff' AND (? = 1 OR (d.owner_user_id IS NOT NULL AND d.owner_user_id = ?)))
		OR (d.visibility = 'owner' AND d.owner_user_id IS NOT NULL AND d.owner_user_id = ?))`}
	args = append(args, c.UserID, staff, c.UserID, c.UserID)
	if req.RegionID != "" {
		cte = `WITH RECURSIVE sub(id) AS (SELECT ? UNION ALL SELECT r.id FROM geographic_regions r JOIN sub ON r.parent_id = sub.id) `
		cteArgs = []any{req.RegionID}
		conds = append(conds, `(d.region_id IN (SELECT id FROM sub)
			OR (d.source_type = 'observation' AND EXISTS (SELECT 1 FROM observation_region_links l WHERE l.observation_id = d.source_id AND l.region_id IN (SELECT id FROM sub)))
			OR (d.source_type = 'rss_article' AND EXISTS (SELECT 1 FROM rss_article_regions ar WHERE ar.article_id = d.source_id AND ar.region_id IN (SELECT id FROM sub))))`)
	}
	if req.From != "" || req.To != "" {
		from, to := req.From, req.To
		if from == "" {
			from = "0000-01-01"
		}
		if to == "" {
			to = "9999-12-31"
		}
		conds = append(conds, `(d.doc_date IS NULL OR substr(d.doc_date, 1, 10) BETWEEN ? AND ?)`)
		args = append(args, from[:min(10, len(from))], to[:min(10, len(to))])
	}
	return cte, cteArgs, strings.Join(conds, " AND "), args
}

// Search runs hybrid retrieval: FTS5 BM25 over chunks and (when embeddings exist) sqlite-vec KNN, both restricted
// by the caller's access filter / region / dates inside the SQL, merged with reciprocal-rank fusion.
func (e *Engine) Search(ctx context.Context, req SearchRequest, caller Caller) ([]Hit, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 8
	}
	perDoc := req.MaxPerDoc
	if perDoc <= 0 {
		perDoc = 2
	}
	pool := limit * 6
	if pool < 40 {
		pool = 40
	}
	cte, cteArgs, where, fargs := filterSQL(caller, req)

	// (a) lexical
	var ftsIDs []int64
	if match := BuildFTSQuery(req.Query); match != "" {
		q := cte + `SELECT c.id FROM rag_chunks_fts
			JOIN rag_document_chunks c ON c.id = rag_chunks_fts.rowid
			JOIN rag_documents d ON d.id = c.document_id
			WHERE rag_chunks_fts MATCH ? AND ` + where + `
			ORDER BY bm25(rag_chunks_fts) LIMIT ?`
		args := append(append(append([]any{}, cteArgs...), match), fargs...)
		args = append(args, pool)
		ids, err := e.queryIDs(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		ftsIDs = ids
	}

	// (b) vector: candidate set is computed by the same filters; KNN is constrained to it (rowid IN ...)
	var vecIDs []int64
	if e.AI.HasEmbedding() && strings.TrimSpace(req.Query) != "" && e.vecTableExists(ctx) {
		if ok, err := e.EnsureVectorIndex(ctx); err == nil && ok {
			if er, err := e.AI.Embedding.Embed(ctx, []string{req.Query}); err != nil {
				e.Log.Warn("rag: query embedding failed; using lexical retrieval only", "err", err)
			} else if blob, err := vecBlob(er.Vectors[0]); err == nil {
				cteV := cte
				cand := `cand(id) AS (SELECT c.id FROM rag_document_chunks c JOIN rag_documents d ON d.id = c.document_id WHERE ` + where + `)`
				if cteV == "" {
					cteV = `WITH ` + cand + ` `
				} else {
					cteV = strings.TrimSuffix(cteV, " ") + `, ` + cand + ` `
				}
				q := cteV + `SELECT rowid FROM ` + vecTable + ` WHERE embedding MATCH ? AND k = ? AND rowid IN (SELECT id FROM cand) ORDER BY distance`
				args := append(append(append([]any{}, cteArgs...), fargs...), blob, pool)
				ids, err := e.queryIDs(ctx, q, args...)
				if err != nil {
					e.Log.Warn("rag: vector search failed; using lexical retrieval only", "err", err)
				} else {
					vecIDs = ids
				}
			}
		}
	}

	// (c) reciprocal rank fusion
	score := map[int64]float64{}
	ftsRank, vecRank := map[int64]int{}, map[int64]int{}
	for i, id := range ftsIDs {
		score[id] += 1.0 / float64(rrfK+i+1)
		ftsRank[id] = i + 1
	}
	for i, id := range vecIDs {
		score[id] += 1.0 / float64(rrfK+i+1)
		vecRank[id] = i + 1
	}
	if len(score) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if score[ids[i]] != score[ids[j]] {
			return score[ids[i]] > score[ids[j]]
		}
		return ids[i] < ids[j]
	})

	// (d) hydrate. The access filter is applied AGAIN here (defence in depth): even a bug in the candidate stage
	// could not leak a restricted chunk.
	hits, err := e.hydrate(ctx, ids, caller)
	if err != nil {
		return nil, err
	}
	byID := map[int64]*Hit{}
	for i := range hits {
		h := &hits[i]
		h.Score, h.FTSRank, h.VecRank = score[h.ChunkID], ftsRank[h.ChunkID], vecRank[h.ChunkID]
		byID[h.ChunkID] = h
	}
	var out []Hit
	perDocCount := map[string]int{}
	for _, id := range ids {
		h, ok := byID[id]
		if !ok {
			continue
		}
		if perDocCount[h.DocumentID] >= perDoc {
			continue
		}
		perDocCount[h.DocumentID]++
		out = append(out, *h)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (e *Engine) queryIDs(ctx context.Context, q string, args ...any) ([]int64, error) {
	rows, err := e.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (e *Engine) hydrate(ctx context.Context, ids []int64, caller Caller) ([]Hit, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	_, _, where, fargs := filterSQL(caller, SearchRequest{})
	args := make([]any, 0, len(ids)+len(fargs))
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, fargs...)
	rows, err := e.DB.QueryContext(ctx, `SELECT c.id, c.document_id, d.source_type, d.source_id, d.title, d.url,
		COALESCE(d.doc_date,''), COALESCE(d.region_id,''), c.text
		FROM rag_document_chunks c JOIN rag_documents d ON d.id = c.document_id
		WHERE c.id IN (`+placeholders(len(ids))+`) AND `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ChunkID, &h.DocumentID, &h.SourceType, &h.SourceID, &h.Title, &h.URL, &h.DocDate, &h.RegionID, &h.Text); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

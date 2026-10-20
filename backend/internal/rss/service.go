// Package rss fetches, parses, sanitizes, de-duplicates and region-tags official RSS/Atom feeds, and keeps
// them in rss_sources / rss_articles / rss_article_regions. It never invents sources: the seed list
// (datasets/seed/rss_sources.json) contains only feeds that were fetched and verified (docs/data_sources/rss.md).
package rss

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// ArticleIndexer is implemented by internal/rag (kept as an interface to avoid an import cycle).
type ArticleIndexer interface {
	IndexArticle(ctx context.Context, articleID string) error
}

// Errors returned to HTTP handlers.
var (
	ErrSourceNotFound   = errors.New("rss: source not found")
	ErrDuplicateSource  = errors.New("rss: a source with this feed URL already exists")
	ErrSyncInProgress   = errors.New("rss: a sync for this source is already running")
	ErrValidation       = errors.New("rss: validation failed")
	maxItemsPerSync     = 200
	minIntervalMinutes  = 5
	maxErrorTextRunes   = 300
	sourceCategoryOther = CatGeneral
)

// Source mirrors rss_sources (and the OpenAPI FeedSource schema).
type Source struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Organization    string  `json:"organization"`
	FeedURL         string  `json:"feed_url"`
	Category        string  `json:"category"`
	Enabled         bool    `json:"enabled"`
	IntervalMinutes int     `json:"interval_minutes"`
	VerifiedAt      *string `json:"verified_at"`
	LastFetchedAt   *string `json:"last_fetched_at"`
	LastStatus      string  `json:"last_status"`
	LastError       string  `json:"last_error"`
	etag, lastMod   string
}

// SyncResult summarizes one source sync.
type SyncResult struct {
	SourceID    string `json:"source_id"`
	Status      string `json:"status"` // ok | not_modified | error
	Fetched     int    `json:"fetched"`
	New         int    `json:"new"`
	Updated     int    `json:"updated"`
	Duplicates  int    `json:"duplicates"`
	IndexErrors int    `json:"index_errors"`
	Error       string `json:"error,omitempty"`
}

// Service is the RSS facade used by handlers, the scheduler and the job worker.
type Service struct {
	DB      *sql.DB
	Cfg     config.Config
	Fetcher *Fetcher
	Indexer ArticleIndexer // optional
	Now     func() time.Time
	Log     *slog.Logger

	mu       sync.Mutex
	inflight map[string]bool
}

// NewService builds a Service. indexer may be nil (articles then are not added to the RAG index).
func NewService(db *sql.DB, cfg config.Config, indexer ArticleIndexer) *Service {
	return &Service{
		DB: db, Cfg: cfg, Fetcher: NewFetcher(cfg, false), Indexer: indexer,
		Now: time.Now, Log: slog.Default(), inflight: map[string]bool{},
	}
}

func (s *Service) now() time.Time { return s.Now().UTC() }

// ─── sources ───────────────────────────────────────────────────────

const sourceCols = `id, name, organization, feed_url, category, enabled, interval_minutes, verified_at, last_fetched_at, last_status, last_error, etag`

func scanSource(sc interface{ Scan(...any) error }) (Source, error) {
	var src Source
	var en int
	var ver, last sql.NullString
	var etagField string
	if err := sc.Scan(&src.ID, &src.Name, &src.Organization, &src.FeedURL, &src.Category, &en, &src.IntervalMinutes,
		&ver, &last, &src.LastStatus, &src.LastError, &etagField); err != nil {
		return src, err
	}
	src.Enabled = en == 1
	if ver.Valid {
		src.VerifiedAt = &ver.String
	}
	if last.Valid {
		src.LastFetchedAt = &last.String
	}
	// rss_sources has a single etag column: "etag\nlast-modified".
	src.etag, src.lastMod, _ = strings.Cut(etagField, "\n")
	return src, nil
}

// GetSource loads one source.
func (s *Service) GetSource(ctx context.Context, id string) (Source, error) {
	src, err := scanSource(s.DB.QueryRowContext(ctx, `SELECT `+sourceCols+` FROM rss_sources WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Source{}, ErrSourceNotFound
	}
	return src, err
}

// ListSources returns sources ordered by name; includeDisabled=false hides disabled ones (public /feeds).
func (s *Service) ListSources(ctx context.Context, includeDisabled bool) ([]Source, error) {
	q := `SELECT ` + sourceCols + ` FROM rss_sources`
	if !includeDisabled {
		q += ` WHERE enabled = 1`
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// SetSourceEnabled enables/disables a source.
func (s *Service) SetSourceEnabled(ctx context.Context, id string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE rss_sources SET enabled=?, updated_at=? WHERE id=?`, v, storage.Time(s.now()), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSourceNotFound
	}
	return nil
}

// FeedInfo is what VerifyFeedURL learned about a feed.
type FeedInfo struct {
	Title      string     `json:"title"`
	Format     string     `json:"format"`
	ItemCount  int        `json:"item_count"`
	NewestItem *time.Time `json:"newest_item,omitempty"`
	FinalURL   string     `json:"final_url"`
	etag, lm   string
}

// VerifyFeedURL fetches rawURL once (SSRF-guarded) and confirms it parses as RSS/Atom with at least one item.
// It is used by the admin "add feed" flow. Errors: ErrInvalidURL, ErrBlockedAddress, *HTTPStatusError,
// ErrNotFeed, ErrTooLarge, or a network error.
func (s *Service) VerifyFeedURL(ctx context.Context, rawURL string) (FeedInfo, error) {
	res, err := s.Fetcher.Fetch(ctx, rawURL, "", "")
	if err != nil {
		return FeedInfo{}, err
	}
	feed, err := Parse(res.Body, res.FinalURL, s.now())
	if err != nil {
		return FeedInfo{}, err
	}
	if len(feed.Items) == 0 {
		return FeedInfo{}, fmt.Errorf("%w: the feed contains no items", ErrNotFeed)
	}
	info := FeedInfo{Title: feed.Title, Format: feed.Format, ItemCount: len(feed.Items), FinalURL: res.FinalURL, etag: res.ETag, lm: res.LastModified}
	for _, it := range feed.Items {
		if it.Published != nil && (info.NewestItem == nil || it.Published.After(*info.NewestItem)) {
			t := *it.Published
			info.NewestItem = &t
		}
	}
	return info, nil
}

// AddSourceInput is the body of POST /admin/feeds.
type AddSourceInput struct {
	Name            string
	Organization    string
	FeedURL         string
	Category        string
	IntervalMinutes int
}

// VerificationError distinguishes remote feed verification failures from database failures.
type VerificationError struct{ Err error }

func (e *VerificationError) Error() string { return "rss: feed verification failed: " + e.Err.Error() }
func (e *VerificationError) Unwrap() error { return e.Err }

// AddSource verifies the URL (fetch + parse), then atomically stores the source and audit entry.
// The optional actor is supplied by the authenticated HTTP adapter; omitted means system.
func (s *Service) AddSource(ctx context.Context, in AddSourceInput, actors ...audit.Actor) (Source, error) {
	in.Name = SingleLine(in.Name)
	in.Organization = SingleLine(in.Organization)
	in.FeedURL = strings.TrimSpace(in.FeedURL)
	if in.Name == "" || len([]rune(in.Name)) > 120 {
		return Source{}, fmt.Errorf("%w: name is required (max 120 characters)", ErrValidation)
	}
	if in.FeedURL == "" {
		return Source{}, fmt.Errorf("%w: feed_url is required", ErrValidation)
	}
	if in.Category == "" {
		in.Category = sourceCategoryOther
	}
	interval := in.IntervalMinutes
	if interval <= 0 {
		interval = int(s.Cfg.RSS.DefaultInterval / time.Minute)
	}
	if interval < minIntervalMinutes {
		interval = max(interval, minIntervalMinutes)
	}
	var dup int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM rss_sources WHERE feed_url = ?`, in.FeedURL).Scan(&dup); err != nil {
		return Source{}, err
	}
	if dup > 0 {
		return Source{}, ErrDuplicateSource
	}
	if _, err := s.VerifyFeedURL(ctx, in.FeedURL); err != nil {
		return Source{}, &VerificationError{Err: err}
	}
	now := storage.Time(s.now())
	id := storage.NewID()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Source{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO rss_sources(id,name,organization,feed_url,category,enabled,interval_minutes,verified_at,created_at,updated_at)
		VALUES (?,?,?,?,?,1,?,?,?,?)`, id, in.Name, in.Organization, in.FeedURL, in.Category, interval, now, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Source{}, ErrDuplicateSource
		}
		return Source{}, err
	}
	actor := audit.System
	if len(actors) > 0 {
		actor = actors[0]
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "feed.create", EntityType: "rss_source", EntityID: id, After: map[string]string{"name": in.Name, "feed_url": in.FeedURL}}); err != nil {
		return Source{}, err
	}
	if err := tx.Commit(); err != nil {
		return Source{}, err
	}
	return s.GetSource(ctx, id)
}

// ─── sync ──────────────────────────────────────────────────────────

func (s *Service) acquire(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight[id] {
		return false
	}
	s.inflight[id] = true
	return true
}

func (s *Service) release(id string) {
	s.mu.Lock()
	delete(s.inflight, id)
	s.mu.Unlock()
}

// SyncNow synchronizes one source immediately (alias used by the admin endpoint / job handler).
func (s *Service) SyncNow(ctx context.Context, sourceID string) (SyncResult, error) {
	return s.SyncSource(ctx, sourceID)
}

// SyncAll syncs every enabled source sequentially.
func (s *Service) SyncAll(ctx context.Context) []SyncResult {
	srcs, err := s.ListSources(ctx, false)
	if err != nil {
		return []SyncResult{{Status: "error", Error: err.Error()}}
	}
	var out []SyncResult
	for _, src := range srcs {
		if ctx.Err() != nil {
			break
		}
		r, _ := s.SyncSource(ctx, src.ID)
		out = append(out, r)
	}
	return out
}

// SyncSource fetches one source and stores new/changed articles. Fetch/parse failures are recorded on the
// source (last_status/last_error) and returned in the result; the returned error is non-nil only for
// ErrSourceNotFound, ErrSyncInProgress and ctx cancellation.
func (s *Service) SyncSource(ctx context.Context, sourceID string) (SyncResult, error) {
	res := SyncResult{SourceID: sourceID}
	if !s.acquire(sourceID) {
		return res, ErrSyncInProgress
	}
	defer s.release(sourceID)
	src, err := s.GetSource(ctx, sourceID)
	if err != nil {
		return res, err
	}
	fail := func(err error) (SyncResult, error) {
		res.Status = "error"
		res.Error = truncateRunes(err.Error(), maxErrorTextRunes)
		s.recordSync(ctx, src, res, "", "", false)
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		return res, nil
	}
	fr, err := s.Fetcher.Fetch(ctx, src.FeedURL, src.etag, src.lastMod)
	if err != nil {
		return fail(err)
	}
	if fr.NotModified {
		res.Status = "not_modified"
		s.recordSync(ctx, src, res, src.etag, src.lastMod, false)
		return res, nil
	}
	feed, err := Parse(fr.Body, fr.FinalURL, s.now())
	if err != nil {
		return fail(err)
	}
	tagger, err := LoadRegionTagger(ctx, s.DB)
	if err != nil {
		s.Log.Warn("rss: loading region names failed; articles will not be region-tagged", "err", err)
	}
	items := feed.Items
	if len(items) > maxItemsPerSync {
		items = items[:maxItemsPerSync]
	}
	res.Fetched = len(items)
	for _, it := range items {
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		out, id, err := s.upsertItem(ctx, src, it, tagger)
		if err != nil {
			s.Log.Warn("rss: storing item failed", "source", src.ID, "guid", it.GUID, "err", err)
			continue
		}
		switch out {
		case outcomeNew:
			res.New++
		case outcomeUpdated:
			res.Updated++
		case outcomeDuplicate:
			res.Duplicates++
		}
		if (out == outcomeNew || out == outcomeUpdated) && s.Indexer != nil {
			if err := s.Indexer.IndexArticle(ctx, id); err != nil {
				res.IndexErrors++
				s.Log.Warn("rss: indexing article failed", "article", id, "err", err)
			}
		}
	}
	res.Status = "ok"
	s.recordSync(ctx, src, res, fr.ETag, fr.LastModified, true)
	return res, nil
}

type outcome int

const (
	outcomeUnchanged outcome = iota
	outcomeNew
	outcomeUpdated
	outcomeDuplicate
)

func (s *Service) upsertItem(ctx context.Context, src Source, it Item, tagger *RegionTagger) (outcome, string, error) {
	now := storage.Time(s.now())
	category := SuggestCategory(it.Title, it.Summary, it.Categories, src.Category)
	regions := tagger.Match(it.Title + " " + it.Summary + " " + strings.Join(it.Categories, " "))
	note := ""
	if len(regions) == 0 {
		note = LocationUnknownNote
	}
	var pub any
	if it.Published != nil {
		pub = storage.Time(*it.Published)
	}
	url := it.Link
	if url == "" {
		url = src.FeedURL
	}

	var existingID, existingHash string
	err := s.DB.QueryRowContext(ctx, `SELECT id, content_hash FROM rss_articles WHERE source_id=? AND guid=?`, src.ID, it.GUID).Scan(&existingID, &existingHash)
	switch {
	case err == nil && existingHash == it.ContentHash:
		return outcomeUnchanged, existingID, nil
	case err == nil:
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return 0, "", err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `UPDATE rss_articles SET url=?, title=?, summary=?, content_hash=?, category=?, published_at=?, location_note=?, updated_at=? WHERE id=?`,
			url, it.Title, it.Summary, it.ContentHash, category, pub, note, now, existingID); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return outcomeDuplicate, existingID, nil // changed into a copy of another article
			}
			return 0, "", err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM rss_article_regions WHERE article_id=?`, existingID); err != nil {
			return 0, "", err
		}
		if err := insertRegions(ctx, tx, existingID, regions); err != nil {
			return 0, "", err
		}
		return outcomeUpdated, existingID, tx.Commit()
	case !errors.Is(err, sql.ErrNoRows):
		return 0, "", err
	}

	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM rss_articles WHERE content_hash=?`, it.ContentHash).Scan(&n); err != nil {
		return 0, "", err
	}
	if n > 0 {
		return outcomeDuplicate, "", nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	id := storage.NewID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO rss_articles(id,source_id,guid,url,title,summary,content_hash,category,published_at,ingested_at,location_note,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, src.ID, it.GUID, url, it.Title, it.Summary, it.ContentHash, category, pub, now, note, now, now); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return outcomeDuplicate, "", nil
		}
		return 0, "", err
	}
	if err := insertRegions(ctx, tx, id, regions); err != nil {
		return 0, "", err
	}
	return outcomeNew, id, tx.Commit()
}

func insertRegions(ctx context.Context, tx *sql.Tx, articleID string, regions []RegionRef) error {
	for _, r := range regions {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO rss_article_regions(article_id, region_id) VALUES (?,?)`, articleID, r.ID); err != nil {
			return err
		}
	}
	return nil
}

// recordSync stores the sync outcome on the source and writes a system audit entry when something happened.
func (s *Service) recordSync(ctx context.Context, src Source, res SyncResult, etag, lastMod string, verified bool) {
	now := storage.Time(s.now())
	bg := context.WithoutCancel(ctx) // the outcome must be recorded even when ctx was cancelled mid-sync
	etagField := etag
	if lastMod != "" {
		etagField += "\n" + lastMod
	}
	q := `UPDATE rss_sources SET last_fetched_at=?, last_status=?, last_error=?, updated_at=?`
	args := []any{now, res.Status, res.Error, now}
	if res.Status != "error" {
		q += `, etag=?`
		args = append(args, etagField)
	}
	if verified {
		q += `, verified_at=?`
		args = append(args, now)
	}
	q += ` WHERE id=?`
	args = append(args, src.ID)
	if _, err := s.DB.ExecContext(bg, q, args...); err != nil {
		s.Log.Warn("rss: recording sync status failed", "source", src.ID, "err", err)
	}
	if res.New > 0 || res.Updated > 0 || res.Status == "error" {
		after := fmt.Sprintf(`{"status":%q,"fetched":%d,"new":%d,"updated":%d,"duplicates":%d,"error":%q}`,
			res.Status, res.Fetched, res.New, res.Updated, res.Duplicates, res.Error)
		// System actor (actor_id NULL). Direct insert: this package must not depend on internal/audit.
		if _, err := s.DB.ExecContext(bg, `INSERT INTO audit_logs(id,actor_id,actor_role,action,entity_type,entity_id,before_json,after_json,ip,created_at)
			VALUES (?,NULL,'system','feed.sync','rss_source',?, '', ?, '', ?)`, storage.NewID(), src.ID, after, now); err != nil {
			s.Log.Warn("rss: audit insert failed", "err", err)
		}
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ─── articles ──────────────────────────────────────────────────────

// ArticleFilter filters ListArticles. Zero values mean "no filter".
type ArticleFilter struct {
	RegionID        string // includes child regions
	SourceID        string
	Category        string
	From, To        string // YYYY-MM-DD (published_at, falling back to ingested_at)
	IncludeDisabled bool   // include articles of disabled sources (admin only)
	Page, PageSize  int
}

// Article mirrors the OpenAPI FeedArticle schema.
type Article struct {
	ID           string      `json:"id"`
	SourceID     string      `json:"source_id"`
	SourceName   string      `json:"source_name"`
	Title        string      `json:"title"`
	Summary      string      `json:"summary"`
	URL          string      `json:"url"`
	Category     string      `json:"category"`
	PublishedAt  *string     `json:"published_at"`
	IngestedAt   string      `json:"ingested_at"`
	Regions      []RegionRef `json:"regions"`
	LocationNote string      `json:"location_note"`
}

// ListArticles returns a page of articles (newest first) and the total count.
func (s *Service) ListArticles(ctx context.Context, f ArticleFilter) ([]Article, int, error) {
	if f.PageSize <= 0 {
		f.PageSize = 50
	}
	if f.PageSize > 200 {
		f.PageSize = 200
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	where := []string{"1=1"}
	var args []any
	if !f.IncludeDisabled {
		where = append(where, "s.enabled = 1")
	}
	if f.SourceID != "" {
		where = append(where, "a.source_id = ?")
		args = append(args, f.SourceID)
	}
	if f.Category != "" {
		where = append(where, "a.category = ?")
		args = append(args, f.Category)
	}
	if f.RegionID != "" {
		where = append(where, `EXISTS (WITH RECURSIVE sub(id) AS (SELECT ? UNION ALL SELECT r.id FROM geographic_regions r JOIN sub ON r.parent_id = sub.id)
			SELECT 1 FROM rss_article_regions ar WHERE ar.article_id = a.id AND ar.region_id IN (SELECT id FROM sub))`)
		args = append(args, f.RegionID)
	}
	if f.From != "" {
		where = append(where, "substr(COALESCE(a.published_at, a.ingested_at),1,10) >= ?")
		args = append(args, f.From[:min(10, len(f.From))])
	}
	if f.To != "" {
		where = append(where, "substr(COALESCE(a.published_at, a.ingested_at),1,10) <= ?")
		args = append(args, f.To[:min(10, len(f.To))])
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM rss_articles a JOIN rss_sources s ON s.id=a.source_id WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT a.id,a.source_id,s.name,a.title,a.summary,a.url,a.category,a.published_at,a.ingested_at,a.location_note
		FROM rss_articles a JOIN rss_sources s ON s.id=a.source_id WHERE `+cond+`
		ORDER BY COALESCE(a.published_at, a.ingested_at) DESC, a.id LIMIT ? OFFSET ?`,
		append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return nil, 0, err
	}
	var out []Article
	for rows.Next() {
		var a Article
		var pub sql.NullString
		if err := rows.Scan(&a.ID, &a.SourceID, &a.SourceName, &a.Title, &a.Summary, &a.URL, &a.Category, &pub, &a.IngestedAt, &a.LocationNote); err != nil {
			rows.Close()
			return nil, 0, err
		}
		if pub.Valid {
			a.PublishedAt = &pub.String
		}
		a.Regions = []RegionRef{}
		out = append(out, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range out {
		rr, err := s.DB.QueryContext(ctx, `SELECT g.id, g.name, g.kind FROM rss_article_regions ar JOIN geographic_regions g ON g.id=ar.region_id WHERE ar.article_id=? ORDER BY g.name`, out[i].ID)
		if err != nil {
			return nil, 0, err
		}
		for rr.Next() {
			var r RegionRef
			if err := rr.Scan(&r.ID, &r.Name, &r.Kind); err != nil {
				rr.Close()
				return nil, 0, err
			}
			out[i].Regions = append(out[i].Regions, r)
		}
		rr.Close()
	}
	if out == nil {
		out = []Article{}
	}
	return out, total, nil
}

package rss

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"sync"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Clock abstracts time for the scheduler (tests inject a fake).
type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Scheduler periodically syncs enabled sources whose interval elapsed. It runs syncs in background goroutines,
// so neither Run's tick nor any caller of RunOnce ever blocks on a slow feed.
type Scheduler struct {
	Svc           *Service
	Clock         Clock
	Tick          time.Duration // how often due sources are looked up (default 1 minute)
	MaxConcurrent int           // max simultaneous syncs (default 3)
	// SyncFn performs one sync; defaults to Svc.SyncSource. Tests replace it.
	SyncFn func(ctx context.Context, sourceID string)

	mu      sync.Mutex
	running map[string]bool
	wg      sync.WaitGroup
}

// NewScheduler returns a scheduler with defaults.
func NewScheduler(svc *Service) *Scheduler {
	s := &Scheduler{Svc: svc, Clock: realClock{}, Tick: time.Minute, MaxConcurrent: 3, running: map[string]bool{}}
	s.SyncFn = func(ctx context.Context, id string) { _, _ = svc.SyncSource(ctx, id) }
	return s
}

// effectiveInterval clamps the configured interval to a sane minimum.
func effectiveInterval(src Source) time.Duration {
	m := src.IntervalMinutes
	if m < minIntervalMinutes {
		m = minIntervalMinutes
	}
	return time.Duration(m) * time.Minute
}

// jitter is a stable per-source offset of up to 10% of the interval, so many sources do not fire together.
func jitter(sourceID string, interval time.Duration) time.Duration {
	h := fnv.New32a()
	_, _ = h.Write([]byte(sourceID))
	span := int64(interval / 10)
	if span <= 0 {
		return 0
	}
	return time.Duration(int64(h.Sum32()) % span)
}

// NextDue returns when src should be fetched next. Never-fetched sources are due immediately; sources whose
// last attempt failed wait twice the interval (simple backoff so a dead feed is not hammered).
func NextDue(src Source) time.Time {
	if !src.Enabled {
		return time.Time{}
	}
	if src.LastFetchedAt == nil {
		return time.Time{}
	}
	last, err := time.Parse(time.RFC3339, *src.LastFetchedAt)
	if err != nil {
		return time.Time{}
	}
	iv := effectiveInterval(src)
	wait := iv
	if src.LastStatus == "error" {
		wait = 2 * iv
	}
	return last.Add(wait + jitter(src.ID, iv))
}

// DueSources lists enabled sources whose next-due time has passed at now.
func (s *Scheduler) DueSources(ctx context.Context, now time.Time) ([]Source, error) {
	all, err := s.Svc.ListSources(ctx, false)
	if err != nil {
		return nil, err
	}
	var due []Source
	for _, src := range all {
		if !now.Before(NextDue(src)) {
			due = append(due, src)
		}
	}
	return due, nil
}

// RunOnce starts background syncs for all due sources (respecting MaxConcurrent and not double-starting a
// running source) and returns immediately with the number started.
func (s *Scheduler) RunOnce(ctx context.Context) int {
	if !s.Svc.Cfg.RSS.Enabled {
		return 0
	}
	due, err := s.DueSources(ctx, s.Clock.Now())
	if err != nil {
		s.Svc.Log.Warn("rss scheduler: listing sources failed", "err", err)
		return 0
	}
	max := s.MaxConcurrent
	if max <= 0 {
		max = 3
	}
	started := 0
	for _, src := range due {
		s.mu.Lock()
		if s.running[src.ID] || len(s.running) >= max {
			s.mu.Unlock()
			continue
		}
		s.running[src.ID] = true
		s.mu.Unlock()
		started++
		s.wg.Add(1)
		go func(id string) {
			defer s.wg.Done()
			defer func() {
				s.mu.Lock()
				delete(s.running, id)
				s.mu.Unlock()
			}()
			defer func() {
				if r := recover(); r != nil {
					s.Svc.Log.Error("rss scheduler: sync panicked", "source", id, "panic", r)
				}
			}()
			s.SyncFn(ctx, id)
		}(src.ID)
	}
	return started
}

// Wait blocks until all started syncs finished (tests, graceful shutdown).
func (s *Scheduler) Wait() { s.wg.Wait() }

// Run ticks until ctx is cancelled, then waits for in-flight syncs. A disabled RSS config returns immediately.
func (s *Scheduler) Run(ctx context.Context) {
	if !s.Svc.Cfg.RSS.Enabled {
		s.Svc.Log.Info("rss scheduler disabled (ECO_RSS_ENABLED=false)")
		return
	}
	tick := s.Tick
	if tick <= 0 {
		tick = time.Minute
	}
	// first pass shortly after startup, not synchronously with it
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			s.wg.Wait()
			return
		case <-timer.C:
			s.RunOnce(ctx)
			timer.Reset(tick)
		}
	}
}

// ─── seed ──────────────────────────────────────────────────────────

// SeedSource is one entry of datasets/seed/rss_sources.json (fields follow rss_sources).
type SeedSource struct {
	Name            string `json:"name"`
	Organization    string `json:"organization"`
	FeedURL         string `json:"feed_url"`
	Category        string `json:"category"`
	Enabled         *bool  `json:"enabled"`
	IntervalMinutes int    `json:"interval_minutes"`
	VerifiedAt      string `json:"verified_at"`
}

// SeedSources loads the verified source list into an EMPTY rss_sources table (it never overwrites admin
// changes). It returns the number of inserted rows; a missing file returns an error wrapping os.ErrNotExist.
func SeedSources(db *sql.DB, path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("rss: reading seed file: %w", err)
	}
	var seeds []SeedSource
	if err := json.Unmarshal(data, &seeds); err != nil {
		return 0, fmt.Errorf("rss: parsing seed file: %w", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM rss_sources`).Scan(&n); err != nil {
		return 0, err
	}
	if n > 0 {
		return 0, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := storage.Now()
	inserted := 0
	for _, sd := range seeds {
		if sd.Name == "" || sd.FeedURL == "" {
			return 0, fmt.Errorf("rss: seed entry without name/feed_url")
		}
		enabled := 1
		if sd.Enabled != nil && !*sd.Enabled {
			enabled = 0
		}
		iv := sd.IntervalMinutes
		if iv <= 0 {
			iv = 60
		}
		cat := sd.Category
		if cat == "" {
			cat = CatGeneral
		}
		var ver any
		if sd.VerifiedAt != "" {
			ver = sd.VerifiedAt
		}
		if _, err := tx.Exec(`INSERT INTO rss_sources(id,name,organization,feed_url,category,enabled,interval_minutes,verified_at,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`, storage.NewID(), sd.Name, sd.Organization, sd.FeedURL, cat, enabled, iv, ver, now, now); err != nil {
			return 0, err
		}
		inserted++
	}
	return inserted, tx.Commit()
}

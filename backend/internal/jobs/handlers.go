package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/ai"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/rag"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/rss"
)

// Job kinds.
const (
	KindAnalyzeObservation = "analyze_observation"
	KindIndexDocument      = "index_document"
	KindSyncFeed           = "sync_feed"
	KindEmbedBackfill      = "embed_backfill"
)

// Payloads (JSON).
type (
	AnalyzePayload struct {
		ObservationID string `json:"observation_id"`
		AnalysisID    string `json:"analysis_id,omitempty"` // pre-created pending row (from POST /ai/analyze-observation)
	}
	IndexPayload struct {
		SourceType string `json:"source_type"` // observation | rss_article
		SourceID   string `json:"source_id"`
	}
	SyncPayload struct {
		SourceID string `json:"source_id,omitempty"` // empty = all enabled sources
	}
)

// Deps are the collaborators of the built-in handlers. Only *sql.DB, config and the intelligence services are
// needed - the handlers deliberately do not depend on the observations/auth packages.
type Deps struct {
	DB   *sql.DB
	Cfg  config.Config
	AI   *ai.Providers
	RAG  *rag.Engine
	RSS  *rss.Service
	Log  *slog.Logger
	Jobs *Worker // used to enqueue follow-up jobs
	Now  func() time.Time
}

func (d *Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Deps) log() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.Default()
}

// RegisterDefaultHandlers registers analyze_observation, index_document, sync_feed and embed_backfill.
func RegisterDefaultHandlers(w *Worker, d *Deps) {
	if d.Jobs == nil {
		d.Jobs = w
	}
	w.Register(KindAnalyzeObservation, d.AnalyzeObservation)
	w.Register(KindIndexDocument, d.IndexDocument)
	w.Register(KindSyncFeed, d.SyncFeed)
	w.Register(KindEmbedBackfill, d.EmbedBackfill)
}

func decode(payload []byte, v any) error {
	if err := json.Unmarshal(payload, v); err != nil {
		return Permanent(fmt.Errorf("invalid job payload: %w", err))
	}
	return nil
}

// IndexDocument (re)indexes one observation or RSS article for RAG.
func (d *Deps) IndexDocument(ctx context.Context, payload []byte) error {
	var p IndexPayload
	if err := decode(payload, &p); err != nil {
		return err
	}
	if d.RAG == nil {
		return Permanent(errors.New("rag engine not configured"))
	}
	switch p.SourceType {
	case "observation":
		_, err := d.RAG.IndexObservation(ctx, p.SourceID)
		return err
	case "rss_article":
		return d.RAG.IndexArticle(ctx, p.SourceID)
	}
	return Permanent(fmt.Errorf("unknown source_type %q", p.SourceType))
}

// SyncFeed syncs one RSS source, or all enabled sources when source_id is empty.
func (d *Deps) SyncFeed(ctx context.Context, payload []byte) error {
	var p SyncPayload
	if err := decode(payload, &p); err != nil {
		return err
	}
	if d.RSS == nil {
		return Permanent(errors.New("rss service not configured"))
	}
	if p.SourceID == "" {
		d.RSS.SyncAll(ctx)
		return nil
	}
	res, err := d.RSS.SyncSource(ctx, p.SourceID)
	switch {
	case errors.Is(err, rss.ErrSourceNotFound):
		return Permanent(err)
	case errors.Is(err, rss.ErrSyncInProgress):
		return nil // the running sync does the work
	case err != nil:
		return err
	}
	if res.Status == "error" {
		d.log().Warn("sync_feed: source sync failed", "source", p.SourceID, "err", res.Error)
	}
	return nil // fetch problems are recorded on the source (last_status/last_error); retrying immediately would not help
}

// EmbedBackfill embeds chunks that have no vector yet, in bounded batches; it re-enqueues itself while work remains.
func (d *Deps) EmbedBackfill(ctx context.Context, _ []byte) error {
	if d.RAG == nil {
		return Permanent(errors.New("rag engine not configured"))
	}
	if !d.AI.HasEmbedding() {
		return nil
	}
	total := 0
	for i := 0; i < 10; i++ {
		n, err := d.RAG.EmbedBackfill(ctx, 64)
		if err != nil {
			return err
		}
		total += n
		if n == 0 {
			return nil
		}
	}
	if d.Jobs != nil {
		_, err := d.Jobs.Enqueue(ctx, KindEmbedBackfill, nil, d.now().Add(2*time.Second))
		return err
	}
	return nil
}

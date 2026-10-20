package observations

import (
	"context"
	"encoding/json"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Job kinds enqueued by this package (the worker lives in internal/jobs).
const (
	JobAnalyzeObservation = "analyze_observation" // payload {"observation_id": id}
	JobIndexDocument      = "index_document"      // payload {"source_type":"observation","source_id": id}
)

// EnqueueJob inserts a queued row into the jobs table using ex (pass the surrounding tx).
func EnqueueJob(ctx context.Context, ex audit.Execer, kind string, payload any, runAfter time.Time) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	now := storage.Now()
	_, err = ex.ExecContext(ctx, `INSERT INTO jobs(id, kind, payload_json, status, attempts, last_error, run_after, created_at, updated_at)
		VALUES (?,?,?, 'queued', 0, '', ?, ?, ?)`, storage.NewID(), kind, string(b), storage.Time(runAfter), now, now)
	return err
}

// EnqueueIndex queues re-indexing of an observation for RAG (also removes it from the index when
// it is no longer visible).
func EnqueueIndex(ctx context.Context, ex audit.Execer, obsID string) error {
	return EnqueueJob(ctx, ex, JobIndexDocument, map[string]string{"source_type": "observation", "source_id": obsID}, time.Now())
}

// EnqueueAnalyze queues AI analysis of an observation's photos.
func EnqueueAnalyze(ctx context.Context, ex audit.Execer, obsID string, runAfter time.Time) error {
	return EnqueueJob(ctx, ex, JobAnalyzeObservation, map[string]string{"observation_id": obsID}, runAfter)
}

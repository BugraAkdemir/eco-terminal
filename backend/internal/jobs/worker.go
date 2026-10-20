// Package jobs is a durable background worker over the `jobs` table (no external queue): AI photo analysis,
// RAG indexing, embedding backfill and RSS sync run here so HTTP requests never wait for them.
//
// Guarantees:
//   - claiming is a single atomic UPDATE ... WHERE id=(SELECT ... LIMIT 1) RETURNING, so concurrent workers never
//     run the same job;
//   - failed attempts are retried with exponential backoff up to MaxAttempts, then marked failed (kept for inspection);
//   - jobs left 'running' by a crash are re-queued when the worker starts (the server is a single process);
//   - only kinds that have a registered handler are claimed.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Handler processes one job. Returning an error schedules a retry (unless wrapped with Permanent).
type Handler func(ctx context.Context, payload []byte) error

// Job is a claimed job.
type Job struct {
	ID       string
	Kind     string
	Payload  []byte
	Attempts int // including the current attempt
}

// JobInfo is the stored state of a job (GET /admin/feeds/sync follow-ups, diagnostics).
type JobInfo struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error"`
	RunAfter  string `json:"run_after"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type permanentError struct{ err error }

func (p permanentError) Error() string { return p.err.Error() }
func (p permanentError) Unwrap() error { return p.err }

// Permanent marks an error as not worth retrying (bad payload, missing record, provider 4xx).
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err}
}

// ErrJobNotFound is returned by Get.
var ErrJobNotFound = errors.New("jobs: job not found")

// Worker claims and runs jobs.
type Worker struct {
	DB          *sql.DB
	Workers     int           // concurrent goroutines (default 2)
	Poll        time.Duration // idle polling interval (default 2s)
	MaxAttempts int           // default 5
	JobTimeout  time.Duration // per-attempt timeout (default 5m)
	// Backoff returns the delay before retry number `attempts` (1 = after the first failure).
	Backoff func(attempts int) time.Duration
	Now     func() time.Time
	Log     *slog.Logger

	mu       sync.RWMutex
	handlers map[string]Handler
	wake     chan struct{}
}

// NewWorker returns a Worker with defaults.
func NewWorker(db *sql.DB) *Worker {
	return &Worker{
		DB: db, Workers: 2, Poll: 2 * time.Second, MaxAttempts: 5, JobTimeout: 5 * time.Minute,
		Now: time.Now, Log: slog.Default(), handlers: map[string]Handler{}, wake: make(chan struct{}, 1),
	}
}

// Register binds a handler to a job kind (replacing any previous one).
func (w *Worker) Register(kind string, h Handler) {
	w.mu.Lock()
	w.handlers[kind] = h
	w.mu.Unlock()
}

func (w *Worker) kinds() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	ks := make([]string, 0, len(w.handlers))
	for k := range w.handlers {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func (w *Worker) backoff(attempts int) time.Duration {
	if w.Backoff != nil {
		return w.Backoff(attempts)
	}
	d := 30 * time.Second << (attempts - 1)
	if d > time.Hour || d <= 0 {
		d = time.Hour
	}
	return d
}

func (w *Worker) now() time.Time { return w.Now().UTC() }

// execer is satisfied by *sql.DB and *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func enqueue(ctx context.Context, x execer, kind string, payload any, runAfter time.Time) (string, error) {
	if strings.TrimSpace(kind) == "" {
		return "", errors.New("jobs: kind is required")
	}
	var raw []byte
	switch p := payload.(type) {
	case nil:
		raw = []byte("{}")
	case []byte:
		raw = p
	case json.RawMessage:
		raw = p
	default:
		b, err := json.Marshal(p)
		if err != nil {
			return "", err
		}
		raw = b
	}
	if !json.Valid(raw) {
		return "", errors.New("jobs: payload is not valid JSON")
	}
	if runAfter.IsZero() {
		runAfter = time.Now()
	}
	id, now := storage.NewID(), storage.Now()
	_, err := x.ExecContext(ctx, `INSERT INTO jobs(id,kind,payload_json,status,attempts,last_error,run_after,created_at,updated_at)
		VALUES (?,?,?,'queued',0,'',?,?,?)`, id, kind, string(raw), storage.Time(runAfter), now, now)
	return id, err
}

// Enqueue stores a job to run at/after runAfter (zero = now). payload may be a struct, []byte or json.RawMessage.
func (w *Worker) Enqueue(ctx context.Context, kind string, payload any, runAfter time.Time) (string, error) {
	id, err := enqueue(ctx, w.DB, kind, payload, runAfter)
	if err == nil {
		w.Notify()
	}
	return id, err
}

// EnqueueTx stores a job inside the caller's transaction, so it exists iff the surrounding change commits.
func EnqueueTx(ctx context.Context, tx *sql.Tx, kind string, payload any, runAfter time.Time) (string, error) {
	return enqueue(ctx, tx, kind, payload, runAfter)
}

// Notify wakes an idle worker immediately (non-blocking).
func (w *Worker) Notify() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Get returns the stored state of a job.
func (w *Worker) Get(ctx context.Context, id string) (JobInfo, error) {
	var j JobInfo
	err := w.DB.QueryRowContext(ctx, `SELECT id,kind,status,attempts,last_error,run_after,created_at,updated_at FROM jobs WHERE id=?`, id).
		Scan(&j.ID, &j.Kind, &j.Status, &j.Attempts, &j.LastError, &j.RunAfter, &j.CreatedAt, &j.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrJobNotFound
	}
	return j, err
}

// claim atomically moves one due job of a registered kind to 'running'. (nil, nil) means nothing is due.
func (w *Worker) claim(ctx context.Context) (*Job, error) {
	kinds := w.kinds()
	if len(kinds) == 0 {
		return nil, nil
	}
	args := []any{storage.Time(w.now()), storage.Time(w.now())}
	for _, k := range kinds {
		args = append(args, k)
	}
	q := `UPDATE jobs SET status='running', attempts=attempts+1, updated_at=?
		WHERE id = (SELECT id FROM jobs WHERE status='queued' AND run_after <= ? AND kind IN (` + strings.TrimSuffix(strings.Repeat("?,", len(kinds)), ",") + `)
		            ORDER BY run_after, created_at, id LIMIT 1)
		RETURNING id, kind, payload_json, attempts`
	var j Job
	var payload string
	err := w.DB.QueryRowContext(ctx, q, args...).Scan(&j.ID, &j.Kind, &payload, &j.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j.Payload = []byte(payload)
	return &j, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// run executes one claimed job and records the outcome.
func (w *Worker) run(ctx context.Context, j *Job) {
	w.mu.RLock()
	h := w.handlers[j.Kind]
	w.mu.RUnlock()
	jctx, cancel := context.WithTimeout(ctx, w.JobTimeout)
	defer cancel()
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("handler panicked: %v", r)
			}
		}()
		return h(jctx, j.Payload)
	}()
	bg := context.WithoutCancel(ctx) // record the outcome even during shutdown
	now := storage.Time(w.now())
	switch {
	case err == nil:
		_, uerr := w.DB.ExecContext(bg, `UPDATE jobs SET status='done', last_error='', updated_at=? WHERE id=?`, now, j.ID)
		if uerr != nil {
			w.Log.Error("jobs: marking done failed", "job", j.ID, "err", uerr)
		}
	default:
		var perm permanentError
		final := errors.As(err, &perm) || j.Attempts >= w.MaxAttempts
		msg := truncate(err.Error(), 500)
		if final {
			_, uerr := w.DB.ExecContext(bg, `UPDATE jobs SET status='failed', last_error=?, updated_at=? WHERE id=?`, msg, now, j.ID)
			if uerr != nil {
				w.Log.Error("jobs: marking failed failed", "job", j.ID, "err", uerr)
			}
			w.Log.Warn("jobs: job failed permanently", "job", j.ID, "kind", j.Kind, "attempts", j.Attempts, "err", msg)
			return
		}
		next := storage.Time(w.now().Add(w.backoff(j.Attempts)))
		_, uerr := w.DB.ExecContext(bg, `UPDATE jobs SET status='queued', last_error=?, run_after=?, updated_at=? WHERE id=?`, msg, next, now, j.ID)
		if uerr != nil {
			w.Log.Error("jobs: rescheduling failed", "job", j.ID, "err", uerr)
		}
		w.Log.Warn("jobs: job failed, will retry", "job", j.ID, "kind", j.Kind, "attempts", j.Attempts, "retry_at", next, "err", msg)
	}
}

// RunOnce claims and runs a single due job synchronously. It reports whether a job was run (tests, tools).
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	j, err := w.claim(ctx)
	if err != nil || j == nil {
		return false, err
	}
	w.run(ctx, j)
	return true, nil
}

// RecoverStale re-queues jobs stuck in 'running' whose last update is older than olderThan (0 = all of them,
// which is correct at process start because the server is a single process).
func (w *Worker) RecoverStale(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := storage.Time(w.now().Add(-olderThan).Add(time.Second))
	res, err := w.DB.ExecContext(ctx, `UPDATE jobs SET status='queued', updated_at=?, last_error=CASE WHEN last_error='' THEN 'recovered after crash' ELSE last_error END
		WHERE status='running' AND updated_at <= ?`, storage.Time(w.now()), cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Run starts Workers goroutines and blocks until ctx is cancelled and in-flight jobs finished.
func (w *Worker) Run(ctx context.Context) {
	if n, err := w.RecoverStale(ctx, 0); err != nil {
		w.Log.Error("jobs: recovering stale jobs failed", "err", err)
	} else if n > 0 {
		w.Log.Warn("jobs: re-queued jobs left running by a previous process", "count", n)
	}
	n := w.Workers
	if n <= 0 {
		n = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx)
		}()
	}
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context) {
	poll := w.Poll
	if poll <= 0 {
		poll = 2 * time.Second
	}
	for ctx.Err() == nil {
		j, err := w.claim(ctx)
		if err != nil {
			if ctx.Err() == nil {
				w.Log.Error("jobs: claim failed", "err", err)
			}
		} else if j != nil {
			w.run(ctx, j)
			continue
		}
		t := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-w.wake:
		case <-t.C:
		}
		t.Stop()
	}
}

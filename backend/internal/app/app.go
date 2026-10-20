// Package app wires the server: it builds the services, registers every route under /api/v1,
// applies the middleware chain and manages startup/shutdown.
//
// EXTENSION POINT: other modules (ai, rag, rss, simulation handlers, background workers) plug in
// through extensions.go - one line each. See the Module and Worker interfaces below.
package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/geographic"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/media"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/notifications"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/observations"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/review"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Version is reported by /health (override with -ldflags "-X .../internal/app.Version=...").
var Version = "dev"

// Deps is what a Module receives when it registers its routes.
type Deps struct {
	DB     *sql.DB
	Config config.Config
	Logger *slog.Logger

	Auth         *auth.Service
	Geo          *geographic.Service
	Observations *observations.Service

	// Require wraps a handler so only the given roles (admin always passes) may call it:
	//   mux.Handle("POST /api/v1/x", deps.Require(auth.RoleResearcher)(http.HandlerFunc(h)))
	// Inside any handler the caller is available through auth.UserFrom(r.Context()) because the
	// whole mux is already wrapped by the optional-auth middleware.
	Require func(roles ...string) httpx.Middleware
	// Strict is the stricter per-IP rate limiter (used for credential endpoints); wrap sensitive routes with it.
	Strict *httpx.RateLimiter
}

// Module registers HTTP routes. Paths must include the /api/v1 prefix (Go 1.22 mux patterns,
// e.g. "GET /api/v1/ai/providers/status").
type Module interface {
	Register(mux *http.ServeMux, deps Deps)
}

// Worker is a background component. Start must return promptly (spawn goroutines); the goroutines
// must stop when ctx is cancelled. Stop blocks until they have exited. A Module that also
// implements Worker is started and stopped automatically.
type Worker interface {
	Start(ctx context.Context) error
	Stop()
}

// App is the assembled server.
type App struct {
	cfg     config.Config
	db      *sql.DB
	log     *slog.Logger
	deps    Deps
	handler http.Handler
	workers []Worker
	stopped bool

	reviewSvc *review.Service
}

// AIConfigured reports whether a text or vision model is configured.
func AIConfigured(c config.Config) bool {
	return c.AI.BaseURL != "" && (c.AI.TextModel != "" || c.AI.VisionModel != "")
}

// EmbeddingsConfigured reports whether embeddings are configured.
func EmbeddingsConfigured(c config.Config) bool {
	return c.AI.BaseURL != "" && c.AI.EmbeddingModel != "" && c.AI.EmbeddingDim > 0
}

// New assembles the application on an open, migrated database.
func New(cfg config.Config, db *sql.DB, log *slog.Logger) (*App, error) {
	if log == nil {
		log = slog.Default()
	}
	general := httpx.NewRateLimiter(cfg.RateLimitPerMin, max(cfg.RateLimitPerMin, 10))
	strictPerMin := max(cfg.RateLimitPerMin/10, 5)
	strict := httpx.NewRateLimiter(strictPerMin, max(strictPerMin, 10))

	authSvc := auth.NewService(db, cfg.SessionTTL, log)
	geo := geographic.NewService(db, log)
	obs := observations.NewService(db, geo, AIConfigured(cfg), log)
	mediaSvc, err := media.NewService(db, obs, cfg.StorageDir, cfg.MaxUploadMB, AIConfigured(cfg), log)
	if err != nil {
		return nil, err
	}
	a := &App{cfg: cfg, db: db, log: log, reviewSvc: review.NewService(db, obs)}
	a.deps = Deps{DB: db, Config: cfg, Logger: log, Auth: authSvc, Geo: geo, Observations: obs, Require: auth.Require, Strict: strict}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"status":                "ok",
			"version":               Version,
			"ai_configured":         AIConfigured(cfg),
			"embeddings_configured": EmbeddingsConfigured(cfg),
			"vector_search":         true,
		})
	})
	authSvc.Routes(mux, strict)
	mux.Handle("GET /api/v1/admin/audit-logs", auth.Require(auth.RoleAdmin)(audit.ListHandler(db)))
	geo.Routes(mux)
	obs.Routes(mux)
	mediaSvc.Routes(mux)
	a.reviewSvc.Routes(mux)
	notifications.NewService(db).Routes(mux)

	mods, workers := extensions(a.deps)
	for _, m := range mods {
		m.Register(mux, a.deps)
		if w, ok := m.(Worker); ok {
			workers = append(workers, w)
		}
	}
	a.workers = workers

	a.handler = httpx.Chain(mux,
		httpx.RequestID,
		httpx.AccessLog(log),
		httpx.Recover(log),
		httpx.CORS(cfg.CORSOrigins),
		skipMedia(general),
		httpx.BodyLimit(mediaSvc.BodyLimit()),
		authSvc.Middleware,
		httpx.JSONFallback,
	)
	return a, nil
}

// skipMedia exempts image downloads from the general limiter: a map screen legitimately loads
// dozens of thumbnails at once.
func skipMedia(l *httpx.RateLimiter) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		limited := l.Middleware(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/media/") {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}

// Handler is the root http.Handler (used by Run and by httptest).
func (a *App) Handler() http.Handler { return a.handler }

// Deps exposes the assembled dependencies (tests, tooling).
func (a *App) Deps() Deps { return a.deps }

// Bootstrap performs idempotent startup data work: seeds Kastamonu regions, creates the first
// admin from ECO_ADMIN_EMAIL/ECO_ADMIN_PASSWORD and (ECO_SEED_DEMO=true) the demo data.
func (a *App) Bootstrap(ctx context.Context) error {
	if err := a.deps.Geo.SeedKastamonu(ctx); err != nil {
		return err
	}
	if err := a.deps.Auth.EnsureAdmin(ctx, a.cfg.AdminEmail, a.cfg.AdminPassword); err != nil {
		// a misconfigured admin must not stop the API; the operator sees the reason in the log
		a.log.Error("admin bootstrap failed", "err", err)
	}
	var admins int
	if err := a.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role='admin' AND status='active'`).Scan(&admins); err == nil && admins == 0 {
		a.log.Warn("no administrator exists: set ECO_ADMIN_EMAIL and ECO_ADMIN_PASSWORD (min 10 chars) and restart")
	}
	if a.cfg.SeedDemo {
		if err := a.seedDemo(ctx); err != nil {
			return err
		}
	}
	return nil
}

// StartWorkers starts all registered workers.
func (a *App) StartWorkers(ctx context.Context) error {
	for i, w := range a.workers {
		if err := w.Start(ctx); err != nil {
			for _, started := range a.workers[:i] {
				started.Stop()
			}
			return err
		}
	}
	return nil
}

// StopWorkers stops all workers (blocks until they exited). Safe to call more than once.
func (a *App) StopWorkers() {
	if a.stopped {
		return
	}
	a.stopped = true
	for i := len(a.workers) - 1; i >= 0; i-- {
		a.workers[i].Stop()
	}
}

// Run opens the database, bootstraps, serves HTTP and shuts down gracefully on SIGINT/SIGTERM.
func Run(cfg config.Config) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	a, err := New(cfg, db, log)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := a.Bootstrap(ctx); err != nil {
		return err
	}
	if err := a.StartWorkers(ctx); err != nil {
		return err
	}
	defer a.StopWorkers()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 16,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr, "env", cfg.Env, "ai_configured", AIConfigured(cfg))
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

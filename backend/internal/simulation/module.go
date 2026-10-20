package simulation

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/climate"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/model"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Deps are the host-provided dependencies (internal/app adapts its own Deps to this; the package
// cannot import internal/app because app imports it).
type Deps struct {
	DB     *sql.DB
	Config config.Config
	Logger *slog.Logger
}

// Option customises NewModule.
type Option func(*Module)

// WithExplainer plugs the optional narrative provider (internal/ai text provider adapter).
func WithExplainer(e Explainer) Option { return func(m *Module) { m.svc.Explainer = e } }

// WithInlineWait sets how long POST /simulations/run waits for missing climate data before it
// answers "data is being prepared" (default 20 s).
func WithInlineWait(d time.Duration) Option { return func(m *Module) { m.inlineWait = d } }

// WithFetcher overrides the provider client (tests; default built from config.Climate).
func WithFetcher(f *climate.Fetcher) Option { return func(m *Module) { m.store.Fetcher = f } }

// DefaultDataFrom/To is the period the startup worker prepares for the province region.
var (
	DefaultDataFrom = time.Date(1991, 1, 1, 0, 0, 0, 0, time.UTC)
	DefaultDataTo   = time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
)

// Module is the HTTP + background part of the simulation package. It implements app.Module
// (via an adapter) and app.Worker.
type Module struct {
	svc        *Service
	store      *climate.Store
	db         *sql.DB
	log        *slog.Logger
	inlineWait time.Duration

	mu       sync.Mutex
	inflight map[string]*prep
	base     context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewModule builds the module: climate store (provider client from config, offline snapshot
// resolved from ECO_CLIMATE_SNAPSHOT or searched), service and options.
func NewModule(d Deps, opts ...Option) *Module {
	log := d.Logger
	if log == nil {
		log = slog.Default()
	}
	store := &climate.Store{DB: d.DB, Fetcher: climate.NewFetcher(d.Config.Climate),
		SnapshotPath: climate.ResolveSnapshotPath(d.Config.Climate.SnapshotPath)}
	if store.SnapshotPath == "" {
		log.Warn("climate: no offline snapshot found (set ECO_CLIMATE_SNAPSHOT); every region's data will be downloaded on first use")
	} else {
		log.Info("climate: offline snapshot", "path", store.SnapshotPath)
	}
	m := &Module{svc: NewService(d.DB, store, nil), store: store, db: d.DB, log: log,
		inlineWait: 20 * time.Second, inflight: map[string]*prep{}}
	m.base, m.cancel = context.WithCancel(context.Background())
	for _, o := range opts {
		o(m)
	}
	return m
}

// Service exposes the underlying service (tests, other packages).
func (m *Module) Service() *Service { return m.svc }

// Register mounts the routes (paths include /api/v1).
func (m *Module) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/simulations/run", auth.RequireFunc(m.run, auth.RoleResearcher))
	mux.Handle("GET /api/v1/simulations", auth.RequireFunc(m.list))
	mux.Handle("GET /api/v1/simulations/compare", auth.RequireFunc(m.compare))
	mux.Handle("GET /api/v1/simulations/{id}", auth.RequireFunc(m.get))
	mux.HandleFunc("GET /api/v1/environment/measurements", m.measurements)
}

// ───────────── background preparation ─────────────

// Start launches the startup preparation: for every province region the climate data of
// [DefaultDataFrom, DefaultDataTo] is ensured, offline snapshot first. It returns immediately.
func (m *Module) Start(ctx context.Context) error {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		rows, err := m.db.QueryContext(m.base, `SELECT id, name FROM geographic_regions WHERE kind = 'province' ORDER BY name`)
		if err != nil {
			m.log.Error("climate: list provinces", "err", err)
			return
		}
		type reg struct{ id, name string }
		var regs []reg
		for rows.Next() {
			var r reg
			if rows.Scan(&r.id, &r.name) == nil {
				regs = append(regs, r)
			}
		}
		rows.Close()
		for _, r := range regs {
			p := m.prepare(r.id, DefaultDataFrom, DefaultDataTo)
			select {
			case <-p.done:
			case <-m.base.Done():
				return
			case <-ctx.Done():
				return
			}
			if p.err != nil {
				m.log.Warn("climate: province data not complete", "region", r.name, "err", p.err)
			} else {
				m.log.Info("climate: province data ready", "region", r.name, "from_snapshot_days", p.rep.FromSnapshot, "from_api_days", p.rep.FromAPI)
			}
		}
	}()
	return nil
}

// Stop cancels running downloads and waits for the goroutines.
func (m *Module) Stop() {
	m.cancel()
	m.wg.Wait()
}

type prep struct {
	done chan struct{}
	err  error
	rep  climate.EnsureReport
}

// prepare starts (or joins) a background EnsureRegionData for the region and range. The download
// is not tied to any HTTP request: it survives a client that gave up, and its progress is kept by
// the fetch ledger.
func (m *Module) prepare(regionID string, from, to time.Time) *prep {
	key := regionID + "|" + from.Format("2006-01-02") + "|" + to.Format("2006-01-02")
	m.mu.Lock()
	if p, ok := m.inflight[key]; ok {
		m.mu.Unlock()
		return p
	}
	p := &prep{done: make(chan struct{})}
	m.inflight[key] = p
	m.mu.Unlock()
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ctx, cancel := context.WithTimeout(m.base, 20*time.Minute)
		defer cancel()
		p.rep, p.err = m.store.EnsureRegionData(ctx, regionID, from, to)
		m.mu.Lock()
		delete(m.inflight, key)
		m.mu.Unlock()
		close(p.done)
	}()
	return p
}

// ───────────── handlers ─────────────

func toHTTPErr(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return httpx.NotFound("not found")
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrUnknown):
		return httpx.Validation(strings.TrimPrefix(err.Error(), "simulation: "), nil)
	}
	return err
}

func (m *Module) run(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var req RunRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	params, err := m.svc.Normalize(req)
	if err != nil {
		httpx.WriteError(w, toHTTPErr(err))
		return
	}
	ctx := WithActor(r.Context(), auth.ActorFrom(r))

	// Make sure the region's climate data exists. Data already present (ledger / offline snapshot)
	// returns at once. A missing download (a district in another ERA5 cell, first use) continues in
	// the background; we wait inlineWait for it and otherwise answer 200 "insufficient_data /
	// data is being prepared" without persisting anything, so the client can simply retry.
	if from, err1 := model.ParseYM(params.ReferenceFrom); err1 == nil {
		if to, err2 := model.ParseYM(params.ReferenceTo); err2 == nil {
			first := time.Date(from.Year, time.Month(from.Month), 1, 0, 0, 0, 0, time.UTC)
			last := time.Date(to.Year, time.Month(to.Month), to.DaysIn(), 0, 0, 0, 0, time.UTC)
			if _, _, err := m.store.RegionCentroid(ctx, params.RegionID); err != nil {
				if errors.Is(err, climate.ErrRegionNotFound) {
					httpx.WriteError(w, httpx.NotFound("region not found"))
				} else {
					httpx.WriteError(w, err)
				}
				return
			}
			p := m.prepare(params.RegionID, first, last)
			select {
			case <-p.done:
				ctx = WithPreparedData(ctx, p.err)
			case <-time.After(m.inlineWait):
				httpx.WriteJSON(w, http.StatusOK, m.preparingResult(req, params))
				return
			case <-r.Context().Done():
				return
			}
		}
	}
	res, err := m.svc.Run(ctx, u.ID, req)
	if err != nil {
		httpx.WriteError(w, toHTTPErr(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// preparingResult is the (unpersisted) answer while climate data is still being downloaded.
func (m *Module) preparingResult(req RunRequest, p Params) Result {
	reason := "Bu bölge için iklim verisi ilk kez hazırlanıyor (Open-Meteo'dan indiriliyor); işlem arka planda sürüyor. " +
		"Birkaç dakika sonra aynı isteği tekrarlayın."
	name := strings.TrimSpace(req.Name)
	return Result{
		Name: name, Model: p.Model, RegionID: p.RegionID, Status: StatusInsufficientData, Params: p,
		Indicators: []Indicator{}, Series: []Series{}, DataSources: []DataSource{},
		Assumptions: []string{}, Limitations: model.LimitationsTR(), InsufficientReason: &reason,
		CreatedAt: storage.Now(),
	}
}

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	page, err := httpx.ParsePage(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	items, err := m.svc.List(r.Context(), u.ID, page.Size)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	res, err := m.svc.GetOwned(r.Context(), u.ID, r.PathValue("id"), u.Role == auth.RoleAdmin)
	if err != nil {
		httpx.WriteError(w, toHTTPErr(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

func (m *Module) compare(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	a, b := r.URL.Query().Get("a"), r.URL.Query().Get("b")
	if a == "" || b == "" {
		httpx.WriteError(w, httpx.Validation("query parameters a and b (result ids) are required", nil))
		return
	}
	admin := u.Role == auth.RoleAdmin
	ra, err := m.svc.GetOwned(r.Context(), u.ID, a, admin)
	if err != nil {
		httpx.WriteError(w, toHTTPErr(err))
		return
	}
	rb, err := m.svc.GetOwned(r.Context(), u.ID, b, admin)
	if err != nil {
		httpx.WriteError(w, toHTTPErr(err))
		return
	}
	c, err := Compare(ra, rb)
	if err != nil {
		httpx.WriteError(w, toHTTPErr(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (m *Module) measurements(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("region_id") == "" || q.Get("type") == "" {
		httpx.WriteError(w, httpx.Validation("region_id and type are required", nil))
		return
	}
	s, err := m.svc.MeasurementsSeries(r.Context(), q.Get("region_id"), q.Get("type"), q.Get("from"), q.Get("to"), q.Get("resolution"))
	if err != nil {
		httpx.WriteError(w, toHTTPErr(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}

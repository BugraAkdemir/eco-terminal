package simulation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/climate"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/model"
)

// Env is what a Model may use to obtain its inputs.
type Env struct {
	Climate ClimateData
}

// Outcome is the model-independent result of one run (everything except persistence metadata).
type Outcome struct {
	Status             string
	InsufficientReason string
	Params             Params
	Indicators         []Indicator
	Series             []Series
	DataSources        []DataSource
	Coverage           *model.Coverage
	Assumptions        []string
	Limitations        []string
}

// Model is a scenario model. Future models (habitat fragmentation, water stress, ...) implement this
// interface and are added to a Registry; the service, persistence and API stay unchanged.
type Model interface {
	// Name is the registry key stored in simulation_scenarios.model.
	Name() string
	// Normalize validates the request and returns the parameters to persist (ErrInvalid on bad input).
	Normalize(req RunRequest) (Params, error)
	// Run computes the outcome. Insufficient data is an Outcome with that status, not an error;
	// an error means the run itself failed (storage, programming error, ...).
	Run(ctx context.Context, env Env, p Params) (Outcome, error)
}

// Registry maps model names to implementations.
type Registry struct {
	mu     sync.RWMutex
	models map[string]Model
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{models: map[string]Model{}} }

// Register adds (or replaces) a model.
func (r *Registry) Register(m Model) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[m.Name()] = m
}

// Get returns the model registered under name.
func (r *Registry) Get(name string) (Model, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.models[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, name)
	}
	return m, nil
}

// Names lists the registered model names (sorted).
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.models))
	for n := range r.models {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ClimateDroughtV1 is the built-in model: delta-change experiment on a reference period with
// Thornthwaite PET, SPI and a water-balance standardized index (see model.RunClimateDrought).
type ClimateDroughtV1 struct{}

func (ClimateDroughtV1) Name() string { return ModelClimateDroughtV1 }

func (ClimateDroughtV1) Normalize(req RunRequest) (Params, error) {
	p := Params{
		Model: ModelClimateDroughtV1, RegionID: req.RegionID,
		PrecipChangePct: req.PrecipChangePct, TempChangeC: req.TempChangeC, Scale: req.Scale,
	}
	if p.Scale == 0 {
		p.Scale = model.DefaultScale
	}
	if req.RegionID == "" {
		return p, fmt.Errorf("%w: region_id is required", ErrInvalid)
	}
	from, err := model.ParseYM(req.ReferenceFrom)
	if err != nil {
		return p, fmt.Errorf("%w: reference_from: %v", ErrInvalid, err)
	}
	to, err := model.ParseYM(req.ReferenceTo)
	if err != nil {
		return p, fmt.Errorf("%w: reference_to: %v", ErrInvalid, err)
	}
	if to.Index() < from.Index() {
		return p, fmt.Errorf("%w: reference_to is before reference_from", ErrInvalid)
	}
	if from.Year < 1940 {
		return p, fmt.Errorf("%w: reference_from before 1940 (start of ERA5)", ErrInvalid)
	}
	if p.Scale != 1 && p.Scale != 3 && p.Scale != 6 {
		return p, fmt.Errorf("%w: scale must be 1, 3 or 6", ErrInvalid)
	}
	if model.IsMissing(p.PrecipChangePct) || p.PrecipChangePct < -100 || p.PrecipChangePct > MaxPrecipChangePct {
		return p, fmt.Errorf("%w: precip_change_pct must be within [-100, %g]", ErrInvalid, MaxPrecipChangePct)
	}
	if model.IsMissing(p.TempChangeC) || p.TempChangeC < -MaxAbsTempChangeC || p.TempChangeC > MaxAbsTempChangeC {
		return p, fmt.Errorf("%w: temp_change_c must be within [-%g, %g]", ErrInvalid, MaxAbsTempChangeC, MaxAbsTempChangeC)
	}
	p.ReferenceFrom, p.ReferenceTo = from.String(), to.String()
	return p, nil
}

func (ClimateDroughtV1) Run(ctx context.Context, env Env, p Params) (Outcome, error) {
	from, _ := model.ParseYM(p.ReferenceFrom)
	to, _ := model.ParseYM(p.ReferenceTo)
	first := time.Date(from.Year, time.Month(from.Month), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(to.Year, time.Month(to.Month), to.DaysIn(), 0, 0, 0, 0, time.UTC)

	lat, _, err := env.Climate.RegionCentroid(ctx, p.RegionID)
	if errors.Is(err, climate.ErrRegionNotFound) {
		return Outcome{}, fmt.Errorf("%w: region %q", ErrNotFound, p.RegionID)
	}
	if err != nil {
		return Outcome{}, err
	}
	// Make sure the data is there; a provider failure is not fatal — whatever is stored is used,
	// and if that is not enough the result says so (and why) instead of inventing numbers.
	_, fetchErr := env.Climate.EnsureRegionData(ctx, p.RegionID, first, last)
	if ctx.Err() != nil {
		return Outcome{}, ctx.Err()
	}
	days, err := env.Climate.ReadDaily(ctx, p.RegionID, first, last)
	if err != nil {
		return Outcome{}, err
	}
	out, err := model.RunClimateDrought(model.Input{
		Daily: days, LatDeg: lat, From: from, To: to,
		Params: model.Params{PrecipChangePct: p.PrecipChangePct, TempChangeC: p.TempChangeC, Scale: p.Scale},
	})
	if err != nil {
		return Outcome{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	o := Outcome{
		Status: out.Status, InsufficientReason: out.InsufficientReason, Params: p,
		Indicators: out.Indicators, Series: out.Series, Coverage: &out.Coverage,
		Assumptions: out.Assumptions, Limitations: out.Limitations,
	}
	if ds, err := env.Climate.Dataset(ctx); err == nil {
		o.DataSources = []DataSource{{Dataset: ds.Name, Source: ds.Source, License: ds.License, Coverage: round3(out.Coverage.Both)}}
	}
	if out.Status == model.StatusInsufficientData && fetchErr != nil {
		o.InsufficientReason += " " + fetchFailureTR(fetchErr)
	}
	return o, nil
}

// fetchFailureTR explains in Turkish why the climate data could not be completed.
func fetchFailureTR(err error) string {
	if errors.Is(err, climate.ErrQuota) {
		return "İklim veri sağlayıcısının (Open-Meteo) ücretsiz kotası doldu; eksik veriler indirilemedi."
	}
	return "Eksik iklim verileri indirilemedi (Open-Meteo erişilemiyor veya yanıt vermedi); kayıtlı veri yeterli değil."
}

func round3(v float64) float64 {
	if model.IsMissing(v) {
		return v
	}
	return math.Round(v*1000) / 1000
}

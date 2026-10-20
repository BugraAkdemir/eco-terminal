// Package simulation is the deterministic climate/drought scenario service of Eco Terminal.
//
// Numbers come only from internal/simulation/model (pure maths) fed by internal/simulation/climate
// (Open-Meteo ERA5 data in environmental_measurements). An optional Explainer (LLM) may only put the
// already-computed numbers into words; it can neither compute nor change them (AGENTS.md rule
// "LLM never produces numbers"). See docs/simulations/SIMULATION.md.
package simulation

import (
	"context"
	"errors"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/climate"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/model"
)

// Sentinel errors (handlers map them to HTTP codes).
var (
	ErrInvalid  = errors.New("simulation: invalid request") // 400 validation_error
	ErrNotFound = errors.New("simulation: not found")       // 404
	ErrUnknown  = errors.New("simulation: unknown model")   // 400
)

// Status values of a Result.
const (
	StatusOK               = model.StatusOK
	StatusInsufficientData = model.StatusInsufficientData
	StatusError            = model.StatusError
)

// Indicator and Series are the chart/table building blocks of a Result (shared with the pure model).
type (
	Indicator = model.Indicator
	Series    = model.Series
	Point     = model.Point
)

// ModelClimateDroughtV1 is the name of the built-in model.
const ModelClimateDroughtV1 = "climate_drought_v1"

// Sanity bounds of scenario parameters (a larger perturbation is not a meaningful delta-change).
const (
	MaxPrecipChangePct = 500.0
	MaxAbsTempChangeC  = 15.0
)

// RunRequest is the body of POST /simulations/run (openapi SimulationRunRequest). Scale is an
// optional extension (1, 3 or 6 months; default 3) that is not yet part of openapi.yaml.
type RunRequest struct {
	Name            string  `json:"name"`
	Model           string  `json:"model"`
	RegionID        string  `json:"region_id"`
	ReferenceFrom   string  `json:"reference_from"` // YYYY-MM
	ReferenceTo     string  `json:"reference_to"`   // YYYY-MM
	PrecipChangePct float64 `json:"precip_change_pct"`
	TempChangeC     float64 `json:"temp_change_c"`
	Explain         bool    `json:"explain"`
	Scale           int     `json:"scale,omitempty"`
}

// Params are the normalised parameters persisted in simulation_scenarios.params_json and echoed
// in Result.Params.
type Params struct {
	Model           string  `json:"model"`
	RegionID        string  `json:"region_id"`
	ReferenceFrom   string  `json:"reference_from"`
	ReferenceTo     string  `json:"reference_to"`
	PrecipChangePct float64 `json:"precip_change_pct"`
	TempChangeC     float64 `json:"temp_change_c"`
	Scale           int     `json:"scale"`
}

// DataSource documents a dataset used by a result.
type DataSource struct {
	Dataset  string  `json:"dataset"`
	Source   string  `json:"source"`
	License  string  `json:"license"`
	Coverage float64 `json:"coverage"` // fraction (0..1) of reference months with usable data
}

// Result is openapi SimulationResult (plus model, region_id and coverage as additive fields).
type Result struct {
	ID                 string          `json:"id"`
	ScenarioID         string          `json:"scenario_id"`
	Name               string          `json:"name"`
	Model              string          `json:"model"`
	RegionID           string          `json:"region_id"`
	Status             string          `json:"status"`
	Params             Params          `json:"params"`
	Indicators         []Indicator     `json:"indicators"`
	Series             []Series        `json:"series"`
	DataSources        []DataSource    `json:"data_sources"`
	Coverage           *model.Coverage `json:"coverage,omitempty"`
	Assumptions        []string        `json:"assumptions"`
	Limitations        []string        `json:"limitations"`
	InsufficientReason *string         `json:"insufficient_reason"`
	Narrative          *string         `json:"narrative"`
	CreatedAt          string          `json:"created_at"`
}

// Explainer turns a prompt into prose. The text provider of internal/ai is expected to satisfy it.
// The service passes a prompt that already contains every computed number and forbids new ones.
type Explainer interface {
	Explain(ctx context.Context, prompt string) (string, error)
}

// ClimateData is the data layer the service needs; *climate.Store implements it.
type ClimateData interface {
	EnsureRegionData(ctx context.Context, regionID string, from, to time.Time) (climate.EnsureReport, error)
	ReadDaily(ctx context.Context, regionID string, from, to time.Time) ([]model.DailyRecord, error)
	Dataset(ctx context.Context) (climate.DatasetInfo, error)
	RegionCentroid(ctx context.Context, regionID string) (lat, lon float64, err error)
}

var _ ClimateData = (*climate.Store)(nil)

package simulation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

type actorKey struct{}

// WithActor attaches the audit actor (role, IP) used for the audit_logs entry of a run.
func WithActor(ctx context.Context, a audit.Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// Service runs, persists and reads simulations.
type Service struct {
	DB        *sql.DB
	Climate   ClimateData
	Explainer Explainer // optional; nil => no narrative
	Registry  *Registry
	// ExplainTimeout bounds the optional narrative call (default 45 s). A slow or failing
	// explainer never fails or delays a run beyond this.
	ExplainTimeout time.Duration
}

// NewService builds a service with the built-in climate_drought_v1 model registered.
// explainer may be nil.
func NewService(db *sql.DB, climateData ClimateData, explainer Explainer) *Service {
	reg := NewRegistry()
	reg.Register(ClimateDroughtV1{})
	return &Service{DB: db, Climate: climateData, Explainer: explainer, Registry: reg, ExplainTimeout: 45 * time.Second}
}

// Normalize validates a request and returns the parameters that Run would persist (ErrInvalid /
// ErrUnknown on bad input). Handlers use it to prepare data before Run.
func (s *Service) Normalize(req RunRequest) (Params, error) {
	name := req.Model
	if name == "" {
		name = ModelClimateDroughtV1
	}
	m, err := s.Registry.Get(name)
	if err != nil {
		return Params{}, err
	}
	return m.Normalize(req)
}

// Run executes the requested scenario for userID, persists scenario + result and returns the
// result. Insufficient data is NOT an error: it returns Status "insufficient_data" with a Turkish
// reason and no numbers (and is persisted like any other run).
func (s *Service) Run(ctx context.Context, userID string, req RunRequest) (Result, error) {
	if strings.TrimSpace(userID) == "" {
		return Result{}, fmt.Errorf("%w: user is required", ErrInvalid)
	}
	name := req.Model
	if name == "" {
		name = ModelClimateDroughtV1
	}
	m, err := s.Registry.Get(name)
	if err != nil {
		return Result{}, err
	}
	params, err := m.Normalize(req)
	if err != nil {
		return Result{}, err
	}
	scName := strings.TrimSpace(req.Name)
	if len([]rune(scName)) > 120 {
		return Result{}, fmt.Errorf("%w: name longer than 120 characters", ErrInvalid)
	}
	if scName == "" {
		scName = fmt.Sprintf("Senaryo: yağış %+g%%, sıcaklık %+g °C (%s – %s)", params.PrecipChangePct, params.TempChangeC, params.ReferenceFrom, params.ReferenceTo)
	}

	out, err := m.Run(ctx, Env{Climate: s.Climate}, params)
	if err != nil {
		return Result{}, err
	}
	res := Result{
		Name: scName, Model: m.Name(), RegionID: params.RegionID, Status: out.Status, Params: out.Params,
		Indicators: nonNilI(out.Indicators), Series: nonNilS(out.Series), DataSources: nonNilD(out.DataSources),
		Coverage: out.Coverage, Assumptions: nonNilStr(out.Assumptions), Limitations: nonNilStr(out.Limitations),
	}
	if out.Status != StatusOK {
		reason := out.InsufficientReason
		res.InsufficientReason = &reason
		res.Indicators, res.Series = []Indicator{}, []Series{} // no numbers without enough data
	}

	// optional narrative: explains numbers that already exist; cannot change any of them
	if req.Explain && out.Status == StatusOK && s.Explainer != nil {
		if text := s.explain(ctx, res); text != "" {
			res.Narrative = &text
		}
	}

	if err := s.persist(ctx, userID, &res); err != nil {
		return Result{}, err
	}
	return res, nil
}

func (s *Service) persist(ctx context.Context, userID string, res *Result) error {
	now := storage.Now()
	res.ScenarioID, res.ID, res.CreatedAt = storage.NewID(), storage.NewID(), now
	paramsJSON, err := json.Marshal(res.Params)
	if err != nil {
		return err
	}
	stored := *res
	stored.Narrative = nil // narrative lives in its own column
	resultJSON, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	narrative := ""
	if res.Narrative != nil {
		narrative = *res.Narrative
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO simulation_scenarios (id, user_id, name, model, region_id, params_json, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`, res.ScenarioID, userID, res.Name, res.Model, res.RegionID, string(paramsJSON), now, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO simulation_results (id, scenario_id, status, result_json, narrative, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)`, res.ID, res.ScenarioID, res.Status, string(resultJSON), narrative, now, now); err != nil {
		return err
	}
	actor, ok := ctx.Value(actorKey{}).(audit.Actor)
	if !ok {
		actor = audit.Actor{ID: userID}
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "simulation.run", EntityType: "simulation_result", EntityID: res.ID,
		After: map[string]any{"scenario_id": res.ScenarioID, "model": res.Model, "region_id": res.RegionID, "status": res.Status,
			"precip_change_pct": res.Params.PrecipChangePct, "temp_change_c": res.Params.TempChangeC}}); err != nil {
		return err
	}
	return tx.Commit()
}

const selectResult = `SELECT r.id, r.scenario_id, r.status, r.result_json, r.narrative, r.created_at, sc.name, sc.user_id
	FROM simulation_results r JOIN simulation_scenarios sc ON sc.id = r.scenario_id`

type scanner interface{ Scan(dest ...any) error }

func scanResult(sc scanner) (Result, string, error) {
	var id, scenarioID, status, resultJSON, narrative, created, name, owner string
	if err := sc.Scan(&id, &scenarioID, &status, &resultJSON, &narrative, &created, &name, &owner); err != nil {
		return Result{}, "", err
	}
	var res Result
	if err := json.Unmarshal([]byte(resultJSON), &res); err != nil {
		return Result{}, "", fmt.Errorf("stored result %s is corrupt: %w", id, err)
	}
	res.ID, res.ScenarioID, res.Status, res.CreatedAt, res.Name = id, scenarioID, status, created, name
	res.Narrative = nil
	if narrative != "" {
		res.Narrative = &narrative
	}
	res.Indicators, res.Series, res.DataSources = nonNilI(res.Indicators), nonNilS(res.Series), nonNilD(res.DataSources)
	res.Assumptions, res.Limitations = nonNilStr(res.Assumptions), nonNilStr(res.Limitations)
	return res, owner, nil
}

// Get returns a result by id (ErrNotFound if unknown). Access control is the caller's job; use
// GetOwned for "own results only".
func (s *Service) Get(ctx context.Context, id string) (Result, error) {
	res, _, err := scanResult(s.DB.QueryRowContext(ctx, selectResult+` WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	return res, err
}

// GetOwned returns the result only if it belongs to userID; otherwise ErrNotFound (never reveals
// that the id exists). Pass anyOwner=true for staff (researcher/admin policies decided by the handler).
func (s *Service) GetOwned(ctx context.Context, userID, id string, anyOwner bool) (Result, error) {
	res, owner, err := scanResult(s.DB.QueryRowContext(ctx, selectResult+` WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	if !anyOwner && owner != userID {
		return Result{}, ErrNotFound
	}
	return res, nil
}

// List returns the user's scenarios, each with its latest result, newest first (at most limit,
// default 50, max 200).
func (s *Service) List(ctx context.Context, userID string, limit int) ([]Result, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, selectResult+`
		WHERE sc.user_id = ? AND r.id = (
			SELECT r2.id FROM simulation_results r2 WHERE r2.scenario_id = sc.id ORDER BY r2.created_at DESC, r2.rowid DESC LIMIT 1)
		ORDER BY r.created_at DESC, r.rowid DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Result{}
	for rows.Next() {
		res, _, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

// EnsureData downloads (or imports from the offline snapshot) the climate data a region needs for
// [from, to]; used by workers / admin actions, not by read endpoints.
func (s *Service) EnsureData(ctx context.Context, regionID string, from, to time.Time) error {
	_, err := s.Climate.EnsureRegionData(ctx, regionID, from, to)
	return err
}

func nonNilI(v []Indicator) []Indicator {
	if v == nil {
		return []Indicator{}
	}
	return v
}
func nonNilS(v []Series) []Series {
	if v == nil {
		return []Series{}
	}
	return v
}
func nonNilD(v []DataSource) []DataSource {
	if v == nil {
		return []DataSource{}
	}
	return v
}
func nonNilStr(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

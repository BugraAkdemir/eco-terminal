package app

import (
	"context"
	crand "crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/geographic"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/observations"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/review"
)

// demoObservations is a copy of datasets/seed/demo_observations.json (go:embed cannot reach outside
// the module; a test keeps both copies identical).
//
//go:embed demo_observations.json
var demoObservations []byte

// Demo account e-mails (".invalid" is a reserved TLD: these can never receive mail).
const (
	DemoCitizenEmail  = "demo.citizen@ecoterminal.invalid"
	DemoReviewerEmail = "demo.reviewer@ecoterminal.invalid"
)

type demoItem struct {
	District        string         `json:"district"`
	RecordType      string         `json:"record_type"`
	Category        string         `json:"category"`
	Title           string         `json:"title"`
	Description     string         `json:"description"`
	SpeciesGuess    string         `json:"species_guess"`
	SpeciesVerified string         `json:"species_verified"`
	Count           int            `json:"count"`
	DaysAgo         int            `json:"days_ago"`
	Sensitive       bool           `json:"sensitive"`
	Priority        string         `json:"priority"`
	Note            string         `json:"note"`
	Details         map[string]any `json:"details"`
	Steps           []string       `json:"steps"`
}

func randomPassword() string {
	var b [12]byte
	_, _ = crand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// ensureDemoUser returns the id of the demo account, creating it (and logging its generated
// credentials ONCE) when absent.
func (a *App) ensureDemoUser(ctx context.Context, email, role, name string) (string, error) {
	var id string
	err := a.db.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ?`, email).Scan(&id)
	if err == nil {
		return id, nil
	}
	pw := randomPassword()
	id, err = a.deps.Auth.CreateUser(ctx, a.db, email, pw, role, name)
	if err != nil {
		return "", err
	}
	_ = audit.Log(ctx, a.db, audit.Entry{Actor: audit.System, Action: "demo.user_create", EntityType: "user", EntityID: id,
		After: map[string]any{"role": role}})
	a.log.Warn("DEMO account created (shown once, demo mode only)", "email", email, "password", pw, "role", role)
	return id, nil
}

// seedDemo inserts clearly flagged (is_demo=1) observations through the normal service paths
// (validation, state machine, history, audit, notifications). Stable client refs let newer seed entries
// be added to an existing demo database without duplicating or resetting old records.
func (a *App) seedDemo(ctx context.Context) error {
	var doc struct {
		Observations []demoItem `json:"observations"`
	}
	if err := json.Unmarshal(demoObservations, &doc); err != nil {
		return fmt.Errorf("demo seed: %w", err)
	}
	citizenID, err := a.ensureDemoUser(ctx, DemoCitizenEmail, auth.RoleCitizen, "Demo Gönüllü")
	if err != nil {
		return err
	}
	reviewerID, err := a.ensureDemoUser(ctx, DemoReviewerEmail, auth.RoleReviewer, "Demo Uzman")
	if err != nil {
		return err
	}
	citizen, err := a.deps.Auth.GetUser(ctx, citizenID)
	if err != nil {
		return err
	}
	reviewer, err := a.deps.Auth.GetUser(ctx, reviewerID)
	if err != nil {
		return err
	}
	districts, err := a.deps.Geo.List(ctx, "district", "", "", true)
	if err != nil {
		return err
	}
	geoms := map[string]*geographic.Geometry{}
	for _, d := range districts {
		if g, err := geographic.ParseGeometry(d.Geometry); err == nil {
			geoms[d.Name] = g
		}
	}
	cActor := audit.Actor{ID: citizen.ID, Role: citizen.Role}
	rActor := audit.Actor{ID: reviewer.ID, Role: reviewer.Role}
	rv := review.NewService(a.db, a.deps.Observations)
	now := time.Now().UTC()
	rng := rand.New(rand.NewPCG(42, 1337)) // deterministic placement
	created := 0
	for i, it := range doc.Observations {
		g := geoms[it.District]
		if g == nil {
			return fmt.Errorf("demo seed: unknown district %q", it.District)
		}
		lon, lat := pointInside(g, rng)
		var exists int
		clientRef := fmt.Sprintf("demo-seed-%03d", i+1)
		if err := a.db.QueryRowContext(ctx, `SELECT count(*) FROM observations WHERE user_id=? AND client_ref=?`, citizenID, clientRef).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		details, _ := json.Marshal(it.Details)
		count := it.Count
		in := observations.CreateInput{
			RecordType: it.RecordType, Category: it.Category, Title: it.Title, Description: it.Description,
			SpeciesGuess: it.SpeciesGuess, CountObserved: &count, Details: details, Lat: &lat, Lon: &lon,
			LocationSource: "manual", ObservedAt: now.AddDate(0, 0, -it.DaysAgo).Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
			ClientRef: clientRef,
		}
		obs, _, err := a.deps.Observations.Create(ctx, citizen, cActor, in, observations.CreateOptions{Demo: true})
		if err != nil {
			return fmt.Errorf("demo seed %q: %w", it.Title, err)
		}
		created++
		for si, step := range it.Steps {
			d := review.Decision{Decision: step, Note: it.Note}
			if si == 0 {
				d.Priority = it.Priority
				if it.Sensitive {
					t := true
					d.MarkSensitive = &t
				}
			}
			if step == "verify" {
				d.CorrectedSpecies = it.SpeciesVerified
			}
			if _, err := rv.Decide(ctx, reviewer, rActor, obs.ID, d); err != nil {
				return fmt.Errorf("demo seed %q step %s: %w", it.Title, step, err)
			}
		}
	}
	_ = audit.Log(ctx, a.db, audit.Entry{Actor: audit.System, Action: "demo.seed", EntityType: "observation",
		After: map[string]any{"observations": created}})
	a.log.Info("demo data seeded (is_demo=1)", "observations", created)
	return nil
}

// pointInside returns a random point inside g (rejection sampling in its bbox).
func pointInside(g *geographic.Geometry, rng *rand.Rand) (lon, lat float64) {
	b := g.BBox()
	for i := 0; i < 500; i++ {
		x := b[0] + rng.Float64()*(b[2]-b[0])
		y := b[1] + rng.Float64()*(b[3]-b[1])
		if g.Contains(x, y) {
			return round6(x), round6(y)
		}
	}
	x, y := g.InteriorPoint()
	return round6(x), round6(y)
}

func round6(v float64) float64 { return float64(int64(v*1e6+0.5)) / 1e6 }

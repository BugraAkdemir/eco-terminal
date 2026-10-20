package simulation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/climate"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/model"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// the REAL committed ERA5 download (see docs/data_sources/climate.md)
const snapshotFile = "../../../datasets/environmental/kastamonu_era5_daily_1991_2025.json"

type env struct {
	db    *sql.DB
	store *climate.Store
	svc   *Service
}

func newEnv(t *testing.T, ex Explainer) *env {
	t.Helper()
	db, err := storage.OpenTemp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	now := storage.Now()
	for _, u := range []string{"u1", "u2"} {
		if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, role, status, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
			u, u+"@example.test", "x", "researcher", "active", now, now); err != nil {
			t.Fatal(err)
		}
	}
	insertRegion(t, db, "kastamonu", 41.38, 33.78)
	store := &climate.Store{DB: db, SnapshotPath: snapshotFile, Now: func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) }}
	return &env{db: db, store: store, svc: NewService(db, store, ex)}
}

func insertRegion(t *testing.T, db *sql.DB, id string, lat, lon float64) {
	t.Helper()
	now := storage.Now()
	if _, err := db.Exec(`INSERT INTO geographic_regions (id, kind, name, geojson, min_lon, max_lon, min_lat, max_lat, centroid_lon, centroid_lat, source, is_demo, created_at, updated_at)
		VALUES (?, 'custom', ?, '{"type":"Polygon","coordinates":[]}', ?, ?, ?, ?, ?, ?, 'test', 1, ?, ?)`,
		id, "Bölge "+id, lon-0.1, lon+0.1, lat-0.1, lat+0.1, lon, lat, now, now); err != nil {
		t.Fatal(err)
	}
}

func req(p, tc float64) RunRequest {
	return RunRequest{Name: "deneme", RegionID: "kastamonu", ReferenceFrom: "1991-01", ReferenceTo: "2020-12", PrecipChangePct: p, TempChangeC: tc}
}

func find(t *testing.T, r Result, key string) Indicator {
	t.Helper()
	for _, i := range r.Indicators {
		if i.Key == key {
			return i
		}
	}
	t.Fatalf("indicator %s missing", key)
	return Indicator{}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunKastamonuWithRealSnapshotAndPersistRoundTrip(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	res, err := e.svc.Run(ctx, "u1", req(-20, 2))
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusOK || res.InsufficientReason != nil || res.Narrative != nil {
		t.Fatalf("%+v", res)
	}
	if res.ID == "" || res.ScenarioID == "" || res.Name != "deneme" || res.Model != ModelClimateDroughtV1 || res.CreatedAt == "" {
		t.Fatalf("%+v", res)
	}
	if res.Params.ReferenceFrom != "1991-01" || res.Params.Scale != 3 || res.Params.PrecipChangePct != -20 || res.Params.TempChangeC != 2 {
		t.Fatalf("%+v", res.Params)
	}
	if len(res.Indicators) < 20 || len(res.Series) != 6 || len(res.Series[0].Points) != 360 {
		t.Fatalf("indicators=%d series=%d", len(res.Indicators), len(res.Series))
	}
	if len(res.DataSources) != 1 || res.DataSources[0].Dataset != "open-meteo-era5-daily" || res.DataSources[0].Coverage != 1 ||
		!strings.Contains(res.DataSources[0].License, "CC BY 4.0") {
		t.Fatalf("%+v", res.DataSources)
	}
	if len(res.Assumptions) < 5 || len(res.Limitations) < 5 || !strings.Contains(strings.Join(res.Limitations, " "), "tahmin") {
		t.Fatal("assumptions / limitations (Turkish) missing")
	}
	// expected physical direction for -20 % precipitation, +2 degC
	if find(t, res, "precip_pct_of_normal").Scenario != 80 || find(t, res, "pet_annual_mm").Change <= 0 ||
		find(t, res, "water_balance_annual_mm").Change >= 0 || find(t, res, "spi_dry_months").Change <= 0 {
		t.Fatalf("unexpected indicator directions: %+v", res.Indicators)
	}
	t.Logf("coverage: %+v", *res.Coverage)
	for _, a := range res.Assumptions {
		t.Log("assumption:", a)
	}
	for _, i := range res.Indicators {
		t.Logf("%-28s %-10s ref=%9.2f scen=%9.2f change=%9.2f  %s", i.Key, i.Unit, i.Reference, i.Scenario, i.Change, i.Label)
	}

	// persistence round trip: Get returns exactly what Run returned
	got, err := e.svc.Get(ctx, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mustJSON(t, got) != mustJSON(t, res) {
		t.Fatalf("Get differs from Run:\n%s\n%s", mustJSON(t, got)[:400], mustJSON(t, res)[:400])
	}
	var nScen, nRes int
	e.db.QueryRow(`SELECT count(*) FROM simulation_scenarios WHERE user_id='u1' AND model='climate_drought_v1' AND region_id='kastamonu'`).Scan(&nScen)
	e.db.QueryRow(`SELECT count(*) FROM simulation_results WHERE status='ok'`).Scan(&nRes)
	if nScen != 1 || nRes != 1 {
		t.Fatalf("rows %d %d", nScen, nRes)
	}
	var pj string
	e.db.QueryRow(`SELECT params_json FROM simulation_scenarios`).Scan(&pj)
	if !strings.Contains(pj, `"precip_change_pct":-20`) {
		t.Fatalf("params_json=%s", pj)
	}

	// ownership
	if _, err := e.svc.GetOwned(ctx, "u2", res.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user must not see it: %v", err)
	}
	if _, err := e.svc.GetOwned(ctx, "u2", res.ID, true); err != nil {
		t.Fatalf("staff may: %v", err)
	}
	if _, err := e.svc.Get(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	list, err := e.svc.List(ctx, "u1", 0)
	if err != nil || len(list) != 1 || list[0].ID != res.ID {
		t.Fatalf("%v %v", list, err)
	}
	if l2, _ := e.svc.List(ctx, "u2", 0); len(l2) != 0 {
		t.Fatal("u2 has no scenarios")
	}
}

func TestIdentityScenarioOnRealData(t *testing.T) {
	e := newEnv(t, nil)
	res, err := e.svc.Run(context.Background(), "u1", req(0, 0))
	if err != nil || res.Status != StatusOK {
		t.Fatalf("%v %+v", err, res.Status)
	}
	for _, i := range res.Indicators {
		if i.Change != 0 && i.Key != "precip_anomaly_z_mean" {
			t.Fatalf("%s changed under p=0,t=0: %v", i.Key, i.Change)
		}
	}
	for _, s := range res.Series {
		for _, p := range s.Points {
			if (p.Reference == nil) != (p.Scenario == nil) || (p.Reference != nil && *p.Reference != *p.Scenario) {
				t.Fatalf("series %s %s differs", s.Key, p.Month)
			}
		}
	}
	// reference values are plausible for the Kastamonu ERA5 cell
	if v := find(t, res, "precip_annual_mm").Reference; v < 400 || v > 900 {
		t.Fatalf("annual precip %v", v)
	}
	if v := find(t, res, "pet_annual_mm").Reference; v < 400 || v > 900 {
		t.Fatalf("annual PET %v", v)
	}
	// monotone with the strength of the cut on real data
	prev := math.Inf(1)
	for _, p := range []float64{0, -10, -20, -30} {
		r, err := e.svc.Run(context.Background(), "u1", req(p, 0))
		if err != nil {
			t.Fatal(err)
		}
		m := find(t, r, "spi_mean").Scenario
		if m > prev {
			t.Fatalf("spi_mean not monotone: %v after %v", m, prev)
		}
		prev = m
	}
}

func TestInsufficientDataPaths(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()

	// 1) reference period shorter than 10 years
	short := req(-20, 2)
	short.ReferenceFrom, short.ReferenceTo = "2015-01", "2020-12"
	res, err := e.svc.Run(ctx, "u1", short)
	if err != nil {
		t.Fatal(err)
	}
	assertNoNumbers(t, res)
	if !strings.Contains(*res.InsufficientReason, "10 yıl") {
		t.Fatalf("reason: %s", *res.InsufficientReason)
	}
	got, _ := e.svc.Get(ctx, res.ID)
	assertNoNumbers(t, got)
	if mustJSON(t, got) != mustJSON(t, res) {
		t.Fatal("insufficient result must round-trip too")
	}
	if l, _ := e.svc.List(ctx, "u1", 10); len(l) != 1 || l[0].Status != StatusInsufficientData {
		t.Fatalf("%v", l)
	}

	// 2) coverage below 90 %: period reaches far beyond the available data (future, inside the
	// provider delay and nothing stored) => only 2021-2025 exist after 2020... use 2016-2035
	future := req(0, 0)
	future.ReferenceFrom, future.ReferenceTo = "2016-01", "2035-12"
	res, err = e.svc.Run(ctx, "u1", future)
	if err != nil {
		t.Fatal(err)
	}
	assertNoNumbers(t, res)
	if !strings.Contains(*res.InsufficientReason, "kapsaması") {
		t.Fatalf("reason: %s", *res.InsufficientReason)
	}
	if res.Coverage == nil || res.Coverage.Both >= 0.9 {
		t.Fatalf("%+v", res.Coverage)
	}

	// 3) no data at all and the provider fails: region in another ERA5 cell
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":true,"reason":"down"}`)
	}))
	defer srv.Close()
	insertRegion(t, e.db, "other", 40.0, 30.0)
	e.store.Fetcher = &climate.Fetcher{BaseURL: srv.URL, MinInterval: time.Nanosecond, MaxRetries: 1, Backoff: time.Nanosecond}
	bad := req(-20, 2)
	bad.RegionID = "other"
	res, err = e.svc.Run(ctx, "u1", bad)
	if err != nil {
		t.Fatalf("a provider outage must not fail the run: %v", err)
	}
	assertNoNumbers(t, res)
	if !strings.Contains(*res.InsufficientReason, "indirilemedi") {
		t.Fatalf("reason: %s", *res.InsufficientReason)
	}
	var n int
	e.db.QueryRow(`SELECT count(*) FROM environmental_measurements WHERE region_id='other'`).Scan(&n)
	if n != 0 {
		t.Fatal("nothing may be fabricated for a region without data")
	}
	// 4) nothing configured at all (no provider, no snapshot cover) behaves the same
	e.store.Fetcher = nil
	res, err = e.svc.Run(ctx, "u1", bad)
	if err != nil || res.Status != StatusInsufficientData {
		t.Fatalf("%v %+v", err, res.Status)
	}
	assertNoNumbers(t, res)
}

func assertNoNumbers(t *testing.T, r Result) {
	t.Helper()
	if r.Status != StatusInsufficientData || r.InsufficientReason == nil || *r.InsufficientReason == "" {
		t.Fatalf("want insufficient_data with a reason: %+v", r.Status)
	}
	if len(r.Indicators) != 0 || len(r.Series) != 0 || r.Narrative != nil {
		t.Fatal("an insufficient result must not carry numbers or a narrative")
	}
	if r.Indicators == nil || r.Series == nil || r.Assumptions == nil || r.Limitations == nil || r.DataSources == nil {
		t.Fatal("collections must serialize as [] not null")
	}
	if len(r.Limitations) == 0 {
		t.Fatal("limitations must still be listed")
	}
}

func TestValidation(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	cases := map[string]func(r *RunRequest){
		"no region":     func(r *RunRequest) { r.RegionID = "" },
		"bad from":      func(r *RunRequest) { r.ReferenceFrom = "1991" },
		"bad to":        func(r *RunRequest) { r.ReferenceTo = "2020-13" },
		"reversed":      func(r *RunRequest) { r.ReferenceFrom, r.ReferenceTo = "2020-01", "1991-01" },
		"pre ERA5":      func(r *RunRequest) { r.ReferenceFrom = "1900-01" },
		"scale":         func(r *RunRequest) { r.Scale = 4 },
		"precip < -100": func(r *RunRequest) { r.PrecipChangePct = -150 },
		"precip huge":   func(r *RunRequest) { r.PrecipChangePct = 10000 },
		"temp huge":     func(r *RunRequest) { r.TempChangeC = 40 },
		"nan":           func(r *RunRequest) { r.TempChangeC = math.NaN() },
		"unknown model": func(r *RunRequest) { r.Model = "habitat_v9" },
		"name too long": func(r *RunRequest) { r.Name = strings.Repeat("a", 121) },
	}
	for name, mut := range cases {
		r := req(-20, 2)
		mut(&r)
		_, err := e.svc.Run(ctx, "u1", r)
		if !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrUnknown) {
			t.Fatalf("%s: want validation error, got %v", name, err)
		}
	}
	r := req(0, 0)
	r.RegionID = "does-not-exist"
	if _, err := e.svc.Run(ctx, "u1", r); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown region: %v", err)
	}
	if _, err := e.svc.Run(ctx, "", req(0, 0)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing user: %v", err)
	}
	var n int
	e.db.QueryRow(`SELECT count(*) FROM simulation_scenarios`).Scan(&n)
	if n != 0 {
		t.Fatalf("rejected requests must not persist anything (%d)", n)
	}
	// default name when none is given
	r = req(-10, 1)
	r.Name = ""
	res, err := e.svc.Run(ctx, "u1", r)
	if err != nil || !strings.Contains(res.Name, "yağış -10%") {
		t.Fatalf("%v %q", err, res.Name)
	}
}

// ───────────── narrative isolation ─────────────

type stubExplainer struct {
	fn     func(ctx context.Context, prompt string) (string, error)
	calls  atomic.Int32
	prompt atomic.Value
}

func (s *stubExplainer) Explain(ctx context.Context, prompt string) (string, error) {
	s.calls.Add(1)
	s.prompt.Store(prompt)
	return s.fn(ctx, prompt)
}

// numericFields serialises everything except the narrative: if the narrative path touched any
// number, this string would differ between runs.
func numericFields(t *testing.T, r Result) string {
	t.Helper()
	c := r
	c.ID, c.ScenarioID, c.CreatedAt, c.Narrative = "", "", "", nil
	return mustJSON(t, c)
}

func TestNarrativeNeverAltersNumbers(t *testing.T) {
	ctx := context.Background()
	base, err := newEnv(t, nil).svc.Run(ctx, "u1", req(-20, 2))
	if err != nil {
		t.Fatal(err)
	}

	// well-behaved explainer: quotes numbers that are in the prompt
	good := &stubExplainer{fn: func(ctx context.Context, prompt string) (string, error) {
		return "Yağış 20 yüzde azaltıldığında ve sıcaklık 2 °C artırıldığında su dengesi kötüleşir. Bu bir tahmin değildir.", nil
	}}
	rq := req(-20, 2)
	rq.Explain = true
	withN, err := newEnv(t, good).svc.Run(ctx, "u1", rq)
	if err != nil {
		t.Fatal(err)
	}
	if withN.Narrative == nil || !strings.Contains(*withN.Narrative, "tahmin değildir") || good.calls.Load() != 1 {
		t.Fatalf("narrative missing: %+v", withN.Narrative)
	}
	if numericFields(t, withN) != numericFields(t, base) {
		t.Fatal("the narrative path changed numeric fields")
	}
	// the prompt carries every computed number and forbids new ones
	p := good.prompt.Load().(string)
	for _, i := range base.Indicators {
		if !strings.Contains(p, i.Label) || !strings.Contains(p, fmtNum(i.Reference)) || !strings.Contains(p, fmtNum(i.Scenario)) {
			t.Fatalf("prompt lacks indicator %s", i.Key)
		}
	}
	if !strings.Contains(p, "Yeni sayı üretme") || !strings.Contains(p, "tahmin") {
		t.Fatal("prompt lacks the explain-only instruction")
	}

	// persisted narrative is returned by Get without touching the numbers
	e := newEnv(t, good)
	r2, _ := e.svc.Run(ctx, "u1", rq)
	got, _ := e.svc.Get(ctx, r2.ID)
	if got.Narrative == nil || *got.Narrative != *r2.Narrative || numericFields(t, got) != numericFields(t, base) {
		t.Fatal("stored narrative / numbers differ")
	}
	var col string
	e.db.QueryRow(`SELECT narrative FROM simulation_results WHERE id=?`, r2.ID).Scan(&col)
	if col != *r2.Narrative {
		t.Fatal("narrative column")
	}

	// an explainer that invents a number is discarded
	liar := &stubExplainer{fn: func(ctx context.Context, prompt string) (string, error) {
		return "Kuraklık riski yüzde 73.4 artacak.", nil
	}}
	res, err := newEnv(t, liar).svc.Run(ctx, "u1", rq)
	if err != nil || res.Narrative != nil || res.Status != StatusOK || numericFields(t, res) != numericFields(t, base) {
		t.Fatalf("hallucinated number must drop the narrative: %v %v", err, res.Narrative)
	}

	// failing, empty, panicking and hanging explainers: the result is still returned, without narrative
	fails := map[string]func(ctx context.Context, prompt string) (string, error){
		"error": func(ctx context.Context, p string) (string, error) { return "", errors.New("provider down") },
		"empty": func(ctx context.Context, p string) (string, error) { return "  ", nil },
		"panic": func(ctx context.Context, p string) (string, error) { panic("boom") },
		"hang": func(ctx context.Context, p string) (string, error) {
			time.Sleep(2 * time.Second)
			return "geç cevap", nil
		},
	}
	for name, fn := range fails {
		e := newEnv(t, &stubExplainer{fn: fn})
		e.svc.ExplainTimeout = 100 * time.Millisecond
		start := time.Now()
		res, err := e.svc.Run(ctx, "u1", rq)
		if err != nil || res.Status != StatusOK || res.Narrative != nil {
			t.Fatalf("%s: err=%v status=%s narrative=%v", name, err, res.Status, res.Narrative)
		}
		if numericFields(t, res) != numericFields(t, base) {
			t.Fatalf("%s: numbers differ", name)
		}
		if time.Since(start) > 1500*time.Millisecond {
			t.Fatalf("%s: run was delayed by the explainer", name)
		}
		if _, err := e.svc.Get(ctx, res.ID); err != nil {
			t.Fatalf("%s: result not persisted: %v", name, err)
		}
	}

	// nil explainer with explain=true, and explain=false with an explainer: no call, no narrative
	res, err = newEnv(t, nil).svc.Run(ctx, "u1", rq)
	if err != nil || res.Narrative != nil || res.Status != StatusOK {
		t.Fatal("nil explainer")
	}
	spy := &stubExplainer{fn: func(ctx context.Context, p string) (string, error) { return "x", nil }}
	if _, err := newEnv(t, spy).svc.Run(ctx, "u1", req(-20, 2)); err != nil || spy.calls.Load() != 0 {
		t.Fatalf("explain=false must not call the explainer (calls=%d)", spy.calls.Load())
	}
	// insufficient data never reaches the explainer
	short := rq
	short.ReferenceFrom, short.ReferenceTo = "2015-01", "2020-12"
	if r, err := newEnv(t, spy).svc.Run(ctx, "u1", short); err != nil || r.Narrative != nil || spy.calls.Load() != 0 {
		t.Fatal("insufficient data must not be narrated")
	}
}

func TestNumbersSubsetOf(t *testing.T) {
	prompt := "referans 603.11, senaryo 482.49, değişim -120.62; dönem 1991-01 – 2020-12; yağış -20 yüzde; 2 °C"
	ok := []string{
		"Yağış 482,49 mm olur.", "Yaklaşık 603 mm ve 482.5 mm.", "1991 ile 2020 arası, 20 yüzde, 2 derece.",
		"1. Birinci madde\n2. İkinci madde", "Sayı yok.", "-120.62 fark",
	}
	for _, s := range ok {
		if !numbersSubsetOf(s, prompt) {
			t.Fatalf("should pass: %q", s)
		}
	}
	bad := []string{"Yüzde 37 artar.", "605 mm", "482.49 ve 500", "2030 yılında"}
	for _, s := range bad {
		if numbersSubsetOf(s, prompt) {
			t.Fatalf("should fail: %q", s)
		}
	}
}

// ───────────── compare, registry, measurements ─────────────

func TestCompare(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	a, _ := e.svc.Run(ctx, "u1", req(-20, 0))
	b, _ := e.svc.Run(ctx, "u1", req(-20, 2))
	c, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !c.SameBaseline || len(c.Deltas) != len(a.Indicators) || len(c.OnlyInA)+len(c.OnlyInB) != 0 {
		t.Fatalf("%+v", c)
	}
	for _, d := range c.Deltas {
		ia, ib := find(t, a, d.Key), find(t, b, d.Key)
		if d.AScenario != ia.Scenario || d.BScenario != ib.Scenario || math.Abs(d.Delta-(ib.Scenario-ia.Scenario)) > 0.0051 || d.ReferenceDiffers {
			t.Fatalf("%+v", d)
		}
	}
	// only temperature differs: PET and temperature deltas are positive, precipitation delta is 0
	for _, d := range c.Deltas {
		switch d.Key {
		case "pet_annual_mm", "temp_annual_c":
			if d.Delta <= 0 {
				t.Fatalf("%s delta %v", d.Key, d.Delta)
			}
		case "precip_annual_mm", "spi_mean":
			if d.Delta != 0 {
				t.Fatalf("%s delta %v", d.Key, d.Delta)
			}
		}
	}
	// different baseline is flagged
	other := req(-20, 0)
	other.ReferenceFrom = "1981-01"
	_ = other
	short := req(0, 0)
	short.ReferenceFrom, short.ReferenceTo = "2015-01", "2020-12"
	ins, _ := e.svc.Run(ctx, "u1", short)
	if _, err := Compare(a, ins); !errors.Is(err, ErrInvalid) {
		t.Fatalf("insufficient result must be refused: %v", err)
	}
	r2 := req(-20, 0)
	r2.ReferenceFrom, r2.ReferenceTo = "1996-01", "2025-12"
	d2, _ := e.svc.Run(ctx, "u1", r2)
	c2, err := Compare(a, d2)
	if err != nil || c2.SameBaseline {
		t.Fatalf("%v same=%v", err, c2.SameBaseline)
	}
	flagged := false
	for _, d := range c2.Deltas {
		flagged = flagged || d.ReferenceDiffers
	}
	if !flagged {
		t.Fatal("ReferenceDiffers should be set when baselines differ")
	}
}

type dummyModel struct{ ran *int }

func (dummyModel) Name() string { return "dummy_v1" }
func (dummyModel) Normalize(r RunRequest) (Params, error) {
	return Params{Model: "dummy_v1", RegionID: r.RegionID, ReferenceFrom: "2000-01", ReferenceTo: "2010-12"}, nil
}
func (d dummyModel) Run(ctx context.Context, env Env, p Params) (Outcome, error) {
	*d.ran++
	return Outcome{Status: StatusOK, Params: p, Indicators: []Indicator{{Key: "k", Label: "L", Unit: "u", Reference: 1, Scenario: 2, Change: 1}}}, nil
}

func TestRegistryExtensibility(t *testing.T) {
	e := newEnv(t, nil)
	ran := 0
	e.svc.Registry.Register(dummyModel{&ran})
	names := e.svc.Registry.Names()
	if !reflect.DeepEqual(names, []string{"climate_drought_v1", "dummy_v1"}) {
		t.Fatal(names)
	}
	res, err := e.svc.Run(context.Background(), "u1", RunRequest{Model: "dummy_v1", RegionID: "kastamonu"})
	if err != nil || ran != 1 || res.Model != "dummy_v1" || len(res.Indicators) != 1 {
		t.Fatalf("%v %d %+v", err, ran, res)
	}
	got, _ := e.svc.Get(context.Background(), res.ID)
	if got.Model != "dummy_v1" || got.Indicators[0].Key != "k" {
		t.Fatalf("%+v", got)
	}
	if _, err := e.svc.Registry.Get("nope"); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
}

func TestMeasurementsSeries(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()

	// nothing ingested yet: empty series, zero coverage, nothing invented
	ms, err := e.svc.MeasurementsSeries(ctx, "kastamonu", "precipitation", "2000-01", "2000-12", "monthly")
	if err != nil || len(ms.Points) != 0 || ms.Coverage != 0 || ms.Unit != "mm" {
		t.Fatalf("%v %+v", err, ms)
	}
	if err := e.svc.EnsureData(ctx, "kastamonu", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2000, 12, 31, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	ms, err = e.svc.MeasurementsSeries(ctx, "kastamonu", "precipitation", "2000-01", "2000-12", "monthly")
	if err != nil || len(ms.Points) != 12 || ms.Coverage != 1 || ms.Dataset != "open-meteo-era5-daily" || ms.Points[0].TS != "2000-01" {
		t.Fatalf("%v %+v", err, ms)
	}
	// the monthly value is the sum of the stored daily values
	daily, err := e.svc.MeasurementsSeries(ctx, "kastamonu", "precipitation", "2000-01-01", "2000-01-31", "daily")
	if err != nil || len(daily.Points) != 31 || daily.Coverage != 1 {
		t.Fatalf("%v %+v", err, daily)
	}
	var sum float64
	for _, p := range daily.Points {
		sum += p.Value
	}
	if math.Abs(sum-ms.Points[0].Value) > 0.01 {
		t.Fatalf("monthly %v != sum of daily %v", ms.Points[0].Value, sum)
	}
	temp, _ := e.svc.MeasurementsSeries(ctx, "kastamonu", "temperature_mean", "2000-01", "2000-12", "")
	if temp.Unit != "degC" || temp.Resolution != "monthly" || len(temp.Points) != 12 || temp.Points[6].Value < temp.Points[0].Value {
		t.Fatalf("%+v", temp)
	}
	// full stored range when from/to are omitted
	all, _ := e.svc.MeasurementsSeries(ctx, "kastamonu", "precipitation", "", "", "monthly")
	if len(all.Points) != 12 || all.From != "2000-01-01" || all.To != "2000-12-31" {
		t.Fatalf("%+v", all)
	}
	// partial coverage: a requested range that is half stored
	half, _ := e.svc.MeasurementsSeries(ctx, "kastamonu", "precipitation", "2000-07", "2001-06", "monthly")
	if len(half.Points) != 6 || math.Abs(half.Coverage-0.5) > 1e-9 {
		t.Fatalf("%+v", half)
	}
	// a month with too many missing days is dropped
	if _, err := e.db.Exec(`DELETE FROM environmental_measurements WHERE ts >= '2000-03-01' AND ts <= '2000-03-10' AND measurement_type='precipitation'`); err != nil {
		t.Fatal(err)
	}
	ms, _ = e.svc.MeasurementsSeries(ctx, "kastamonu", "precipitation", "2000-01", "2000-12", "monthly")
	if len(ms.Points) != 11 || math.Abs(ms.Coverage-11.0/12) > 0.001 {
		t.Fatalf("%d %v", len(ms.Points), ms.Coverage)
	}
	daily, _ = e.svc.MeasurementsSeries(ctx, "kastamonu", "precipitation", "2000-03-01", "2000-03-31", "daily")
	if len(daily.Points) != 21 || math.Abs(daily.Coverage-21.0/31) > 0.001 {
		t.Fatalf("%d %v", len(daily.Points), daily.Coverage)
	}
	// errors
	if _, err := e.svc.MeasurementsSeries(ctx, "kastamonu", "humidity", "", "", ""); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := e.svc.MeasurementsSeries(ctx, "kastamonu", "precipitation", "", "", "hourly"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := e.svc.MeasurementsSeries(ctx, "kastamonu", "precipitation", "garbage", "", ""); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := e.svc.MeasurementsSeries(ctx, "kastamonu", "precipitation", "2001-01", "2000-01", ""); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := e.svc.MeasurementsSeries(ctx, "nope", "precipitation", "", "", ""); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

var _ = model.Missing

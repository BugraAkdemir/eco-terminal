package climate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/model"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

const snapshotFile = "../../../../datasets/environmental/kastamonu_era5_daily_1991_2025.json"

func d(s string) time.Time {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		panic(err)
	}
	return t
}

// fakeAPI is an httptest server that answers like Open-Meteo, slicing the REAL committed ERA5
// snapshot (so every value in these tests comes from a genuine provider download).
type fakeAPI struct {
	*httptest.Server
	mu       sync.Mutex
	requests []url2
	script   []func(w http.ResponseWriter, r *http.Request) bool // consulted in order; true = handled
}

type url2 struct{ Start, End, Models, Daily, TZ, Lat, Lon string }

func newFakeAPI(t *testing.T, sn *Snapshot) *fakeAPI {
	t.Helper()
	ser, err := sn.Series()
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeAPI{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f.mu.Lock()
		f.requests = append(f.requests, url2{q.Get("start_date"), q.Get("end_date"), q.Get("models"), q.Get("daily"), q.Get("timezone"), q.Get("latitude"), q.Get("longitude")})
		var script func(w http.ResponseWriter, r *http.Request) bool
		if len(f.script) > 0 {
			script, f.script = f.script[0], f.script[1:]
		}
		f.mu.Unlock()
		if script != nil && script(w, r) {
			return
		}
		from, to := d(q.Get("start_date")), d(q.Get("end_date"))
		part := ser.Slice(from, to)
		if part == nil || !part.Dates[0].Equal(from) || !part.Dates[len(part.Dates)-1].Equal(to) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":true,"reason":"out of range"}`)
			return
		}
		type daily struct {
			Time   []string   `json:"time"`
			Precip []*float64 `json:"precipitation_sum"`
			Temp   []*float64 `json:"temperature_2m_mean"`
		}
		dd := daily{}
		for i, day := range part.Dates {
			dd.Time = append(dd.Time, day.Format(dateLayout))
			p, tm := part.Precip[i], part.Temp[i]
			if !model.IsMissing(p) {
				dd.Precip = append(dd.Precip, &p)
			} else {
				dd.Precip = append(dd.Precip, nil)
			}
			if !model.IsMissing(tm) {
				dd.Temp = append(dd.Temp, &tm)
			} else {
				dd.Temp = append(dd.Temp, nil)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"latitude": 41.5, "longitude": 33.75, "elevation": 778.0,
			"daily_units": map[string]string{"time": "iso8601", "precipitation_sum": "mm", "temperature_2m_mean": "°C"},
			"daily":       dd,
		})
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAPI) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *fakeAPI) reqs() []url2 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url2(nil), f.requests...)
}

func testFetcher(base string) (*Fetcher, *[]time.Duration) {
	var waits []time.Duration
	var mu sync.Mutex
	f := &Fetcher{BaseURL: base, Model: DefaultModel, Client: &http.Client{Timeout: 5 * time.Second}, MinInterval: 10 * time.Millisecond,
		MaxRetries: 3, Backoff: 2 * time.Second, Backoff429: 15 * time.Second}
	f.sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		waits = append(waits, d)
		mu.Unlock()
		return ctx.Err()
	}
	return f, &waits
}

func loadSnap(t *testing.T) *Snapshot {
	t.Helper()
	sn, err := LoadSnapshot(snapshotFile)
	if err != nil {
		t.Fatal(err)
	}
	return sn
}

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := storage.OpenTemp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertRegion(t *testing.T, db *sql.DB, id string, lat, lon float64) {
	t.Helper()
	now := storage.Now()
	_, err := db.Exec(`INSERT INTO geographic_regions (id, kind, name, geojson, min_lon, max_lon, min_lat, max_lat, centroid_lon, centroid_lat, source, is_demo, created_at, updated_at)
		VALUES (?, 'custom', ?, '{"type":"Polygon","coordinates":[]}', ?, ?, ?, ?, ?, ?, 'test', 1, ?, ?)`,
		id, id, lon-0.1, lon+0.1, lat-0.1, lat+0.1, lon, lat, now, now)
	if err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, db *sql.DB, region string) (precip, temp int) {
	t.Helper()
	db.QueryRow(`SELECT count(*) FROM environmental_measurements WHERE region_id=? AND measurement_type='precipitation'`, region).Scan(&precip)
	db.QueryRow(`SELECT count(*) FROM environmental_measurements WHERE region_id=? AND measurement_type='temperature_mean'`, region).Scan(&temp)
	return
}

// ───────────── fetcher ─────────────

func TestFetcherParsesRealResponseFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/openmeteo_era5_2020-02-27_2020-03-02.json")
	if err != nil {
		t.Fatal(err)
	}
	var got url2
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		got = url2{q.Get("start_date"), q.Get("end_date"), q.Get("models"), q.Get("daily"), q.Get("timezone"), q.Get("latitude"), q.Get("longitude")}
		w.Write(body)
	}))
	defer srv.Close()
	f, _ := testFetcher(srv.URL)
	s, err := f.FetchDaily(context.Background(), 41.38, 33.78, d("2020-02-27"), d("2020-03-02"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Models != "era5" || got.TZ != "UTC" || got.Daily != "precipitation_sum,temperature_2m_mean" || got.Start != "2020-02-27" || got.End != "2020-03-02" || got.Lat != "41.3800" || got.Lon != "33.7800" {
		t.Fatalf("request params: %+v", got)
	}
	if len(s.Dates) != 5 || s.GridLat != 41.5 || s.GridLon != 33.75 || s.ElevationM != 778 {
		t.Fatalf("%+v", s)
	}
	wantP := []float64{0.4, 5.6, 5.0, 0.1, 0}
	wantT := []float64{10.8, 7.1, 5.5, 5.2, 7.0}
	for i := range wantP {
		if s.Precip[i] != wantP[i] || s.Temp[i] != wantT[i] {
			t.Fatalf("day %d: %v %v", i, s.Precip[i], s.Temp[i])
		}
	}
	if s.Dates[2].Format(dateLayout) != "2020-02-29" {
		t.Fatal("leap day missing")
	}
	// the fixture IS the first 5 days of a genuine response; the snapshot must agree with it
	sn := loadSnap(t)
	ser, _ := sn.Series()
	part := ser.Slice(d("2020-02-27"), d("2020-03-02"))
	for i := range wantP {
		if part.Precip[i] != wantP[i] || part.Temp[i] != wantT[i] {
			t.Fatalf("snapshot disagrees with live response on day %d", i)
		}
	}
}

func TestFetcherSplitsIntoYearlyChunks(t *testing.T) {
	api := newFakeAPI(t, loadSnap(t))
	f, waits := testFetcher(api.URL)
	s, err := f.FetchDaily(context.Background(), 41.38, 33.78, d("2019-06-01"), d("2021-02-10"))
	if err != nil {
		t.Fatal(err)
	}
	r := api.reqs()
	if len(r) != 3 || r[0].Start != "2019-06-01" || r[0].End != "2019-12-31" || r[1].Start != "2020-01-01" || r[1].End != "2020-12-31" || r[2].Start != "2021-01-01" || r[2].End != "2021-02-10" {
		t.Fatalf("chunks: %+v", r)
	}
	if len(s.Dates) != 214+366+41 {
		t.Fatalf("len=%d", len(s.Dates))
	}
	for i := 1; i < len(s.Dates); i++ {
		if !s.Dates[i].Equal(s.Dates[i-1].AddDate(0, 0, 1)) {
			t.Fatalf("not consecutive at %d", i)
		}
	}
	// politeness: the throttle slept between requests (never more than MinInterval)
	for _, w := range *waits {
		if w <= 0 || w > f.MinInterval {
			t.Fatalf("throttle wait %v", w)
		}
	}
	if _, err := f.FetchDaily(context.Background(), 41, 33, d("2020-02-01"), d("2020-01-01")); err == nil {
		t.Fatal("reversed range must fail")
	}
	if _, err := f.FetchDaily(context.Background(), 91, 33, d("2020-01-01"), d("2020-01-02")); err == nil {
		t.Fatal("bad latitude must fail")
	}
}

func jsonReason(status int, reason string, hdr map[string]string) func(w http.ResponseWriter, r *http.Request) bool {
	return func(w http.ResponseWriter, r *http.Request) bool {
		for k, v := range hdr {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"error":true,"reason":%q}`, reason)
		return true
	}
}

func TestFetcherRetryAndBackoff(t *testing.T) {
	api := newFakeAPI(t, loadSnap(t))
	f, waits := testFetcher(api.URL)
	ctx := context.Background()

	// 429 with Retry-After: waits exactly that long, then succeeds
	api.script = []func(http.ResponseWriter, *http.Request) bool{jsonReason(429, "Minutely API request limit exceeded. Please try again in one minute.", map[string]string{"Retry-After": "7"})}
	if _, err := f.FetchDaily(ctx, 41.38, 33.78, d("2020-01-01"), d("2020-01-10")); err != nil {
		t.Fatal(err)
	}
	if api.count() != 2 {
		t.Fatalf("requests=%d", api.count())
	}
	var retryWaits []time.Duration
	for _, w := range *waits {
		if w >= time.Second {
			retryWaits = append(retryWaits, w)
		}
	}
	if len(retryWaits) != 1 || retryWaits[0] != 7*time.Second {
		t.Fatalf("429 waits: %v", retryWaits)
	}

	// 500, 503 then success: exponential back-off 2 s, 4 s
	*waits = nil
	before := api.count()
	api.script = []func(http.ResponseWriter, *http.Request) bool{jsonReason(500, "boom", nil), jsonReason(503, "busy", nil)}
	if _, err := f.FetchDaily(ctx, 41.38, 33.78, d("2020-01-01"), d("2020-01-10")); err != nil {
		t.Fatal(err)
	}
	if api.count()-before != 3 {
		t.Fatalf("requests=%d", api.count()-before)
	}
	retryWaits = nil
	for _, w := range *waits {
		if w >= time.Second {
			retryWaits = append(retryWaits, w)
		}
	}
	if len(retryWaits) != 2 || retryWaits[0] != 2*time.Second || retryWaits[1] != 4*time.Second {
		t.Fatalf("5xx back-off: %v", retryWaits)
	}

	// 429 without Retry-After uses the 429 base (15 s)
	*waits = nil
	api.script = []func(http.ResponseWriter, *http.Request) bool{jsonReason(429, "Minutely API request limit exceeded", nil)}
	if _, err := f.FetchDaily(ctx, 41.38, 33.78, d("2020-01-01"), d("2020-01-10")); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range *waits {
		if w == 15*time.Second {
			found = true
		}
	}
	if !found {
		t.Fatalf("waits=%v", *waits)
	}

	// client errors are not retried
	before = api.count()
	api.script = []func(http.ResponseWriter, *http.Request) bool{jsonReason(400, "Parameter 'start_date' is out of allowed range", nil)}
	_, err := f.FetchDaily(ctx, 41.38, 33.78, d("2020-01-01"), d("2020-01-10"))
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 400 || !strings.Contains(ae.Reason, "out of allowed range") || api.count()-before != 1 {
		t.Fatalf("400: err=%v requests=%d", err, api.count()-before)
	}

	// exhausted daily quota is surfaced as ErrQuota without retries
	before = api.count()
	api.script = []func(http.ResponseWriter, *http.Request) bool{jsonReason(429, "Daily API request limit exceeded. Please try again tomorrow.", nil)}
	_, err = f.FetchDaily(ctx, 41.38, 33.78, d("2020-01-01"), d("2020-01-10"))
	if !errors.Is(err, ErrQuota) || api.count()-before != 1 {
		t.Fatalf("quota: err=%v requests=%d", err, api.count()-before)
	}

	// persistent 500: gives up after MaxRetries (3 retries => 4 attempts)
	before = api.count()
	api.script = []func(http.ResponseWriter, *http.Request) bool{jsonReason(500, "x", nil), jsonReason(500, "x", nil), jsonReason(500, "x", nil), jsonReason(500, "x", nil), jsonReason(500, "x", nil)}
	if _, err = f.FetchDaily(ctx, 41.38, 33.78, d("2020-01-01"), d("2020-01-10")); err == nil || api.count()-before != 4 {
		t.Fatalf("persistent 500: err=%v requests=%d", err, api.count()-before)
	}

	// cancelled context stops promptly
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.FetchDaily(cctx, 41.38, 33.78, d("2020-01-01"), d("2020-01-10")); err == nil {
		t.Fatal("cancelled context must fail")
	}
}

func TestFetcherRejectsBadPayloads(t *testing.T) {
	for name, body := range map[string]string{
		"units":       `{"latitude":41.5,"longitude":33.75,"elevation":1,"daily_units":{"precipitation_sum":"inch","temperature_2m_mean":"°C"},"daily":{"time":["2020-01-01"],"precipitation_sum":[1],"temperature_2m_mean":[1]}}`,
		"length":      `{"latitude":41.5,"longitude":33.75,"elevation":1,"daily_units":{"precipitation_sum":"mm","temperature_2m_mean":"°C"},"daily":{"time":["2020-01-01"],"precipitation_sum":[1],"temperature_2m_mean":[1]}}`,
		"date":        `{"latitude":41.5,"longitude":33.75,"elevation":1,"daily_units":{"precipitation_sum":"mm","temperature_2m_mean":"°C"},"daily":{"time":["2020-01-01","2020-01-03"],"precipitation_sum":[1,1],"temperature_2m_mean":[1,1]}}`,
		"implausible": `{"latitude":41.5,"longitude":33.75,"elevation":1,"daily_units":{"precipitation_sum":"mm","temperature_2m_mean":"°C"},"daily":{"time":["2020-01-01","2020-01-02"],"precipitation_sum":[-5,1],"temperature_2m_mean":[1,1]}}`,
		"not json":    `<html>`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		f, _ := testFetcher(srv.URL)
		_, err := f.FetchDaily(context.Background(), 41, 33, d("2020-01-01"), d("2020-01-02"))
		srv.Close()
		if err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
	// nulls are legal and become Missing
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"latitude":41.5,"longitude":33.75,"elevation":1,"daily_units":{"precipitation_sum":"mm","temperature_2m_mean":"°C"},"daily":{"time":["2020-01-01","2020-01-02"],"precipitation_sum":[null,2],"temperature_2m_mean":[1,null]}}`)
	}))
	defer srv.Close()
	f, _ := testFetcher(srv.URL)
	s, err := f.FetchDaily(context.Background(), 41, 33, d("2020-01-01"), d("2020-01-02"))
	if err != nil || !model.IsMissing(s.Precip[0]) || s.Precip[1] != 2 || s.Temp[0] != 1 || !model.IsMissing(s.Temp[1]) {
		t.Fatalf("%v %+v", err, s)
	}
}

// ───────────── store ─────────────

func newStore(t *testing.T, api *fakeAPI, snapshot bool) (*Store, *sql.DB) {
	t.Helper()
	db := newDB(t)
	st := &Store{DB: db, Now: func() time.Time { return d("2026-10-20") }}
	if api != nil {
		st.Fetcher, _ = testFetcher(api.URL)
	}
	if snapshot {
		st.SnapshotPath = snapshotFile
	}
	return st, db
}

func TestEnsureRegionDataIsIdempotentAndFetchesOnlyMissing(t *testing.T) {
	api := newFakeAPI(t, loadSnap(t))
	st, db := newStore(t, api, false)
	insertRegion(t, db, "r1", 41.39, 33.78)
	ctx := context.Background()

	rep, err := st.EnsureRegionData(ctx, "r1", d("2018-01-01"), d("2019-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	if rep.FromAPI != 365+365 || rep.AlreadyPresent || api.count() != 2 {
		t.Fatalf("%+v requests=%d", rep, api.count())
	}
	if p, tm := countRows(t, db, "r1"); p != 730 || tm != 730 {
		t.Fatalf("rows %d/%d", p, tm)
	}
	// second call: nothing to do, no HTTP
	rep, err = st.EnsureRegionData(ctx, "r1", d("2018-03-01"), d("2019-06-30"))
	if err != nil || !rep.AlreadyPresent || api.count() != 2 {
		t.Fatalf("%+v %v requests=%d", rep, err, api.count())
	}
	// extend by one year on each side: exactly the two missing years are requested
	if _, err = st.EnsureRegionData(ctx, "r1", d("2017-01-01"), d("2020-12-31")); err != nil {
		t.Fatal(err)
	}
	r := api.reqs()
	if len(r) != 4 || r[2].Start != "2017-01-01" || r[2].End != "2017-12-31" || r[3].Start != "2020-01-01" || r[3].End != "2020-12-31" {
		t.Fatalf("requests: %+v", r)
	}
	p, tm := countRows(t, db, "r1")
	if p != 365*3+366 || tm != p {
		t.Fatalf("rows %d/%d", p, tm)
	}
	// stored values equal the provider's, with provenance columns
	var v float64
	var unit, res, flag string
	db.QueryRow(`SELECT value, unit, spatial_resolution, quality_flag FROM environmental_measurements WHERE region_id='r1' AND ts='2020-02-29' AND measurement_type='precipitation'`).Scan(&v, &unit, &res, &flag)
	if v != 5.0 || unit != "mm" || flag != "ok" || !strings.Contains(res, "0.25deg") || !strings.Contains(res, "41.50N") {
		t.Fatalf("row: %v %q %q %q", v, unit, res, flag)
	}
	db.QueryRow(`SELECT value, unit FROM environmental_measurements WHERE region_id='r1' AND ts='2020-02-29' AND measurement_type='temperature_mean'`).Scan(&v, &unit)
	if v != 5.5 || unit != "degC" {
		t.Fatalf("temp row: %v %q", v, unit)
	}
	// dataset provenance row
	ds, err := st.Dataset(ctx)
	if err != nil || ds.Name != DatasetName || !strings.Contains(ds.License, "CC BY 4.0") || ds.RetrievedAt == "" || !strings.Contains(ds.SpatialResolution, "0.25") {
		t.Fatalf("%+v %v", ds, err)
	}
	// calling EnsureDataset again does not duplicate it
	if _, err := st.EnsureDataset(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM environmental_datasets`).Scan(&n)
	if n != 1 {
		t.Fatalf("datasets=%d", n)
	}
	// a second region is independent
	insertRegion(t, db, "r2", 41.45, 33.7)
	if _, err = st.EnsureRegionData(ctx, "r2", d("2019-01-01"), d("2019-12-31")); err != nil {
		t.Fatal(err)
	}
	if p, _ := countRows(t, db, "r2"); p != 365 {
		t.Fatalf("r2 rows %d", p)
	}
}

func TestEnsureRegionDataConcurrentCallsFetchOnce(t *testing.T) {
	api := newFakeAPI(t, loadSnap(t))
	st, db := newStore(t, api, false)
	insertRegion(t, db, "r1", 41.39, 33.78)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := st.EnsureRegionData(context.Background(), "r1", d("2019-01-01"), d("2019-12-31")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if api.count() != 1 {
		t.Fatalf("expected a single provider call, got %d", api.count())
	}
}

func TestEnsureRegionDataClipsToProviderDelay(t *testing.T) {
	api := newFakeAPI(t, loadSnap(t))
	st, db := newStore(t, api, false)
	st.Now = func() time.Time { return d("2025-12-31") } // limit = 2025-12-25
	insertRegion(t, db, "r1", 41.39, 33.78)
	rep, err := st.EnsureRegionData(context.Background(), "r1", d("2025-12-01"), d("2026-02-01"))
	if err != nil {
		t.Fatal(err)
	}
	r := api.reqs()
	if !rep.ClippedForDelay || len(r) != 1 || r[0].End != "2025-12-25" {
		t.Fatalf("%+v %+v", rep, r)
	}
	// entirely inside the delay window: nothing to do
	rep, err = st.EnsureRegionData(context.Background(), "r1", d("2025-12-28"), d("2026-02-01"))
	if err != nil || !rep.AlreadyPresent || api.count() != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
}

func TestEnsureRegionDataKeepsProgressWhenProviderFails(t *testing.T) {
	api := newFakeAPI(t, loadSnap(t))
	st, db := newStore(t, api, false)
	st.Fetcher.MaxRetries = 1
	insertRegion(t, db, "r1", 41.39, 33.78)
	ok := func(w http.ResponseWriter, r *http.Request) bool { return false }
	// year 1 ok, year 2 fails (twice: attempt + 1 retry)
	api.script = []func(http.ResponseWriter, *http.Request) bool{ok, jsonReason(500, "x", nil), jsonReason(500, "x", nil)}
	rep, err := st.EnsureRegionData(context.Background(), "r1", d("2018-01-01"), d("2019-12-31"))
	if err == nil || rep.ProviderError == nil || rep.FromAPI != 365 {
		t.Fatalf("%+v %v", rep, err)
	}
	// the successful year is kept and a retry fetches only the failed year
	n := api.count()
	rep, err = st.EnsureRegionData(context.Background(), "r1", d("2018-01-01"), d("2019-12-31"))
	if err != nil || rep.FromAPI != 365 || api.count()-n != 1 {
		t.Fatalf("%+v %v requests=%d", rep, err, api.count()-n)
	}
	if p, _ := countRows(t, db, "r1"); p != 730 {
		t.Fatalf("rows %d", p)
	}
}

func TestEnsureRegionDataUsesSnapshotBeforeNetwork(t *testing.T) {
	api := newFakeAPI(t, loadSnap(t))
	st, db := newStore(t, api, true)
	insertRegion(t, db, "kast", 41.40, 33.80) // same 0.25 deg ERA5 cell (41.5, 33.75)
	rep, err := st.EnsureRegionData(context.Background(), "kast", d("1991-01-01"), d("2020-12-31"))
	if err != nil {
		t.Fatal(err)
	}
	if rep.FromSnapshot != 10958 || rep.FromAPI != 0 || api.count() != 0 {
		t.Fatalf("%+v requests=%d", rep, api.count())
	}
	if p, tm := countRows(t, db, "kast"); p != 10958 || tm != 10958 {
		t.Fatalf("rows %d/%d", p, tm)
	}
	// a range partly beyond the snapshot: the snapshot part is imported, only the rest hits the API
	st.Now = func() time.Time { return d("2026-10-20") }
	api2 := newFakeAPI(t, loadSnap(t))
	st.Fetcher, _ = testFetcher(api2.URL)
	rep, err = st.EnsureRegionData(context.Background(), "kast", d("2025-12-01"), d("2025-12-31")) // inside snapshot
	if err != nil || rep.FromSnapshot != 31 || api2.count() != 0 {
		t.Fatalf("%+v %v", rep, err)
	}

	// a region in another ERA5 cell must NOT be served from the snapshot
	insertRegion(t, db, "far", 40.0, 30.0)
	st.Fetcher = nil
	rep, err = st.EnsureRegionData(context.Background(), "far", d("2019-01-01"), d("2019-12-31"))
	if err == nil || rep.FromSnapshot != 0 {
		t.Fatalf("other cell served from snapshot: %+v %v", rep, err)
	}
	// offline + no snapshot cover => error, no fabricated data
	if p, _ := countRows(t, db, "far"); p != 0 {
		t.Fatalf("far rows=%d", p)
	}
}

func TestReadDailyAndErrors(t *testing.T) {
	api := newFakeAPI(t, loadSnap(t))
	st, db := newStore(t, api, false)
	insertRegion(t, db, "r1", 41.39, 33.78)
	ctx := context.Background()
	if recs, err := st.ReadDaily(ctx, "r1", d("2020-01-01"), d("2020-01-31")); err != nil || len(recs) != 0 {
		t.Fatalf("before ingestion: %v %v", recs, err)
	}
	if _, err := st.EnsureRegionData(ctx, "r1", d("2020-02-01"), d("2020-03-31")); err != nil {
		t.Fatal(err)
	}
	recs, err := st.ReadDaily(ctx, "r1", d("2020-02-27"), d("2020-03-02"))
	if err != nil || len(recs) != 5 {
		t.Fatalf("%v %v", recs, err)
	}
	if recs[2].Date != d("2020-02-29") || recs[2].Precip != 5.0 || recs[2].Temp != 5.5 {
		t.Fatalf("%+v", recs[2])
	}
	// one-sided gaps stay Missing
	if _, err := db.Exec(`DELETE FROM environmental_measurements WHERE region_id='r1' AND ts='2020-03-01' AND measurement_type='temperature_mean'`); err != nil {
		t.Fatal(err)
	}
	recs, _ = st.ReadDaily(ctx, "r1", d("2020-03-01"), d("2020-03-01"))
	if len(recs) != 1 || recs[0].Precip != 0.1 || !model.IsMissing(recs[0].Temp) {
		t.Fatalf("%+v", recs)
	}
	if _, err := st.EnsureRegionData(ctx, "nope", d("2020-01-01"), d("2020-01-02")); !errors.Is(err, ErrRegionNotFound) {
		t.Fatalf("want ErrRegionNotFound, got %v", err)
	}
}

func TestSubtractRanges(t *testing.T) {
	r := [2]time.Time{d("2020-01-01"), d("2020-12-31")}
	cov := [][2]time.Time{{d("2020-03-01"), d("2020-03-31")}, {d("2019-01-01"), d("2020-01-10")}, {d("2020-03-20"), d("2020-04-10")}, {d("2020-12-31"), d("2021-05-01")}}
	got := subtract(r, cov)
	want := [][2]string{{"2020-01-11", "2020-02-29"}, {"2020-04-11", "2020-12-30"}}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i][0].Format(dateLayout) != want[i][0] || got[i][1].Format(dateLayout) != want[i][1] {
			t.Fatalf("%v", got)
		}
	}
	if len(subtract(r, [][2]time.Time{{d("2019-01-01"), d("2021-01-01")}})) != 0 {
		t.Fatal("fully covered")
	}
	if g := subtract(r, nil); len(g) != 1 || g[0] != r {
		t.Fatal("nothing covered")
	}
}

// ───────────── snapshot ─────────────

func TestCommittedSnapshotIsGenuineAndConsistent(t *testing.T) {
	sn := loadSnap(t)
	if sn.Dataset != DatasetName || sn.Model != "era5" || sn.Grid.Latitude != 41.5 || sn.Grid.Longitude != 33.75 {
		t.Fatalf("%+v", sn.Grid)
	}
	if sn.RetrievedAt == "" || !strings.Contains(sn.License, "CC BY 4.0") || !strings.Contains(sn.Attribution, "Open-Meteo") {
		t.Fatal("provenance missing")
	}
	ser, err := sn.Series()
	if err != nil {
		t.Fatal(err)
	}
	if len(ser.Dates) != 12784 || ser.Dates[0] != d("1991-01-01") || ser.Dates[len(ser.Dates)-1] != d("2025-12-31") {
		t.Fatalf("%d days", len(ser.Dates))
	}
	var sumP, sumT float64
	n := 0
	for i, day := range ser.Dates {
		if day.Year() > 2020 {
			break
		}
		if model.IsMissing(ser.Precip[i]) || model.IsMissing(ser.Temp[i]) {
			t.Fatalf("missing on %s", day.Format(dateLayout))
		}
		sumP += ser.Precip[i]
		sumT += ser.Temp[i]
		n++
	}
	annual := sumP / 30
	if annual < 400 || annual > 900 || sumT/float64(n) < 5 || sumT/float64(n) > 15 {
		t.Fatalf("implausible climate: %v mm/yr, %v degC", annual, sumT/float64(n))
	}
	// 1991 snapshot value on 1991-01-03 is the 0.8 mm that the live API returned on 2026-10-20
	if ser.Precip[2] != 0.8 || ser.Temp[0] != 2.8 {
		t.Fatalf("first days: %v %v", ser.Precip[2], ser.Temp[0])
	}
}

func TestSnapshotRoundTripAndValidation(t *testing.T) {
	ser := &Series{Model: "era5", RequestLat: 41.38, RequestLon: 33.78, GridLat: 41.5, GridLon: 33.75, ElevationM: 778,
		Dates:  []time.Time{d("2020-01-01"), d("2020-01-02"), d("2020-01-03")},
		Precip: []float64{1.5, model.Missing, 0}, Temp: []float64{-2, 3, model.Missing}, RetrievedAt: d("2026-10-20")}
	sn, err := SnapshotFromSeries(ser, "test")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "s.json")
	if err := WriteSnapshot(path, sn); err != nil {
		t.Fatal(err)
	}
	back, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := back.Series()
	if len(s2.Dates) != 3 || s2.Precip[0] != 1.5 || !model.IsMissing(s2.Precip[1]) || s2.Temp[0] != -2 || !model.IsMissing(s2.Temp[2]) {
		t.Fatalf("%+v", s2)
	}
	back.PrecipMM = back.PrecipMM[:2]
	if back.Validate() == nil {
		t.Fatal("length mismatch must fail validation")
	}
	back.Format = "x"
	if back.Validate() == nil {
		t.Fatal("format")
	}
	if _, err := LoadSnapshot(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing file")
	}
}

func TestSameCell(t *testing.T) {
	if !SameCell(41.38, 33.78, 41.5, 33.75) || !SameCell(41.45, 33.7, 41.5, 33.75) {
		t.Fatal("Kastamonu points belong to the 41.5N/33.75E cell")
	}
	if SameCell(41.7, 33.78, 41.5, 33.75) || SameCell(41.38, 34.0, 41.5, 33.75) {
		t.Fatal("neighbouring cells")
	}
}

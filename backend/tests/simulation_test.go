package tests

import (
	"strings"
	"testing"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
)

// simEnv wires the climate module to the REAL committed ERA5 snapshot and an unreachable provider,
// so no test ever touches the network.
func simEnv(t *testing.T) *env {
	return newEnv(t, func(c *config.Config) {
		c.Climate = config.Climate{BaseURL: "http://127.0.0.1:1/unreachable", SnapshotPath: "../../datasets/environmental/kastamonu_era5_daily_1991_2025.json"}
	})
}

func provinceID(t *testing.T, e *env) string {
	t.Helper()
	r := e.get("/regions?kind=province", "")
	l := r.list()
	if r.Status != 200 || len(l) != 1 {
		t.Fatalf("regions: %d %s", r.Status, r.Raw)
	}
	return l[0].(map[string]any)["id"].(string)
}

func TestSimulationHTTP(t *testing.T) {
	e := simEnv(t)
	prov := provinceID(t, e)
	researcher, _ := e.userWithRole("res@example.org", "researcher")
	citizen, _ := e.register("cit@example.org")
	run := map[string]any{"name": "-20/+2", "region_id": prov, "reference_from": "1991-01", "reference_to": "2020-12",
		"precip_change_pct": -20, "temp_change_c": 2}

	// anonymous 401, citizen 403, bad input 400, unknown region 404
	if r := e.do("POST", "/simulations/run", "", run); r.Status != 401 {
		t.Fatalf("anon: %d", r.Status)
	}
	if r := e.do("POST", "/simulations/run", citizen, run); r.Status != 403 {
		t.Fatalf("citizen: %d %s", r.Status, r.Raw)
	}
	bad := map[string]any{"region_id": prov, "reference_from": "1991-01", "reference_to": "2020-12", "precip_change_pct": -150}
	if r := e.do("POST", "/simulations/run", researcher, bad); r.Status != 400 || r.errCode() != "validation_failed" {
		t.Fatalf("bad: %d %s", r.Status, r.Raw)
	}
	nf := map[string]any{"region_id": "nope", "reference_from": "1991-01", "reference_to": "2020-12"}
	if r := e.do("POST", "/simulations/run", researcher, nf); r.Status != 404 {
		t.Fatalf("unknown region: %d %s", r.Status, r.Raw)
	}

	// happy path from the offline snapshot
	r := e.do("POST", "/simulations/run", researcher, run)
	if r.Status != 200 || r.data()["status"] != "ok" {
		t.Fatalf("run: %d %s", r.Status, r.Raw)
	}
	d := r.data()
	id := d["id"].(string)
	if id == "" || len(d["indicators"].([]any)) < 20 || len(d["series"].([]any)) != 6 {
		t.Fatalf("result shape: %s", r.Raw[:300])
	}
	src := d["data_sources"].([]any)[0].(map[string]any)
	if !strings.Contains(src["attribution"].(string), "Open-Meteo") || src["coverage"].(float64) != 1 {
		t.Fatalf("data source: %v", src)
	}

	// insufficient data is a normal 200 without numbers
	short := map[string]any{"region_id": prov, "reference_from": "2015-01", "reference_to": "2020-12"}
	r = e.do("POST", "/simulations/run", researcher, short)
	if r.Status != 200 || r.data()["status"] != "insufficient_data" || len(r.data()["indicators"].([]any)) != 0 || r.data()["insufficient_reason"] == nil {
		t.Fatalf("insufficient: %d %s", r.Status, r.Raw)
	}

	// read back: owner, admin, stranger
	if g := e.get("/simulations/"+id, researcher); g.Status != 200 || g.data()["id"] != id {
		t.Fatalf("get own: %d", g.Status)
	}
	if g := e.get("/simulations/"+id, e.admin); g.Status != 200 {
		t.Fatalf("admin get: %d", g.Status)
	}
	other, _ := e.userWithRole("res2@example.org", "researcher")
	if g := e.get("/simulations/"+id, other); g.Status != 404 {
		t.Fatalf("stranger must get 404, got %d", g.Status)
	}
	if g := e.get("/simulations", researcher); g.Status != 200 || len(g.list()) != 2 {
		t.Fatalf("list: %d %s", g.Status, g.Raw)
	}
	if g := e.get("/simulations", other); len(g.list()) != 0 {
		t.Fatal("list must be own only")
	}

	// compare
	run2 := map[string]any{"region_id": prov, "reference_from": "1991-01", "reference_to": "2020-12", "temp_change_c": 2}
	id2 := e.do("POST", "/simulations/run", researcher, run2).data()["id"].(string)
	c := e.get("/simulations/compare?a="+id+"&b="+id2, researcher)
	if c.Status != 200 || c.data()["same_baseline"] != true || len(c.data()["deltas"].([]any)) < 20 {
		t.Fatalf("compare: %d %s", c.Status, c.Raw)
	}
	if c := e.get("/simulations/compare?a="+id+"&b="+id2, other); c.Status != 404 {
		t.Fatalf("compare foreign: %d", c.Status)
	}
	if c := e.get("/simulations/compare?a="+id, researcher); c.Status != 400 {
		t.Fatalf("compare missing b: %d", c.Status)
	}

	// audit trail
	var n int
	e.db.QueryRow(`SELECT count(*) FROM audit_logs WHERE action='simulation.run'`).Scan(&n)
	if n != 3 {
		t.Fatalf("audit entries: %d", n)
	}

	// public read-only measurements (data was ingested by the runs above)
	m := e.get("/environment/measurements?region_id="+prov+"&type=precipitation&from=2000-01&to=2000-12&resolution=monthly", "")
	pts, _ := m.data()["points"].([]any)
	if m.Status != 200 || len(pts) != 12 || m.data()["unit"] != "mm" || m.data()["coverage"].(float64) != 1 ||
		!strings.Contains(m.data()["attribution"].(string), "Open-Meteo") {
		t.Fatalf("measurements: %d %s", m.Status, m.Raw)
	}
	if m := e.get("/environment/measurements?region_id="+prov, ""); m.Status != 400 {
		t.Fatalf("missing type: %d", m.Status)
	}
	if m := e.get("/environment/measurements?region_id=nope&type=precipitation", ""); m.Status != 404 {
		t.Fatalf("unknown region: %d", m.Status)
	}
}

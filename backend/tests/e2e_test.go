package tests

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/app"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
)

// TestEndToEndFlow is the same story scripts/e2e.sh tells against a running server:
// citizen creates + uploads -> reviewer queue -> verify -> citizen sees verified + notification ->
// anonymous map list shows it -> region analytics counts it.
func TestEndToEndFlow(t *testing.T) {
	e := newEnv(t)

	// seeded real boundaries
	prov := e.get("/regions?kind=province", "")
	if len(prov.list()) != 1 || prov.list()[0].(map[string]any)["name"] != "Kastamonu" || prov.list()[0].(map[string]any)["geometry"] != nil {
		t.Fatalf("province: %s", prov.Raw)
	}
	if n := len(e.get("/regions?kind=district", "").list()); n != 20 {
		t.Fatalf("districts: %d", n)
	}
	merkez := e.get("/regions?kind=district&q=Merkez", "").list()[0].(map[string]any)
	regionID := str(merkez, "id")
	if g := e.get("/regions/"+regionID, ""); g.Status != 200 || g.data()["geometry"] == nil || g.data()["source"] == "" {
		t.Fatalf("region detail: %d", g.Status)
	}
	if g := e.get("/regions/"+regionID+"?x=1", ""); g.data()["parent_id"] == nil {
		t.Fatal("district has no parent")
	}
	// point lookup: smallest first
	pl := e.get(fmt.Sprintf("/regions?lon=%f&lat=%f", merkezPt[0], merkezPt[1]), "")
	if len(pl.list()) != 2 || pl.list()[0].(map[string]any)["kind"] != "district" || pl.list()[1].(map[string]any)["kind"] != "province" {
		t.Fatalf("point lookup: %s", pl.Raw)
	}
	if pl := e.get(fmt.Sprintf("/regions?lon=%f&lat=%f", outsideTR[0], outsideTR[1]), ""); len(pl.list()) != 0 {
		t.Fatalf("lookup outside: %s", pl.Raw)
	}
	if r := e.get("/regions/nope", ""); r.Status != 404 {
		t.Fatalf("unknown region: %d", r.Status)
	}

	citizen, citizenID := e.register("citizen@example.org")
	obs := e.createObs(citizen, merkezPt, map[string]any{"species_guess": "Kızıl geyik", "count_observed": 2})
	id := obs["id"].(string)
	up := e.upload(citizen, id, "deer.png", tinyPNG(16, 16, 9))
	if up.Status != 201 {
		t.Fatalf("upload: %d %s", up.Status, up.Raw)
	}
	pending := e.createObs(citizen, merkezPt, map[string]any{"title": "second, unverified", "record_type": "issue", "category": "pollution"})["id"].(string)

	// reviewer queue (admin acts as reviewer)
	q := e.get("/reviews/queue?category=animal", e.admin)
	if q.Status != 200 || len(q.list()) != 1 {
		t.Fatalf("queue: %d %s", q.Status, q.Raw)
	}
	item := q.list()[0].(map[string]any)
	if item["id"] != id || len(item["media"].([]any)) != 1 || item["exact_location"] == nil || item["ai_analysis"] == nil {
		t.Fatalf("queue item: %v", item)
	}
	if all := e.get("/reviews/queue", e.admin); len(all.list()) != 2 || all.meta("total") != 2 {
		t.Fatalf("queue total: %s", all.Raw)
	}
	if e.get("/reviews/queue?ai_status=done", e.admin).meta("total") != 0 {
		t.Fatal("ai_status filter")
	}
	if e.get("/reviews/queue?priority=urgent", e.admin).meta("total") != 0 {
		t.Fatal("priority filter")
	}

	// before verification: anonymous sees nothing, analytics are empty
	if l := e.get("/observations", ""); len(l.list()) != 0 {
		t.Fatalf("anonymous sees unverified: %s", l.Raw)
	}
	if a := e.get("/regions/"+regionID+"/analytics", ""); a.data()["totals"].(map[string]any)["observations"].(float64) != 0 {
		t.Fatalf("analytics before verification: %s", a.Raw)
	}

	v := e.decide(e.admin, id, "verify", map[string]any{"corrected_species": "Cervus elaphus", "note": "Uzman onayı"})
	if v.Status != 200 || v.data()["status"] != "verified" {
		t.Fatalf("verify: %d %s", v.Status, v.Raw)
	}

	// citizen sees verified + notification
	if d := e.get("/observations/"+id, citizen).data(); d["status"] != "verified" || d["species_verified"] != "Cervus elaphus" {
		t.Fatalf("citizen detail: %v", d)
	}
	n := e.get("/notifications", citizen)
	if len(n.list()) != 1 || n.list()[0].(map[string]any)["observation_id"] != id {
		t.Fatalf("notifications: %s", n.Raw)
	}

	// anonymous map list incl. bbox around Kastamonu and region filter
	for _, path := range []string{
		"/observations?bbox=32.5,40.8,35.5,42.2",
		"/observations?region_id=" + regionID,
		"/regions/" + regionID + "/observations",
		"/observations?category=animal&record_type=observation&q=geyik",
	} {
		l := e.get(path, "")
		if l.Status != 200 || len(l.list()) != 1 || l.list()[0].(map[string]any)["id"] != id || l.meta("total") != 1 {
			t.Fatalf("anonymous %s: %d %s", path, l.Status, l.Raw)
		}
	}
	if l := e.get("/observations?bbox=0,0,1,1", ""); len(l.list()) != 0 {
		t.Fatalf("bbox elsewhere: %s", l.Raw)
	}
	if l := e.get("/observations?bbox=1,2,3", ""); l.Status != 400 {
		t.Fatalf("bad bbox: %d", l.Status)
	}
	if l := e.get("/regions/nope/observations", ""); l.Status != 404 {
		t.Fatalf("observations of unknown region: %d", l.Status)
	}
	if e.get("/media/"+up.data()["id"].(string)+"?thumb=1", "").Status != 200 {
		t.Fatal("anonymous thumbnail of verified record")
	}

	// region analytics (SQL numbers; only the verified record counts for the public)
	a := e.get("/regions/"+regionID+"/analytics", "").data()
	totals := a["totals"].(map[string]any)
	if totals["observations"].(float64) != 1 || totals["verified"].(float64) != 1 || totals["issues"].(float64) != 0 || totals["pending_review"].(float64) != 0 {
		t.Fatalf("public analytics: %v", a)
	}
	bc := a["by_category"].([]any)
	if len(bc) != 1 || bc[0].(map[string]any)["category"] != "animal" {
		t.Fatalf("by_category: %v", bc)
	}
	thisMonth := time.Now().UTC().Add(-2 * time.Hour).Format("2006-01")
	foundMonth := false
	for _, m := range a["by_month"].([]any) {
		mm := m.(map[string]any)
		if mm["month"] == thisMonth && mm["count"].(float64) == 1 && mm["distinct_observers"].(float64) == 1 {
			foundMonth = true
		}
	}
	if !foundMonth {
		t.Fatalf("by_month: %v", a["by_month"])
	}
	ts := a["top_species"].([]any)
	if len(ts) != 1 || ts[0].(map[string]any)["species"] != "Cervus elaphus" {
		t.Fatalf("top_species: %v", ts)
	}
	dq := a["data_quality"].(map[string]any)
	warn := fmt.Sprint(dq["warnings"])
	if !strings.Contains(warn, "popülasyon değişimi anlamına gelmez") || !strings.Contains(warn, "1 farklı gözlemci") || dq["distinct_observers"].(float64) != 1 {
		t.Fatalf("data_quality: %v", dq)
	}
	// staff additionally see pending records
	sa := e.get("/regions/"+regionID+"/analytics", e.admin).data()["totals"].(map[string]any)
	if sa["observations"].(float64) != 1 || sa["issues"].(float64) != 1 || sa["pending_review"].(float64) != 1 {
		t.Fatalf("staff analytics: %v", sa)
	}
	// province contains district records too; bad period rejected
	provID := str(prov.list()[0].(map[string]any), "id")
	if e.get("/regions/"+provID+"/analytics", "").data()["totals"].(map[string]any)["observations"].(float64) != 1 {
		t.Fatal("province analytics")
	}
	if r := e.get("/regions/"+regionID+"/analytics?from=garbage", ""); r.Status != 400 {
		t.Fatalf("bad from: %d", r.Status)
	}
	if r := e.get("/regions/nope/analytics", ""); r.Status != 404 {
		t.Fatalf("analytics unknown region: %d", r.Status)
	}

	// custom region (researcher): validation + backfilled counts; probing sensitive records is blocked
	researcher, _ := e.userWithRole("res@example.org", "researcher")
	for name, geom := range map[string]any{
		"point":   map[string]any{"type": "Point", "coordinates": []any{1, 2}},
		"open":    map[string]any{"type": "Polygon", "coordinates": []any{[]any{[]any{0, 0}, []any{1, 0}, []any{1, 1}, []any{0, 1}}}},
		"too far": map[string]any{"type": "Polygon", "coordinates": []any{[]any{[]any{0, 0}, []any{500, 0}, []any{1, 1}, []any{0, 0}}}},
	} {
		if r := e.do("POST", "/regions", researcher, map[string]any{"name": "bad", "geometry": geom}); r.Status != 400 {
			t.Errorf("custom region %s: %d", name, r.Status)
		}
	}
	if r := e.do("POST", "/regions", researcher, map[string]any{"name": "", "geometry": map[string]any{}}); r.Status != 400 {
		t.Errorf("empty name: %d", r.Status)
	}
	box := map[string]any{"type": "Polygon", "coordinates": []any{[]any{[]any{33.7, 41.3}, []any{33.9, 41.3}, []any{33.9, 41.5}, []any{33.7, 41.5}, []any{33.7, 41.3}}}}
	cr := e.do("POST", "/regions", researcher, map[string]any{"name": "Araştırma alanı", "geometry": box})
	if cr.Status != 201 || cr.data()["kind"] != "custom" || cr.data()["is_demo"] != false {
		t.Fatalf("custom region: %d %s", cr.Status, cr.Raw)
	}
	cid := str(cr.data(), "id")
	if c := e.get("/regions/"+cid+"/analytics", "").data()["totals"].(map[string]any)["observations"].(float64); c != 1 {
		t.Fatalf("custom region count: %v", c)
	}
	if l := e.get("/regions/"+cid+"/observations", ""); len(l.list()) != 1 {
		t.Fatalf("custom region observations: %s", l.Raw)
	}
	// new record inside the box is linked on creation (as soon as it is verified it counts)
	id2 := e.createObs(citizen, [2]float64{33.8, 41.4}, nil)["id"].(string)
	e.decide(e.admin, id2, "verify", nil)
	if c := e.get("/regions/"+cid+"/analytics", "").data()["totals"].(map[string]any)["observations"].(float64); c != 2 {
		t.Fatalf("custom region count after create: %v", c)
	}
	// a primary region never points to a custom polygon
	if got := e.get("/observations/"+id2, "").data()["primary_region_name"]; got != "Merkez" {
		t.Fatalf("primary region: %v", got)
	}
	// audit trail covers the whole story
	al := e.get("/admin/audit-logs?page_size=200", e.admin)
	have := map[string]bool{}
	for _, x := range al.list() {
		have[str(x.(map[string]any), "action")] = true
	}
	for _, want := range []string{"user.register", "observation.create", "media.upload", "observation.status_change", "review.decision", "region.create", "region.seed", "user.role_change"} {
		if !have[want] {
			t.Errorf("audit missing %q", want)
		}
	}
	_ = citizenID
	_ = pending
}

func TestDemoSeed(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.SeedDemo = true })
	var total, demo int
	e.db.QueryRow(`SELECT count(*), COALESCE(SUM(is_demo),0) FROM observations`).Scan(&total, &demo)
	if total != 30 || demo != 30 {
		t.Fatalf("demo observations: total=%d demo=%d", total, demo)
	}
	statuses := map[string]int{}
	rows, _ := e.db.Query(`SELECT status, count(*) FROM observations GROUP BY status`)
	for rows.Next() {
		var s string
		var n int
		rows.Scan(&s, &n)
		statuses[s] = n
	}
	rows.Close()
	for _, s := range []string{"submitted", "verified", "rejected", "needs_information", "under_review", "resolved"} {
		if statuses[s] == 0 {
			t.Errorf("no demo record with status %s: %v", s, statuses)
		}
	}
	var noRegion int
	e.db.QueryRow(`SELECT count(*) FROM observations WHERE primary_region_id IS NULL`).Scan(&noRegion)
	if noRegion != 0 {
		t.Errorf("%d demo records fell outside every district", noRegion)
	}
	var sens int
	e.db.QueryRow(`SELECT count(*) FROM observations WHERE is_sensitive = 1`).Scan(&sens)
	if sens < 2 {
		t.Errorf("sensitive demo records: %d", sens)
	}
	// public sees demo records flagged is_demo and never the sensitive exact point
	l := e.get("/observations?page_size=200", "")
	if len(l.list()) == 0 {
		t.Fatal("no public demo records")
	}
	for _, x := range l.list() {
		m := x.(map[string]any)
		if m["is_demo"] != true || m["exact_location"] != nil {
			t.Fatalf("public demo item: %v", m)
		}
	}
	// idempotent: bootstrapping again adds nothing
	if err := e.app.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.db.QueryRow(`SELECT count(*) FROM observations`).Scan(&total)
	if total != 30 {
		t.Fatalf("re-seed duplicated records: %d", total)
	}
	// Simulate upgrading the older 25-record demo database. Preserve edited old
	// records while adding only the five new presentation client refs.
	if _, err := e.db.Exec(`DELETE FROM observations WHERE client_ref IN ('demo-seed-026','demo-seed-027','demo-seed-028','demo-seed-029','demo-seed-030')`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE observations SET title='Preserved demo edit' WHERE client_ref='demo-seed-001'`); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.db.QueryRow(`SELECT count(*) FROM observations`).Scan(&total)
	var preserved int
	e.db.QueryRow(`SELECT count(*) FROM observations WHERE client_ref='demo-seed-001' AND title='Preserved demo edit'`).Scan(&preserved)
	if total != 30 || preserved != 1 {
		t.Fatalf("incremental demo seed: total=%d preserved=%d", total, preserved)
	}
	// both copies of the embedded datasets are identical to the files under datasets/
	for _, p := range [][2]string{
		{"../../datasets/seed/demo_observations.json", "../internal/app/demo_observations.json"},
		{"../../datasets/geographical/kastamonu_regions.geojson", "../internal/geographic/seed/kastamonu_regions.geojson"},
	} {
		a, err1 := os.ReadFile(p[0])
		b, err2 := os.ReadFile(p[1])
		if err1 != nil || err2 != nil || !bytes.Equal(a, b) {
			t.Errorf("dataset copy out of sync: %s vs %s (%v %v)", p[0], p[1], err1, err2)
		}
	}
	_ = app.Version
}

package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
)

func TestObservationCRUD(t *testing.T) {
	e := newEnv(t)
	tok, uid := e.register("owner@example.org")
	other, _ := e.register("other@example.org")

	o := e.createObs(tok, merkezPt, map[string]any{"count_observed": 3, "details": map[string]any{"alive": true}})
	id := o["id"].(string)
	if !refRe.MatchString(str(o, "reference_no")) || o["status"] != "submitted" || o["user_id"] != uid {
		t.Fatalf("created: %v", o)
	}
	if o["primary_region_name"] != "Merkez" {
		t.Fatalf("primary region = %v", o["primary_region_name"])
	}
	// second reference number increments
	o2 := e.createObs(tok, merkezPt, nil)
	if str(o2, "reference_no") <= str(o, "reference_no") {
		t.Fatalf("reference numbers not increasing: %s %s", o["reference_no"], o2["reference_no"])
	}
	// linked to province + district
	var links int
	e.db.QueryRow(`SELECT count(*) FROM observation_region_links WHERE observation_id = ?`, id).Scan(&links)
	if links != 2 {
		t.Fatalf("region links = %d, want 2 (province+district)", links)
	}
	// jobs: index job queued, no analyze job (AI not configured)
	var idx, ana int
	e.db.QueryRow(`SELECT count(*) FROM jobs WHERE kind='index_document' AND payload_json LIKE ?`, "%"+id+"%").Scan(&idx)
	e.db.QueryRow(`SELECT count(*) FROM jobs WHERE kind='analyze_observation'`).Scan(&ana)
	if idx == 0 || ana != 0 {
		t.Fatalf("jobs index=%d analyze=%d", idx, ana)
	}

	// owner reads detail with history and exact location
	d := e.get("/observations/"+id, tok)
	if d.Status != 200 || d.data()["exact_location"] == nil || len(d.data()["history"].([]any)) != 1 {
		t.Fatalf("detail: %d %s", d.Status, d.Raw)
	}
	// edit
	p := e.do("PATCH", "/observations/"+id, tok, map[string]any{"title": "Yeni başlık", "count_observed": 4})
	if p.Status != 200 || p.data()["title"] != "Yeni başlık" || p.data()["count_observed"].(float64) != 4 {
		t.Fatalf("patch: %d %s", p.Status, p.Raw)
	}
	// move to İnebolu -> region follows
	p = e.do("PATCH", "/observations/"+id, tok, map[string]any{"lat": inebolu[1], "lon": inebolu[0]})
	if p.Status != 200 || p.data()["primary_region_name"] != "İnebolu" {
		t.Fatalf("move: %d %s", p.Status, p.Raw)
	}
	if r := e.do("PATCH", "/observations/"+id, tok, map[string]any{}); r.Status != 400 {
		t.Fatalf("empty patch: %d", r.Status)
	}
	// not the owner: 403 when visible, 404 when not
	if r := e.do("PATCH", "/observations/"+id, other, map[string]any{"title": "x"}); r.Status != 404 {
		t.Fatalf("other patch of invisible record: %d", r.Status)
	}
	if r := e.decide(e.admin, id, "verify", nil); r.Status != 200 {
		t.Fatalf("verify: %d %s", r.Status, r.Raw)
	}
	if r := e.do("PATCH", "/observations/"+id, other, map[string]any{"title": "x"}); r.Status != 403 {
		t.Fatalf("other patch of visible record: %d", r.Status)
	}
	if r := e.do("DELETE", "/observations/"+id, other, nil); r.Status != 403 {
		t.Fatalf("other delete: %d", r.Status)
	}
	// verified records are frozen for the owner
	if r := e.do("PATCH", "/observations/"+id, tok, map[string]any{"title": "x"}); r.Status != 409 {
		t.Fatalf("edit verified: %d", r.Status)
	}
	// soft delete by owner
	if r := e.do("DELETE", "/observations/"+id, tok, nil); r.Status != 204 {
		t.Fatalf("delete: %d", r.Status)
	}
	if r := e.get("/observations/"+id, tok); r.Status != 404 {
		t.Fatalf("deleted still readable: %d", r.Status)
	}
	var deleted string
	e.db.QueryRow(`SELECT COALESCE(deleted_at,'') FROM observations WHERE id = ?`, id).Scan(&deleted)
	if deleted == "" {
		t.Fatal("not a soft delete")
	}
	if r := e.get("/observations", ""); contains(r.list(), id) {
		t.Fatal("deleted record in public list")
	}
}

func contains(list []any, id string) bool {
	for _, x := range list {
		if x.(map[string]any)["id"] == id {
			return true
		}
	}
	return false
}

func TestObservationValidation(t *testing.T) {
	e := newEnv(t)
	tok, _ := e.register("v@example.org")
	long := strings.Repeat("a", 5001)
	cases := map[string]map[string]any{
		"bad category":    {"category": "mineral"},
		"bad record_type": {"record_type": "rumor"},
		"lat too big":     {"lat": 91.0},
		"lon too small":   {"lon": -181.0},
		"future":          {"observed_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339)},
		"ancient":         {"observed_at": time.Now().UTC().AddDate(-11, 0, 0).Format(time.RFC3339)},
		"bad time":        {"observed_at": "yesterday"},
		"long desc":       {"description": long},
		"long title":      {"title": strings.Repeat("t", 201)},
		"neg count":       {"count_observed": -1},
		"details array":   {"details": []any{1}},
		"bad source":      {"location_source": "magic"},
		"bad client_ref":  {"client_ref": "short"},
	}
	for name, extra := range cases {
		r := e.do("POST", "/observations", tok, obsBody(merkezPt, extra))
		if r.Status != 400 || r.errCode() != "validation_failed" {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	// required fields
	for _, k := range []string{"lat", "lon", "observed_at", "category", "record_type"} {
		b := obsBody(merkezPt, nil)
		delete(b, k)
		if r := e.do("POST", "/observations", tok, b); r.Status != 400 {
			t.Errorf("missing %s: %d", k, r.Status)
		}
	}
	// within slack (+4 min) and an old offline capture (9 years) are accepted
	for _, ts := range []string{time.Now().UTC().Add(4 * time.Minute).Format(time.RFC3339), time.Now().UTC().AddDate(-9, 0, 0).Format(time.RFC3339)} {
		if r := e.do("POST", "/observations", tok, obsBody(merkezPt, map[string]any{"observed_at": ts})); r.Status != 201 {
			t.Errorf("observed_at %s: %d %s", ts, r.Status, r.Raw)
		}
	}
	// outside any region is allowed but has no primary region
	r := e.do("POST", "/observations", tok, obsBody(outsideTR, nil))
	if r.Status != 201 || r.data()["primary_region_id"] != nil {
		t.Errorf("outside: %d %s", r.Status, r.Raw)
	}
	// malformed JSON / oversize body
	if r := e.do("POST", "/observations", tok, "not an object"); r.Status != 400 {
		t.Errorf("string body: %d", r.Status)
	}
}

func TestClientRefIdempotency(t *testing.T) {
	e := newEnv(t)
	a, _ := e.register("a@example.org")
	b, _ := e.register("b@example.org")
	body := obsBody(merkezPt, map[string]any{"client_ref": "device-uuid-0001"})
	r1 := e.do("POST", "/observations", a, body)
	r2 := e.do("POST", "/observations", a, body)
	if r1.Status != 201 || r2.Status != 200 || r1.data()["id"] != r2.data()["id"] {
		t.Fatalf("idempotent create: %d/%d %s | %s", r1.Status, r2.Status, r1.Raw, r2.Raw)
	}
	if r1.data()["client_ref"] != "device-uuid-0001" {
		t.Fatalf("client_ref echo: %v", r1.data()["client_ref"])
	}
	var n int
	e.db.QueryRow(`SELECT count(*) FROM observations WHERE client_ref = 'device-uuid-0001'`).Scan(&n)
	if n != 1 {
		t.Fatalf("rows: %d", n)
	}
	// another user with the same client_ref gets their own record
	r3 := e.do("POST", "/observations", b, body)
	if r3.Status != 201 || r3.data()["id"] == r1.data()["id"] {
		t.Fatalf("other user: %d %s", r3.Status, r3.Raw)
	}
	// a retry after the owner deleted it is refused, not resurrected
	e.do("DELETE", "/observations/"+r1.data()["id"].(string), a, nil)
	if r := e.do("POST", "/observations", a, body); r.Status != 409 {
		t.Fatalf("retry after delete: %d", r.Status)
	}
}

func TestDraftFlow(t *testing.T) {
	e := newEnv(t)
	tok, _ := e.register("d@example.org")
	r := e.do("POST", "/observations", tok, obsBody(merkezPt, map[string]any{"draft": true}))
	if r.Status != 201 || r.data()["status"] != "draft" {
		t.Fatalf("draft: %d %s", r.Status, r.Raw)
	}
	id := r.data()["id"].(string)
	// invisible to others, even staff, and not in public/queue lists
	if x := e.get("/observations/"+id, e.admin); x.Status != 404 {
		t.Fatalf("staff sees draft: %d", x.Status)
	}
	if x := e.decide(e.admin, id, "verify", nil); x.Status != 404 {
		t.Fatalf("decide on draft: %d", x.Status)
	}
	// cannot submit a non-draft/ cannot "answer" a draft; submit moves it
	p := e.do("PATCH", "/observations/"+id, tok, map[string]any{"submit": true})
	if p.Status != 200 || p.data()["status"] != "submitted" {
		t.Fatalf("submit: %d %s", p.Status, p.Raw)
	}
	if x := e.do("PATCH", "/observations/"+id, tok, map[string]any{"submit": true}); x.Status != 409 {
		t.Fatalf("double submit: %d", x.Status)
	}
	if x := e.get("/reviews/queue", e.admin); !contains(x.list(), id) {
		t.Fatalf("submitted record not in queue: %s", x.Raw)
	}
}

func TestStateMachineAndReviewDecisions(t *testing.T) {
	e := newEnv(t)
	reviewer, _ := e.userWithRole("rev@example.org", "reviewer")
	tok, _ := e.register("citizen@example.org")
	id := e.createObs(tok, merkezPt, nil)["id"].(string)

	// note required for reject / request_info; unknown decision
	for _, d := range []string{"reject", "request_info"} {
		if r := e.decide(reviewer, id, d, nil); r.Status != 400 {
			t.Errorf("%s without note: %d", d, r.Status)
		}
	}
	if r := e.decide(reviewer, id, "explode", nil); r.Status != 400 {
		t.Errorf("unknown decision: %d", r.Status)
	}
	if r := e.decide(reviewer, id, "verify", map[string]any{"corrected_category": "spaceship"}); r.Status != 400 {
		t.Errorf("bad corrected category: %d", r.Status)
	}
	// illegal: resolve a submitted record; verify -> start_review
	if r := e.decide(reviewer, id, "resolve", nil); r.Status != 409 || r.errCode() != "conflict" {
		t.Errorf("resolve submitted: %d %s", r.Status, r.Raw)
	}
	if r := e.decide(reviewer, id, "start_review", map[string]any{"priority": "high"}); r.Status != 200 || r.data()["status"] != "under_review" || r.data()["priority"] != "high" {
		t.Fatalf("start_review: %d %s", r.Status, r.Raw)
	}
	if r := e.decide(reviewer, id, "start_review", nil); r.Status != 409 {
		t.Errorf("start_review twice: %d", r.Status)
	}
	// request info -> owner sees notification -> owner answers -> back to under_review
	r := e.decide(reviewer, id, "request_info", map[string]any{"note": "Fotoğraf ekler misiniz?"})
	if r.Status != 200 || r.data()["status"] != "needs_information" {
		t.Fatalf("request_info: %d %s", r.Status, r.Raw)
	}
	n := e.get("/notifications?unread=true", tok)
	if len(n.list()) != 2 { // under_review + info requested
		t.Fatalf("notifications: %s", n.Raw)
	}
	first := n.list()[0].(map[string]any)
	if first["kind"] != "info_requested" || first["observation_id"] != id || !strings.Contains(str(first, "title"), "ET-") {
		t.Fatalf("notification content: %v", first)
	}
	if x := e.do("POST", "/notifications/"+str(first, "id")+"/read", tok, nil); x.Status != 204 {
		t.Fatalf("mark read: %d", x.Status)
	}
	if x := e.do("POST", "/notifications/"+str(first, "id")+"/read", reviewer, nil); x.Status != 404 {
		t.Fatalf("reading other's notification: %d", x.Status)
	}
	if len(e.get("/notifications?unread=true", tok).list()) != 1 {
		t.Fatal("unread filter")
	}
	// owner edits + submits the answer
	p := e.do("PATCH", "/observations/"+id, tok, map[string]any{"description": "Ek bilgi", "submit": true})
	if p.Status != 200 || p.data()["status"] != "under_review" {
		t.Fatalf("owner answer: %d %s", p.Status, p.Raw)
	}
	// verify with corrections + sensitivity; species is copied from the reviewer's correction only
	v := e.decide(reviewer, id, "verify", map[string]any{"corrected_species": "Lynx lynx", "corrected_category": "animal", "mark_sensitive": true, "note": "ok"})
	if v.Status != 200 {
		t.Fatalf("verify: %d %s", v.Status, v.Raw)
	}
	d := v.data()
	if d["status"] != "verified" || d["species_verified"] != "Lynx lynx" || d["is_sensitive"] != true || d["species_guess"] != "Cervus elaphus" {
		t.Fatalf("verified detail: %v", d)
	}
	if len(d["reviews"].([]any)) != 3 || len(d["history"].([]any)) != 5 {
		t.Fatalf("reviews/history: %d/%d", len(d["reviews"].([]any)), len(d["history"].([]any)))
	}
	// cannot re-verify / reject a verified record, but can resolve
	if x := e.decide(reviewer, id, "reject", map[string]any{"note": "x"}); x.Status != 409 {
		t.Errorf("reject verified: %d", x.Status)
	}
	if x := e.decide(reviewer, id, "resolve", nil); x.Status != 200 || x.data()["status"] != "resolved" {
		t.Errorf("resolve: %d %s", x.Status, x.Raw)
	}
	// owner cannot see reviews but sees history with anonymized reviewer names
	od := e.get("/observations/"+id, tok).data()
	if len(od["reviews"].([]any)) != 0 {
		t.Error("owner sees internal reviews")
	}
	for _, h := range od["history"].([]any) {
		hm := h.(map[string]any)
		if hm["actor_type"] == "user" && hm["actor_id"] != od["user_id"] && hm["actor_name"] != "Uzman" {
			t.Errorf("reviewer name leaked to owner: %v", hm)
		}
	}
	// audit trail contains every transition
	var n2 int
	e.db.QueryRow(`SELECT count(*) FROM audit_logs WHERE entity_id = ? AND action = 'observation.status_change'`, id).Scan(&n2)
	if n2 != 5 { // under_review, needs_information, under_review, verified, resolved
		t.Errorf("status_change audit rows = %d", n2)
	}
	// index jobs were queued for status changes
	var jobs int
	e.db.QueryRow(`SELECT count(*) FROM jobs WHERE kind='index_document' AND payload_json LIKE ?`, "%"+id+"%").Scan(&jobs)
	if jobs < 6 {
		t.Errorf("index jobs = %d", jobs)
	}
}

func TestReviewerCannotDecideOwnRecord(t *testing.T) {
	e := newEnv(t)
	reviewer, _ := e.userWithRole("rev@example.org", "reviewer")
	id := e.createObs(reviewer, merkezPt, nil)["id"].(string)
	if r := e.decide(reviewer, id, "verify", nil); r.Status != 403 {
		t.Fatalf("self review: %d %s", r.Status, r.Raw)
	}
}

func TestVisibilityRules(t *testing.T) {
	e := newEnv(t)
	a, _ := e.register("a@example.org")
	b, _ := e.register("b@example.org")
	reviewer, _ := e.userWithRole("rev@example.org", "reviewer")

	sub := e.createObs(a, merkezPt, map[string]any{"title": "pending one"})["id"].(string)
	ver := e.createObs(a, merkezPt, map[string]any{"title": "verified one"})["id"].(string)
	rej := e.createObs(a, merkezPt, map[string]any{"title": "rejected one"})["id"].(string)
	e.decide(reviewer, ver, "verify", nil)
	e.decide(reviewer, rej, "reject", map[string]any{"note": "no"})

	check := func(who, tok string, wantIDs []string, hideIDs []string) {
		t.Helper()
		l := e.get("/observations?page_size=200", tok)
		if l.Status != 200 {
			t.Fatalf("%s list: %d", who, l.Status)
		}
		for _, id := range wantIDs {
			if !contains(l.list(), id) {
				t.Errorf("%s should see %s", who, id)
			}
		}
		for _, id := range hideIDs {
			if contains(l.list(), id) {
				t.Errorf("%s must not see %s", who, id)
			}
			if r := e.get("/observations/"+id, tok); r.Status != 404 {
				t.Errorf("%s detail of hidden %s: %d", who, id, r.Status)
			}
		}
	}
	check("anonymous", "", []string{ver}, []string{sub, rej})
	check("other citizen", b, []string{ver}, []string{sub, rej})
	check("owner", a, []string{sub, ver, rej}, nil)
	check("reviewer", reviewer, []string{sub, ver, rej}, nil)
	check("admin", e.admin, []string{sub, ver, rej}, nil)

	// mine=true
	if r := e.get("/observations?mine=true", ""); r.Status != 401 {
		t.Errorf("mine anonymous: %d", r.Status)
	}
	if r := e.get("/observations?mine=true", b); len(r.list()) != 0 {
		t.Errorf("mine of b: %s", r.Raw)
	}
	if r := e.get("/observations?mine=true", a); len(r.list()) != 3 {
		t.Errorf("mine of a: %d", len(r.list()))
	}
	// filters
	if r := e.get("/observations?status=verified", a); len(r.list()) != 1 {
		t.Errorf("status filter: %d", len(r.list()))
	}
	if r := e.get("/observations?status=bogus", a); r.Status != 400 {
		t.Errorf("bad status: %d", r.Status)
	}
	if r := e.get("/observations?q=pending", a); len(r.list()) != 1 {
		t.Errorf("q filter: %d", len(r.list()))
	}
	if r := e.get("/observations?q=%25", a); len(r.list()) != 0 { // literal % must not act as a wildcard
		t.Errorf("q wildcard escape: %d", len(r.list()))
	}
	if r := e.get("/observations?record_type=issue", a); len(r.list()) != 0 {
		t.Errorf("record_type filter: %d", len(r.list()))
	}
	day := time.Now().UTC().Format("2006-01-02")
	if r := e.get("/observations?from="+day+"&to="+day, a); len(r.list()) != 3 {
		t.Errorf("date filter inclusive day: %d", len(r.list()))
	}
	if r := e.get("/observations?to=2000-01-01", a); len(r.list()) != 0 {
		t.Errorf("to filter: %d", len(r.list()))
	}
	// pagination
	for i := 0; i < 3; i++ {
		e.createObs(a, merkezPt, nil)
	}
	pg := e.get("/observations?page_size=2&page=2", a)
	if len(pg.list()) != 2 || pg.meta("total") != 6 || pg.meta("page") != 2 {
		t.Errorf("pagination: %d %v", len(pg.list()), pg.Body["meta"])
	}
	if r := e.get("/observations?page=0", a); r.Status != 400 {
		t.Errorf("page=0: %d", r.Status)
	}
}

func TestSensitiveLocationGeneralization(t *testing.T) {
	e := newEnv(t)
	owner, _ := e.register("o@example.org")
	other, _ := e.register("x@example.org")
	reviewer, _ := e.userWithRole("rev@example.org", "reviewer")
	exact := [2]float64{33.78274, 41.38871}
	id := e.createObs(owner, exact, nil)["id"].(string)
	if r := e.decide(reviewer, id, "verify", map[string]any{"mark_sensitive": true}); r.Status != 200 {
		t.Fatalf("verify: %d %s", r.Status, r.Raw)
	}
	// anonymous + other citizen: rounded to 0.1deg, no exact_location, flagged generalized
	for who, tok := range map[string]string{"anon": "", "other": other} {
		d := e.get("/observations/"+id, tok).data()
		if d["lat"].(float64) != 41.4 || d["lon"].(float64) != 33.8 || d["exact_location"] != nil || d["location_generalized"] != true {
			t.Errorf("%s sees: lat=%v lon=%v exact=%v", who, d["lat"], d["lon"], d["exact_location"])
		}
		l := e.get("/observations", tok)
		for _, x := range l.list() {
			m := x.(map[string]any)
			if m["id"] == id && (m["lat"].(float64) != 41.4 || m["exact_location"] != nil) {
				t.Errorf("%s list leaks: %v", who, m)
			}
		}
		if strings.Contains(string(e.get("/observations/"+id, tok).Raw), "41.3887") {
			t.Errorf("%s response contains exact latitude", who)
		}
	}
	// owner, reviewer, admin get the exact point as well
	for who, tok := range map[string]string{"owner": owner, "reviewer": reviewer, "admin": e.admin} {
		d := e.get("/observations/"+id, tok).data()
		ex, _ := d["exact_location"].(map[string]any)
		if ex == nil || ex["lat"].(float64) != exact[1] || ex["lon"].(float64) != exact[0] {
			t.Errorf("%s exact_location: %v", who, d["exact_location"])
		}
		if d["lat"].(float64) != 41.4 {
			t.Errorf("%s public lat should stay generalized: %v", who, d["lat"])
		}
	}
	// bbox queries run on public coordinates: a tight box around the exact point finds nothing,
	// a box around the generalized point finds it
	tight := fmt.Sprintf("bbox=%f,%f,%f,%f", exact[0]-0.01, exact[1]-0.01, exact[0]+0.01, exact[1]+0.01)
	if r := e.get("/observations?"+tight, ""); len(r.list()) != 0 {
		t.Errorf("bbox around exact point leaks sensitive record: %s", r.Raw)
	}
	if r := e.get("/observations?bbox=33.75,41.35,33.85,41.45", ""); !contains(r.list(), id) {
		t.Errorf("bbox around public point should match: %s", r.Raw)
	}
	// un-marking restores exact public coordinates
	e.decide(reviewer, id, "resolve", map[string]any{"mark_sensitive": false})
	if d := e.get("/observations/"+id, "").data(); d["lat"].(float64) != exact[1] || d["location_generalized"] != false {
		t.Errorf("after unmark: %v", d)
	}
}

func TestAIJobsEnqueuedWhenConfigured(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {
		c.AI.BaseURL = "http://127.0.0.1:1/v1"
		c.AI.TextModel = "m"
	})
	if r := e.get("/health", ""); r.data()["ai_configured"] != true {
		t.Fatalf("health: %s", r.Raw)
	}
	tok, _ := e.register("ai@example.org")
	id := e.createObs(tok, merkezPt, nil)["id"].(string)
	var payload, runAfter string
	if err := e.db.QueryRow(`SELECT payload_json, run_after FROM jobs WHERE kind='analyze_observation'`).Scan(&payload, &runAfter); err != nil {
		t.Fatal(err)
	}
	if payload != `{"observation_id":"`+id+`"}` {
		t.Fatalf("payload: %s", payload)
	}
}

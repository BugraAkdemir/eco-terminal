package tests

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
)

func TestHealthAndEnvelope(t *testing.T) {
	e := newEnv(t)
	r := e.get("/health", "")
	if r.Status != 200 || r.data()["status"] != "ok" || r.data()["vector_search"] != true || r.data()["ai_configured"] != false {
		t.Fatalf("health: %d %s", r.Status, r.Raw)
	}
	if r.Header.Get("X-Request-ID") == "" {
		t.Fatal("missing request id")
	}
	// unknown route -> JSON error envelope, not text
	r = e.get("/nope", "")
	if r.Status != 404 || r.errCode() != "not_found" {
		t.Fatalf("404 envelope: %d %s", r.Status, r.Raw)
	}
	r = e.do("PUT", "/health", "", nil)
	if r.Status != 405 || r.errCode() != "method_not_allowed" {
		t.Fatalf("405 envelope: %d %s", r.Status, r.Raw)
	}
	// CORS preflight
	req, _ := http.NewRequest("OPTIONS", e.srv.URL+"/api/v1/observations", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	if pr := e.send(req); pr.Status != 204 || pr.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("preflight: %d %v", pr.Status, pr.Header)
	}
}

func TestAuthFlow(t *testing.T) {
	e := newEnv(t)
	// validation
	for name, body := range map[string]map[string]any{
		"short password": {"email": "a@example.org", "password": "short"},
		"bad email":      {"email": "not-an-email", "password": userPassword},
		"no domain dot":  {"email": "a@localhost", "password": userPassword},
		"empty":          {},
	} {
		if r := e.do("POST", "/auth/register", "", body); r.Status != 400 || r.errCode() != "validation_failed" {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	tok, id := e.register("Citizen@Example.org")
	// duplicate, case-insensitive
	if r := e.do("POST", "/auth/register", "", map[string]any{"email": "citizen@example.ORG", "password": userPassword}); r.Status != 409 {
		t.Fatalf("duplicate email: %d %s", r.Status, r.Raw)
	}
	me := e.get("/auth/me", tok)
	if me.Status != 200 || me.data()["id"] != id || me.data()["role"] != "citizen" || me.data()["email"] != "citizen@example.org" {
		t.Fatalf("me: %d %s", me.Status, me.Raw)
	}
	if _, leaked := me.data()["password_hash"]; leaked {
		t.Fatal("password hash leaked")
	}
	// raw token must not be stored
	var n int
	e.db.QueryRow(`SELECT count(*) FROM sessions WHERE token_hash = ?`, tok).Scan(&n)
	if n != 0 {
		t.Fatal("raw token stored in sessions")
	}
	e.db.QueryRow(`SELECT count(*) FROM sessions WHERE length(token_hash) = 64`).Scan(&n)
	if n == 0 {
		t.Fatal("expected sha256 token hashes")
	}
	// login failures are indistinguishable
	a := e.do("POST", "/auth/login", "", map[string]any{"email": "citizen@example.org", "password": "wrong-password-x"})
	b := e.do("POST", "/auth/login", "", map[string]any{"email": "nobody@example.org", "password": "wrong-password-x"})
	if a.Status != 401 || b.Status != 401 || string(a.Raw) != string(b.Raw) {
		t.Fatalf("login failure: %d/%d %s | %s", a.Status, b.Status, a.Raw, b.Raw)
	}
	// profile update
	pr := e.do("PATCH", "/auth/me", tok, map[string]any{"display_name": "Yeni Ad", "locale": "en", "consent_location": true})
	if pr.Status != 200 || pr.data()["display_name"] != "Yeni Ad" || pr.data()["locale"] != "en" || pr.data()["location_consent"] != true {
		t.Fatalf("patch me: %d %s", pr.Status, pr.Raw)
	}
	if r := e.do("PATCH", "/auth/me", tok, map[string]any{"locale": "xx"}); r.Status != 400 {
		t.Fatalf("bad locale: %d", r.Status)
	}
	// bad / missing token
	if r := e.get("/auth/me", "garbage"); r.Status != 401 {
		t.Fatalf("garbage token: %d", r.Status)
	}
	if r := e.get("/auth/me", ""); r.Status != 401 {
		t.Fatalf("no token: %d", r.Status)
	}
	// logout revokes only that session
	tok2 := e.login("citizen@example.org", userPassword)
	if r := e.do("POST", "/auth/logout", tok, nil); r.Status != 204 {
		t.Fatalf("logout: %d", r.Status)
	}
	if r := e.get("/auth/me", tok); r.Status != 401 {
		t.Fatalf("revoked token still works: %d", r.Status)
	}
	if r := e.get("/auth/me", tok2); r.Status != 200 {
		t.Fatalf("other session should survive: %d", r.Status)
	}
}

func TestAccountDeletionAnonymizes(t *testing.T) {
	e := newEnv(t)
	tok, uid := e.register("leaving@example.org")
	other, _ := e.register("other@example.org")
	obs := e.createObs(tok, merkezPt, nil)
	id := obs["id"].(string)
	if r := e.decide(e.admin, id, "verify", nil); r.Status != 200 {
		t.Fatalf("verify: %d %s", r.Status, r.Raw)
	}
	draft := e.do("POST", "/observations", tok, obsBody(merkezPt, map[string]any{"draft": true}))
	if r := e.do("DELETE", "/auth/me", tok, nil); r.Status != 204 {
		t.Fatalf("delete me: %d", r.Status)
	}
	if r := e.get("/auth/me", tok); r.Status != 401 {
		t.Fatalf("session must be revoked: %d", r.Status)
	}
	if r := e.do("POST", "/auth/login", "", map[string]any{"email": "leaving@example.org", "password": userPassword}); r.Status != 401 {
		t.Fatalf("login after deletion: %d", r.Status)
	}
	var email, hash, status string
	e.db.QueryRow(`SELECT email, password_hash, status FROM users WHERE id = ?`, uid).Scan(&email, &hash, &status)
	if strings.Contains(email, "leaving") || status != "deleted" || hash != "!" {
		t.Fatalf("not anonymized: %q %q %q", email, hash, status)
	}
	var name string
	e.db.QueryRow(`SELECT display_name FROM user_profiles WHERE user_id = ?`, uid).Scan(&name)
	if name != "" {
		t.Fatalf("profile name kept: %q", name)
	}
	// verified observation survives, still pointing to the (anonymized) user row; draft is gone
	r := e.get("/observations/"+id, other)
	if r.Status != 200 || r.data()["user_id"] != uid {
		t.Fatalf("de-identified observation: %d %s", r.Status, r.Raw)
	}
	if r := e.get("/observations/"+draft.data()["id"].(string), e.admin); r.Status != 404 {
		t.Fatalf("draft should be removed: %d", r.Status)
	}
	// audit has no trace of the e-mail
	var n int
	e.db.QueryRow(`SELECT count(*) FROM audit_logs WHERE before_json LIKE '%leaving@%' OR after_json LIKE '%leaving@%'`).Scan(&n)
	if n != 0 {
		t.Fatal("email in audit log")
	}
}

func TestRoleMatrix(t *testing.T) {
	e := newEnv(t)
	citizen, _ := e.register("c@example.org")
	reviewer, _ := e.userWithRole("r@example.org", "reviewer")
	researcher, _ := e.userWithRole("s@example.org", "researcher")
	poly := map[string]any{"name": "Test", "geometry": map[string]any{"type": "Polygon",
		"coordinates": []any{[]any{[]any{33.0, 41.0}, []any{33.5, 41.0}, []any{33.5, 41.5}, []any{33.0, 41.5}, []any{33.0, 41.0}}}}}
	type tc struct {
		method, path string
		body         any
		want         map[string]int // anon, citizen, reviewer, researcher, admin
	}
	cases := []tc{
		{"GET", "/admin/users", nil, map[string]int{"anon": 401, "citizen": 403, "reviewer": 403, "researcher": 403, "admin": 200}},
		{"GET", "/admin/audit-logs", nil, map[string]int{"anon": 401, "citizen": 403, "reviewer": 403, "researcher": 403, "admin": 200}},
		{"GET", "/reviews/queue", nil, map[string]int{"anon": 401, "citizen": 403, "reviewer": 200, "researcher": 403, "admin": 200}},
		{"POST", "/regions", poly, map[string]int{"anon": 401, "citizen": 403, "reviewer": 403, "researcher": 201, "admin": 201}},
		{"GET", "/notifications", nil, map[string]int{"anon": 401, "citizen": 200, "reviewer": 200, "researcher": 200, "admin": 200}},
		{"POST", "/observations", obsBody(merkezPt, nil), map[string]int{"anon": 401, "citizen": 201, "reviewer": 201, "researcher": 201, "admin": 201}},
		{"GET", "/observations", nil, map[string]int{"anon": 200, "citizen": 200, "reviewer": 200, "researcher": 200, "admin": 200}},
		{"GET", "/regions", nil, map[string]int{"anon": 200, "citizen": 200, "reviewer": 200, "researcher": 200, "admin": 200}},
		{"GET", "/auth/me", nil, map[string]int{"anon": 401, "citizen": 200, "reviewer": 200, "researcher": 200, "admin": 200}},
		{"POST", "/auth/logout", nil, nil}, // handled separately below (would revoke the shared tokens)
	}
	toks := map[string]string{"anon": "", "citizen": citizen, "reviewer": reviewer, "researcher": researcher, "admin": e.admin}
	for _, c := range cases {
		if c.want == nil {
			continue
		}
		for role, want := range c.want {
			if got := e.do(c.method, c.path, toks[role], c.body); got.Status != want {
				t.Errorf("%s %s as %s: got %d want %d (%s)", c.method, c.path, role, got.Status, want, got.Raw)
			}
		}
	}
	// decision endpoint
	obs := e.createObs(citizen, merkezPt, nil)["id"].(string)
	for role, want := range map[string]int{"anon": 401, "citizen": 403, "researcher": 403} {
		if got := e.decide(toks[role], obs, "start_review", nil); got.Status != want {
			t.Errorf("decision as %s: got %d want %d", role, got.Status, want)
		}
	}
	// admin may promote, but not change themselves
	_, uid := e.register("promote@example.org")
	if r := e.do("PATCH", "/admin/users/"+uid, citizen, map[string]any{"role": "admin"}); r.Status != 403 {
		t.Errorf("citizen promoting: %d", r.Status)
	}
	me := e.get("/auth/me", e.admin).data()["id"].(string)
	if r := e.do("PATCH", "/admin/users/"+me, e.admin, map[string]any{"role": "citizen"}); r.Status != 409 {
		t.Errorf("admin self-demotion: %d", r.Status)
	}
	if r := e.do("PATCH", "/admin/users/"+uid, e.admin, map[string]any{"role": "wizard"}); r.Status != 400 {
		t.Errorf("bad role: %d", r.Status)
	}
	// suspension locks the user out immediately
	stok, sid := e.register("susp@example.org")
	if r := e.do("PATCH", "/admin/users/"+sid, e.admin, map[string]any{"status": "suspended"}); r.Status != 200 {
		t.Fatalf("suspend: %d", r.Status)
	}
	if r := e.get("/auth/me", stok); r.Status != 401 && r.Status != 403 {
		t.Errorf("suspended token: %d", r.Status)
	}
	if r := e.do("POST", "/auth/login", "", map[string]any{"email": "susp@example.org", "password": userPassword}); r.Status != 403 {
		t.Errorf("suspended login: %d", r.Status)
	}
	// admin user list + audit trail of role change
	if r := e.get("/admin/users?page_size=500", e.admin); r.Status != 200 || r.meta("page_size") != 200 {
		t.Errorf("page_size cap: %d %v", r.Status, r.Body["meta"])
	}
	al := e.get("/admin/audit-logs?entity_type=user&entity_id="+uid, e.admin)
	found := false
	for _, x := range al.list() {
		if x.(map[string]any)["action"] == "user.role_change" || x.(map[string]any)["action"] == "user.register" {
			found = true
		}
	}
	if !found {
		t.Errorf("audit log lacks entries: %s", al.Raw)
	}
}

func TestBootstrapAdminOnlyWhenNoneExists(t *testing.T) {
	e := newEnv(t)
	var n int
	e.db.QueryRow(`SELECT count(*) FROM users WHERE role='admin'`).Scan(&n)
	if n != 1 {
		t.Fatalf("admins: %d", n)
	}
	// second bootstrap with another e-mail must not create a second admin
	e.app.Deps().Auth.EnsureAdmin(t.Context(), "second@example.org", "another-long-password")
	e.db.QueryRow(`SELECT count(*) FROM users WHERE role='admin'`).Scan(&n)
	if n != 1 {
		t.Fatalf("admins after re-bootstrap: %d", n)
	}
}

func TestStrictAuthRateLimit(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.RateLimitPerMin = 50 }) // strict = 5/min, burst 10
	limited := 0
	for i := 0; i < 15; i++ {
		r := e.do("POST", "/auth/login", "", map[string]any{"email": "x@example.org", "password": "wrong-password-x"})
		if r.Status == 429 {
			limited++
			if r.errCode() != "rate_limited" || r.Header.Get("Retry-After") == "" {
				t.Fatalf("429 shape: %s", r.Raw)
			}
		}
	}
	if limited == 0 {
		t.Fatal("login was never rate limited")
	}
}

var refRe = regexp.MustCompile(`^ET-\d{4}-\d{6}$`)

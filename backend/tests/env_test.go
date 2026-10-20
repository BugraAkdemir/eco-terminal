// Package tests holds end-to-end tests that run the real application (real SQLite, real HTTP
// handlers via httptest) from the outside.
package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/app"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

const (
	adminEmail    = "admin@example.org"
	adminPassword = "correct-horse-battery"
	userPassword  = "a-long-enough-password"
)

type env struct {
	t     *testing.T
	srv   *httptest.Server
	app   *app.App
	db    *sql.DB
	cfg   config.Config
	admin string // admin token
}

func newEnv(t *testing.T, mutate ...func(*config.Config)) *env {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.OpenTemp(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := config.Config{
		Addr: ":0", Env: "test", DBPath: dir + "/test.db", StorageDir: dir + "/media", MaxUploadMB: 2,
		CORSOrigins: []string{"*"}, SessionTTL: time.Hour, RateLimitPerMin: 100000,
		AdminEmail: adminEmail, AdminPassword: adminPassword,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	a, err := app.New(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	e := &env{t: t, srv: srv, app: a, db: db, cfg: cfg}
	e.admin = e.login(adminEmail, adminPassword)
	return e
}

type resp struct {
	Status int
	Header http.Header
	Raw    []byte
	Body   map[string]any
}

func (r resp) data() map[string]any {
	d, _ := r.Body["data"].(map[string]any)
	return d
}

func (r resp) list() []any {
	d, _ := r.Body["data"].([]any)
	return d
}

func (r resp) errCode() string {
	e, _ := r.Body["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func (r resp) meta(k string) float64 {
	m, _ := r.Body["meta"].(map[string]any)
	f, _ := m[k].(float64)
	return f
}

func (e *env) do(method, path, token string, body any) resp {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+"/api/v1"+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return e.send(req)
}

func (e *env) send(req *http.Request) resp {
	e.t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	r := resp{Status: res.StatusCode, Header: res.Header, Raw: raw}
	if len(raw) > 0 && raw[0] == '{' {
		_ = json.Unmarshal(raw, &r.Body)
	}
	return r
}

func (e *env) login(email, password string) string {
	e.t.Helper()
	r := e.do("POST", "/auth/login", "", map[string]any{"email": email, "password": password})
	if r.Status != 200 {
		e.t.Fatalf("login %s: %d %s", email, r.Status, r.Raw)
	}
	return r.data()["token"].(string)
}

// register creates a citizen and returns (token, user id).
func (e *env) register(email string) (string, string) {
	e.t.Helper()
	r := e.do("POST", "/auth/register", "", map[string]any{"email": email, "password": userPassword, "display_name": "T " + email})
	if r.Status != 201 {
		e.t.Fatalf("register %s: %d %s", email, r.Status, r.Raw)
	}
	u := r.data()["user"].(map[string]any)
	return r.data()["token"].(string), u["id"].(string)
}

// userWithRole registers a user and lets the admin change the role.
func (e *env) userWithRole(email, role string) (string, string) {
	e.t.Helper()
	tok, id := e.register(email)
	r := e.do("PATCH", "/admin/users/"+id, e.admin, map[string]any{"role": role})
	if r.Status != 200 {
		e.t.Fatalf("set role: %d %s", r.Status, r.Raw)
	}
	return tok, id
}

// Kastamonu points used by tests (inside the real district polygons).
var (
	merkezPt  = [2]float64{33.7827, 41.3887} // lon, lat
	inebolu   = [2]float64{33.7600, 41.9780}
	outsideTR = [2]float64{2.35, 48.85} // Paris
	validTime = func() string { return time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339) }
)

func obsBody(pt [2]float64, extra map[string]any) map[string]any {
	b := map[string]any{
		"record_type": "observation", "category": "animal", "title": "Test sighting", "description": "d",
		"species_guess": "Cervus elaphus", "lon": pt[0], "lat": pt[1], "observed_at": validTime(),
	}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

func (e *env) createObs(token string, pt [2]float64, extra map[string]any) map[string]any {
	e.t.Helper()
	r := e.do("POST", "/observations", token, obsBody(pt, extra))
	if r.Status != 201 {
		e.t.Fatalf("create obs: %d %s", r.Status, r.Raw)
	}
	return r.data()
}

func (e *env) decide(token, obsID, decision string, extra map[string]any) resp {
	body := map[string]any{"decision": decision}
	for k, v := range extra {
		body[k] = v
	}
	return e.do("POST", "/reviews/"+obsID+"/decision", token, body)
}

func tinyPNG(w, h int, shade uint8) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: shade, G: uint8(x * 10), B: uint8(y * 10), A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func (e *env) upload(token, obsID, filename string, data []byte) resp {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	h.Set("Content-Type", "image/png") // the declared type is irrelevant; only magic bytes count
	part, _ := mw.CreatePart(h)
	_, _ = part.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/observations/"+obsID+"/media", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return e.send(req)
}

func (e *env) get(path, token string) resp { return e.do("GET", path, token, nil) }

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

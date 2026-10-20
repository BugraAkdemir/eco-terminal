package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/rss"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Complete admin feed -> durable job -> article -> cited chat flow, with a local
// RSS fixture. The private-network exception exists only in this test instance.
func TestFeedIngestHTTPToRAG(t *testing.T) {
	db, err := storage.OpenTemp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := storage.Now()
	_, err = db.Exec(`INSERT INTO users(id,email,password_hash,role,created_at,updated_at) VALUES ('admin','fixture@example.test','unused','admin',?,?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Env: "test", RSS: config.RSS{DefaultInterval: time.Hour, FetchTimeout: time.Second}}
	m := newIntelligenceModule(Deps{DB: db, Config: cfg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	m.feeds.Fetcher = rss.NewFetcher(cfg, true)
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		io.WriteString(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Test fixture</title><link>https://example.test</link><description>Fixture</description><item><guid>fixture-one</guid><title>Duyurutesti orman araştırması</title><link>https://example.test/item</link><description>Duyurutesti orman çalışmasının duyurusu.</description></item></channel></rss>`)
	}))
	defer fixture.Close()
	mux := http.NewServeMux()
	m.Register(mux, m.d)
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
		r = r.WithContext(auth.WithUser(r.Context(), &auth.User{ID: "admin", Role: "admin", Status: "active"}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	input := map[string]any{"name": "Local test fixture", "feed_url": fixture.URL}
	added := call("POST", "/admin/feeds", input)
	if added.Code != 201 {
		t.Fatalf("add: %d %s", added.Code, added.Body.String())
	}
	var envelope struct {
		Data rss.Source `json:"data"`
	}
	if err := json.Unmarshal(added.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if repeated := call("POST", "/admin/feeds", input); repeated.Code != 409 {
		t.Fatalf("duplicate %d", repeated.Code)
	}
	queued := call("POST", "/admin/feeds/sync", map[string]any{"source_id": envelope.Data.ID})
	if queued.Code != 202 {
		t.Fatalf("sync %d %s", queued.Code, queued.Body.String())
	}
	if worked, err := m.jobs.RunOnce(context.Background()); err != nil || !worked {
		t.Fatalf("worker %t %v", worked, err)
	}
	articles := call("GET", "/feeds/articles", nil)
	if articles.Code != 200 || !bytes.Contains(articles.Body.Bytes(), []byte("Duyurutesti")) {
		t.Fatalf("articles: %s", articles.Body.String())
	}
	answer := call("POST", "/ai/chat", map[string]string{"message": "Duyurutesti"})
	var chat struct {
		Data struct {
			Mode    string `json:"mode"`
			Sources []struct {
				Type string `json:"type"`
				URL  string `json:"url"`
			} `json:"sources"`
		} `json:"data"`
	}
	if err := json.Unmarshal(answer.Body.Bytes(), &chat); err != nil {
		t.Fatal(err)
	}
	if answer.Code != 200 || chat.Data.Mode != "retrieval_only" || len(chat.Data.Sources) != 1 || chat.Data.Sources[0].Type != "rss_article" || chat.Data.Sources[0].URL != "https://example.test/item" {
		t.Fatalf("cited answer %d %s", answer.Code, answer.Body.String())
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM audit_logs WHERE action='feed.create' AND actor_id='admin'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("actor audit %d %v", count, err)
	}
	// Failure to record an audit entry must roll back the feed insert.
	if _, err := db.Exec(`CREATE TRIGGER test_audit_failure BEFORE INSERT ON audit_logs WHEN NEW.action='feed.create' BEGIN SELECT RAISE(ABORT,'test audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	failed := call("POST", "/admin/feeds", map[string]string{"name": "Rollback fixture", "feed_url": fixture.URL + "/second"})
	if failed.Code != 500 {
		t.Fatalf("audit rollback %d %s", failed.Code, failed.Body.String())
	}
	if err := db.QueryRow(`SELECT count(*) FROM rss_sources`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("source escaped rollback %d %v", count, err)
	}
}

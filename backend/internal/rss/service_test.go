package rss

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

type fakeIndexer struct {
	mu  sync.Mutex
	ids []string
}

func (f *fakeIndexer) IndexArticle(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = append(f.ids, id)
	return nil
}

func testConfig() config.Config {
	return config.Config{Env: "development", RSS: config.RSS{Enabled: true, DefaultInterval: time.Hour, FetchTimeout: 5 * time.Second, UserAgent: "EcoTerminalTest/1.0"}}
}

func newSvc(t *testing.T) (*Service, *sql.DB, *fakeIndexer) {
	t.Helper()
	db, err := storage.OpenTemp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	idx := &fakeIndexer{}
	s := NewService(db, testConfig(), idx)
	s.Fetcher = NewFetcher(testConfig(), true) // httptest servers live on 127.0.0.1
	s.Now = func() time.Time { return testNow }
	return s, db, idx
}

func addRegion(t *testing.T, db *sql.DB, id, kind, name, parent string) {
	t.Helper()
	var p any
	if parent != "" {
		p = parent
	}
	now := storage.Now()
	if _, err := db.Exec(`INSERT INTO geographic_regions(id,kind,name,parent_id,geojson,min_lon,max_lon,min_lat,max_lat,centroid_lon,centroid_lat,created_at,updated_at)
		VALUES (?,?,?,?, '{}',0,1,0,1,0.5,0.5,?,?)`, id, kind, name, p, now, now); err != nil {
		t.Fatal(err)
	}
}

func addSource(t *testing.T, db *sql.DB, id, url string, interval int) {
	t.Helper()
	now := storage.Now()
	if _, err := db.Exec(`INSERT INTO rss_sources(id,name,organization,feed_url,category,enabled,interval_minutes,created_at,updated_at) VALUES (?,?,?,?,?,1,?,?,?)`,
		id, "Test "+id, "Org", url, "general", interval, now, now); err != nil {
		t.Fatal(err)
	}
}

// feedServer serves a mutable RSS body with ETag support.
type feedServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	body string
	etag string
	hits atomic.Int32
	cond atomic.Int32
}

func newFeedServer(t *testing.T, body string) *feedServer {
	f := &feedServer{body: body, etag: `"v1"`}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("If-None-Match") == f.etag {
			f.cond.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", f.etag)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(f.body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *feedServer) set(body, etag string) {
	f.mu.Lock()
	f.body, f.etag = body, etag
	f.mu.Unlock()
}

func rssDoc(items ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>T</title><link>http://x.example</link>` + strings.Join(items, "") + `</channel></rss>`
}

func mkItem(guid, title, desc, date string) string {
	return fmt.Sprintf(`<item><title>%s</title><link>http://x.example/%s</link><guid>%s</guid><description>%s</description><pubDate>%s</pubDate></item>`, title, guid, guid, desc, date)
}

func TestSyncStoresDedupsAndUsesConditionalGET(t *testing.T) {
	s, db, idx := newSvc(t)
	fs := newFeedServer(t, rssDoc(
		mkItem("a", "Tosya'da orman yangını", "Tosya ilçesinde yangın çıktı.", "Mon, 19 Oct 2026 13:17:02 GMT"),
		mkItem("b", "Meteoroloji uyarısı", "Sağanak yağış bekleniyor.", "Tue, 20 Oct 2026 09:00:00 GMT"),
		mkItem("c", "Meteoroloji uyarısı", "Sağanak yağış bekleniyor.", "Tue, 20 Oct 2026 09:00:00 GMT"), // same content, other guid & link differ -> distinct url => distinct hash
	))
	addRegion(t, db, "r-kast", "province", "Kastamonu", "")
	addRegion(t, db, "r-tosya", "district", "Tosya", "r-kast")
	addSource(t, db, "s1", fs.srv.URL, 60)

	res, err := s.SyncSource(context.Background(), "s1")
	if err != nil || res.Status != "ok" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if res.New != 3 || len(idx.ids) != 3 {
		t.Fatalf("new=%d indexed=%d", res.New, len(idx.ids))
	}
	// region tagging: only the Tosya article
	var tagged, unknown int
	_ = db.QueryRow(`SELECT count(*) FROM rss_article_regions WHERE region_id='r-tosya'`).Scan(&tagged)
	_ = db.QueryRow(`SELECT count(*) FROM rss_articles WHERE location_note=?`, LocationUnknownNote).Scan(&unknown)
	if tagged != 1 || unknown != 2 {
		t.Fatalf("tagged=%d unknown=%d", tagged, unknown)
	}
	var cat string
	_ = db.QueryRow(`SELECT category FROM rss_articles WHERE guid='a'`).Scan(&cat)
	if cat != CatForest {
		t.Fatalf("category = %s", cat)
	}
	src, _ := s.GetSource(context.Background(), "s1")
	if src.LastStatus != "ok" || src.VerifiedAt == nil || src.LastFetchedAt == nil || src.etag != `"v1"` {
		t.Fatalf("source = %+v", src)
	}

	// second sync: conditional GET -> 304
	res, _ = s.SyncSource(context.Background(), "s1")
	if res.Status != "not_modified" || fs.cond.Load() != 1 {
		t.Fatalf("res=%+v cond=%d", res, fs.cond.Load())
	}

	// feed changes (new etag): same items again => nothing new, nothing re-indexed
	fs.set(fs.body, `"v2"`)
	res, _ = s.SyncSource(context.Background(), "s1")
	if res.Status != "ok" || res.New != 0 || res.Updated != 0 || len(idx.ids) != 3 {
		t.Fatalf("res=%+v indexed=%d", res, len(idx.ids))
	}

	// an item changes text under the same guid => updated + re-indexed
	fs.set(rssDoc(mkItem("a", "Tosya'da orman yangını", "Yangın kontrol altına alındı.", "Mon, 19 Oct 2026 13:17:02 GMT")), `"v3"`)
	res, _ = s.SyncSource(context.Background(), "s1")
	if res.Updated != 1 || len(idx.ids) != 4 {
		t.Fatalf("res=%+v indexed=%d", res, len(idx.ids))
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM rss_articles`).Scan(&n)
	if n != 3 {
		t.Fatalf("articles = %d", n)
	}
	// audit row for the sync
	_ = db.QueryRow(`SELECT count(*) FROM audit_logs WHERE action='feed.sync' AND entity_id='s1'`).Scan(&n)
	if n < 1 {
		t.Fatal("no feed.sync audit entry")
	}
}

func TestDuplicateContentAcrossSourcesIsSkipped(t *testing.T) {
	s, db, _ := newSvc(t)
	body := rssDoc(mkItem("same", "Ortak duyuru", "Aynı içerik.", "Mon, 19 Oct 2026 13:17:02 GMT"))
	f1, f2 := newFeedServer(t, body), newFeedServer(t, body)
	addSource(t, db, "s1", f1.srv.URL, 60)
	addSource(t, db, "s2", f2.srv.URL, 60)
	r1, _ := s.SyncSource(context.Background(), "s1")
	r2, _ := s.SyncSource(context.Background(), "s2")
	// links are identical (http://x.example/same) so content_hash collides across sources
	if r1.New != 1 || r2.New != 0 || r2.Duplicates != 1 {
		t.Fatalf("r1=%+v r2=%+v", r1, r2)
	}
}

func TestSyncErrorsAreRecordedNotThrown(t *testing.T) {
	s, db, _ := newSvc(t)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) }))
	defer bad.Close()
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<!DOCTYPE html><html><body>hi</body></html>"))
	}))
	defer html.Close()
	addSource(t, db, "s500", bad.URL, 60)
	addSource(t, db, "shtml", html.URL, 60)
	for id, want := range map[string]string{"s500": "HTTP 500", "shtml": "not an RSS"} {
		res, err := s.SyncSource(context.Background(), id)
		if err != nil || res.Status != "error" || !strings.Contains(res.Error, want) {
			t.Fatalf("%s: res=%+v err=%v", id, res, err)
		}
		src, _ := s.GetSource(context.Background(), id)
		if src.LastStatus != "error" || !strings.Contains(src.LastError, want) || src.LastFetchedAt == nil {
			t.Fatalf("%s: source=%+v", id, src)
		}
	}
	if _, err := s.SyncSource(context.Background(), "nope"); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestSyncInProgressGuard(t *testing.T) {
	s, db, _ := newSvc(t)
	release := make(chan struct{})
	entered := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = w.Write([]byte(rssDoc(mkItem("a", "T", "d", "2026-10-20"))))
	}))
	defer srv.Close()
	addSource(t, db, "s1", srv.URL, 60)
	done := make(chan SyncResult)
	go func() { r, _ := s.SyncSource(context.Background(), "s1"); done <- r }()
	select {
	case <-entered: // the first sync holds the in-flight slot and is inside the HTTP handler
	case <-time.After(3 * time.Second):
		t.Fatal("first sync never reached the server")
	}
	if _, err := s.SyncSource(context.Background(), "s1"); !errors.Is(err, ErrSyncInProgress) {
		t.Fatalf("err = %v, want ErrSyncInProgress", err)
	}
	close(release)
	if r := <-done; r.Status != "ok" {
		t.Fatalf("r = %+v", r)
	}
}

func TestVerifyFeedURLAndAddSource(t *testing.T) {
	s, _, _ := newSvc(t)
	fs := newFeedServer(t, rssDoc(mkItem("a", "T", "d", "Mon, 19 Oct 2026 13:17:02 GMT")))
	info, err := s.VerifyFeedURL(context.Background(), fs.srv.URL)
	if err != nil || info.Format != "rss2" || info.ItemCount != 1 || info.NewestItem == nil {
		t.Fatalf("info=%+v err=%v", info, err)
	}
	src, err := s.AddSource(context.Background(), AddSourceInput{Name: " Deneme ", FeedURL: fs.srv.URL, IntervalMinutes: 1})
	if err != nil {
		t.Fatal(err)
	}
	if src.Name != "Deneme" || src.IntervalMinutes != minIntervalMinutes || src.VerifiedAt == nil || !src.Enabled {
		t.Fatalf("src = %+v", src)
	}
	if _, err := s.AddSource(context.Background(), AddSourceInput{Name: "x", FeedURL: fs.srv.URL}); !errors.Is(err, ErrDuplicateSource) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.AddSource(context.Background(), AddSourceInput{FeedURL: fs.srv.URL + "/z"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v", err)
	}
	htmlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html></html>")) }))
	defer htmlSrv.Close()
	if _, err := s.AddSource(context.Background(), AddSourceInput{Name: "h", FeedURL: htmlSrv.URL}); !errors.Is(err, ErrNotFeed) {
		t.Fatalf("err = %v", err)
	}
	empty := newFeedServer(t, rssDoc())
	if _, err := s.VerifyFeedURL(context.Background(), empty.srv.URL); !errors.Is(err, ErrNotFeed) {
		t.Fatalf("empty feed err = %v", err)
	}
}

func TestListArticlesFilters(t *testing.T) {
	s, db, _ := newSvc(t)
	addRegion(t, db, "r-kast", "province", "Kastamonu", "")
	addRegion(t, db, "r-daday", "district", "Daday", "r-kast")
	fs := newFeedServer(t, rssDoc(
		mkItem("1", "Daday'da su kesintisi", "Daday ilçesinde baraj bakımı.", "Tue, 20 Oct 2026 09:00:00 GMT"),
		mkItem("2", "Genel duyuru", "Bir şey.", "Sat, 10 Oct 2026 09:00:00 GMT"),
	))
	addSource(t, db, "s1", fs.srv.URL, 60)
	if _, err := s.SyncSource(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	all, total, err := s.ListArticles(context.Background(), ArticleFilter{})
	if err != nil || total != 2 || all[0].Title != "Daday'da su kesintisi" || all[0].SourceName != "Test s1" {
		t.Fatalf("all=%+v total=%d err=%v", all, total, err)
	}
	if len(all[0].Regions) != 1 || all[0].Regions[0].Name != "Daday" || all[1].LocationNote != LocationUnknownNote {
		t.Fatalf("regions: %+v / %+v", all[0], all[1])
	}
	// province filter includes child district
	byReg, total, _ := s.ListArticles(context.Background(), ArticleFilter{RegionID: "r-kast"})
	if total != 1 || byReg[0].ID != all[0].ID {
		t.Fatalf("region filter: %+v", byReg)
	}
	_, total, _ = s.ListArticles(context.Background(), ArticleFilter{From: "2026-10-15"})
	if total != 1 {
		t.Fatalf("from filter total = %d", total)
	}
	_, total, _ = s.ListArticles(context.Background(), ArticleFilter{Category: "nonexistent"})
	if total != 0 {
		t.Fatal("category filter")
	}
	// disabled sources are hidden from public listing
	if err := s.SetSourceEnabled(context.Background(), "s1", false); err != nil {
		t.Fatal(err)
	}
	_, total, _ = s.ListArticles(context.Background(), ArticleFilter{})
	if total != 0 {
		t.Fatal("disabled source articles visible")
	}
	_, total, _ = s.ListArticles(context.Background(), ArticleFilter{IncludeDisabled: true})
	if total != 2 {
		t.Fatal("admin listing")
	}
}

func TestSeedSources(t *testing.T) {
	s, db, _ := newSvc(t)
	n, err := SeedSources(db, "../../../datasets/seed/rss_sources.json")
	if err != nil || n < 5 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if n2, err := SeedSources(db, "../../../datasets/seed/rss_sources.json"); err != nil || n2 != 0 {
		t.Fatalf("second seed n=%d err=%v", n2, err)
	}
	srcs, _ := s.ListSources(context.Background(), true)
	if len(srcs) != n {
		t.Fatalf("sources = %d", len(srcs))
	}
	seen := map[string]bool{}
	for _, src := range srcs {
		if !strings.HasPrefix(src.FeedURL, "https://") || src.VerifiedAt == nil || seen[src.FeedURL] {
			t.Fatalf("bad seed source %+v", src)
		}
		seen[src.FeedURL] = true
	}
	if _, err := SeedSources(db, "/nonexistent/seed.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}

func TestFeedSizeCap(t *testing.T) {
	s, db, _ := newSvc(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<rss version="2.0"><channel><title>x</title>`))
		chunk := []byte(strings.Repeat("a", 1<<20))
		for i := 0; i < 6; i++ {
			_, _ = w.Write(chunk)
		}
	}))
	defer srv.Close()
	addSource(t, db, "big", srv.URL, 60)
	res, _ := s.SyncSource(context.Background(), "big")
	if res.Status != "error" || !strings.Contains(res.Error, "larger") {
		t.Fatalf("res = %+v", res)
	}
}

func TestSuggestCategory(t *testing.T) {
	cases := []struct{ title, summary, fallback, want string }{
		{"Orman yangınına müdahale", "", "general", CatForest},
		{"Deprem sonrası açıklama", "AFAD", "", CatDisaster},
		{"İklim değişikliği raporu", "karbon emisyonu", "", CatClimate},
		{"Yaban hayatı koruma alanı", "", "", CatWildlife},
		{"Belediye meclis toplantısı", "gündem", "general", "general"},
		{"Kültür festivali", "", "", CatGeneral}, // "kultur" must not trigger a whole-word rule
		{"Sağanak yağış uyarısı", "meteoroloji", "", CatWeather},
	}
	for _, c := range cases {
		if got := SuggestCategory(c.title, c.summary, nil, c.fallback); got != c.want {
			t.Errorf("%q -> %s, want %s", c.title, got, c.want)
		}
	}
}

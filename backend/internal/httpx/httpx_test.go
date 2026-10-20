package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterTokenBucket(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewRateLimiter(60, 3) // 1 token/second, burst 3
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("burst request %d refused", i)
		}
	}
	if ok, wait := l.Allow("a"); ok || wait <= 0 || wait > time.Second {
		t.Fatalf("4th request: ok=%v wait=%v", ok, wait)
	}
	if ok, _ := l.Allow("b"); !ok { // other keys are independent
		t.Fatal("independent key refused")
	}
	now = now.Add(1500 * time.Millisecond) // refills 1.5 tokens
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("no refill after 1.5s")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("only one token should have been refilled")
	}
}

func TestRateLimitMiddleware429(t *testing.T) {
	l := NewRateLimiter(1, 1)
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	do := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "203.0.113.9:5555"
		req.Header.Set("X-Forwarded-For", "1.2.3.4") // must not influence the key
		h.ServeHTTP(rec, req)
		return rec
	}
	if r := do(); r.Code != 204 {
		t.Fatalf("first: %d", r.Code)
	}
	r := do()
	if r.Code != 429 || r.Header().Get("Retry-After") == "" || r.Body.Len() == 0 {
		t.Fatalf("second: %d %v %s", r.Code, r.Header(), r.Body.String())
	}
}

func TestParsePageCapsAndValidates(t *testing.T) {
	get := func(q string) (Page, error) { return ParsePage(httptest.NewRequest("GET", "/?"+q, nil)) }
	p, err := get("")
	if err != nil || p.Page != 1 || p.Size != 50 || p.Offset != 0 {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	if p, _ = get("page=3&page_size=1000"); p.Size != 200 || p.Offset != 400 {
		t.Fatalf("cap: %+v", p)
	}
	for _, q := range []string{"page=0", "page=x", "page_size=-1", "page_size=abc"} {
		if _, err := get(q); err == nil {
			t.Errorf("%s should fail", q)
		}
	}
}

func TestErrorEnvelopeHidesInternalDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.ErrBodyNotAllowed) // arbitrary non-API error
	if rec.Code != 500 || rec.Body.String() == "" || contains(rec.Body.String(), "not allowed") {
		t.Fatalf("leaky internal error: %d %s", rec.Code, rec.Body.String())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

package httpx

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// Chain applies mws so that the first one is the outermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

type ctxKey int

const requestIDKey ctxKey = 1

// RequestIDFrom returns the request id attached by RequestID ("" if none).
func RequestIDFrom(ctx context.Context) string {
	s, _ := ctx.Value(requestIDKey).(string)
	return s
}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9_\-]{8,64}$`)

// RequestID reuses a sane incoming X-Request-ID or generates one, stores it in the context
// and echoes it in the response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID.MatchString(id) {
			var b [8]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// ClientIP returns the peer IP of the connection. Forwarding headers are deliberately NOT
// trusted (they are client-controlled and would defeat the rate limiter); if the server runs
// behind a reverse proxy, terminate rate limiting there or set RemoteAddr via the proxy.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// statusRecorder captures status and size for logging while staying transparent to
// http.ResponseController (Unwrap) and Flusher.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status, s.wrote = http.StatusOK, true
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := s.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("hijack not supported")
}

// AccessLog logs one line per request: method, path (no query string, never headers), status,
// size, duration, request id, client ip.
func AccessLog(l *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			l.Info("http",
				"method", r.Method, "path", r.URL.Path, "status", rec.status, "bytes", rec.bytes,
				"dur_ms", time.Since(start).Milliseconds(), "req_id", RequestIDFrom(r.Context()), "ip", ClientIP(r))
		})
	}
}

// Recover converts handler panics into a 500 JSON error (stack goes to the log only).
func Recover(l *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				l.Error("panic in handler", "panic", rec, "path", r.URL.Path,
					"req_id", RequestIDFrom(r.Context()), "stack", string(debug.Stack()))
				WriteError(w, Internal())
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORS allows the configured origins ("*" = any). Tokens travel in the Authorization header, so
// credentials/cookies are never allowed.
func CORS(origins []string) Middleware {
	anyOrigin := false
	allowed := map[string]bool{}
	for _, o := range origins {
		if o == "*" {
			anyOrigin = true
		}
		allowed[strings.TrimRight(o, "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (anyOrigin || allowed[origin]) {
				h := w.Header()
				if anyOrigin {
					h.Set("Access-Control-Allow-Origin", "*")
				} else {
					h.Set("Access-Control-Allow-Origin", origin)
					h.Add("Vary", "Origin")
				}
				h.Set("Access-Control-Expose-Headers", "X-Request-ID, Retry-After")
				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
					h.Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BodyLimit caps every request body at n bytes (handlers may narrow it further).
func BodyLimit(n int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > n {
				WriteError(w, TooLarge("request body too large"))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

// JSONFallback rewrites the plain-text 404/405 that net/http's ServeMux produces into the JSON
// error envelope. Responses written by our own handlers (JSON content type) pass through.
func JSONFallback(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&fallbackWriter{ResponseWriter: w}, r)
	})
}

type fallbackWriter struct {
	http.ResponseWriter
	swallow bool
}

func (f *fallbackWriter) WriteHeader(code int) {
	ct := f.Header().Get("Content-Type")
	if (code == http.StatusNotFound || code == http.StatusMethodNotAllowed) && strings.HasPrefix(ct, "text/plain") {
		f.swallow = true
		f.Header().Del("Content-Length")
		if code == http.StatusNotFound {
			WriteError(f.ResponseWriter, NotFound("no such endpoint"))
		} else {
			WriteError(f.ResponseWriter, newErr(code, "method_not_allowed", "method not allowed"))
		}
		return
	}
	f.ResponseWriter.WriteHeader(code)
}

func (f *fallbackWriter) Write(b []byte) (int, error) {
	if f.swallow {
		return len(b), nil
	}
	return f.ResponseWriter.Write(b)
}

func (f *fallbackWriter) Unwrap() http.ResponseWriter { return f.ResponseWriter }

// ───────────── rate limiting ─────────────

type bucket struct {
	tokens float64
	last   time.Time
}

// RateLimiter is a per-key token bucket: `burst` tokens, refilled at `perMinute`/60 per second.
type RateLimiter struct {
	rate  float64 // tokens per second
	burst float64
	now   func() time.Time

	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
}

// NewRateLimiter creates a limiter allowing `perMinute` sustained requests per key and `burst` at once.
func NewRateLimiter(perMinute, burst int) *RateLimiter {
	if perMinute < 1 {
		perMinute = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &RateLimiter{rate: float64(perMinute) / 60, burst: float64(burst), now: time.Now, buckets: map[string]*bucket{}}
}

// Allow consumes one token for key. When it refuses, retryAfter says when a token will be available.
func (l *RateLimiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastSweep) > 5*time.Minute || len(l.buckets) > 50000 {
		for k, b := range l.buckets { // drop buckets that are full again: equivalent to "never seen"
			if b.tokens+now.Sub(b.last).Seconds()*l.rate >= l.burst {
				delete(l.buckets, k)
			}
		}
		l.lastSweep = now
	}
	b := l.buckets[key]
	if b == nil {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
	return false, wait
}

// Middleware rate-limits by client IP and answers 429 rate_limited with Retry-After.
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if ok, wait := l.Allow(ClientIP(r)); !ok {
			secs := int(wait.Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			WriteError(w, RateLimited("too many requests, slow down"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Wrap applies the limiter to a single handler (used for stricter per-route limits).
func (l *RateLimiter) Wrap(h http.Handler) http.Handler { return l.Middleware(h) }

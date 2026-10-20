// Package httpx holds the HTTP plumbing shared by every handler: the JSON envelope
// ({"data":..,"meta":..} / {"error":{"code","message","details"}}), typed API errors,
// pagination helpers, and the middleware chain (request id, access log, panic recovery,
// CORS, rate limiting, body limits). See README.md in this directory.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// Error is an API error that maps 1:1 to the OpenAPI error envelope.
type Error struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// WithDetails returns a copy of e carrying details.
func (e *Error) WithDetails(d map[string]any) *Error {
	c := *e
	c.Details = d
	return &c
}

func newErr(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

// Constructors for the documented error codes.
func Validation(msg string, details map[string]any) *Error {
	e := newErr(http.StatusBadRequest, "validation_failed", msg)
	e.Details = details
	return e
}
func Unauthorized(msg string) *Error { return newErr(http.StatusUnauthorized, "unauthorized", msg) }
func Forbidden(msg string) *Error    { return newErr(http.StatusForbidden, "forbidden", msg) }
func NotFound(msg string) *Error     { return newErr(http.StatusNotFound, "not_found", msg) }
func Conflict(msg string) *Error     { return newErr(http.StatusConflict, "conflict", msg) }
func TooLarge(msg string) *Error {
	return newErr(http.StatusRequestEntityTooLarge, "payload_too_large", msg)
}
func UnsupportedMedia(msg string) *Error {
	return newErr(http.StatusUnsupportedMediaType, "unsupported_media_type", msg)
}
func RateLimited(msg string) *Error { return newErr(http.StatusTooManyRequests, "rate_limited", msg) }
func AINotConfigured() *Error {
	return newErr(http.StatusServiceUnavailable, "ai_not_configured", "AI provider is not configured")
}
func Internal() *Error {
	return newErr(http.StatusInternalServerError, "internal", "internal server error")
}

type envelope struct {
	Data any `json:"data"`
	Meta any `json:"meta,omitempty"`
}

// WriteJSON writes {"data": data} with the given status.
func WriteJSON(w http.ResponseWriter, status int, data any) { WriteJSONMeta(w, status, data, nil) }

// WriteJSONMeta writes {"data": data, "meta": meta}.
func WriteJSONMeta(w http.ResponseWriter, status int, data, meta any) {
	body, err := json.Marshal(envelope{Data: data, Meta: meta})
	if err != nil {
		slog.Error("httpx: marshal response", "err", err)
		WriteError(w, Internal())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

// NoContent writes a 204.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// WriteError writes the error envelope. A non-*Error is logged and reported as 500 "internal"
// (its text is never sent to the client).
func WriteError(w http.ResponseWriter, err error) {
	var he *Error
	if !errors.As(err, &he) {
		slog.Error("httpx: unhandled error", "err", err)
		he = Internal()
	}
	payload := map[string]any{"code": he.Code, "message": he.Message}
	if len(he.Details) > 0 {
		payload["details"] = he.Details
	}
	body, _ := json.Marshal(map[string]any{"error": payload})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(he.Status)
	_, _ = w.Write(append(body, '\n'))
}

// MaxJSONBody is the default size cap of JSON request bodies.
const MaxJSONBody = 1 << 20

// DecodeJSON reads a single JSON object from the request body into dst. Unknown fields are
// tolerated (forward compatible clients); trailing data, empty bodies and oversize bodies are errors.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxJSONBody)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		switch {
		case errors.As(err, &mbe):
			return TooLarge("request body too large")
		case errors.Is(err, io.EOF):
			return Validation("request body is required", nil)
		default:
			return Validation("malformed JSON body", nil)
		}
	}
	if dec.More() {
		return Validation("unexpected data after JSON body", nil)
	}
	return nil
}

// PageMeta is the pagination block of list responses.
type PageMeta struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

const (
	DefaultPageSize = 50
	MaxPageSize     = 200
)

// Page holds parsed pagination parameters.
type Page struct{ Page, Size, Offset int }

// ParsePage reads ?page=&page_size= (defaults 1/50, page_size capped at 200).
// Invalid numbers are a validation error.
func ParsePage(r *http.Request) (Page, error) {
	p := Page{Page: 1, Size: DefaultPageSize}
	q := r.URL.Query()
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return p, Validation("page must be a positive integer", map[string]any{"field": "page"})
		}
		p.Page = n
	}
	if v := q.Get("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return p, Validation("page_size must be a positive integer", map[string]any{"field": "page_size"})
		}
		if n > MaxPageSize {
			n = MaxPageSize
		}
		p.Size = n
	}
	if p.Page > 1_000_000 {
		return p, Validation("page too large", map[string]any{"field": "page"})
	}
	p.Offset = (p.Page - 1) * p.Size
	return p, nil
}

// Meta builds the response meta for the page and total count.
func (p Page) Meta(total int) PageMeta { return PageMeta{Page: p.Page, PageSize: p.Size, Total: total} }

// QueryBool parses ?name=true|1 (case-insensitive). Absent = false.
func QueryBool(r *http.Request, name string) bool {
	switch strings.ToLower(r.URL.Query().Get(name)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// BearerToken extracts the token of an "Authorization: Bearer <token>" header ("" if absent).
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

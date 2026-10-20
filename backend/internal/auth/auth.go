// Package auth implements accounts, argon2id passwords, opaque bearer sessions, role checks and
// the /auth/* and /admin/users endpoints.
//
// Other packages use: UserFrom(ctx), Require(roles...), RequireFunc, HasRole, IsStaff,
// ActorFrom(r) (for audit entries) and the Role* constants.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Roles. admin implies all other roles; reviewer and researcher are siblings.
const (
	RoleCitizen    = "citizen"
	RoleResearcher = "researcher"
	RoleReviewer   = "reviewer"
	RoleAdmin      = "admin"
)

// MinPasswordLen is the minimum password length in characters.
const MinPasswordLen = 10

// User is the API representation of an account (User schema) plus the request-scoped session id.
type User struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	Role            string `json:"role"`
	Status          string `json:"status"`
	DisplayName     string `json:"display_name"`
	Locale          string `json:"locale"`
	LocationConsent bool   `json:"location_consent"`
	CreatedAt       string `json:"created_at"`

	SessionID string `json:"-"`
}

type ctxKey struct{}

// WithUser returns ctx carrying u (used by Authenticate; handy in tests).
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFrom returns the authenticated user of the request context, if any.
func UserFrom(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(ctxKey{}).(*User)
	return u, ok && u != nil
}

// HasRole reports whether u holds one of roles; admin holds every role. With no roles given any
// authenticated user qualifies.
func HasRole(u *User, roles ...string) bool {
	if u == nil {
		return false
	}
	if len(roles) == 0 || u.Role == RoleAdmin {
		return true
	}
	for _, r := range roles {
		if u.Role == r {
			return true
		}
	}
	return false
}

// IsStaff reports whether u may see all observations and review them (reviewer or admin).
func IsStaff(u *User) bool { return u != nil && (u.Role == RoleReviewer || u.Role == RoleAdmin) }

// ActorFrom builds the audit actor of a request (user from context + client IP).
func ActorFrom(r *http.Request) audit.Actor {
	a := audit.Actor{IP: httpx.ClientIP(r)}
	if u, ok := UserFrom(r.Context()); ok {
		a.ID, a.Role = u.ID, u.Role
	}
	return a
}

// Require returns middleware that answers 401 for anonymous callers and 403 when the user holds
// none of roles (admin always passes). It relies on Authenticate having run earlier in the chain
// (internal/app wraps the whole mux with it).
func Require(roles ...string) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := UserFrom(r.Context())
			if !ok {
				httpx.WriteError(w, httpx.Unauthorized("authentication required"))
				return
			}
			if !HasRole(u, roles...) {
				httpx.WriteError(w, httpx.Forbidden("insufficient role"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireFunc is Require for a handler func.
func RequireFunc(h http.HandlerFunc, roles ...string) http.Handler { return Require(roles...)(h) }

// Service owns accounts and sessions.
type Service struct {
	db  *sql.DB
	ttl time.Duration
	log *slog.Logger
}

// NewService creates the auth service. ttl is the session lifetime.
func NewService(db *sql.DB, ttl time.Duration, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	if ttl <= 0 {
		ttl = 720 * time.Hour
	}
	return &Service{db: db, ttl: ttl, log: log}
}

func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// ───────────── validation ─────────────

// NormalizeEmail trims, lowercases and validates an e-mail address.
func NormalizeEmail(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > 254 {
		return "", httpx.Validation("a valid email is required", map[string]any{"field": "email"})
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || !strings.Contains(s[strings.LastIndex(s, "@"):], ".") {
		return "", httpx.Validation("a valid email is required", map[string]any{"field": "email"})
	}
	return s, nil
}

func checkPassword(pw string) error {
	if utf8.RuneCountInString(pw) < MinPasswordLen {
		return httpx.Validation(fmt.Sprintf("password must be at least %d characters", MinPasswordLen), map[string]any{"field": "password"})
	}
	if len(pw) > 1024 {
		return httpx.Validation("password too long", map[string]any{"field": "password"})
	}
	return nil
}

func checkDisplayName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > 80 {
		return "", httpx.Validation("display_name must be at most 80 characters", map[string]any{"field": "display_name"})
	}
	return s, nil
}

// ───────────── core operations ─────────────

// Session is the result of Register/Login.
type Session struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	User      *User  `json:"user"`
}

const userSelect = `SELECT u.id, u.email, u.role, u.status, u.created_at,
	COALESCE(p.display_name,''), COALESCE(p.locale,'tr'), p.consent_location_at IS NOT NULL
	FROM users u LEFT JOIN user_profiles p ON p.user_id = u.id`

type rowScanner interface{ Scan(dest ...any) error }

func scanUser(r rowScanner) (*User, error) {
	var u User
	if err := r.Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.CreatedAt, &u.DisplayName, &u.Locale, &u.LocationConsent); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUser loads a user by id.
func (s *Service) GetUser(ctx context.Context, id string) (*User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, userSelect+` WHERE u.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.NotFound("user not found")
	}
	return u, err
}

func (s *Service) createSession(ctx context.Context, ex audit.Execer, userID, ua string) (token, expires string, err error) {
	token, err = newToken()
	if err != nil {
		return "", "", err
	}
	now := time.Now().UTC()
	expires = storage.Time(now.Add(s.ttl))
	if len(ua) > 200 {
		ua = ua[:200]
	}
	_, err = ex.ExecContext(ctx, `INSERT INTO sessions(id, user_id, token_hash, created_at, expires_at, user_agent)
		VALUES (?,?,?,?,?,?)`, storage.NewID(), userID, hashToken(token), storage.Time(now), expires, ua)
	return token, expires, err
}

// CreateUser inserts a user + profile. Callers must have validated email/password. Used by Register,
// the admin bootstrap and the demo seeder.
func (s *Service) CreateUser(ctx context.Context, ex audit.Execer, email, password, role, displayName string) (string, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return "", err
	}
	id, now := storage.NewID(), storage.Now()
	if _, err := ex.ExecContext(ctx, `INSERT INTO users(id, email, password_hash, role, status, created_at, updated_at)
		VALUES (?,?,?,?, 'active', ?, ?)`, id, email, hash, role, now, now); err != nil {
		if isUnique(err) {
			return "", httpx.Conflict("an account with this email already exists")
		}
		return "", err
	}
	if _, err := ex.ExecContext(ctx, `INSERT INTO user_profiles(user_id, display_name, locale, created_at, updated_at)
		VALUES (?,?, 'tr', ?, ?)`, id, displayName, now, now); err != nil {
		return "", err
	}
	return id, nil
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// Register creates a citizen account and logs it in.
func (s *Service) Register(ctx context.Context, email, password, displayName, ua string, actor audit.Actor) (*Session, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if err := checkPassword(password); err != nil {
		return nil, err
	}
	displayName, err = checkDisplayName(displayName)
	if err != nil {
		return nil, err
	}
	hash, err := HashPassword(password) // computed outside the write transaction (CPU/memory heavy)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id, now := storage.NewID(), storage.Now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO users(id, email, password_hash, role, status, created_at, updated_at)
		VALUES (?,?,?, 'citizen', 'active', ?, ?)`, id, email, hash, now, now); err != nil {
		if isUnique(err) {
			return nil, httpx.Conflict("an account with this email already exists")
		}
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_profiles(user_id, display_name, locale, created_at, updated_at)
		VALUES (?,?, 'tr', ?, ?)`, id, displayName, now, now); err != nil {
		return nil, err
	}
	token, expires, err := s.createSession(ctx, tx, id, ua)
	if err != nil {
		return nil, err
	}
	actor.ID, actor.Role = id, RoleCitizen
	if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "user.register", EntityType: "user", EntityID: id,
		After: map[string]any{"role": RoleCitizen}}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	u, err := s.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Session{Token: token, ExpiresAt: expires, User: u}, nil
}

// Login verifies credentials and opens a session.
func (s *Service) Login(ctx context.Context, email, password, ua string, actor audit.Actor) (*Session, error) {
	bad := httpx.Unauthorized("invalid email or password")
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" || len(password) > 1024 {
		return nil, bad
	}
	var id, hash, status string
	err := s.db.QueryRowContext(ctx, `SELECT id, password_hash, status FROM users WHERE email = ?`, email).Scan(&id, &hash, &status)
	if errors.Is(err, sql.ErrNoRows) {
		burnPassword(password)
		return nil, bad
	}
	if err != nil {
		return nil, err
	}
	if !VerifyPassword(password, hash) || status == "deleted" {
		return nil, bad
	}
	if status == "suspended" {
		return nil, httpx.Forbidden("account is suspended")
	}
	now := storage.Now()
	_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ? OR revoked_at IS NOT NULL AND revoked_at < ?`, now,
		storage.Time(time.Now().Add(-30*24*time.Hour)))
	token, expires, err := s.createSession(ctx, s.db, id, ua)
	if err != nil {
		return nil, err
	}
	u, err := s.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	actor.ID, actor.Role = id, u.Role
	if err := audit.Log(ctx, s.db, audit.Entry{Actor: actor, Action: "user.login", EntityType: "user", EntityID: id}); err != nil {
		s.log.Error("audit login", "err", err)
	}
	return &Session{Token: token, ExpiresAt: expires, User: u}, nil
}

// Logout revokes one session.
func (s *Service) Logout(ctx context.Context, sessionID string, actor audit.Actor) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, storage.Now(), sessionID); err != nil {
		return err
	}
	return audit.Log(ctx, s.db, audit.Entry{Actor: actor, Action: "user.logout", EntityType: "user", EntityID: actor.ID})
}

// Authenticate resolves a bearer token to its user.
func (s *Service) Authenticate(ctx context.Context, token string) (*User, error) {
	invalid := httpx.Unauthorized("invalid or expired token")
	if token == "" || len(token) > 200 {
		return nil, invalid
	}
	var sid string
	row := s.db.QueryRowContext(ctx, `SELECT u.id, u.email, u.role, u.status, u.created_at,
		COALESCE(p.display_name,''), COALESCE(p.locale,'tr'), p.consent_location_at IS NOT NULL, s.id
		FROM sessions s JOIN users u ON u.id = s.user_id LEFT JOIN user_profiles p ON p.user_id = u.id
		WHERE s.token_hash = ? AND s.revoked_at IS NULL AND s.expires_at > ?`, hashToken(token), storage.Now())
	var u User
	if err := row.Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.CreatedAt, &u.DisplayName, &u.Locale, &u.LocationConsent, &sid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, invalid
		}
		return nil, err
	}
	switch u.Status {
	case "deleted":
		return nil, invalid
	case "suspended":
		return nil, httpx.Forbidden("account is suspended")
	}
	u.SessionID = sid
	return &u, nil
}

// Middleware resolves an optional bearer token. No header = anonymous request (public endpoints
// work); a header with an invalid/expired token is a 401 so clients notice they must log in again.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			next.ServeHTTP(w, r)
			return
		}
		u, err := s.Authenticate(r.Context(), httpx.BearerToken(r))
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}

// EnsureAdmin creates the first admin from the environment when no admin exists. An existing
// account with that email is promoted instead. It never logs the password.
func (s *Service) EnsureAdmin(ctx context.Context, email, password string) error {
	if email == "" && password == "" {
		return nil
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role = 'admin' AND status = 'active'`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	email, err := NormalizeEmail(email)
	if err != nil {
		return fmt.Errorf("ECO_ADMIN_EMAIL: %w", err)
	}
	if err := checkPassword(password); err != nil {
		return fmt.Errorf("ECO_ADMIN_PASSWORD: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ?`, email).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if id, err = s.CreateUser(ctx, tx, email, password, RoleAdmin, "Admin"); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE users SET role='admin', status='active', updated_at=? WHERE id=?`, storage.Now(), id); err != nil {
			return err
		}
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: audit.System, Action: "user.bootstrap_admin", EntityType: "user", EntityID: id}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.log.Info("bootstrap admin ready", "user_id", id)
	return nil
}

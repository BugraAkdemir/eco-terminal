package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Routes registers /auth/* and /admin/users* on mux (paths include the /api/v1 prefix).
// strict is the stricter rate limiter applied to register/login (may be nil).
func (s *Service) Routes(mux *http.ServeMux, strict *httpx.RateLimiter) {
	limit := func(h http.Handler) http.Handler {
		if strict == nil {
			return h
		}
		return strict.Wrap(h)
	}
	mux.Handle("POST /api/v1/auth/register", limit(http.HandlerFunc(s.handleRegister)))
	mux.Handle("POST /api/v1/auth/login", limit(http.HandlerFunc(s.handleLogin)))
	mux.Handle("POST /api/v1/auth/logout", RequireFunc(s.handleLogout))
	mux.Handle("GET /api/v1/auth/me", RequireFunc(s.handleMe))
	mux.Handle("PATCH /api/v1/auth/me", RequireFunc(s.handlePatchMe))
	mux.Handle("DELETE /api/v1/auth/me", RequireFunc(s.handleDeleteMe))
	mux.Handle("GET /api/v1/admin/users", RequireFunc(s.handleListUsers, RoleAdmin))
	mux.Handle("PATCH /api/v1/admin/users/{id}", RequireFunc(s.handlePatchUser, RoleAdmin))
}

func (s *Service) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, err)
		return
	}
	sess, err := s.Register(r.Context(), in.Email, in.Password, in.DisplayName, r.UserAgent(), ActorFrom(r))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sess)
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, err)
		return
	}
	sess, err := s.Login(r.Context(), in.Email, in.Password, r.UserAgent(), ActorFrom(r))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sess)
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	if err := s.Logout(r.Context(), u.SessionID, ActorFrom(r)); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.NoContent(w)
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	httpx.WriteJSON(w, http.StatusOK, u)
}

func (s *Service) handlePatchMe(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	var in struct {
		DisplayName     *string `json:"display_name"`
		Locale          *string `json:"locale"`
		ConsentLocation *bool   `json:"consent_location"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, err)
		return
	}
	before := map[string]any{"display_name": u.DisplayName, "locale": u.Locale, "location_consent": u.LocationConsent}
	name, locale, consent := u.DisplayName, u.Locale, u.LocationConsent
	if in.DisplayName != nil {
		n, err := checkDisplayName(*in.DisplayName)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		name = n
	}
	if in.Locale != nil {
		l := strings.ToLower(strings.TrimSpace(*in.Locale))
		if l != "tr" && l != "en" {
			httpx.WriteError(w, httpx.Validation("locale must be 'tr' or 'en'", map[string]any{"field": "locale"}))
			return
		}
		locale = l
	}
	if in.ConsentLocation != nil {
		consent = *in.ConsentLocation
	}
	now := storage.Now()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `UPDATE user_profiles SET display_name = ?, locale = ?,
		consent_location_at = CASE WHEN ? THEN COALESCE(consent_location_at, ?) ELSE NULL END, updated_at = ? WHERE user_id = ?`,
		name, locale, consent, now, now, u.ID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	after := map[string]any{"display_name": name, "locale": locale, "location_consent": consent}
	if err := audit.Log(r.Context(), tx, audit.Entry{Actor: ActorFrom(r), Action: "user.update_profile", EntityType: "user",
		EntityID: u.ID, Before: before, After: after}); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.WriteError(w, err)
		return
	}
	nu, err := s.GetUser(r.Context(), u.ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, nu)
}

func (s *Service) handleDeleteMe(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFrom(r.Context())
	if err := s.Anonymize(r.Context(), u.ID, ActorFrom(r)); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.NoContent(w)
}

// Anonymize implements account deletion: personal data (email, name, avatar, consent, notifications,
// sessions) is scrubbed, the user row stays so de-identified observations keep their user_id.
// Draft observations are soft-deleted (never public anyway). The last active admin cannot be removed.
func (s *Service) Anonymize(ctx context.Context, userID string, actor audit.Actor) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var role, status string
	if err := tx.QueryRowContext(ctx, `SELECT role, status FROM users WHERE id = ?`, userID).Scan(&role, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return httpx.NotFound("user not found")
		}
		return err
	}
	if status == "deleted" {
		return httpx.NotFound("user not found")
	}
	if role == RoleAdmin {
		var admins int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role='admin' AND status='active'`).Scan(&admins); err != nil {
			return err
		}
		if admins <= 1 {
			return httpx.Conflict("the last administrator cannot delete the account")
		}
	}
	now := storage.Now()
	steps := []struct {
		q    string
		args []any
	}{
		{`UPDATE users SET email = ?, password_hash = '!', status = 'deleted', role = 'citizen', deleted_at = ?, updated_at = ? WHERE id = ?`,
			[]any{"deleted-" + userID + "@deleted.invalid", now, now, userID}},
		{`UPDATE user_profiles SET display_name = '', avatar_path = NULL, consent_location_at = NULL, locale = 'tr', updated_at = ? WHERE user_id = ?`,
			[]any{now, userID}},
		{`UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, []any{now, userID}},
		{`DELETE FROM notifications WHERE user_id = ?`, []any{userID}},
		{`UPDATE observations SET deleted_at = ?, updated_at = ? WHERE user_id = ? AND status = 'draft' AND deleted_at IS NULL`,
			[]any{now, now, userID}},
	}
	for _, st := range steps {
		if _, err := tx.ExecContext(ctx, st.q, st.args...); err != nil {
			return err
		}
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "user.delete", EntityType: "user", EntityID: userID,
		Before: map[string]any{"status": status}, After: map[string]any{"status": "deleted", "anonymized": true}}); err != nil {
		return err
	}
	return tx.Commit()
}

// ───────────── admin ─────────────

func (s *Service) handleListUsers(w http.ResponseWriter, r *http.Request) {
	page, err := httpx.ParsePage(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	q := r.URL.Query()
	where, args := "1=1", []any{}
	if v := q.Get("role"); v != "" {
		where += " AND u.role = ?"
		args = append(args, v)
	}
	if v := q.Get("status"); v != "" {
		where += " AND u.status = ?"
		args = append(args, v)
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		where += ` AND (u.email LIKE ? ESCAPE '\' OR p.display_name LIKE ? ESCAPE '\')`
		like := "%" + escapeLike(v) + "%"
		args = append(args, like, like)
	}
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM users u LEFT JOIN user_profiles p ON p.user_id=u.id WHERE `+where, args...).Scan(&total); err != nil {
		httpx.WriteError(w, err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), userSelect+` WHERE `+where+` ORDER BY u.created_at DESC, u.rowid DESC LIMIT ? OFFSET ?`,
		append(args, page.Size, page.Offset)...)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	defer rows.Close()
	out := []*User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSONMeta(w, http.StatusOK, out, page.Meta(total))
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (s *Service) handlePatchUser(w http.ResponseWriter, r *http.Request) {
	actorUser, _ := UserFrom(r.Context())
	id := r.PathValue("id")
	var in struct {
		Role   *string `json:"role"`
		Status *string `json:"status"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if in.Role == nil && in.Status == nil {
		httpx.WriteError(w, httpx.Validation("role or status is required", nil))
		return
	}
	if in.Role != nil {
		switch *in.Role {
		case RoleCitizen, RoleResearcher, RoleReviewer, RoleAdmin:
		default:
			httpx.WriteError(w, httpx.Validation("invalid role", map[string]any{"field": "role"}))
			return
		}
	}
	if in.Status != nil && *in.Status != "active" && *in.Status != "suspended" {
		httpx.WriteError(w, httpx.Validation("status must be 'active' or 'suspended'", map[string]any{"field": "status"}))
		return
	}
	if id == actorUser.ID {
		httpx.WriteError(w, httpx.Conflict("administrators cannot change their own role or status"))
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	defer tx.Rollback()
	var oldRole, oldStatus string
	if err := tx.QueryRowContext(r.Context(), `SELECT role, status FROM users WHERE id = ?`, id).Scan(&oldRole, &oldStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.WriteError(w, httpx.NotFound("user not found"))
			return
		}
		httpx.WriteError(w, err)
		return
	}
	if oldStatus == "deleted" {
		httpx.WriteError(w, httpx.Conflict("account is deleted"))
		return
	}
	newRole, newStatus := oldRole, oldStatus
	if in.Role != nil {
		newRole = *in.Role
	}
	if in.Status != nil {
		newStatus = *in.Status
	}
	now := storage.Now()
	if _, err := tx.ExecContext(r.Context(), `UPDATE users SET role=?, status=?, updated_at=? WHERE id=?`, newRole, newStatus, now, id); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if newStatus == "suspended" {
		if _, err := tx.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`, now, id); err != nil {
			httpx.WriteError(w, err)
			return
		}
	}
	actor := ActorFrom(r)
	if newRole != oldRole {
		if err := audit.Log(r.Context(), tx, audit.Entry{Actor: actor, Action: "user.role_change", EntityType: "user", EntityID: id,
			Before: map[string]any{"role": oldRole}, After: map[string]any{"role": newRole}}); err != nil {
			httpx.WriteError(w, err)
			return
		}
	}
	if newStatus != oldStatus {
		if err := audit.Log(r.Context(), tx, audit.Entry{Actor: actor, Action: "user.status_change", EntityType: "user", EntityID: id,
			Before: map[string]any{"status": oldStatus}, After: map[string]any{"status": newStatus}}); err != nil {
			httpx.WriteError(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.WriteError(w, err)
		return
	}
	u, err := s.GetUser(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}

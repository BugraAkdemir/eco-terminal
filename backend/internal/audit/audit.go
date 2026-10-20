// Package audit writes and lists audit_logs. Every state-changing code path calls Log inside
// the same transaction as the change, so the log can never disagree with the data.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Execer is satisfied by *sql.DB, *sql.Tx and *sql.Conn.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Actor identifies who performed an action. ID may be empty for system actions.
type Actor struct {
	ID   string
	Role string
	IP   string
}

// System is the actor of background/system actions.
var System = Actor{Role: "system"}

// Entry is one audit record. Before/After are JSON-marshalled (nil = empty). Never put secrets
// (passwords, tokens) or e-mail addresses of other users into them.
type Entry struct {
	Actor      Actor
	Action     string // e.g. observation.status_change, user.role_change
	EntityType string
	EntityID   string
	Before     any
	After      any
}

// Log inserts the entry using ex (pass the surrounding *sql.Tx when there is one).
func Log(ctx context.Context, ex Execer, e Entry) error {
	before, err := marshal(e.Before)
	if err != nil {
		return err
	}
	after, err := marshal(e.After)
	if err != nil {
		return err
	}
	var actorID any
	if e.Actor.ID != "" {
		actorID = e.Actor.ID
	}
	_, err = ex.ExecContext(ctx, `INSERT INTO audit_logs
		(id, actor_id, actor_role, action, entity_type, entity_id, before_json, after_json, ip, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		storage.NewID(), actorID, e.Actor.Role, e.Action, e.EntityType, e.EntityID, before, after, e.Actor.IP, storage.Now())
	return err
}

func marshal(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

// Record is the API representation (AuditLog schema).
type Record struct {
	ID         string          `json:"id"`
	ActorID    *string         `json:"actor_id"`
	ActorRole  string          `json:"actor_role"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	Before     json.RawMessage `json:"before"`
	After      json.RawMessage `json:"after"`
	CreatedAt  string          `json:"created_at"`
}

// ListHandler serves GET /admin/audit-logs. Mount it behind auth.Require(auth.RoleAdmin).
func ListHandler(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, err := httpx.ParsePage(r)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		q := r.URL.Query()
		where, args := "1=1", []any{}
		for _, f := range [][2]string{{"entity_type", "entity_type"}, {"entity_id", "entity_id"}, {"actor_id", "actor_id"}, {"action", "action"}} {
			if v := q.Get(f[0]); v != "" {
				where += " AND " + f[1] + " = ?"
				args = append(args, v)
			}
		}
		var total int
		if err := db.QueryRowContext(r.Context(), "SELECT count(*) FROM audit_logs WHERE "+where, args...).Scan(&total); err != nil {
			httpx.WriteError(w, err)
			return
		}
		rows, err := db.QueryContext(r.Context(), `SELECT id, actor_id, actor_role, action, entity_type, entity_id,
			before_json, after_json, created_at FROM audit_logs WHERE `+where+
			` ORDER BY created_at DESC, rowid DESC LIMIT ? OFFSET ?`, append(args, page.Size, page.Offset)...)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		defer rows.Close()
		out := []Record{}
		for rows.Next() {
			var rec Record
			var before, after string
			if err := rows.Scan(&rec.ID, &rec.ActorID, &rec.ActorRole, &rec.Action, &rec.EntityType, &rec.EntityID,
				&before, &after, &rec.CreatedAt); err != nil {
				httpx.WriteError(w, err)
				return
			}
			if before != "" {
				rec.Before = json.RawMessage(before)
			}
			if after != "" {
				rec.After = json.RawMessage(after)
			}
			out = append(out, rec)
		}
		if err := rows.Err(); err != nil {
			httpx.WriteError(w, err)
			return
		}
		httpx.WriteJSONMeta(w, http.StatusOK, out, page.Meta(total))
	})
}

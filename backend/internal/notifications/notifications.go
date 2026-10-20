// Package notifications stores in-app notifications and serves GET /notifications and
// POST /notifications/{id}/read.
package notifications

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Kinds.
const (
	KindObservationStatus = "observation_status"
	KindInfoRequested     = "info_requested"
	KindSystem            = "system"
)

// Notification is the API representation.
type Notification struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	Title         string  `json:"title"`
	Body          string  `json:"body"`
	ObservationID *string `json:"observation_id"`
	ReadAt        *string `json:"read_at"`
	CreatedAt     string  `json:"created_at"`
}

// Create inserts a notification for userID using ex (pass the surrounding transaction).
// observationID may be empty.
func Create(ctx context.Context, ex audit.Execer, userID, kind, title, body, observationID string) error {
	var obs any
	if observationID != "" {
		obs = observationID
	}
	_, err := ex.ExecContext(ctx, `INSERT INTO notifications(id, user_id, kind, title, body, observation_id, created_at)
		VALUES (?,?,?,?,?,?,?)`, storage.NewID(), userID, kind, title, body, obs, storage.Now())
	return err
}

// Service serves the notification endpoints.
type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }

// Routes registers the endpoints (paths include /api/v1).
func (s *Service) Routes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/notifications", auth.RequireFunc(s.handleList))
	mux.Handle("POST /api/v1/notifications/{id}/read", auth.RequireFunc(s.handleRead))
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	q := `SELECT id, kind, title, body, observation_id, read_at, created_at FROM notifications WHERE user_id = ?`
	if httpx.QueryBool(r, "unread") {
		q += ` AND read_at IS NULL`
	}
	q += ` ORDER BY created_at DESC, rowid DESC LIMIT 200`
	rows, err := s.db.QueryContext(r.Context(), q, u.ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Kind, &n.Title, &n.Body, &n.ObservationID, &n.ReadAt, &n.CreatedAt); err != nil {
			httpx.WriteError(w, err)
			return
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (s *Service) handleRead(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	res, err := s.db.ExecContext(r.Context(), `UPDATE notifications SET read_at = COALESCE(read_at, ?) WHERE id = ? AND user_id = ?`,
		storage.Now(), r.PathValue("id"), u.ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpx.WriteError(w, httpx.NotFound("notification not found"))
		return
	}
	httpx.NoContent(w)
}

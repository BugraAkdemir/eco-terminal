// Package review serves the expert review queue and applies reviewer decisions on top of the
// observation state machine.
package review

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/observations"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Service implements the review endpoints.
type Service struct {
	db  *sql.DB
	obs *observations.Service
}

func NewService(db *sql.DB, obs *observations.Service) *Service { return &Service{db: db, obs: obs} }

// Routes registers /reviews/* (reviewer or admin; paths include /api/v1).
func (s *Service) Routes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/reviews/queue", auth.RequireFunc(s.handleQueue, auth.RoleReviewer))
	mux.Handle("POST /api/v1/reviews/{observationId}/decision", auth.RequireFunc(s.handleDecision, auth.RoleReviewer))
}

// Decision is the request body (ReviewDecision).
type Decision struct {
	Decision          string `json:"decision"`
	Note              string `json:"note"`
	CorrectedCategory string `json:"corrected_category"`
	CorrectedSpecies  string `json:"corrected_species"`
	MarkSensitive     *bool  `json:"mark_sensitive"`
	Priority          string `json:"priority"`
}

// decisionTarget maps decisions to the status they lead to.
var decisionTarget = map[string]string{
	"start_review": "under_review",
	"verify":       "verified",
	"reject":       "rejected",
	"request_info": "needs_information",
	"resolve":      "resolved",
}

// defaultQueueStatuses are shown when the queue is requested without a status filter.
var defaultQueueStatuses = []string{"submitted", "ai_processed", "under_review"}

func (s *Service) handleQueue(w http.ResponseWriter, r *http.Request) {
	viewer, _ := auth.UserFrom(r.Context())
	f, err := observations.ParseFilter(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if len(f.Statuses) == 0 {
		f.Statuses = defaultQueueStatuses
	}
	f.QueueOrder = true
	page, err := httpx.ParsePage(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	items, total, err := s.obs.List(r.Context(), viewer, f, page)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out := make([]*observations.Detail, 0, len(items))
	for _, it := range items {
		d, err := s.obs.Detail(r.Context(), viewer, it)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		out = append(out, d)
	}
	httpx.WriteJSONMeta(w, http.StatusOK, out, page.Meta(total))
}

func (s *Service) handleDecision(w http.ResponseWriter, r *http.Request) {
	viewer, _ := auth.UserFrom(r.Context())
	var in Decision
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, err)
		return
	}
	d, err := s.Decide(r.Context(), viewer, auth.ActorFrom(r), r.PathValue("observationId"), in)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func bad(field, msg string) *httpx.Error {
	return httpx.Validation(msg, map[string]any{"field": field})
}

// Decide validates and applies a reviewer decision atomically: status transition (409 when illegal),
// optional corrections (category, verified species, sensitivity, priority), observation_reviews row,
// status history, audit log and the notification to the owner. Reviewers cannot decide on their own records.
func (s *Service) Decide(ctx context.Context, user *auth.User, actor audit.Actor, obsID string, in Decision) (*observations.Detail, error) {
	to, ok := decisionTarget[in.Decision]
	if !ok {
		return nil, bad("decision", "decision must be one of start_review, verify, reject, request_info, resolve")
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > 2000 {
		return nil, bad("note", "note is too long")
	}
	if (in.Decision == "reject" || in.Decision == "request_info") && note == "" {
		return nil, bad("note", "note is required for reject and request_info")
	}
	if in.CorrectedCategory != "" && !observations.IsCategory(in.CorrectedCategory) {
		return nil, bad("corrected_category", "invalid corrected_category")
	}
	species := strings.TrimSpace(in.CorrectedSpecies)
	if utf8.RuneCountInString(species) > observations.MaxSpecies {
		return nil, bad("corrected_species", "corrected_species is too long")
	}
	if in.Priority != "" && !observations.IsPriority(in.Priority) {
		return nil, bad("priority", "invalid priority")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var owner, category, status string
	err = tx.QueryRowContext(ctx, `SELECT user_id, category, status FROM observations WHERE id = ? AND deleted_at IS NULL`, obsID).Scan(&owner, &category, &status)
	if err == sql.ErrNoRows || (err == nil && status == "draft") {
		return nil, httpx.NotFound("observation not found")
	}
	if err != nil {
		return nil, err
	}
	if owner == user.ID {
		return nil, httpx.Forbidden("reviewers cannot decide on their own records")
	}
	if _, err := s.obs.Transition(ctx, tx, observations.TransitionRequest{
		ObservationID: obsID, To: to, Kind: observations.ActorReviewer, Actor: actor, Note: note}); err != nil {
		return nil, err
	}

	now := storage.Now()
	applyCorrections := in.Decision == "verify" || in.Decision == "resolve"
	var corrCat, corrSpecies any
	if applyCorrections {
		if in.CorrectedCategory != "" && in.CorrectedCategory != category {
			if _, err := tx.ExecContext(ctx, `UPDATE observations SET category = ?, updated_at = ? WHERE id = ?`, in.CorrectedCategory, now, obsID); err != nil {
				return nil, err
			}
			if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "observation.category_correction", EntityType: "observation", EntityID: obsID,
				Before: map[string]any{"category": category}, After: map[string]any{"category": in.CorrectedCategory}}); err != nil {
				return nil, err
			}
		}
		if in.CorrectedCategory != "" {
			corrCat = in.CorrectedCategory
		}
		if species != "" {
			// the reviewer-confirmed name is the only source of species_verified
			if _, err := tx.ExecContext(ctx, `UPDATE observations SET species_verified = ?, updated_at = ? WHERE id = ?`, species, now, obsID); err != nil {
				return nil, err
			}
			corrSpecies = species
			if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "observation.species_verified", EntityType: "observation", EntityID: obsID,
				After: map[string]any{"species_verified": species}}); err != nil {
				return nil, err
			}
		}
	}
	if in.Priority != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE observations SET priority = ?, updated_at = ? WHERE id = ?`, in.Priority, now, obsID); err != nil {
			return nil, err
		}
		if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "observation.priority_change", EntityType: "observation", EntityID: obsID,
			After: map[string]any{"priority": in.Priority}}); err != nil {
			return nil, err
		}
	}
	var markArg any
	if in.MarkSensitive != nil {
		markArg = *in.MarkSensitive
		if err := s.obs.SetSensitive(ctx, tx, obsID, *in.MarkSensitive, actor); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO observation_reviews(id, observation_id, reviewer_id, decision, corrected_category,
		corrected_species, mark_sensitive, note, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		storage.NewID(), obsID, user.ID, in.Decision, corrCat, corrSpecies, markArg, note, now, now); err != nil {
		return nil, err
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "review.decision", EntityType: "observation", EntityID: obsID,
		After: map[string]any{"decision": in.Decision, "status": to}}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.obs.GetDetail(ctx, user, obsID)
}

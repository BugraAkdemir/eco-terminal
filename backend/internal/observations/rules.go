package observations

import (
	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
)

// ActorKind says who causes a status transition.
type ActorKind string

const (
	ActorOwner    ActorKind = "owner"
	ActorReviewer ActorKind = "reviewer"
	ActorSystem   ActorKind = "system"
	ActorAI       ActorKind = "ai"
)

type edge struct {
	from, to string
	kind     ActorKind
}

// transitions is the state machine of ARCHITECTURE.md. The AI actor may only produce
// submitted -> ai_processed, never verification decisions.
var transitions = []edge{
	{"draft", "submitted", ActorOwner},
	{"submitted", "ai_processed", ActorSystem},
	{"submitted", "ai_processed", ActorAI},
	{"submitted", "under_review", ActorReviewer},
	{"ai_processed", "under_review", ActorReviewer},
	{"submitted", "verified", ActorReviewer},
	{"submitted", "rejected", ActorReviewer},
	{"submitted", "needs_information", ActorReviewer},
	{"ai_processed", "verified", ActorReviewer},
	{"ai_processed", "rejected", ActorReviewer},
	{"ai_processed", "needs_information", ActorReviewer},
	{"under_review", "verified", ActorReviewer},
	{"under_review", "rejected", ActorReviewer},
	{"under_review", "needs_information", ActorReviewer},
	{"needs_information", "under_review", ActorOwner},
	{"verified", "resolved", ActorReviewer},
}

// CanTransition reports whether `kind` may move a record from -> to.
func CanTransition(from, to string, kind ActorKind) bool {
	for _, e := range transitions {
		if e.from == from && e.to == to && e.kind == kind {
			return true
		}
	}
	return false
}

// OwnerEditable lists statuses in which the owner may edit the record or attach media.
func OwnerEditable(status string) bool {
	switch status {
	case "draft", "submitted", "ai_processed", "needs_information":
		return true
	}
	return false
}

// CanView implements the visibility rules for a single record:
// owners see all of their own records; reviewers/admins see everything except other users'
// drafts; everyone else (including anonymous callers) sees only verified/resolved records.
// Soft-deleted records are never visible (callers filter deleted_at beforehand).
func CanView(u *auth.User, ownerID, status string) bool {
	if u != nil && u.ID == ownerID {
		return true
	}
	if status == "verified" || status == "resolved" {
		return true
	}
	return auth.IsStaff(u) && status != "draft"
}

// Privileged reports whether u may see the exact location and internal details of a record:
// its owner, a reviewer or an admin.
func Privileged(u *auth.User, ownerID string) bool {
	return u != nil && (u.ID == ownerID || auth.IsStaff(u))
}

// visibilitySQL returns the WHERE fragment (alias o) restricting rows to those u may view.
func visibilitySQL(u *auth.User) (string, []any) {
	const base = `o.deleted_at IS NULL AND `
	switch {
	case u == nil:
		return base + `o.status IN ('verified','resolved')`, nil
	case auth.IsStaff(u):
		return base + `(o.status <> 'draft' OR o.user_id = ?)`, []any{u.ID}
	default:
		return base + `(o.status IN ('verified','resolved') OR o.user_id = ?)`, []any{u.ID}
	}
}

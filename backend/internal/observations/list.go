package observations

import (
	"context"
	"net/http"
	"strings"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/geographic"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
)

// Filter holds list criteria. The viewer's visibility rules are always applied on top.
type Filter struct {
	BBox       *geographic.BBox // matched against PUBLIC coordinates via observations_rtree
	RegionID   string
	Categories []string
	RecordType string
	Statuses   []string
	From, To   string // RFC3339 UTC, inclusive
	Mine       bool
	Q          string

	// Review-queue extras.
	Priority   string
	AIStatus   string
	QueueOrder bool // priority desc, oldest first
}

func csvEnum(name, v string, valid func(string) bool) ([]string, error) {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if !valid(p) {
			return nil, httpx.Validation("invalid "+name+": "+p, map[string]any{"field": name})
		}
		out = append(out, p)
	}
	return out, nil
}

// ParseFilter reads the common list query parameters (bbox, region_id, category, record_type, status,
// from, to, mine, q, priority, ai_status).
func ParseFilter(r *http.Request) (Filter, error) {
	q := r.URL.Query()
	var f Filter
	var err error
	if v := q.Get("bbox"); v != "" {
		b, err := geographic.ParseBBox(v)
		if err != nil {
			return f, httpx.Validation(err.Error(), map[string]any{"field": "bbox"})
		}
		f.BBox = &b
	}
	f.RegionID = q.Get("region_id")
	if f.Categories, err = csvEnum("category", q.Get("category"), IsCategory); err != nil {
		return f, err
	}
	if v := q.Get("record_type"); v != "" {
		if !inList(RecordTypes, v) {
			return f, httpx.Validation("invalid record_type", map[string]any{"field": "record_type"})
		}
		f.RecordType = v
	}
	if f.Statuses, err = csvEnum("status", q.Get("status"), IsStatus); err != nil {
		return f, err
	}
	if v := q.Get("from"); v != "" {
		if f.From, err = httpx.ParseTimeParam("from", v, false); err != nil {
			return f, err
		}
	}
	if v := q.Get("to"); v != "" {
		if f.To, err = httpx.ParseTimeParam("to", v, true); err != nil {
			return f, err
		}
	}
	f.Mine = httpx.QueryBool(r, "mine")
	f.Q = strings.TrimSpace(q.Get("q"))
	if len(f.Q) > 200 {
		return f, httpx.Validation("q too long", map[string]any{"field": "q"})
	}
	if v := q.Get("priority"); v != "" {
		if !IsPriority(v) {
			return f, httpx.Validation("invalid priority", map[string]any{"field": "priority"})
		}
		f.Priority = v
	}
	if v := q.Get("ai_status"); v != "" {
		switch v {
		case "pending", "running", "done", "failed", "skipped":
			f.AIStatus = v
		default:
			return f, httpx.Validation("invalid ai_status", map[string]any{"field": "ai_status"})
		}
	}
	return f, nil
}

func inPlaceholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// List returns the page of observations matching f that the viewer may see, plus the total count.
func (s *Service) List(ctx context.Context, viewer *auth.User, f Filter, page httpx.Page) ([]*Observation, int, error) {
	where, args := visibilitySQL(viewer)
	add := func(cond string, a ...any) {
		where += " AND " + cond
		args = append(args, a...)
	}
	if f.Mine {
		if viewer == nil {
			return nil, 0, httpx.Unauthorized("authentication required for mine=true")
		}
		add("o.user_id = ?", viewer.ID)
	}
	if f.BBox != nil {
		b := f.BBox
		add(`o.rowid IN (SELECT oid FROM observations_rtree WHERE min_lon >= ? AND max_lon <= ? AND min_lat >= ? AND max_lat <= ?)`,
			b[0], b[2], b[1], b[3])
	}
	if f.RegionID != "" {
		add(`EXISTS (SELECT 1 FROM observation_region_links l WHERE l.observation_id = o.id AND l.region_id = ?)`, f.RegionID)
	}
	if len(f.Categories) > 0 {
		add("o.category IN ("+inPlaceholders(len(f.Categories))+")", toAny(f.Categories)...)
	}
	if f.RecordType != "" {
		add("o.record_type = ?", f.RecordType)
	}
	if len(f.Statuses) > 0 {
		add("o.status IN ("+inPlaceholders(len(f.Statuses))+")", toAny(f.Statuses)...)
	}
	if f.From != "" {
		add("o.observed_at >= ?", f.From)
	}
	if f.To != "" {
		add("o.observed_at <= ?", f.To)
	}
	if f.Q != "" {
		like := "%" + escapeLike(f.Q) + "%"
		add(`(o.title LIKE ? ESCAPE '\' OR o.description LIKE ? ESCAPE '\' OR o.species_guess LIKE ? ESCAPE '\' OR o.species_verified LIKE ? ESCAPE '\')`,
			like, like, like, like)
	}
	if f.Priority != "" {
		add("o.priority = ?", f.Priority)
	}
	if f.AIStatus != "" {
		add(`EXISTS (SELECT 1 FROM observation_ai_analysis a WHERE a.observation_id = o.id AND a.status = ?)`, f.AIStatus)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM observations o WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := `o.observed_at DESC, o.created_at DESC, o.rowid DESC`
	if f.QueueOrder {
		order = `CASE o.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END, o.created_at ASC, o.rowid ASC`
	}
	rows, err := s.db.QueryContext(ctx, selectObs+` WHERE `+where+` ORDER BY `+order+` LIMIT ? OFFSET ?`, append(args, page.Size, page.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*Observation{}
	for rows.Next() {
		r, err := scanRaw(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, r.api(viewer))
	}
	return out, total, rows.Err()
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// GetDetail returns the ObservationDetail the viewer may see (404 when not visible).
func (s *Service) GetDetail(ctx context.Context, viewer *auth.User, id string) (*Detail, error) {
	o, err := s.Get(ctx, viewer, id)
	if err != nil {
		return nil, err
	}
	return s.Detail(ctx, viewer, o)
}

// Detail enriches an already visibility-checked observation with media, and (for the owner and
// staff) AI analyses, status history and owner name; reviewers/admins additionally get reviews.
func (s *Service) Detail(ctx context.Context, viewer *auth.User, o *Observation) (*Detail, error) {
	d := &Detail{Observation: *o, Media: []Media{}, AIAnalysis: []AIAnalysis{}, History: []HistoryEntry{}, Reviews: []Review{}}
	rows, err := s.db.QueryContext(ctx, `SELECT id, mime_type, COALESCE(width,0), COALESCE(height,0), size_bytes
		FROM observation_media WHERE observation_id = ? ORDER BY position, created_at`, o.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, mime string
		var w, h int
		var size int64
		if err := rows.Scan(&id, &mime, &w, &h, &size); err != nil {
			rows.Close()
			return nil, err
		}
		d.Media = append(d.Media, NewMedia(id, mime, w, h, size))
	}
	rows.Close()
	if !Privileged(viewer, o.UserID) {
		return d, nil
	}
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(display_name,'') FROM user_profiles WHERE user_id = ?`, o.UserID).Scan(&d.OwnerDisplayName)

	rows, err = s.db.QueryContext(ctx, `SELECT id, status, provider, model, suggested_category, suggested_species, suggested_description,
		confidence, error, created_at FROM observation_ai_analysis WHERE observation_id = ? ORDER BY created_at, rowid`, o.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		a := AIAnalysis{Disclaimer: AIDisclaimer}
		if err := rows.Scan(&a.ID, &a.Status, &a.Provider, &a.Model, &a.SuggestedCategory, &a.SuggestedSpecies,
			&a.SuggestedDescription, &a.Confidence, &a.Error, &a.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		d.AIAnalysis = append(d.AIAnalysis, a)
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `SELECT h.id, h.actor_id, h.actor_type, COALESCE(p.display_name,''), COALESCE(u.role,''),
		h.from_status, h.to_status, h.note, h.created_at
		FROM observation_status_history h LEFT JOIN users u ON u.id = h.actor_id LEFT JOIN user_profiles p ON p.user_id = h.actor_id
		WHERE h.observation_id = ? ORDER BY h.created_at, h.rowid`, o.ID)
	if err != nil {
		return nil, err
	}
	staff := auth.IsStaff(viewer)
	for rows.Next() {
		var h HistoryEntry
		var name, role string
		if err := rows.Scan(&h.ID, &h.ActorID, &h.ActorType, &name, &role, &h.FromStatus, &h.ToStatus, &h.Note, &h.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		switch {
		case h.ActorType == "system":
			h.ActorName = "Sistem"
		case h.ActorType == "ai":
			h.ActorName = "Yapay zekâ"
		case staff || (h.ActorID != nil && *h.ActorID == viewer.ID):
			h.ActorName = name
		case role == auth.RoleReviewer || role == auth.RoleAdmin:
			h.ActorName = "Uzman"
		default:
			h.ActorName = name
		}
		d.History = append(d.History, h)
	}
	rows.Close()

	if staff {
		rows, err = s.db.QueryContext(ctx, `SELECT id, reviewer_id, decision, corrected_category, corrected_species, note, created_at
			FROM observation_reviews WHERE observation_id = ? ORDER BY created_at, rowid`, o.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var rv Review
			if err := rows.Scan(&rv.ID, &rv.ReviewerID, &rv.Decision, &rv.CorrectedCategory, &rv.CorrectedSpecies, &rv.Note, &rv.CreatedAt); err != nil {
				rows.Close()
				return nil, err
			}
			d.Reviews = append(d.Reviews, rv)
		}
		rows.Close()
	}
	return d, nil
}

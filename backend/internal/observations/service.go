package observations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/geographic"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/notifications"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Service holds observation business logic.
type Service struct {
	db           *sql.DB
	geo          *geographic.Service
	aiConfigured bool
	log          *slog.Logger
	now          func() time.Time
}

// NewService creates the service. aiConfigured controls whether analyze_observation jobs are queued.
func NewService(db *sql.DB, geo *geographic.Service, aiConfigured bool, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: db, geo: geo, aiConfigured: aiConfigured, log: log, now: time.Now}
}

// DB exposes the handle for sibling packages (media, review) that share the transaction style.
func (s *Service) DB() *sql.DB { return s.db }

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type rowScanner interface{ Scan(dest ...any) error }

const selectObs = `SELECT o.id, o.reference_no, o.user_id, o.client_ref, o.record_type, o.category, o.title, o.description,
	o.species_guess, o.species_verified, o.count_observed, o.details_json, o.lat, o.lon, o.public_lat, o.public_lon,
	o.location_source, o.accuracy_m, o.observed_at, o.status, o.priority, o.is_sensitive, o.is_demo,
	o.primary_region_id, rg.name, o.created_at, o.updated_at,
	(SELECT m.id FROM observation_media m WHERE m.observation_id = o.id ORDER BY m.position, m.created_at LIMIT 1),
	(SELECT count(*) FROM observation_media m WHERE m.observation_id = o.id)
	FROM observations o LEFT JOIN geographic_regions rg ON rg.id = o.primary_region_id`

// raw is a scanned row including the exact coordinates.
type raw struct {
	Observation
	exactLat, exactLon float64
}

func scanRaw(rs rowScanner) (*raw, error) {
	var r raw
	var details string
	if err := rs.Scan(&r.ID, &r.ReferenceNo, &r.UserID, &r.ClientRef, &r.RecordType, &r.Category, &r.Title, &r.Description,
		&r.SpeciesGuess, &r.SpeciesVerified, &r.CountObserved, &details, &r.exactLat, &r.exactLon, &r.Lat, &r.Lon,
		&r.LocationSource, &r.AccuracyM, &r.ObservedAt, &r.Status, &r.Priority, &r.IsSensitive, &r.IsDemo,
		&r.PrimaryRegionID, &r.PrimaryRegionName, &r.CreatedAt, &r.UpdatedAt, &r.ThumbnailMediaID, &r.MediaCount); err != nil {
		return nil, err
	}
	r.Details = json.RawMessage(details)
	return &r, nil
}

// api returns the Observation as the viewer may see it.
func (r *raw) api(viewer *auth.User) *Observation {
	o := r.Observation
	o.LocationGeneralized = o.IsSensitive
	if Privileged(viewer, o.UserID) {
		o.ExactLocation = &LatLon{Lat: r.exactLat, Lon: r.exactLon}
	}
	return &o
}

func (s *Service) loadRaw(ctx context.Context, q queryer, id string) (*raw, error) {
	r, err := scanRaw(q.QueryRowContext(ctx, selectObs+` WHERE o.id = ? AND o.deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.NotFound("observation not found")
	}
	return r, err
}

// Access describes the ownership/status of a record, for sibling packages that must apply the
// same visibility rules (media).
type Access struct {
	ID      string
	OwnerID string
	Status  string
}

// AccessOf loads the Access of a non-deleted observation (404 if missing).
func (s *Service) AccessOf(ctx context.Context, q queryer, id string) (*Access, error) {
	var a Access
	err := q.QueryRowContext(ctx, `SELECT id, user_id, status FROM observations WHERE id = ? AND deleted_at IS NULL`, id).Scan(&a.ID, &a.OwnerID, &a.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.NotFound("observation not found")
	}
	return &a, err
}

// Get returns an observation as the viewer may see it (404 when not visible).
func (s *Service) Get(ctx context.Context, viewer *auth.User, id string) (*Observation, error) {
	r, err := s.loadRaw(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	if !CanView(viewer, r.UserID, r.Status) {
		return nil, httpx.NotFound("observation not found")
	}
	return r.api(viewer), nil
}

// ───────────── create ─────────────

// CreateOptions are server-side switches that are not part of the public request body.
type CreateOptions struct {
	Demo bool // mark the record is_demo=1 (demo seeder only)
}

func (s *Service) nextReferenceNo(ctx context.Context, tx *sql.Tx, year int) (string, error) {
	prefix := fmt.Sprintf("ET-%d-", year)
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(CAST(substr(reference_no, ?) AS INTEGER)), 0) + 1
		FROM observations WHERE reference_no LIKE ?`, len(prefix)+1, prefix+"%").Scan(&n); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%06d", prefix, n), nil
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// Create validates and stores a new record. created=false means the caller's client_ref already
// existed and the EXISTING record is returned unchanged (idempotent retry).
func (s *Service) Create(ctx context.Context, user *auth.User, actor audit.Actor, in CreateInput, opt CreateOptions) (obs *Observation, created bool, err error) {
	now := s.now().UTC()
	v, verr := in.validate(now)
	if verr != nil {
		return nil, false, verr
	}
	for attempt := 0; attempt < 2; attempt++ {
		if v.ClientRef != "" {
			if existing, err := s.findByClientRef(ctx, user, v.ClientRef); err != nil || existing != nil {
				return existing, false, err
			}
		}
		obs, err = s.insert(ctx, user, actor, v, opt, now)
		if isUnique(err) && v.ClientRef != "" { // lost a race with a concurrent identical request
			continue
		}
		return obs, err == nil, err
	}
	return nil, false, httpx.Conflict("could not create the record, please retry")
}

func (s *Service) findByClientRef(ctx context.Context, user *auth.User, ref string) (*Observation, error) {
	var id string
	var deleted sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, deleted_at FROM observations WHERE user_id = ? AND client_ref = ?`, user.ID, ref).Scan(&id, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if deleted.Valid {
		return nil, httpx.Conflict("client_ref was already used by a deleted record")
	}
	return s.Get(ctx, user, id)
}

func (s *Service) insert(ctx context.Context, user *auth.User, actor audit.Actor, v *validated, opt CreateOptions, now time.Time) (*Observation, error) {
	status := "submitted"
	if v.Draft {
		status = "draft"
	}
	id, ts := storage.NewID(), storage.Time(now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ref, err := s.nextReferenceNo(ctx, tx, now.Year())
	if err != nil {
		return nil, err
	}
	var clientRef any
	if v.ClientRef != "" {
		clientRef = v.ClientRef
	}
	lat, lon := *v.Lat, *v.Lon
	if _, err := tx.ExecContext(ctx, `INSERT INTO observations(id, reference_no, client_ref, user_id, record_type, category, title, description,
		species_guess, count_observed, details_json, lat, lon, public_lat, public_lon, location_source, accuracy_m, observed_at,
		status, priority, is_sensitive, is_demo, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?, 'normal', 0, ?, ?, ?)`,
		id, ref, clientRef, user.ID, v.RecordType, v.Category, v.Title, v.Description, v.SpeciesGuess, v.CountObserved, v.detailsJSON,
		lat, lon, lat, lon, v.LocationSource, v.AccuracyM, v.observedAt, status, opt.Demo, ts, ts); err != nil {
		return nil, err
	}
	primary, err := s.geo.LinkObservation(ctx, tx, id, lon, lat, lon, lat)
	if err != nil {
		return nil, err
	}
	if primary != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE observations SET primary_region_id = ? WHERE id = ?`, primary, id); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO observation_status_history(id, observation_id, actor_id, actor_type, from_status, to_status, note, created_at)
		VALUES (?,?,?, 'user', NULL, ?, '', ?)`, storage.NewID(), id, user.ID, status, ts); err != nil {
		return nil, err
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "observation.create", EntityType: "observation", EntityID: id,
		After: map[string]any{"reference_no": ref, "status": status, "category": v.Category, "record_type": v.RecordType, "demo": opt.Demo}}); err != nil {
		return nil, err
	}
	if status != "draft" {
		if err := EnqueueIndex(ctx, tx, id); err != nil {
			return nil, err
		}
		if s.aiConfigured {
			// delayed a little so photos uploaded right after creation are present when the worker runs
			if err := EnqueueAnalyze(ctx, tx, id, now.Add(20*time.Second)); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, user, id)
}

// ───────────── status transitions ─────────────

// TransitionRequest describes one state change.
type TransitionRequest struct {
	ObservationID string
	To            string
	Kind          ActorKind
	Actor         audit.Actor // ID empty for system/ai
	Note          string
}

var statusTitles = map[string]string{
	"under_review":      "incelemeye alındı",
	"verified":          "doğrulandı",
	"rejected":          "reddedildi",
	"needs_information": "için ek bilgi istendi",
	"resolved":          "çözüldü olarak işaretlendi",
}

// Transition applies a status change inside tx: it enforces the state machine (409 conflict on an
// illegal move), writes observation_status_history and the audit log, queues an index job and
// notifies the owner of reviewer-caused changes. It returns the previous status.
// The AI/worker code uses it with Kind ActorAI/ActorSystem (only submitted -> ai_processed is legal).
func (s *Service) Transition(ctx context.Context, tx *sql.Tx, req TransitionRequest) (string, error) {
	var from, owner, ref string
	err := tx.QueryRowContext(ctx, `SELECT status, user_id, reference_no FROM observations WHERE id = ? AND deleted_at IS NULL`, req.ObservationID).Scan(&from, &owner, &ref)
	if errors.Is(err, sql.ErrNoRows) {
		return "", httpx.NotFound("observation not found")
	}
	if err != nil {
		return "", err
	}
	if !CanTransition(from, req.To, req.Kind) {
		return from, httpx.Conflict(fmt.Sprintf("illegal status transition %s -> %s", from, req.To)).
			WithDetails(map[string]any{"from": from, "to": req.To})
	}
	now := storage.Now()
	if _, err := tx.ExecContext(ctx, `UPDATE observations SET status = ?, updated_at = ? WHERE id = ?`, req.To, now, req.ObservationID); err != nil {
		return from, err
	}
	actorType, actorID := "user", any(req.Actor.ID)
	switch req.Kind {
	case ActorSystem:
		actorType, actorID = "system", nil
	case ActorAI:
		actorType, actorID = "ai", nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO observation_status_history(id, observation_id, actor_id, actor_type, from_status, to_status, note, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, storage.NewID(), req.ObservationID, actorID, actorType, from, req.To, req.Note, now); err != nil {
		return from, err
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: req.Actor, Action: "observation.status_change", EntityType: "observation", EntityID: req.ObservationID,
		Before: map[string]any{"status": from}, After: map[string]any{"status": req.To, "note": req.Note, "actor_kind": string(req.Kind)}}); err != nil {
		return from, err
	}
	if err := EnqueueIndex(ctx, tx, req.ObservationID); err != nil {
		return from, err
	}
	if req.Kind == ActorReviewer && owner != req.Actor.ID {
		kind := notifications.KindObservationStatus
		if req.To == "needs_information" {
			kind = notifications.KindInfoRequested
		}
		title := fmt.Sprintf("%s %s", ref, statusTitles[req.To])
		if err := notifications.Create(ctx, tx, owner, kind, title, req.Note, req.ObservationID); err != nil {
			return from, err
		}
	}
	return from, nil
}

// SetSensitive flips the sensitivity flag inside tx, recomputing the public coordinates (RTree
// follows via trigger) and the region links. Used by review decisions.
func (s *Service) SetSensitive(ctx context.Context, tx *sql.Tx, obsID string, sensitive bool, actor audit.Actor) error {
	var lat, lon float64
	var old bool
	err := tx.QueryRowContext(ctx, `SELECT lat, lon, is_sensitive FROM observations WHERE id = ? AND deleted_at IS NULL`, obsID).Scan(&lat, &lon, &old)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.NotFound("observation not found")
	}
	if err != nil || old == sensitive {
		return err
	}
	plat, plon := PublicCoords(lat, lon, sensitive)
	primary, err := s.geo.LinkObservation(ctx, tx, obsID, lon, lat, plon, plat)
	if err != nil {
		return err
	}
	var primaryArg any
	if primary != "" {
		primaryArg = primary
	}
	if _, err := tx.ExecContext(ctx, `UPDATE observations SET is_sensitive = ?, public_lat = ?, public_lon = ?, primary_region_id = ?, updated_at = ? WHERE id = ?`,
		sensitive, plat, plon, primaryArg, storage.Now(), obsID); err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "observation.sensitivity_change", EntityType: "observation", EntityID: obsID,
		Before: map[string]any{"is_sensitive": old}, After: map[string]any{"is_sensitive": sensitive}}); err != nil {
		return err
	}
	return EnqueueIndex(ctx, tx, obsID)
}

// ───────────── patch / delete ─────────────

// loadOwned loads a record for an owner-only mutation: 404 when the caller cannot see it, 403 when
// it is visible but belongs to someone else.
func (s *Service) loadOwned(ctx context.Context, q queryer, user *auth.User, id string) (*raw, error) {
	r, err := s.loadRaw(ctx, q, id)
	if err != nil {
		return nil, err
	}
	if !CanView(user, r.UserID, r.Status) {
		return nil, httpx.NotFound("observation not found")
	}
	if r.UserID != user.ID {
		return nil, httpx.Forbidden("only the owner can change this record")
	}
	return r, nil
}

// Patch lets the owner edit a record (draft/submitted/ai_processed/needs_information) and/or
// submit it (draft -> submitted, needs_information -> under_review).
func (s *Service) Patch(ctx context.Context, user *auth.User, actor audit.Actor, id string, in PatchInput) (*Observation, error) {
	submit := in.Submit != nil && *in.Submit
	if in.Title == nil && in.Description == nil && in.SpeciesGuess == nil && in.CountObserved == nil &&
		len(in.Details) == 0 && in.Lat == nil && in.Lon == nil && !submit {
		return nil, httpx.Validation("no changes given", nil)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	cur, err := s.loadOwned(ctx, tx, user, id)
	if err != nil {
		return nil, err
	}
	if !OwnerEditable(cur.Status) {
		return nil, httpx.Conflict("record can no longer be edited in status " + cur.Status)
	}
	before, after := map[string]any{}, map[string]any{}
	title, desc, species := cur.Title, cur.Description, cur.SpeciesGuess
	count, details := cur.CountObserved, string(cur.Details)
	lat, lon := cur.exactLat, cur.exactLon
	var e *httpx.Error
	if in.Title != nil {
		if title, e = checkText("title", *in.Title, MaxTitle); e != nil {
			return nil, e
		}
		before["title"], after["title"] = cur.Title, title
	}
	if in.Description != nil {
		if desc, e = checkText("description", *in.Description, MaxDescription); e != nil {
			return nil, e
		}
		before["description"], after["description"] = cur.Description, desc
	}
	if in.SpeciesGuess != nil {
		if species, e = checkText("species_guess", *in.SpeciesGuess, MaxSpecies); e != nil {
			return nil, e
		}
		before["species_guess"], after["species_guess"] = cur.SpeciesGuess, species
	}
	if in.CountObserved != nil {
		if *in.CountObserved < 0 || *in.CountObserved > MaxCount {
			return nil, bad("count_observed", "count_observed must be between 0 and 1000000")
		}
		count = in.CountObserved
		before["count_observed"], after["count_observed"] = cur.CountObserved, *in.CountObserved
	}
	if len(in.Details) > 0 {
		if details, e = normalizeDetails(in.Details); e != nil {
			return nil, e
		}
		after["details"] = true
	}
	coordsChanged := false
	if in.Lat != nil || in.Lon != nil {
		if in.Lat != nil {
			lat = *in.Lat
		}
		if in.Lon != nil {
			lon = *in.Lon
		}
		if !validCoord(lat, lon) {
			return nil, bad("lat", "lat must be in [-90,90] and lon in [-180,180]")
		}
		coordsChanged = lat != cur.exactLat || lon != cur.exactLon
		if coordsChanged {
			before["location"], after["location"] = LatLon{cur.exactLat, cur.exactLon}, LatLon{lat, lon}
		}
	}
	plat, plon := PublicCoords(lat, lon, cur.IsSensitive)
	now := storage.Now()
	if _, err := tx.ExecContext(ctx, `UPDATE observations SET title=?, description=?, species_guess=?, count_observed=?, details_json=?,
		lat=?, lon=?, public_lat=?, public_lon=?, updated_at=? WHERE id=?`,
		title, desc, species, count, details, lat, lon, plat, plon, now, id); err != nil {
		return nil, err
	}
	if coordsChanged {
		primary, err := s.geo.LinkObservation(ctx, tx, id, lon, lat, plon, plat)
		if err != nil {
			return nil, err
		}
		var arg any
		if primary != "" {
			arg = primary
		}
		if _, err := tx.ExecContext(ctx, `UPDATE observations SET primary_region_id = ? WHERE id = ?`, arg, id); err != nil {
			return nil, err
		}
	}
	if len(after) > 0 {
		if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "observation.update", EntityType: "observation", EntityID: id,
			Before: before, After: after}); err != nil {
			return nil, err
		}
	}
	if submit {
		var to string
		switch cur.Status {
		case "draft":
			to = "submitted"
		case "needs_information":
			to = "under_review"
		default:
			return nil, httpx.Conflict("record in status " + cur.Status + " cannot be submitted")
		}
		if _, err := s.Transition(ctx, tx, TransitionRequest{ObservationID: id, To: to, Kind: ActorOwner, Actor: actor}); err != nil {
			return nil, err
		}
		if cur.Status == "draft" && s.aiConfigured {
			if err := EnqueueAnalyze(ctx, tx, id, s.now().Add(20*time.Second)); err != nil {
				return nil, err
			}
		}
	} else if cur.Status != "draft" {
		if err := EnqueueIndex(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(ctx, user, id)
}

// Delete soft-deletes the owner's record.
func (s *Service) Delete(ctx context.Context, user *auth.User, actor audit.Actor, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cur, err := s.loadOwned(ctx, tx, user, id)
	if err != nil {
		return err
	}
	now := storage.Now()
	if _, err := tx.ExecContext(ctx, `UPDATE observations SET deleted_at = ?, updated_at = ? WHERE id = ?`, now, now, id); err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "observation.delete", EntityType: "observation", EntityID: id,
		Before: map[string]any{"status": cur.Status, "reference_no": cur.ReferenceNo}}); err != nil {
		return err
	}
	if cur.Status != "draft" {
		if err := EnqueueIndex(ctx, tx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

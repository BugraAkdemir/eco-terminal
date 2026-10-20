// Package geographic owns regions (GeoJSON polygons), point-in-polygon lookups, the
// observation<->region links, region analytics and the Kastamonu boundary seed.
package geographic

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

//go:embed seed/kastamonu_regions.geojson
var kastamonuGeoJSON []byte

// KastamonuSource is the provenance string stored with every seeded region
// (details: docs/data_sources/regions.md).
const KastamonuSource = "geoBoundaries gbOpen TUR ADM1/ADM2 v. 2023-12-12 (OpenStreetMap contributors, ODbL 1.0), simplified ~40 m; retrieved 2026-10-20"

// Region is the API representation (Region schema).
type Region struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"`
	Name     string     `json:"name"`
	ParentID *string    `json:"parent_id"`
	BBox     [4]float64 `json:"bbox"`
	Centroid struct {
		Lon float64 `json:"lon"`
		Lat float64 `json:"lat"`
	} `json:"centroid"`
	Geometry json.RawMessage `json:"geometry"`
	Source   string          `json:"source"`
	IsDemo   bool            `json:"is_demo"`
}

// Querier is satisfied by *sql.DB and *sql.Tx.
type Querier interface {
	audit.Execer
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Service is the geographic service.
type Service struct {
	db    *sql.DB
	log   *slog.Logger
	cache sync.Map // region id -> *Geometry
}

// NewService creates the service.
func NewService(db *sql.DB, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: db, log: log}
}

// RegionRef is a lightweight region match.
type RegionRef struct {
	ID   string
	Kind string
	Name string
	Area float64 // bbox area in deg^2, used to order "smallest first"
}

func (s *Service) geometry(id, geojson string) (*Geometry, error) {
	if g, ok := s.cache.Load(id); ok {
		return g.(*Geometry), nil
	}
	g, err := ParseGeometry([]byte(geojson))
	if err != nil {
		return nil, fmt.Errorf("region %s: %w", id, err)
	}
	s.cache.Store(id, g)
	return g, nil
}

// Containing returns the regions whose polygon contains (lon,lat), smallest first. The RTree
// prunes candidates by bbox before the exact point-in-polygon test. Custom regions are included
// only when includeCustom is set.
func (s *Service) Containing(ctx context.Context, q Querier, lon, lat float64, includeCustom bool) ([]RegionRef, error) {
	rows, err := q.QueryContext(ctx, `SELECT r.id, r.kind, r.name, r.geojson, (r.max_lon-r.min_lon)*(r.max_lat-r.min_lat)
		FROM regions_rtree t JOIN geographic_regions r ON r.rowid = t.rid
		WHERE t.min_lon <= ? AND t.max_lon >= ? AND t.min_lat <= ? AND t.max_lat >= ?`, lon, lon, lat, lat)
	if err != nil {
		return nil, err
	}
	type cand struct {
		RegionRef
		geojson string
	}
	var cands []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.ID, &c.Kind, &c.Name, &c.geojson, &c.Area); err != nil {
			rows.Close()
			return nil, err
		}
		cands = append(cands, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	var out []RegionRef
	for _, c := range cands {
		if c.Kind == "custom" && !includeCustom {
			continue
		}
		g, err := s.geometry(c.ID, c.geojson)
		if err != nil {
			s.log.Warn("skip unparsable region", "err", err)
			continue
		}
		if g.Contains(lon, lat) {
			out = append(out, c.RegionRef)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Area != out[j].Area {
			return out[i].Area < out[j].Area
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// LinkObservation recomputes observation_region_links for an observation and returns the primary
// region id ("" if none). Official regions (province/district/village/grid) are matched against
// the exact point; custom, user-drawn regions only against the PUBLIC point, so a researcher
// cannot probe the exact position of a sensitive record with a tiny polygon.
func (s *Service) LinkObservation(ctx context.Context, q Querier, obsID string, lon, lat, pubLon, pubLat float64) (string, error) {
	if _, err := q.ExecContext(ctx, `DELETE FROM observation_region_links WHERE observation_id = ?`, obsID); err != nil {
		return "", err
	}
	official, err := s.Containing(ctx, q, lon, lat, false)
	if err != nil {
		return "", err
	}
	all, err := s.Containing(ctx, q, pubLon, pubLat, true)
	if err != nil {
		return "", err
	}
	ids := map[string]bool{}
	for _, r := range official {
		ids[r.ID] = true
	}
	for _, r := range all {
		if r.Kind == "custom" {
			ids[r.ID] = true
		}
	}
	for id := range ids {
		if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO observation_region_links(observation_id, region_id) VALUES (?,?)`, obsID, id); err != nil {
			return "", err
		}
	}
	if len(official) > 0 {
		return official[0].ID, nil
	}
	return "", nil
}

// ───────────── queries ─────────────

const regionCols = `id, kind, name, parent_id, min_lon, min_lat, max_lon, max_lat, centroid_lon, centroid_lat, source, is_demo`

func scanRegion(rs interface{ Scan(...any) error }, geo *string) (*Region, error) {
	var r Region
	dest := []any{&r.ID, &r.Kind, &r.Name, &r.ParentID, &r.BBox[0], &r.BBox[1], &r.BBox[2], &r.BBox[3],
		&r.Centroid.Lon, &r.Centroid.Lat, &r.Source, &r.IsDemo}
	if geo != nil {
		dest = append(dest, geo)
	}
	if err := rs.Scan(dest...); err != nil {
		return nil, err
	}
	return &r, nil
}

// List returns regions filtered by kind/parent/name. Geometry is included only when withGeometry.
func (s *Service) List(ctx context.Context, kind, parentID, q string, withGeometry bool) ([]*Region, error) {
	cols := regionCols
	if withGeometry {
		cols += ", geojson"
	}
	where, args := "1=1", []any{}
	if kind != "" {
		where += " AND kind = ?"
		args = append(args, kind)
	}
	if parentID != "" {
		where += " AND parent_id = ?"
		args = append(args, parentID)
	}
	if q != "" {
		where += ` AND name LIKE ? ESCAPE '\'`
		args = append(args, "%"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)+"%")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+cols+` FROM geographic_regions WHERE `+where+` ORDER BY
		CASE kind WHEN 'province' THEN 0 WHEN 'district' THEN 1 WHEN 'village' THEN 2 WHEN 'grid' THEN 3 ELSE 4 END, name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Region{}
	for rows.Next() {
		var geo string
		var r *Region
		if withGeometry {
			r, err = scanRegion(rows, &geo)
		} else {
			r, err = scanRegion(rows, nil)
		}
		if err != nil {
			return nil, err
		}
		if withGeometry {
			r.Geometry = json.RawMessage(geo)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get returns one region with its geometry.
func (s *Service) Get(ctx context.Context, id string) (*Region, error) {
	var geo string
	r, err := scanRegion(s.db.QueryRowContext(ctx, `SELECT `+regionCols+`, geojson FROM geographic_regions WHERE id = ?`, id), &geo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.NotFound("region not found")
	}
	if err != nil {
		return nil, err
	}
	r.Geometry = json.RawMessage(geo)
	return r, nil
}

// Exists reports whether a region id exists.
func (s *Service) Exists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM geographic_regions WHERE id = ?`, id).Scan(&n)
	return n > 0, err
}

// CreateCustom stores a researcher/admin-defined polygon and links the already existing
// observations (matched on public coordinates) to it.
func (s *Service) CreateCustom(ctx context.Context, name string, geometry json.RawMessage, actor audit.Actor) (*Region, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 120 {
		return nil, httpx.Validation("name is required (max 120 characters)", map[string]any{"field": "name"})
	}
	g, err := ParseGeometry(geometry)
	if err != nil {
		return nil, httpx.Validation("invalid geometry: "+err.Error(), map[string]any{"field": "geometry"})
	}
	compact, err := json.Marshal(json.RawMessage(geometry))
	if err != nil {
		return nil, httpx.Validation("invalid geometry", map[string]any{"field": "geometry"})
	}
	b := g.BBox()
	clon, clat := g.Centroid()
	id, now := storage.NewID(), storage.Now()
	var createdBy any
	if actor.ID != "" {
		createdBy = actor.ID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO geographic_regions(id, kind, name, parent_id, geojson, min_lon, max_lon, min_lat, max_lat,
		centroid_lon, centroid_lat, source, is_demo, created_by, created_at, updated_at)
		VALUES (?, 'custom', ?, NULL, ?, ?, ?, ?, ?, ?, ?, 'user-defined analysis region', 0, ?, ?, ?)`,
		id, name, string(compact), b[0], b[2], b[1], b[3], clon, clat, createdBy, now, now); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT o.id, o.public_lon, o.public_lat FROM observations o
		WHERE o.deleted_at IS NULL AND o.rowid IN (SELECT oid FROM observations_rtree
		 WHERE min_lon <= ? AND max_lon >= ? AND min_lat <= ? AND max_lat >= ?)`, b[2], b[0], b[3], b[1])
	if err != nil {
		return nil, err
	}
	var hits []string
	for rows.Next() {
		var oid string
		var lon, lat float64
		if err := rows.Scan(&oid, &lon, &lat); err != nil {
			rows.Close()
			return nil, err
		}
		if g.Contains(lon, lat) {
			hits = append(hits, oid)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, oid := range hits {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO observation_region_links(observation_id, region_id) VALUES (?,?)`, oid, id); err != nil {
			return nil, err
		}
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: actor, Action: "region.create", EntityType: "region", EntityID: id,
		After: map[string]any{"name": name, "kind": "custom", "linked_observations": len(hits)}}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.cache.Store(id, g)
	return s.Get(ctx, id)
}

// ───────────── seeding ─────────────

// SeedKastamonu inserts the province and its districts when geographic_regions is empty.
func (s *Service) SeedKastamonu(ctx context.Context) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM geographic_regions`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	var fc struct {
		Features []struct {
			Properties struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"properties"`
			Geometry json.RawMessage `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(kastamonuGeoJSON, &fc); err != nil {
		return fmt.Errorf("embedded region seed: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := storage.Now()
	var provinceID string
	for _, f := range fc.Features {
		g, err := ParseGeometry(f.Geometry)
		if err != nil {
			return fmt.Errorf("seed region %s: %w", f.Properties.Name, err)
		}
		b := g.BBox()
		clon, clat := g.Centroid()
		id := storage.NewID()
		var parent any
		if f.Properties.Kind == "province" {
			provinceID = id
		} else {
			if provinceID == "" {
				return errors.New("seed file must list the province first")
			}
			parent = provinceID
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO geographic_regions(id, kind, name, parent_id, geojson, min_lon, max_lon, min_lat, max_lat,
			centroid_lon, centroid_lat, source, is_demo, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,0,?,?)`,
			id, f.Properties.Kind, f.Properties.Name, parent, string(f.Geometry), b[0], b[2], b[1], b[3], clon, clat, KastamonuSource, now, now); err != nil {
			return err
		}
	}
	if err := audit.Log(ctx, tx, audit.Entry{Actor: audit.System, Action: "region.seed", EntityType: "region",
		After: map[string]any{"count": len(fc.Features), "source": KastamonuSource}}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.log.Info("seeded Kastamonu regions", "count", len(fc.Features))
	return nil
}

// ───────────── HTTP ─────────────

// Routes registers the region endpoints except /regions/{id}/observations, which is served by
// internal/observations (same list/visibility logic). Paths include /api/v1.
func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/regions", s.handleList)
	mux.Handle("POST /api/v1/regions", auth.RequireFunc(s.handleCreate, auth.RoleResearcher))
	mux.HandleFunc("GET /api/v1/regions/{id}", s.handleGet)
	mux.HandleFunc("GET /api/v1/regions/{id}/analytics", s.handleAnalytics)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := q.Get("kind")
	switch kind {
	case "", "province", "district", "village", "grid", "custom":
	default:
		httpx.WriteError(w, httpx.Validation("invalid kind", map[string]any{"field": "kind"}))
		return
	}
	withGeo := httpx.QueryBool(r, "geometry")
	if q.Get("lon") != "" || q.Get("lat") != "" { // point lookup
		lon, err1 := strconv.ParseFloat(q.Get("lon"), 64)
		lat, err2 := strconv.ParseFloat(q.Get("lat"), 64)
		if err1 != nil || err2 != nil || lon < -180 || lon > 180 || lat < -90 || lat > 90 {
			httpx.WriteError(w, httpx.Validation("lon and lat must be valid coordinates", nil))
			return
		}
		refs, err := s.Containing(r.Context(), s.db, lon, lat, true)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		out := []*Region{}
		for _, ref := range refs {
			if kind != "" && ref.Kind != kind {
				continue
			}
			var reg *Region
			if reg, err = s.Get(r.Context(), ref.ID); err != nil {
				httpx.WriteError(w, err)
				return
			}
			if !withGeo {
				reg.Geometry = nil
			}
			out = append(out, reg)
		}
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	}
	out, err := s.List(r.Context(), kind, q.Get("parent_id"), strings.TrimSpace(q.Get("q")), withGeo)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	reg, err := s.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, reg)
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string          `json:"name"`
		Geometry json.RawMessage `json:"geometry"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if len(in.Geometry) == 0 {
		httpx.WriteError(w, httpx.Validation("geometry is required", map[string]any{"field": "geometry"}))
		return
	}
	reg, err := s.CreateCustom(r.Context(), in.Name, in.Geometry, auth.ActorFrom(r))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, reg)
}

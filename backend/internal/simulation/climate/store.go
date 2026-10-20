package climate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/model"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

// Provenance strings written to environmental_datasets (see docs/data_sources/climate.md).
const (
	Source  = "Open-Meteo Historical Weather API (open-meteo.com) serving the ECMWF ERA5 reanalysis (Copernicus Climate Change Service, C3S)"
	License = "CC BY 4.0 (Open-Meteo API data; attribution required: 'Weather data by Open-Meteo.com'). " +
		"The free API is limited to non-commercial use. Contains modified Copernicus Climate Change Service information."
	Description = "Daily precipitation sum (mm) and daily mean 2 m air temperature (degC) of the ERA5 reanalysis grid cell nearest to the region centroid. " +
		"Reanalysis, not station observations."
	SpatialResolution  = "ERA5 reanalysis grid 0.25 deg (~25 km); one grid cell per region (nearest to the centroid), not a region average"
	TemporalResolution = "daily (UTC days); archive lags real time by about 5 days"
	DatasetURL         = "https://open-meteo.com/en/docs/historical-weather-api"
	// Attribution is the credit line required by the CC BY 4.0 licence of the data.
	Attribution = "Weather data by Open-Meteo.com (https://open-meteo.com/), CC BY 4.0. Contains modified Copernicus Climate Change Service information (ERA5)."
)

// ErrRegionNotFound is returned when a region id does not exist.
var ErrRegionNotFound = errors.New("climate: region not found")

// DatasetInfo mirrors an environmental_datasets row.
type DatasetInfo struct {
	ID, Name, Source, License, Description string
	SpatialResolution, TemporalResolution  string
	URL                                    string
	RetrievedAt                            string
}

// EnsureReport says what EnsureRegionData did.
type EnsureReport struct {
	RegionID        string
	RequestedFrom   time.Time
	RequestedTo     time.Time // after clipping to the provider delay
	AlreadyPresent  bool      // nothing had to be fetched
	FromSnapshot    int       // daily rows (per variable) imported from the snapshot
	FromAPI         int       // daily rows (per variable) imported from the provider
	FetchedRanges   [][2]string
	ProviderError   error // non-nil when part of the range could not be fetched (data may be partial)
	ClippedForDelay bool
}

// Store persists climate series in environmental_measurements and keeps the fetch ledger.
type Store struct {
	DB      *sql.DB
	Fetcher *Fetcher // nil => offline (database + snapshot only)
	// SnapshotPath optionally points at datasets/environmental/*.json. When set and the region's
	// centroid falls into the snapshot's ERA5 cell, missing days covered by the snapshot are imported
	// from it before the network is asked.
	SnapshotPath string
	Now          func() time.Time // default time.Now

	locks sync.Map // regionID -> *sync.Mutex
	snapM sync.Mutex
	snap  *Snapshot
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Store) lock(regionID string) func() {
	v, _ := s.locks.LoadOrStore(regionID, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

// EnsureDataset upserts the environmental_datasets row (provenance) and returns it.
func (s *Store) EnsureDataset(ctx context.Context) (DatasetInfo, error) {
	now := storage.Now()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO environmental_datasets
		(id, name, source, license, description, spatial_resolution, temporal_resolution, url, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET source=excluded.source, license=excluded.license,
			description=excluded.description, spatial_resolution=excluded.spatial_resolution,
			temporal_resolution=excluded.temporal_resolution, url=excluded.url, updated_at=excluded.updated_at`,
		storage.NewID(), DatasetName, Source, License, Description, SpatialResolution, TemporalResolution, DatasetURL, now, now)
	if err != nil {
		return DatasetInfo{}, err
	}
	return s.Dataset(ctx)
}

// Dataset reads the dataset row (sql.ErrNoRows if it was never created).
func (s *Store) Dataset(ctx context.Context) (DatasetInfo, error) {
	var d DatasetInfo
	var retrieved sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT id, name, source, license, description, spatial_resolution,
		temporal_resolution, url, retrieved_at FROM environmental_datasets WHERE name = ?`, DatasetName).
		Scan(&d.ID, &d.Name, &d.Source, &d.License, &d.Description, &d.SpatialResolution, &d.TemporalResolution, &d.URL, &retrieved)
	d.RetrievedAt = retrieved.String
	return d, err
}

func (s *Store) bumpRetrieved(ctx context.Context, t time.Time) error {
	ts := storage.Time(t)
	_, err := s.DB.ExecContext(ctx, `UPDATE environmental_datasets SET retrieved_at = ?, updated_at = ?
		WHERE name = ? AND (retrieved_at IS NULL OR retrieved_at < ?)`, ts, storage.Now(), DatasetName, ts)
	return err
}

// RegionCentroid returns the centroid (lat, lon) stored in geographic_regions.
func (s *Store) RegionCentroid(ctx context.Context, regionID string) (lat, lon float64, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT centroid_lat, centroid_lon FROM geographic_regions WHERE id = ?`, regionID).Scan(&lat, &lon)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrRegionNotFound
	}
	return
}

func (s *Store) snapshot() *Snapshot {
	if s.SnapshotPath == "" {
		return nil
	}
	s.snapM.Lock()
	defer s.snapM.Unlock()
	if s.snap == nil {
		sn, err := LoadSnapshot(s.SnapshotPath)
		if err != nil {
			return nil // an unreadable snapshot is simply not used; the network path still works
		}
		s.snap = sn
	}
	return s.snap
}

// EnsureRegionData makes sure environmental_measurements holds daily precipitation and mean
// temperature for the region over [from, to] (UTC dates, inclusive). Only ranges not yet recorded in
// the fetch ledger are retrieved: first from the offline snapshot (if it covers the region's ERA5
// cell), then from the provider in yearly chunks. It is idempotent and safe to call concurrently.
// Days newer than the provider's ~5 day delay are never requested.
//
// A provider failure does not lose what was already ingested: the report carries ProviderError and
// the function returns it as the error as well, so callers can still decide to work with partial data.
func (s *Store) EnsureRegionData(ctx context.Context, regionID string, from, to time.Time) (EnsureReport, error) {
	from, to = dayOf(from), dayOf(to)
	rep := EnsureReport{RegionID: regionID, RequestedFrom: from, RequestedTo: to}
	if to.Before(from) {
		return rep, fmt.Errorf("climate: to before from")
	}
	unlock := s.lock(regionID)
	defer unlock()

	lat, lon, err := s.RegionCentroid(ctx, regionID)
	if err != nil {
		return rep, err
	}
	ds, err := s.EnsureDataset(ctx)
	if err != nil {
		return rep, err
	}
	if limit := dayOf(s.now().Add(-ProviderDelay)); to.After(limit) {
		to = limit
		rep.ClippedForDelay = true
		rep.RequestedTo = to
	}
	if to.Before(from) {
		rep.AlreadyPresent = true
		return rep, nil
	}
	covered, err := s.ledger(ctx, ds.ID, regionID)
	if err != nil {
		return rep, err
	}
	missing := subtract([2]time.Time{from, to}, covered)
	if len(missing) == 0 {
		rep.AlreadyPresent = true
		return rep, nil
	}

	// 1) offline snapshot
	if sn := s.snapshot(); sn != nil && SameCell(lat, lon, sn.Grid.Latitude, sn.Grid.Longitude) {
		ser, err := sn.Series()
		if err == nil {
			var still [][2]time.Time
			for _, r := range missing {
				part := ser.Slice(r[0], r[1])
				if part == nil {
					still = append(still, r)
					continue
				}
				n, err := s.upsert(ctx, ds.ID, regionID, part, "snapshot")
				if err != nil {
					return rep, err
				}
				rep.FromSnapshot += n
				rep.FetchedRanges = append(rep.FetchedRanges, [2]string{part.Dates[0].Format(dateLayout), part.Dates[len(part.Dates)-1].Format(dateLayout)})
				still = append(still, subtract(r, [][2]time.Time{{part.Dates[0], part.Dates[len(part.Dates)-1]}})...)
			}
			if t := ser.RetrievedAt; !t.IsZero() {
				_ = s.bumpRetrieved(ctx, t)
			}
			missing = still
		}
	}

	// 2) provider, one calendar year per call so that progress survives a failure half-way
providerLoop:
	for _, r := range missing {
		if s.Fetcher == nil {
			rep.ProviderError = errors.New("climate: no provider configured and the snapshot does not cover the requested range")
			break
		}
		for y := r[0].Year(); y <= r[1].Year(); y++ {
			a, b := r[0], r[1]
			if jan := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC); jan.After(a) {
				a = jan
			}
			if dec := time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC); dec.Before(b) {
				b = dec
			}
			ser, err := s.Fetcher.FetchDaily(ctx, lat, lon, a, b)
			if err != nil {
				rep.ProviderError = err
				break providerLoop
			}
			n, err := s.upsert(ctx, ds.ID, regionID, ser, "api")
			if err != nil {
				return rep, err
			}
			rep.FromAPI += n
			rep.FetchedRanges = append(rep.FetchedRanges, [2]string{a.Format(dateLayout), b.Format(dateLayout)})
			_ = s.bumpRetrieved(ctx, ser.RetrievedAt)
		}
	}
	return rep, rep.ProviderError
}

// upsert writes a series (rows for non-missing values only) and records the range in the ledger,
// all in one transaction. It returns the number of days written.
func (s *Store) upsert(ctx context.Context, datasetID, regionID string, ser *Series, source string) (int, error) {
	if len(ser.Dates) == 0 {
		return 0, nil
	}
	res := fmt.Sprintf("ERA5 0.25deg grid cell (%.2fN, %.2fE)", ser.GridLat, ser.GridLon)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO environmental_measurements
		(dataset_id, region_id, ts, measurement_type, value, unit, spatial_resolution, quality_flag)
		VALUES (?,?,?,?,?,?,?, 'ok')
		ON CONFLICT(dataset_id, region_id, ts, measurement_type) DO UPDATE SET
			value=excluded.value, unit=excluded.unit, spatial_resolution=excluded.spatial_resolution, quality_flag='ok'`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for i, d := range ser.Dates {
		ts := d.Format(dateLayout)
		if !model.IsMissing(ser.Precip[i]) {
			if _, err := stmt.ExecContext(ctx, datasetID, regionID, ts, TypePrecip, ser.Precip[i], UnitPrecip, res); err != nil {
				return 0, err
			}
		}
		if !model.IsMissing(ser.Temp[i]) {
			if _, err := stmt.ExecContext(ctx, datasetID, regionID, ts, TypeTemp, ser.Temp[i], UnitTemp, res); err != nil {
				return 0, err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO climate_fetch_ranges (dataset_id, region_id, from_date, to_date, source, grid_lat, grid_lon, fetched_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(dataset_id, region_id, from_date, to_date) DO UPDATE SET fetched_at=excluded.fetched_at, source=excluded.source`,
		datasetID, regionID, ser.Dates[0].Format(dateLayout), ser.Dates[len(ser.Dates)-1].Format(dateLayout), source,
		ser.GridLat, ser.GridLon, storage.Now()); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(ser.Dates), nil
}

func (s *Store) ledger(ctx context.Context, datasetID, regionID string) ([][2]time.Time, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT from_date, to_date FROM climate_fetch_ranges
		WHERE dataset_id = ? AND region_id = ? ORDER BY from_date`, datasetID, regionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]time.Time
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		ta, err1 := time.Parse(dateLayout, a)
		tb, err2 := time.Parse(dateLayout, b)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, [2]time.Time{ta, tb})
	}
	return out, rows.Err()
}

// subtract returns the parts of r (inclusive day range) not covered by any of the ranges in covered.
func subtract(r [2]time.Time, covered [][2]time.Time) [][2]time.Time {
	cov := append([][2]time.Time(nil), covered...)
	sort.Slice(cov, func(i, j int) bool { return cov[i][0].Before(cov[j][0]) })
	cur := r[0]
	var out [][2]time.Time
	for _, c := range cov {
		if c[1].Before(cur) {
			continue
		}
		if c[0].After(r[1]) {
			break
		}
		if c[0].After(cur) {
			out = append(out, [2]time.Time{cur, c[0].AddDate(0, 0, -1)})
		}
		if next := c[1].AddDate(0, 0, 1); next.After(cur) {
			cur = next
		}
		if cur.After(r[1]) {
			return out
		}
	}
	if !cur.After(r[1]) {
		out = append(out, [2]time.Time{cur, r[1]})
	}
	return out
}

// ReadDaily returns the stored daily records of the region in [from, to] (only days that have at
// least one value), ordered by date. Missing variables are model.Missing.
func (s *Store) ReadDaily(ctx context.Context, regionID string, from, to time.Time) ([]model.DailyRecord, error) {
	ds, err := s.Dataset(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT ts, measurement_type, value FROM environmental_measurements
		WHERE dataset_id = ? AND region_id = ? AND ts >= ? AND ts <= ? AND measurement_type IN (?, ?)
		ORDER BY ts`, ds.ID, regionID, dayOf(from).Format(dateLayout), dayOf(to).Format(dateLayout), TypePrecip, TypeTemp)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DailyRecord
	for rows.Next() {
		var ts, typ string
		var v float64
		if err := rows.Scan(&ts, &typ, &v); err != nil {
			return nil, err
		}
		d, err := time.Parse(dateLayout, ts)
		if err != nil {
			continue
		}
		if n := len(out); n == 0 || !out[n-1].Date.Equal(d) {
			out = append(out, model.DailyRecord{Date: d, Precip: model.Missing, Temp: model.Missing})
		}
		switch typ {
		case TypePrecip:
			out[len(out)-1].Precip = v
		case TypeTemp:
			out[len(out)-1].Temp = v
		}
	}
	return out, rows.Err()
}

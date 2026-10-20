package climate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/model"
)

// SnapshotFormat identifies the file layout of datasets/environmental/*.json.
const SnapshotFormat = "eco-terminal.climate-snapshot/1"

// Snapshot is a one-time cached download of a daily series for one location, committed to the
// repository so demos work offline. It is real provider data with its provenance attached, never
// synthetic. Dense arrays start at StartDate; null = missing.
type Snapshot struct {
	Format      string   `json:"format"`
	Dataset     string   `json:"dataset"`
	Source      string   `json:"source"`
	License     string   `json:"license"`
	Attribution string   `json:"attribution"`
	Model       string   `json:"model"`
	APIURL      string   `json:"api_url"`
	RetrievedAt string   `json:"retrieved_at"` // RFC3339 UTC
	Request     SnapReq  `json:"request"`
	Grid        SnapGrid `json:"grid"`
	Notes       string   `json:"notes"`
	StartDate   string   `json:"start_date"`
	EndDate     string   `json:"end_date"`
	// Units: precipitation_mm, temperature_mean_c
	PrecipMM []*float64 `json:"precipitation_mm"`
	TempC    []*float64 `json:"temperature_mean_c"`
}

// SnapReq documents the request that produced the file.
type SnapReq struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Daily     string  `json:"daily"`
	Timezone  string  `json:"timezone"`
	Place     string  `json:"place"`
}

// SnapGrid is the provider grid cell that actually answered.
type SnapGrid struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	ElevationM float64 `json:"elevation_m"`
}

// SnapshotFromSeries packages a fetched series (which must have consecutive days).
func SnapshotFromSeries(s *Series, place string) (*Snapshot, error) {
	if len(s.Dates) == 0 {
		return nil, fmt.Errorf("empty series")
	}
	toPtr := func(xs []float64) []*float64 {
		out := make([]*float64, len(xs))
		for i, v := range xs {
			if !model.IsMissing(v) {
				v := v
				out[i] = &v
			}
		}
		return out
	}
	return &Snapshot{
		Format:  SnapshotFormat,
		Dataset: DatasetName,
		Source:  Source,
		License: License,
		Attribution: "Weather data by Open-Meteo.com (https://open-meteo.com/), licensed CC BY 4.0. " +
			"Contains modified Copernicus Climate Change Service information (ERA5 reanalysis).",
		Model:       s.Model,
		APIURL:      "https://archive-api.open-meteo.com/v1/archive",
		RetrievedAt: s.RetrievedAt.UTC().Format(time.RFC3339),
		Request:     SnapReq{Latitude: s.RequestLat, Longitude: s.RequestLon, Daily: "precipitation_sum,temperature_2m_mean", Timezone: "UTC", Place: place},
		Grid:        SnapGrid{Latitude: s.GridLat, Longitude: s.GridLon, ElevationM: s.ElevationM},
		Notes: "Daily precipitation sum (mm) and daily mean 2 m air temperature (degC) of the ERA5 grid point nearest to the request, " +
			"one value per UTC day from start_date. null = provider returned no value. Not station data; not for commercial use under the free API terms.",
		StartDate: s.Dates[0].Format(dateLayout),
		EndDate:   s.Dates[len(s.Dates)-1].Format(dateLayout),
		PrecipMM:  toPtr(s.Precip),
		TempC:     toPtr(s.Temp),
	}, nil
}

// WriteSnapshot writes the snapshot as indented-header JSON (arrays on single lines to keep it small).
func WriteSnapshot(path string, sn *Snapshot) error {
	b, err := json.Marshal(sn)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// LoadSnapshot reads and validates a snapshot file.
func LoadSnapshot(path string) (*Snapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sn Snapshot
	if err := json.Unmarshal(b, &sn); err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", path, err)
	}
	if err := sn.Validate(); err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", path, err)
	}
	return &sn, nil
}

// Validate checks internal consistency.
func (sn *Snapshot) Validate() error {
	if sn.Format != SnapshotFormat {
		return fmt.Errorf("unknown snapshot format %q", sn.Format)
	}
	start, err := time.Parse(dateLayout, sn.StartDate)
	if err != nil {
		return fmt.Errorf("start_date: %w", err)
	}
	end, err := time.Parse(dateLayout, sn.EndDate)
	if err != nil {
		return fmt.Errorf("end_date: %w", err)
	}
	n := int(end.Sub(start).Hours()/24) + 1
	if n <= 0 || len(sn.PrecipMM) != n || len(sn.TempC) != n {
		return fmt.Errorf("array lengths %d/%d do not match %d days", len(sn.PrecipMM), len(sn.TempC), n)
	}
	if sn.RetrievedAt == "" || sn.Source == "" || sn.License == "" {
		return fmt.Errorf("provenance (retrieved_at/source/license) missing")
	}
	return nil
}

// Series converts the snapshot back into a Series (for the shared upsert path).
func (sn *Snapshot) Series() (*Series, error) {
	if err := sn.Validate(); err != nil {
		return nil, err
	}
	start, _ := time.Parse(dateLayout, sn.StartDate)
	n := len(sn.PrecipMM)
	s := &Series{Model: sn.Model, RequestLat: sn.Request.Latitude, RequestLon: sn.Request.Longitude,
		GridLat: sn.Grid.Latitude, GridLon: sn.Grid.Longitude, ElevationM: sn.Grid.ElevationM,
		Dates: make([]time.Time, n), Precip: make([]float64, n), Temp: make([]float64, n)}
	if t, err := time.Parse(time.RFC3339, sn.RetrievedAt); err == nil {
		s.RetrievedAt = t
	}
	for i := 0; i < n; i++ {
		s.Dates[i] = start.AddDate(0, 0, i)
		s.Precip[i], s.Temp[i] = model.Missing, model.Missing
		if sn.PrecipMM[i] != nil {
			s.Precip[i] = *sn.PrecipMM[i]
		}
		if sn.TempC[i] != nil {
			s.Temp[i] = *sn.TempC[i]
		}
	}
	return s, nil
}

// Slice returns the sub-series [from, to] (clipped to what the snapshot covers); nil if disjoint.
func (s *Series) Slice(from, to time.Time) *Series {
	from, to = dayOf(from), dayOf(to)
	if len(s.Dates) == 0 {
		return nil
	}
	if first := s.Dates[0]; from.Before(first) {
		from = first
	}
	if last := s.Dates[len(s.Dates)-1]; to.After(last) {
		to = last
	}
	if to.Before(from) {
		return nil
	}
	i := int(from.Sub(s.Dates[0]).Hours() / 24)
	j := int(to.Sub(s.Dates[0]).Hours()/24) + 1
	out := *s
	out.Dates, out.Precip, out.Temp = s.Dates[i:j], s.Precip[i:j], s.Temp[i:j]
	return &out
}

// DefaultSnapshotFile is the committed Kastamonu snapshot (datasets/environmental).
const DefaultSnapshotFile = "kastamonu_era5_daily_1991_2025.json"

// ResolveSnapshotPath returns the snapshot file to use, or "" when none is found. An explicit path
// (ECO_CLIMATE_SNAPSHOT) wins if it exists; otherwise datasets/environmental/<DefaultSnapshotFile> is
// searched relative to the working directory (server started from backend/, from the repository root
// or from a sibling directory) and relative to the executable (bin/server inside backend/).
func ResolveSnapshotPath(explicit string) string {
	exists := func(p string) bool {
		st, err := os.Stat(p)
		return err == nil && !st.IsDir()
	}
	if explicit != "" {
		if exists(explicit) {
			return explicit
		}
		return ""
	}
	rel := filepath.Join("datasets", "environmental", DefaultSnapshotFile)
	var bases []string
	if wd, err := os.Getwd(); err == nil {
		for _, up := range []string{".", "..", "../.."} {
			bases = append(bases, filepath.Join(wd, up))
		}
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, up := range []string{".", "..", "../..", "../../.."} {
			bases = append(bases, filepath.Join(dir, up))
		}
	}
	for _, b := range bases {
		if p := filepath.Join(b, rel); exists(p) {
			return p
		}
	}
	return ""
}

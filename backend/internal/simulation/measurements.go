package simulation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/climate"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/model"
)

// MeasurementPoint is one value of GET /environment/measurements.
type MeasurementPoint struct {
	TS    string  `json:"ts"` // YYYY-MM-DD (daily) or YYYY-MM (monthly)
	Value float64 `json:"value"`
}

// MeasurementSeries is the payload of GET /environment/measurements (openapi) plus provenance.
type MeasurementSeries struct {
	Dataset    string             `json:"dataset"`
	Unit       string             `json:"unit"`
	Type       string             `json:"type"`
	Resolution string             `json:"resolution"`
	From       string             `json:"from"`
	To         string             `json:"to"`
	Coverage   float64            `json:"coverage"` // fraction of expected points present (0..1)
	Source     string             `json:"source"`
	License    string             `json:"license"`
	Points     []MeasurementPoint `json:"points"`
}

// MeasurementsSeries reads the STORED series of a region (it never calls the provider; use
// EnsureData for that). typ is "precipitation" or "temperature_mean"; resolution "daily" or
// "monthly" (default). from/to accept YYYY-MM-DD or YYYY-MM; empty means the full stored range.
//
// Monthly precipitation is the monthly total, monthly temperature the monthly mean; a month needs
// >= 90 % of its days (same rule as the model) and otherwise has no point. Coverage is
// points/expected points in the requested range (days for daily, months for monthly).
func (s *Service) MeasurementsSeries(ctx context.Context, regionID, typ, from, to, resolution string) (MeasurementSeries, error) {
	out := MeasurementSeries{Type: typ, Resolution: resolution, Points: []MeasurementPoint{}}
	var unit string
	switch typ {
	case climate.TypePrecip:
		unit = climate.UnitPrecip
	case climate.TypeTemp:
		unit = climate.UnitTemp
	default:
		return out, fmt.Errorf("%w: type must be precipitation or temperature_mean", ErrInvalid)
	}
	if resolution == "" {
		resolution = "monthly"
	}
	if resolution != "daily" && resolution != "monthly" {
		return out, fmt.Errorf("%w: resolution must be daily or monthly", ErrInvalid)
	}
	out.Resolution, out.Unit = resolution, unit

	if _, _, err := s.Climate.RegionCentroid(ctx, regionID); err != nil {
		if errors.Is(err, climate.ErrRegionNotFound) {
			return out, ErrNotFound
		}
		return out, err
	}
	ds, err := s.Climate.Dataset(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	out.Dataset, out.Source, out.License = ds.Name, ds.Source, ds.License
	if out.Dataset == "" {
		out.Dataset = climate.DatasetName
	}

	lo, hi, err := parseRange(from, to)
	if err != nil {
		return out, err
	}
	if lo.IsZero() {
		lo = time.Date(1940, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if hi.IsZero() {
		hi = time.Now().UTC()
	}
	if hi.Before(lo) {
		return out, fmt.Errorf("%w: to is before from", ErrInvalid)
	}
	recs, err := s.Climate.ReadDaily(ctx, regionID, lo, hi)
	if err != nil {
		return out, err
	}
	if len(recs) == 0 {
		// no stored data: report the requested range with zero coverage (nothing is invented)
		out.From, out.To = from, to
		return out, nil
	}
	// when the caller gave no bounds, shrink to what is stored
	if from == "" {
		lo = recs[0].Date
	}
	if to == "" {
		hi = recs[len(recs)-1].Date
	}
	out.From, out.To = lo.Format("2006-01-02"), hi.Format("2006-01-02")

	if resolution == "daily" {
		for _, r := range recs {
			v := r.Precip
			if typ == climate.TypeTemp {
				v = r.Temp
			}
			if !model.IsMissing(v) {
				out.Points = append(out.Points, MeasurementPoint{TS: r.Date.Format("2006-01-02"), Value: round3(v)})
			}
		}
		expected := int(hi.Sub(lo).Hours()/24) + 1
		out.Coverage = round3(float64(len(out.Points)) / float64(expected))
		return out, nil
	}

	fromYM := model.YM{Year: lo.Year(), Month: int(lo.Month())}
	toYM := model.YM{Year: hi.Year(), Month: int(hi.Month())}
	ms := model.MonthlyAggregate(recs, fromYM, toYM, model.MinDayCoverage)
	for _, m := range ms {
		v := m.Precip
		if typ == climate.TypeTemp {
			v = m.Temp
		}
		if !model.IsMissing(v) {
			out.Points = append(out.Points, MeasurementPoint{TS: m.YM.String(), Value: round3(v)})
		}
	}
	if len(ms) > 0 {
		out.Coverage = round3(float64(len(out.Points)) / float64(len(ms)))
	}
	return out, nil
}

func parseRange(from, to string) (lo, hi time.Time, err error) {
	if from != "" {
		lo, err = parseDate(from, false)
		if err != nil {
			return lo, hi, fmt.Errorf("%w: from: %v", ErrInvalid, err)
		}
	}
	if to != "" {
		hi, err = parseDate(to, true)
		if err != nil {
			return lo, hi, fmt.Errorf("%w: to: %v", ErrInvalid, err)
		}
	}
	return
}

// parseDate accepts YYYY-MM-DD or YYYY-MM (end=true gives the last day of that month).
func parseDate(s string, end bool) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	ym, err := model.ParseYM(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q (want YYYY-MM-DD or YYYY-MM)", s)
	}
	if end {
		return time.Date(ym.Year, time.Month(ym.Month), ym.DaysIn(), 0, 0, 0, 0, time.UTC), nil
	}
	return time.Date(ym.Year, time.Month(ym.Month), 1, 0, 0, 0, 0, time.UTC), nil
}

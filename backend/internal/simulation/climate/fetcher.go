// Package climate fetches daily climate series from the Open-Meteo Historical Weather API
// (ERA5 reanalysis, keyless) and stores them in environmental_measurements.
// Provenance, licence and restrictions: docs/data_sources/climate.md.
package climate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/simulation/model"
)

const (
	// DatasetName is the environmental_datasets.name of the ingested series.
	DatasetName = "open-meteo-era5-daily"
	// DefaultModel pins the Open-Meteo `models` parameter. With the default "best_match" the API
	// answers temperature from ERA5-Land and precipitation from ERA5 (verified 2026-10-20), i.e.
	// two different grids; "era5" gives both variables from the same 0.25 degree ERA5 grid.
	DefaultModel = "era5"
	// GridStepDeg is the ERA5 grid spacing used for "same cell" decisions.
	GridStepDeg = 0.25
	// ProviderDelay is the documented lag of the archive (about 5 days); newer days are not requested.
	ProviderDelay = 6 * 24 * time.Hour

	TypePrecip = "precipitation"
	TypeTemp   = "temperature_mean"
	UnitPrecip = "mm"
	UnitTemp   = "degC"

	dateLayout = "2006-01-02"
)

// Series is a daily series for one location as returned by the provider.
type Series struct {
	Model                  string
	RequestLat, RequestLon float64
	GridLat, GridLon       float64 // coordinates of the provider's grid cell
	ElevationM             float64
	Dates                  []time.Time // consecutive UTC days
	Precip, Temp           []float64   // len == len(Dates); model.Missing where the provider returned null
	RetrievedAt            time.Time
}

// APIError is a non-2xx answer of the provider.
type APIError struct {
	Status int
	Reason string
}

func (e *APIError) Error() string { return fmt.Sprintf("open-meteo: HTTP %d: %s", e.Status, e.Reason) }

// ErrQuota means a daily/hourly/monthly quota of the free API is used up (retrying is pointless).
var ErrQuota = errors.New("open-meteo: free API quota exhausted")

// Fetcher downloads daily series in yearly chunks, politely and with retry/backoff.
type Fetcher struct {
	BaseURL     string
	Model       string
	Client      *http.Client
	UserAgent   string
	MinInterval time.Duration // minimum gap between two HTTP requests (default 1 s)
	MaxRetries  int           // retries per chunk for 429/5xx/network errors (default 4)
	Backoff     time.Duration // base back-off for 5xx/network errors, doubled per attempt (default 2 s)
	Backoff429  time.Duration // base back-off for 429 without Retry-After (default 15 s)

	sleep func(ctx context.Context, d time.Duration) error // replaceable in tests
	mu    sync.Mutex
	last  time.Time
}

// NewFetcher builds a Fetcher from the application config (ECO_CLIMATE_BASE_URL / _TIMEOUT_SECONDS).
func NewFetcher(c config.Climate) *Fetcher {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	base := c.BaseURL
	if base == "" {
		base = "https://archive-api.open-meteo.com/v1/archive"
	}
	return &Fetcher{
		BaseURL: base, Model: DefaultModel, Client: &http.Client{Timeout: timeout},
		UserAgent: "EcoTerminal/1.0 (+climate ingestion; non-commercial research)",
	}
}

func (f *Fetcher) defaults() (minInt time.Duration, retries int, b, b429 time.Duration) {
	minInt, retries, b, b429 = f.MinInterval, f.MaxRetries, f.Backoff, f.Backoff429
	if minInt == 0 {
		minInt = time.Second
	}
	if retries == 0 {
		retries = 4
	}
	if b == 0 {
		b = 2 * time.Second
	}
	if b429 == 0 {
		b429 = 15 * time.Second
	}
	return
}

func (f *Fetcher) wait(ctx context.Context, d time.Duration) error {
	if f.sleep != nil {
		return f.sleep(ctx, d)
	}
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// throttle enforces MinInterval between requests (shared by concurrent callers).
func (f *Fetcher) throttle(ctx context.Context) error {
	minInt, _, _, _ := f.defaults()
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.last.IsZero() {
		if d := minInt - time.Since(f.last); d > 0 {
			if err := f.wait(ctx, d); err != nil {
				return err
			}
		}
	}
	f.last = time.Now()
	return nil
}

// FetchDaily returns the daily series for [from, to] (UTC dates, inclusive) at the grid cell nearest
// to lat/lon, requesting one calendar year per HTTP call.
func (f *Fetcher) FetchDaily(ctx context.Context, lat, lon float64, from, to time.Time) (*Series, error) {
	from, to = dayOf(from), dayOf(to)
	if to.Before(from) {
		return nil, fmt.Errorf("climate: to %s before from %s", to.Format(dateLayout), from.Format(dateLayout))
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil, fmt.Errorf("climate: invalid coordinates %v,%v", lat, lon)
	}
	out := &Series{Model: f.model(), RequestLat: lat, RequestLon: lon}
	first := true
	for y := from.Year(); y <= to.Year(); y++ {
		cf, ct := from, to
		if jan := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC); jan.After(cf) {
			cf = jan
		}
		if dec := time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC); dec.Before(ct) {
			ct = dec
		}
		chunk, err := f.fetchChunk(ctx, lat, lon, cf, ct)
		if err != nil {
			return nil, fmt.Errorf("fetch %s..%s: %w", cf.Format(dateLayout), ct.Format(dateLayout), err)
		}
		if first {
			out.GridLat, out.GridLon, out.ElevationM = chunk.GridLat, chunk.GridLon, chunk.ElevationM
			first = false
		} else if chunk.GridLat != out.GridLat || chunk.GridLon != out.GridLon {
			return nil, fmt.Errorf("provider changed grid cell between chunks (%v,%v -> %v,%v)", out.GridLat, out.GridLon, chunk.GridLat, chunk.GridLon)
		}
		out.Dates = append(out.Dates, chunk.Dates...)
		out.Precip = append(out.Precip, chunk.Precip...)
		out.Temp = append(out.Temp, chunk.Temp...)
	}
	out.RetrievedAt = time.Now().UTC()
	return out, nil
}

func (f *Fetcher) model() string {
	if f.Model == "" {
		return DefaultModel
	}
	return f.Model
}

func (f *Fetcher) fetchChunk(ctx context.Context, lat, lon float64, from, to time.Time) (*Series, error) {
	_, retries, b, b429 := f.defaults()
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(lat, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(lon, 'f', 4, 64))
	q.Set("start_date", from.Format(dateLayout))
	q.Set("end_date", to.Format(dateLayout))
	q.Set("daily", "precipitation_sum,temperature_2m_mean")
	q.Set("timezone", "UTC")
	q.Set("models", f.model())
	u := f.BaseURL + "?" + q.Encode()

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if err := f.throttle(ctx); err != nil {
			return nil, err
		}
		s, retryAfter, err := f.do(ctx, u, from, to)
		if err == nil {
			return s, nil
		}
		lastErr = err
		var ae *APIError
		retryable := true
		wait := b << attempt
		switch {
		case errors.Is(err, ErrQuota):
			return nil, err
		case errors.As(err, &ae):
			switch {
			case ae.Status == http.StatusTooManyRequests:
				wait = b429 << attempt
				if retryAfter > 0 {
					wait = retryAfter
				}
			case ae.Status >= 500:
			default:
				retryable = false // 400 & co: the request itself is wrong
			}
		case ctx.Err() != nil:
			return nil, ctx.Err()
		}
		if !retryable || attempt == retries {
			break
		}
		if err := f.wait(ctx, wait); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

type apiResponse struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Elevation  float64 `json:"elevation"`
	Error      bool    `json:"error"`
	Reason     string  `json:"reason"`
	DailyUnits struct {
		Precip string `json:"precipitation_sum"`
		Temp   string `json:"temperature_2m_mean"`
	} `json:"daily_units"`
	Daily struct {
		Time   []string   `json:"time"`
		Precip []*float64 `json:"precipitation_sum"`
		Temp   []*float64 `json:"temperature_2m_mean"`
	} `json:"daily"`
}

func (f *Fetcher) do(ctx context.Context, u string, from, to time.Time) (*Series, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("open-meteo request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, 0, fmt.Errorf("open-meteo read: %w", err)
	}
	var ar apiResponse
	jsonErr := json.Unmarshal(body, &ar)
	if resp.StatusCode != http.StatusOK || ar.Error {
		reason := strings.TrimSpace(ar.Reason)
		if reason == "" {
			reason = strings.TrimSpace(string(body))
			if len(reason) > 200 {
				reason = reason[:200]
			}
		}
		status := resp.StatusCode
		if status == http.StatusOK {
			status = http.StatusBadRequest
		}
		var ra time.Duration
		if v := resp.Header.Get("Retry-After"); v != "" {
			if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs > 0 && secs <= 300 {
				ra = time.Duration(secs) * time.Second
			}
		}
		if status == http.StatusTooManyRequests {
			lr := strings.ToLower(reason)
			if strings.Contains(lr, "daily") || strings.Contains(lr, "hourly") || strings.Contains(lr, "monthly") {
				return nil, 0, fmt.Errorf("%w: %s", ErrQuota, reason)
			}
		}
		return nil, ra, &APIError{Status: status, Reason: reason}
	}
	if jsonErr != nil {
		return nil, 0, fmt.Errorf("open-meteo: invalid JSON: %w", jsonErr)
	}
	return parseResponse(&ar, from, to)
}

func parseResponse(ar *apiResponse, from, to time.Time) (*Series, time.Duration, error) {
	n := int(to.Sub(from).Hours()/24) + 1
	d := ar.Daily
	if len(d.Time) != n || len(d.Precip) != n || len(d.Temp) != n {
		return nil, 0, fmt.Errorf("open-meteo: expected %d daily values, got time=%d precip=%d temp=%d", n, len(d.Time), len(d.Precip), len(d.Temp))
	}
	if ar.DailyUnits.Precip != "mm" || (ar.DailyUnits.Temp != "°C" && ar.DailyUnits.Temp != "degC") {
		return nil, 0, fmt.Errorf("open-meteo: unexpected units precip=%q temp=%q", ar.DailyUnits.Precip, ar.DailyUnits.Temp)
	}
	s := &Series{GridLat: ar.Latitude, GridLon: ar.Longitude, ElevationM: ar.Elevation,
		Dates: make([]time.Time, n), Precip: make([]float64, n), Temp: make([]float64, n)}
	for i := 0; i < n; i++ {
		want := from.AddDate(0, 0, i)
		got, err := time.Parse(dateLayout, d.Time[i])
		if err != nil || !got.Equal(want) {
			return nil, 0, fmt.Errorf("open-meteo: date %d is %q, want %s", i, d.Time[i], want.Format(dateLayout))
		}
		s.Dates[i] = want
		s.Precip[i], s.Temp[i] = model.Missing, model.Missing
		if v := d.Precip[i]; v != nil {
			if math.IsNaN(*v) || *v < 0 || *v > 1000 {
				return nil, 0, fmt.Errorf("open-meteo: implausible precipitation %v on %s", *v, d.Time[i])
			}
			s.Precip[i] = *v
		}
		if v := d.Temp[i]; v != nil {
			if math.IsNaN(*v) || *v < -90 || *v > 60 {
				return nil, 0, fmt.Errorf("open-meteo: implausible temperature %v on %s", *v, d.Time[i])
			}
			s.Temp[i] = *v
		}
	}
	return s, 0, nil
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// CellIndex returns the ERA5 0.25-degree grid cell (row, col) a coordinate falls into (the nearest
// grid point, which is what the provider answers with).
func CellIndex(lat, lon float64) (int, int) {
	return int(math.Round(lat / GridStepDeg)), int(math.Round(lon / GridStepDeg))
}

// SameCell reports whether two coordinates are served by the same ERA5 grid point.
func SameCell(lat1, lon1, lat2, lon2 float64) bool {
	a, b := CellIndex(lat1, lon1)
	c, d := CellIndex(lat2, lon2)
	return a == c && b == d
}

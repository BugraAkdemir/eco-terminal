// Package observations implements observation/issue records: validation, CRUD, the status state
// machine, visibility and sensitive-location rules, listing/filtering and the owner-facing
// endpoints. internal/review and internal/media build on its exported helpers.
package observations

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
)

// Enumerations (mirror the CHECK constraints of the schema).
var (
	Categories  = []string{"animal", "plant", "water", "forest", "pollution", "injured_wildlife", "other"}
	RecordTypes = []string{"observation", "issue"}
	Statuses    = []string{"draft", "submitted", "ai_processed", "under_review", "verified", "needs_information", "rejected", "resolved"}
	Priorities  = []string{"low", "normal", "high", "urgent"}
)

func inList(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// IsCategory / IsStatus / IsPriority validate enum values.
func IsCategory(v string) bool { return inList(Categories, v) }
func IsStatus(v string) bool   { return inList(Statuses, v) }
func IsPriority(v string) bool { return inList(Priorities, v) }

// Limits.
const (
	MaxTitle       = 200
	MaxDescription = 5000
	MaxSpecies     = 200
	MaxCount       = 1_000_000
	MaxDetails     = 8 << 10
	MaxPastAge     = 10 * 365 * 24 * time.Hour // offline captures may be old, but not older than ~10 years
	FutureSlack    = 5 * time.Minute
)

var clientRefRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// LatLon is a coordinate pair.
type LatLon struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Observation is the API representation (Observation schema). Lat/Lon are PUBLIC coordinates.
type Observation struct {
	ID                  string          `json:"id"`
	ReferenceNo         string          `json:"reference_no"`
	ClientRef           *string         `json:"client_ref"`
	UserID              string          `json:"user_id"`
	RecordType          string          `json:"record_type"`
	Category            string          `json:"category"`
	Title               string          `json:"title"`
	Description         string          `json:"description"`
	SpeciesGuess        string          `json:"species_guess"`
	SpeciesVerified     string          `json:"species_verified"`
	CountObserved       *int            `json:"count_observed"`
	Details             json.RawMessage `json:"details"`
	Lat                 float64         `json:"lat"`
	Lon                 float64         `json:"lon"`
	ExactLocation       *LatLon         `json:"exact_location"`
	LocationGeneralized bool            `json:"location_generalized"`
	LocationSource      string          `json:"location_source"`
	AccuracyM           *float64        `json:"accuracy_m"`
	ObservedAt          string          `json:"observed_at"`
	Status              string          `json:"status"`
	Priority            string          `json:"priority"`
	IsSensitive         bool            `json:"is_sensitive"`
	IsDemo              bool            `json:"is_demo"`
	PrimaryRegionID     *string         `json:"primary_region_id"`
	PrimaryRegionName   *string         `json:"primary_region_name"`
	ThumbnailMediaID    *string         `json:"thumbnail_media_id"`
	MediaCount          int             `json:"media_count"`
	CreatedAt           string          `json:"created_at"`
	UpdatedAt           string          `json:"updated_at"`
}

// Media is the API representation of an attached image.
type Media struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	ThumbURL  string `json:"thumb_url"`
	MimeType  string `json:"mime_type"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SizeBytes int64  `json:"size_bytes"`
}

// NewMedia builds a Media with its relative URLs.
func NewMedia(id, mime string, w, h int, size int64) Media {
	return Media{ID: id, URL: "/api/v1/media/" + id, ThumbURL: "/api/v1/media/" + id + "?thumb=1", MimeType: mime, Width: w, Height: h, SizeBytes: size}
}

// AIDisclaimer is attached to every AI analysis.
const AIDisclaimer = "Bu sonuç yapay zekâ tarafından üretilmiş doğrulanmamış bir öneridir; uzman onayı olmadan bilimsel kayıt sayılmaz."

type AIAnalysis struct {
	ID                   string   `json:"id"`
	Status               string   `json:"status"`
	Provider             string   `json:"provider"`
	Model                string   `json:"model"`
	SuggestedCategory    *string  `json:"suggested_category"`
	SuggestedSpecies     *string  `json:"suggested_species"`
	SuggestedDescription *string  `json:"suggested_description"`
	Confidence           *float64 `json:"confidence"`
	Disclaimer           string   `json:"disclaimer"`
	Error                *string  `json:"error"`
	CreatedAt            string   `json:"created_at"`
}

type HistoryEntry struct {
	ID         string  `json:"id"`
	ActorID    *string `json:"actor_id"`
	ActorType  string  `json:"actor_type"`
	ActorName  string  `json:"actor_name"`
	FromStatus *string `json:"from_status"`
	ToStatus   string  `json:"to_status"`
	Note       string  `json:"note"`
	CreatedAt  string  `json:"created_at"`
}

type Review struct {
	ID                string  `json:"id"`
	ReviewerID        string  `json:"reviewer_id"`
	Decision          string  `json:"decision"`
	CorrectedCategory *string `json:"corrected_category"`
	CorrectedSpecies  *string `json:"corrected_species"`
	Note              string  `json:"note"`
	CreatedAt         string  `json:"created_at"`
}

// Detail is the ObservationDetail schema.
type Detail struct {
	Observation
	Media            []Media        `json:"media"`
	AIAnalysis       []AIAnalysis   `json:"ai_analysis"`
	History          []HistoryEntry `json:"history"`
	Reviews          []Review       `json:"reviews"`
	OwnerDisplayName string         `json:"owner_display_name"`
}

// CreateInput is the body of POST /observations (ObservationCreate).
type CreateInput struct {
	RecordType     string          `json:"record_type"`
	Category       string          `json:"category"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	SpeciesGuess   string          `json:"species_guess"`
	CountObserved  *int            `json:"count_observed"`
	Details        json.RawMessage `json:"details"`
	Lat            *float64        `json:"lat"`
	Lon            *float64        `json:"lon"`
	LocationSource string          `json:"location_source"`
	AccuracyM      *float64        `json:"accuracy_m"`
	ObservedAt     string          `json:"observed_at"`
	Draft          bool            `json:"draft"`
	ClientRef      string          `json:"client_ref"`
}

// PatchInput is the body of PATCH /observations/{id}.
type PatchInput struct {
	Title         *string         `json:"title"`
	Description   *string         `json:"description"`
	SpeciesGuess  *string         `json:"species_guess"`
	CountObserved *int            `json:"count_observed"`
	Details       json.RawMessage `json:"details"`
	Lat           *float64        `json:"lat"`
	Lon           *float64        `json:"lon"`
	Submit        *bool           `json:"submit"`
}

func bad(field, msg string) *httpx.Error {
	return httpx.Validation(msg, map[string]any{"field": field})
}

func validCoord(lat, lon float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lon) && !math.IsInf(lat, 0) && !math.IsInf(lon, 0) &&
		lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

func checkText(field, v string, max int) (string, *httpx.Error) {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) > max {
		return "", bad(field, field+" is too long")
	}
	return v, nil
}

func normalizeDetails(raw json.RawMessage) (string, *httpx.Error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return "{}", nil
	}
	if len(s) > MaxDetails {
		return "", bad("details", "details is too large")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return "", bad("details", "details must be a JSON object")
	}
	out, _ := json.Marshal(m)
	return string(out), nil
}

// ParseObservedAt validates an observed_at timestamp: RFC3339, at most 5 minutes in the future
// and at most ~10 years in the past. It returns the RFC3339 UTC form.
func ParseObservedAt(s string, now time.Time) (string, *httpx.Error) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return "", bad("observed_at", "observed_at must be an RFC3339 timestamp")
	}
	if t.After(now.Add(FutureSlack)) {
		return "", bad("observed_at", "observed_at cannot be in the future")
	}
	if t.Before(now.Add(-MaxPastAge)) {
		return "", bad("observed_at", "observed_at is too far in the past")
	}
	return t.UTC().Format(time.RFC3339), nil
}

// validated holds a CreateInput after validation/normalisation.
type validated struct {
	CreateInput
	detailsJSON string
	observedAt  string
}

func (in CreateInput) validate(now time.Time) (*validated, *httpx.Error) {
	v := &validated{CreateInput: in}
	if !inList(RecordTypes, in.RecordType) {
		return nil, bad("record_type", "record_type must be 'observation' or 'issue'")
	}
	if !IsCategory(in.Category) {
		return nil, bad("category", "category must be one of "+strings.Join(Categories, ", "))
	}
	if in.Lat == nil || in.Lon == nil || !validCoord(*in.Lat, *in.Lon) {
		return nil, bad("lat", "lat must be in [-90,90] and lon in [-180,180]")
	}
	var e *httpx.Error
	if v.Title, e = checkText("title", in.Title, MaxTitle); e != nil {
		return nil, e
	}
	if v.Description, e = checkText("description", in.Description, MaxDescription); e != nil {
		return nil, e
	}
	if v.SpeciesGuess, e = checkText("species_guess", in.SpeciesGuess, MaxSpecies); e != nil {
		return nil, e
	}
	if in.CountObserved != nil && (*in.CountObserved < 0 || *in.CountObserved > MaxCount) {
		return nil, bad("count_observed", "count_observed must be between 0 and 1000000")
	}
	if v.detailsJSON, e = normalizeDetails(in.Details); e != nil {
		return nil, e
	}
	if v.LocationSource == "" {
		v.LocationSource = "gps"
	}
	if v.LocationSource != "gps" && v.LocationSource != "manual" {
		return nil, bad("location_source", "location_source must be 'gps' or 'manual'")
	}
	if in.AccuracyM != nil && (math.IsNaN(*in.AccuracyM) || math.IsInf(*in.AccuracyM, 0) || *in.AccuracyM < 0 || *in.AccuracyM > 1e6) {
		return nil, bad("accuracy_m", "accuracy_m must be a non-negative number")
	}
	if v.observedAt, e = ParseObservedAt(in.ObservedAt, now); e != nil {
		return nil, e
	}
	if in.ClientRef != "" && !clientRefRe.MatchString(in.ClientRef) {
		return nil, bad("client_ref", "client_ref must be 8-64 characters of A-Z a-z 0-9 _ -")
	}
	return v, nil
}

// PublicCoords returns the coordinates exposed to non-privileged callers: exact unless the
// record is sensitive, in which case they are rounded to 0.1 degree (~11 km).
func PublicCoords(lat, lon float64, sensitive bool) (float64, float64) {
	if !sensitive {
		return lat, lon
	}
	r := func(v float64) float64 {
		x := math.Round(v*10) / 10
		if x == 0 {
			return 0 // avoid -0
		}
		return x
	}
	return r(lat), r(lon)
}

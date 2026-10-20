// Package config loads all runtime settings from environment variables (see /.env.example).
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr            string
	Env             string
	DBPath          string
	StorageDir      string
	MaxUploadMB     int
	CORSOrigins     []string
	SessionTTL      time.Duration
	RateLimitPerMin int
	AdminEmail      string
	AdminPassword   string
	AI              AI
	RSS             RSS
	Climate         Climate
	SeedDemo        bool
}

type AI struct {
	BaseURL        string
	APIKey         string
	TextModel      string
	VisionModel    string
	EmbeddingModel string
	EmbeddingDim   int
	Timeout        time.Duration
}

type RSS struct {
	Enabled         bool
	DefaultInterval time.Duration
	FetchTimeout    time.Duration
	UserAgent       string
}

type Climate struct {
	BaseURL string
	Timeout time.Duration
	// SnapshotPath: offline ERA5 snapshot (datasets/environmental/*.json). Empty => searched at
	// startup (see simulation/climate.ResolveSnapshotPath). Set ECO_CLIMATE_SNAPSHOT to override.
	SnapshotPath string
}

func (c Config) IsProduction() bool { return c.Env == "production" }

// Load reads the environment. It never fails: missing optional values fall back to safe defaults.
func Load() Config {
	return Config{
		Addr:            get("ECO_ADDR", ":8080"),
		Env:             get("ECO_ENV", "development"),
		DBPath:          get("ECO_DB_PATH", "./data/eco.db"),
		StorageDir:      get("ECO_STORAGE_DIR", "./data/media"),
		MaxUploadMB:     getInt("ECO_MAX_UPLOAD_MB", 10),
		CORSOrigins:     splitCSV(get("ECO_CORS_ORIGINS", "*")),
		SessionTTL:      time.Duration(getInt("ECO_SESSION_TTL_HOURS", 720)) * time.Hour,
		RateLimitPerMin: getInt("ECO_RATE_LIMIT_PER_MIN", 120),
		AdminEmail:      os.Getenv("ECO_ADMIN_EMAIL"),
		AdminPassword:   os.Getenv("ECO_ADMIN_PASSWORD"),
		AI: AI{
			BaseURL:        strings.TrimRight(os.Getenv("ECO_AI_BASE_URL"), "/"),
			APIKey:         os.Getenv("ECO_AI_API_KEY"),
			TextModel:      os.Getenv("ECO_AI_TEXT_MODEL"),
			VisionModel:    os.Getenv("ECO_AI_VISION_MODEL"),
			EmbeddingModel: os.Getenv("ECO_AI_EMBEDDING_MODEL"),
			EmbeddingDim:   getInt("ECO_AI_EMBEDDING_DIM", 0),
			Timeout:        time.Duration(getInt("ECO_AI_TIMEOUT_SECONDS", 60)) * time.Second,
		},
		RSS: RSS{
			Enabled:         getBool("ECO_RSS_ENABLED", true),
			DefaultInterval: time.Duration(getInt("ECO_RSS_DEFAULT_INTERVAL_MINUTES", 60)) * time.Minute,
			FetchTimeout:    time.Duration(getInt("ECO_RSS_FETCH_TIMEOUT_SECONDS", 20)) * time.Second,
			UserAgent:       get("ECO_RSS_USER_AGENT", "EcoTerminal/1.0"),
		},
		Climate: Climate{
			BaseURL:      get("ECO_CLIMATE_BASE_URL", "https://archive-api.open-meteo.com/v1/archive"),
			Timeout:      time.Duration(getInt("ECO_CLIMATE_TIMEOUT_SECONDS", 60)) * time.Second,
			SnapshotPath: strings.TrimSpace(os.Getenv("ECO_CLIMATE_SNAPSHOT")),
		},
		SeedDemo: getBool("ECO_SEED_DEMO", false),
	}
}

func get(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func getInt(k string, def int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getBool(k string, def bool) bool {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

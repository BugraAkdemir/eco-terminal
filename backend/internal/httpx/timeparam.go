package httpx

import "time"

// ParseTimeParam parses a query parameter that is either RFC3339 or YYYY-MM-DD and returns it
// as RFC3339 UTC (the format of every timestamp column, so string comparison is chronological).
// A date-only value means 00:00:00Z, or 23:59:59Z when endOfDay is set (inclusive upper bounds).
func ParseTimeParam(name, v string, endOfDay bool) (string, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		if endOfDay {
			t = t.Add(24*time.Hour - time.Second)
		}
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", Validation(name+" must be RFC3339 or YYYY-MM-DD", map[string]any{"field": name})
}

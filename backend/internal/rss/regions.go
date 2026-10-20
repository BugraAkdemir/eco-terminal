package rss

import (
	"context"
	"database/sql"
	"strings"
)

// LocationUnknownNote is stored in rss_articles.location_note when no region could be identified.
const LocationUnknownNote = "Konum belirlenemedi"

// RegionRef is a geographic region matched in an article.
type RegionRef struct {
	ID   string
	Name string
	Kind string
}

type regionEntry struct {
	RegionRef
	parentID string
	toks     []string
}

// RegionTagger finds Kastamonu district/village/province names in free text, diacritic- and case-insensitively
// ("TOSYA'DA", "Tosyada", "tosya" all match "Tosya"). It deliberately prefers precision over recall:
//   - district names that are also everyday words (Araç, Küre, Bozkurt, ...) and all village names only
//     count when "Kastamonu" also appears in the text;
//   - short names (< 5 letters) must match a whole token, longer names tolerate Turkish case suffixes.
//
// Articles that match nothing are NOT tagged (location_note = LocationUnknownNote).
type RegionTagger struct {
	entries []regionEntry
}

// ambiguousDistricts are district names that are common words elsewhere; they need the "kastamonu" context.
var ambiguousDistricts = map[string]bool{"arac": true, "kure": true, "bozkurt": true, "abana": true, "cide": true, "daday": false}

// LoadRegionTagger reads province/district/village names from geographic_regions.
func LoadRegionTagger(ctx context.Context, db *sql.DB) (*RegionTagger, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, kind, name, COALESCE(parent_id,'') FROM geographic_regions
		WHERE kind IN ('province','district','village')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	t := &RegionTagger{}
	for rows.Next() {
		var e regionEntry
		if err := rows.Scan(&e.ID, &e.Kind, &e.Name, &e.parentID); err != nil {
			return nil, err
		}
		e.toks = tokens(foldTR(e.Name))
		if len(e.toks) == 0 || len(strings.Join(e.toks, "")) < 3 {
			continue
		}
		t.entries = append(t.entries, e)
	}
	return t, rows.Err()
}

// Match returns the regions mentioned in text (most specific first is not guaranteed; callers pick as needed).
func (t *RegionTagger) Match(text string) []RegionRef {
	if t == nil || len(t.entries) == 0 {
		return nil
	}
	toks := tokens(foldTR(text))
	if len(toks) == 0 {
		return nil
	}
	hasKastamonu := false
	for _, tk := range toks {
		if strings.HasPrefix(tk, "kastamonu") {
			hasKastamonu = true
			break
		}
	}
	var out []RegionRef
	seen := map[string]bool{}
	for _, e := range t.entries {
		if seen[e.ID] {
			continue
		}
		joined := strings.Join(e.toks, "")
		needCtx := e.Kind == "village" || (e.Kind == "district" && ambiguousDistricts[joined])
		if needCtx && !hasKastamonu {
			continue
		}
		if containsPhrase(toks, e.toks) {
			seen[e.ID] = true
			out = append(out, e.RegionRef)
		}
	}
	return out
}

// containsPhrase finds phrase in toks; the last phrase token may carry a short Turkish suffix when long enough.
func containsPhrase(toks, phrase []string) bool {
	n := len(phrase)
	for i := 0; i+n <= len(toks); i++ {
		ok := true
		for j := 0; j < n; j++ {
			a, b := toks[i+j], phrase[j]
			if a == b {
				continue
			}
			if j == n-1 && len(b) >= 5 && strings.HasPrefix(a, b) && len(a)-len(b) <= 4 {
				continue
			}
			ok = false
			break
		}
		if ok {
			return true
		}
	}
	return false
}

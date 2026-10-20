package rag

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Fact is a number (or short list) computed by SQL - never by the LLM.
type Fact struct {
	Label     string `json:"label"`
	Value     string `json:"value"`
	QueryNote string `json:"query_note"`
}

// bi is a bilingual string.
type bi struct{ tr, en string }

func (b bi) get(english bool) string {
	if english {
		return b.en
	}
	return b.tr
}

var catLabel = map[string]bi{
	"animal": {"Hayvan", "Animal"}, "plant": {"Bitki", "Plant"}, "water": {"Su", "Water"}, "forest": {"Orman", "Forest"},
	"pollution": {"Kirlilik", "Pollution"}, "injured_wildlife": {"Yaralı yaban hayvanı", "Injured wildlife"}, "other": {"Diğer", "Other"},
}

// scope builds the FROM/WHERE for counting observations that the *public* may see (verified or resolved) in a
// region subtree and period. statuses overrides the status list (used for the staff-only "pending" fact).
func obsScope(regionID string, p Period, category string, statuses string) (cte string, cteArgs []any, where string, args []any) {
	conds := []string{"o.deleted_at IS NULL", "o.status IN (" + statuses + ")"}
	if regionID != "" {
		cte = `WITH RECURSIVE sub(id) AS (SELECT ? UNION ALL SELECT r.id FROM geographic_regions r JOIN sub ON r.parent_id = sub.id) `
		cteArgs = []any{regionID}
		conds = append(conds, `(o.primary_region_id IN (SELECT id FROM sub)
			OR EXISTS (SELECT 1 FROM observation_region_links l WHERE l.observation_id = o.id AND l.region_id IN (SELECT id FROM sub)))`)
	}
	if p.From != "" {
		conds = append(conds, "substr(o.observed_at,1,10) >= ?")
		args = append(args, p.From)
	}
	if p.To != "" {
		conds = append(conds, "substr(o.observed_at,1,10) <= ?")
		args = append(args, p.To)
	}
	if category != "" {
		conds = append(conds, "o.category = ?")
		args = append(args, category)
	}
	return cte, cteArgs, strings.Join(conds, " AND "), args
}

const publicStatuses = `'verified','resolved'`
const pendingStatuses = `'submitted','ai_processed','under_review','needs_information'`

// ComputeFacts answers aggregate questions with SQL over records visible to the public (verified/resolved).
// Staff additionally get the number of pending (unverified) records as a separate fact. The returned limitations
// explain scope, period and data gaps.
func (e *Engine) ComputeFacts(ctx context.Context, in Intent, regionID, regionName string, p Period, caller Caller) ([]Fact, []string, error) {
	en := in.English
	var facts []Fact
	var lims []string
	cte, cteArgs, where, args := obsScope(regionID, p, in.Category, publicStatuses)
	all := func(extra ...any) []any { return append(append(append([]any{}, cteArgs...), args...), extra...) }

	scopeNote := bi{"SQL: doğrulanmış (verified/resolved) gözlemler", "SQL: verified/resolved observations"}.get(en)
	if regionID != "" {
		scopeNote += bi{"; bölge=" + regionName + " (alt bölgeler dahil)", "; region=" + regionName + " (incl. sub-regions)"}.get(en)
	} else {
		scopeNote += bi{"; bölge filtresi yok (tüm veri)", "; no region filter (all data)"}.get(en)
	}
	if !p.IsZero() {
		scopeNote += fmt.Sprintf("; %s..%s", p.From, p.To)
	}
	if in.Category != "" {
		scopeNote += "; category=" + in.Category
	}

	var total, demo, sensitive, observers int
	if err := e.DB.QueryRowContext(ctx, cte+`SELECT count(*), COALESCE(sum(o.is_demo),0), COALESCE(sum(o.is_sensitive),0), count(DISTINCT o.user_id) FROM observations o WHERE `+where, all()...).
		Scan(&total, &demo, &sensitive, &observers); err != nil {
		return nil, nil, err
	}
	totalLabel := bi{"Doğrulanmış gözlem sayısı", "Verified observation count"}.get(en)
	if in.Category != "" {
		totalLabel += " (" + catLabel[in.Category].get(en) + ")"
	}
	facts = append(facts, Fact{Label: totalLabel, Value: fmt.Sprint(total), QueryNote: scopeNote})
	if total > 0 {
		facts = append(facts, Fact{Label: bi{"Farklı gözlemci sayısı", "Distinct observers"}.get(en), Value: fmt.Sprint(observers), QueryNote: scopeNote})
	}
	if demo > 0 {
		facts = append(facts, Fact{Label: bi{"Bunlardan demo (örnek) veri", "Of which demo (sample) data"}.get(en), Value: fmt.Sprint(demo), QueryNote: "is_demo=1"})
	}

	// by category (only when not already filtered)
	if in.Category == "" && total > 0 {
		rows, err := e.DB.QueryContext(ctx, cte+`SELECT o.category, count(*) FROM observations o WHERE `+where+` GROUP BY o.category ORDER BY count(*) DESC`, all()...)
		if err != nil {
			return nil, nil, err
		}
		var parts []string
		for rows.Next() {
			var c string
			var n int
			if err := rows.Scan(&c, &n); err != nil {
				rows.Close()
				return nil, nil, err
			}
			parts = append(parts, fmt.Sprintf("%s: %d", catLabel[c].get(en), n))
		}
		rows.Close()
		if len(parts) > 1 {
			facts = append(facts, Fact{Label: bi{"Kategoriye göre dağılım", "Breakdown by category"}.get(en), Value: strings.Join(parts, "; "), QueryNote: scopeNote})
		}
	}

	// species groups (sensitive species are never broken down)
	if total > 0 && (in.TopSpecies || len(in.Terms) > 0) {
		rows, err := e.DB.QueryContext(ctx, cte+`SELECT COALESCE(NULLIF(trim(o.species_verified),''), NULLIF(trim(o.species_guess),'')) AS sp, count(*)
			FROM observations o WHERE `+where+` AND o.is_sensitive = 0 GROUP BY sp HAVING sp IS NOT NULL ORDER BY count(*) DESC, sp`, all()...)
		if err != nil {
			return nil, nil, err
		}
		type grp struct {
			name string
			n    int
		}
		var groups []grp
		for rows.Next() {
			var g grp
			if err := rows.Scan(&g.name, &g.n); err != nil {
				rows.Close()
				return nil, nil, err
			}
			groups = append(groups, g)
		}
		rows.Close()
		speciesNote := scopeNote + bi{"; hassas türler hariç", "; sensitive species excluded"}.get(en)
		if in.TopSpecies && len(groups) > 0 {
			var parts []string
			for i, g := range groups {
				if i == 5 {
					break
				}
				parts = append(parts, fmt.Sprintf("%s (%d)", g.name, g.n))
			}
			facts = append(facts, Fact{Label: bi{"En çok kaydedilen türler", "Most recorded species"}.get(en), Value: strings.Join(parts, "; "), QueryNote: speciesNote})
		}
		for _, term := range in.Terms {
			sum, matched := 0, []string{}
			for _, g := range groups {
				if termMatches(term, g.name) {
					sum += g.n
					matched = append(matched, g.name)
				}
			}
			if len(matched) > 0 {
				facts = append(facts, Fact{Label: bi{"'" + term + "' ile eşleşen doğrulanmış kayıt", "Verified records matching '" + term + "'"}.get(en),
					Value: fmt.Sprint(sum), QueryNote: speciesNote + bi{"; eşleşen türler: ", "; matched species: "}.get(en) + strings.Join(matched, ", ")})
			}
		}
		if sensitive > 0 {
			lims = append(lims, bi{"Hassas türler, konum güvenliği için tür bazlı dökümlerde gösterilmez (toplam sayıya dahildir).", "Sensitive species are not broken down by species for location safety (they are included in totals)."}.get(en))
		}
	}

	// monthly series
	if in.Trend {
		rows, err := e.DB.QueryContext(ctx, cte+`SELECT substr(o.observed_at,1,7) m, count(*) FROM observations o WHERE `+where+` GROUP BY m ORDER BY m`, all()...)
		if err != nil {
			return nil, nil, err
		}
		counts := map[string]int{}
		var months []string
		for rows.Next() {
			var m string
			var n int
			if err := rows.Scan(&m, &n); err != nil {
				rows.Close()
				return nil, nil, err
			}
			counts[m] = n
			months = append(months, m)
		}
		rows.Close()
		if len(months) > 0 {
			sort.Strings(months)
			from, to := months[0], months[len(months)-1]
			if p.From != "" {
				from = p.From[:7]
			}
			if p.To != "" {
				to = p.To[:7]
			}
			seq := monthRange(from, to, 60)
			var parts, empty []string
			for _, m := range seq {
				parts = append(parts, fmt.Sprintf("%s: %d", m, counts[m]))
				if counts[m] == 0 {
					empty = append(empty, m)
				}
			}
			facts = append(facts, Fact{Label: bi{"Aylık doğrulanmış kayıt sayısı", "Verified records per month"}.get(en), Value: strings.Join(parts, "; "), QueryNote: scopeNote})
			if len(empty) > 0 {
				lims = append(lims, bi{"Doğrulanmış kaydı olmayan aylar (veri boşluğu): ", "Months without verified records (data gaps): "}.get(en)+strings.Join(empty, ", "))
			}
		}
		if p.IsZero() {
			lims = append(lims, bi{"Dönem belirtilmediği için yalnızca verisi olan aralık gösterildi.", "No period given; only the range with data is shown."}.get(en))
		}
	}

	// staff-only: pending records, reported separately
	if caller.IsStaff() {
		c2, ca2, w2, a2 := obsScope(regionID, p, in.Category, pendingStatuses)
		var pending int
		if err := e.DB.QueryRowContext(ctx, c2+`SELECT count(*) FROM observations o WHERE `+w2, append(append([]any{}, ca2...), a2...)...).Scan(&pending); err != nil {
			return nil, nil, err
		}
		facts = append(facts, Fact{Label: bi{"İnceleme bekleyen (doğrulanmamış) kayıt — yalnızca personel görür", "Pending (unverified) records — staff only"}.get(en),
			Value: fmt.Sprint(pending), QueryNote: "status IN (" + pendingStatuses + ")"})
	}

	// limitations
	lims = append(lims, bi{"Sayılar yalnızca doğrulanmış (verified/resolved) kayıtları içerir; doğrulanmamış kayıtlar sayıma dahil edilmedi.",
		"Counts include verified/resolved records only; unverified records are not counted."}.get(en))
	lims = append(lims, bi{"Gözlem yoğunluğu popülasyon değişimi anlamına gelmez; kayıt sayısı gözlemci sayısı ve gözlem çabasına bağlıdır.",
		"Observation density does not imply population change; record counts depend on observers and effort."}.get(en))
	if p.IsZero() {
		lims = append(lims, bi{"Dönem belirtilmedi: tüm zamanlardaki kayıtlar kullanıldı.", "No period given: all records were used."}.get(en))
	} else {
		lims = append(lims, bi{"Kullanılan dönem: " + p.From + " – " + p.To + " (" + p.Label + ").", "Period used: " + p.From + " – " + p.To + " (" + p.Label + ")."}.get(en))
	}
	if regionID == "" {
		lims = append(lims, bi{"Bölge seçilmedi: tüm veri kullanıldı.", "No region selected: all data was used."}.get(en))
	} else {
		lims = append(lims, bi{"Bölge: " + regionName + " (alt bölgeler dahil).", "Region: " + regionName + " (including sub-regions)."}.get(en))
	}
	if total == 0 {
		lims = append(lims, bi{"Seçilen bölge/dönem için doğrulanmış kayıt yok. Kayıt yokluğu, türün veya olayın yokluğu anlamına gelmez.",
			"There are no verified records for the selected region/period. No records does not mean absence."}.get(en))
	}
	for _, term := range in.Terms {
		found := false
		for _, f := range facts {
			if strings.Contains(f.Label, "'"+term+"'") {
				found = true
			}
		}
		if !found && (in.Aggregate || in.TopSpecies) && total > 0 {
			lims = append(lims, bi{"'" + term + "' ile eşleşen doğrulanmış tür adı bulunamadı; toplam sayı bu terim için filtrelenmedi.",
				"No verified species name matched '" + term + "'; the total is not filtered by this term."}.get(en))
		}
	}
	return facts, lims, nil
}

// termMatches compares a folded question term with a species name (Turkish suffix tolerant).
func termMatches(term, species string) bool {
	for _, tk := range tokenize(foldTR(species)) {
		if tk == term {
			return true
		}
		if len(tk) >= 3 && strings.HasPrefix(term, tk) && len(term)-len(tk) <= 5 {
			return true
		}
		if len(term) >= 4 && strings.HasPrefix(tk, term) {
			return true
		}
	}
	return false
}

// monthRange lists YYYY-MM from..to inclusive (capped).
func monthRange(from, to string, max int) []string {
	var out []string
	y, m := atoi(from[:4]), atoi(from[5:7])
	ty, tm := atoi(to[:4]), atoi(to[5:7])
	for (y < ty || (y == ty && m <= tm)) && len(out) < max {
		out = append(out, fmt.Sprintf("%04d-%02d", y, m))
		m++
		if m > 12 {
			m, y = 1, y+1
		}
	}
	return out
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

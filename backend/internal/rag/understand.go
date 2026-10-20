package rag

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Period is a closed date range (YYYY-MM-DD). Label describes how it was derived.
type Period struct {
	From, To string
	Label    string
}

// IsZero reports an unset period.
func (p Period) IsZero() bool { return p.From == "" && p.To == "" }

// Intent is the rule-based reading of a question (Turkish + English; no LLM required).
type Intent struct {
	Aggregate   bool     // asks for a count/total ("kaç", "how many", "toplam")
	Trend       bool     // asks about change over time -> monthly series
	TopSpecies  bool     // asks which species are most frequent
	NeedsRegion bool     // refers to "burada / bu bölgede / here"
	Period      Period   // zero when the question names no period
	Category    string   // observation category hint ("plant", "animal", ...)
	Terms       []string // leftover subject terms (candidate species names), folded
	English     bool
}

var (
	aggWords    = []string{"kac", "sayisi", "sayilari", "adet", "toplam", "how many", "number of", "count", "total", "kactir", "kac tane"}
	trendWords  = []string{"trend", "artis", "azalis", "azal", "degisim", "degis", "aylik", "ay ay", "yil yil", "monthly", "over time", "increase", "decrease", "declin"}
	topWords    = []string{"en cok", "en fazla", "most common", "most frequent", "most observed", "en sik", "hangi tur"}
	hereWords   = []string{"burada", "burasi", "buranin", "bu bolgede", "bu bolge", "bu alanda", "bu alan", "bu cevrede", "bu ilcede", "bu koyde", "here", "this area", "this region", "around here", "in this place"}
	numberWords = map[string]int{"bir": 1, "iki": 2, "uc": 3, "dort": 4, "bes": 5, "alti": 6, "yedi": 7, "sekiz": 8, "dokuz": 9, "on": 10, "onbes": 15, "yirmi": 20, "otuz": 30,
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10}
	categoryWords = map[string]string{
		"bitki": "plant", "bitkiler": "plant", "cicek": "plant", "agac": "plant", "plant": "plant", "plants": "plant", "flora": "plant",
		"hayvan": "animal", "hayvanlar": "animal", "kus": "animal", "kuslar": "animal", "memeli": "animal", "animal": "animal", "animals": "animal", "bird": "animal", "birds": "animal", "fauna": "animal",
		"orman": "forest", "forest": "forest", "kirlilik": "pollution", "pollution": "pollution", "atik": "pollution",
		"yarali": "injured_wildlife", "injured": "injured_wildlife",
	}
	rePeriodRel = regexp.MustCompile(`\b(?:son|last|past)\s+(\d+|[a-z]+)?\s*(yil|yildir|yilda|ay|ayda|hafta|haftada|gun|gunde|year|years|month|months|week|weeks|day|days)\b`)
	reYear      = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	reLastYear  = regexp.MustCompile(`\b(gecen yil|gecen sene|last year|previous year)\b`)
	reThisYear  = regexp.MustCompile(`\b(bu yil|bu sene|this year)\b`)
	reLastMonth = regexp.MustCompile(`\b(gecen ay|last month|previous month)\b`)
	reThisMonth = regexp.MustCompile(`\b(bu ay|this month)\b`)
)

func containsAny(text string, words []string) bool {
	padded := " " + text + " "
	for _, w := range words {
		if strings.Contains(padded, " "+w) { // word-start match, so "kac" matches "kactir" but not "okac"
			return true
		}
	}
	return false
}

// Understand reads a question. now anchors relative periods ("son iki yılda").
func Understand(message string, now time.Time) Intent {
	folded := foldTR(message)
	folded = strings.Join(tokenize(folded), " ")
	in := Intent{English: isEnglish(message)}
	in.Aggregate = containsAny(folded, aggWords)
	in.Trend = containsAny(folded, trendWords)
	in.TopSpecies = containsAny(folded, topWords)
	in.NeedsRegion = containsAny(folded, hereWords)
	in.Period = parsePeriod(folded, now)

	toks := tokenize(folded)
	periodTok := map[string]bool{}
	for _, w := range []string{"yil", "yilda", "yildir", "ay", "ayda", "hafta", "gun", "gunde", "year", "years", "month", "months", "week", "weeks", "day", "days", "son", "gecen", "last", "past"} {
		periodTok[w] = true
	}
	for _, tk := range toks {
		if c, ok := categoryWords[tk]; ok && in.Category == "" {
			in.Category = c
			continue
		}
		if stopwords[tk] || periodTok[tk] || numberWords[tk] > 0 || len(tk) < 3 || isDigits(tk) {
			continue
		}
		in.Terms = append(in.Terms, tk)
	}
	return in
}

func isDigits(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func dateStr(t time.Time) string { return t.UTC().Format("2006-01-02") }

func parsePeriod(folded string, now time.Time) Period {
	now = now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if reLastYear.MatchString(folded) {
		y := now.Year() - 1
		return Period{From: strconv.Itoa(y) + "-01-01", To: strconv.Itoa(y) + "-12-31", Label: "geçen yıl (" + strconv.Itoa(y) + ")"}
	}
	if reThisYear.MatchString(folded) {
		return Period{From: strconv.Itoa(now.Year()) + "-01-01", To: dateStr(today), Label: "bu yıl"}
	}
	if reLastMonth.MatchString(folded) {
		first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
		return Period{From: dateStr(first), To: dateStr(first.AddDate(0, 1, -1)), Label: "geçen ay"}
	}
	if reThisMonth.MatchString(folded) {
		return Period{From: dateStr(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)), To: dateStr(today), Label: "bu ay"}
	}
	if m := rePeriodRel.FindStringSubmatch(folded); m != nil {
		n := 1
		if m[1] != "" {
			if v, err := strconv.Atoi(m[1]); err == nil {
				n = v
			} else if v, ok := numberWords[m[1]]; ok {
				n = v
			} else {
				// "son <word> yıl" where word is not a number: not a relative period
				return Period{}
			}
		}
		if n <= 0 || n > 200 {
			return Period{}
		}
		unit := m[2]
		var from time.Time
		var label string
		switch {
		case strings.HasPrefix(unit, "yil") || strings.HasPrefix(unit, "year"):
			from, label = today.AddDate(-n, 0, 0), "son "+strconv.Itoa(n)+" yıl"
		case strings.HasPrefix(unit, "ay") || strings.HasPrefix(unit, "month"):
			from, label = today.AddDate(0, -n, 0), "son "+strconv.Itoa(n)+" ay"
		case strings.HasPrefix(unit, "hafta") || strings.HasPrefix(unit, "week"):
			from, label = today.AddDate(0, 0, -7*n), "son "+strconv.Itoa(n)+" hafta"
		default:
			from, label = today.AddDate(0, 0, -n), "son "+strconv.Itoa(n)+" gün"
		}
		return Period{From: dateStr(from), To: dateStr(today), Label: label}
	}
	if ys := reYear.FindAllString(folded, -1); len(ys) > 0 {
		sort.Strings(ys)
		lo, hi := ys[0], ys[len(ys)-1]
		to := hi + "-12-31"
		if hi == strconv.Itoa(now.Year()) {
			to = dateStr(today)
		}
		label := lo
		if lo != hi {
			label = lo + "–" + hi
		}
		return Period{From: lo + "-01-01", To: to, Label: label}
	}
	return Period{}
}

// ResolveRegionByName finds the most specific province/district/village named in the message
// (case/diacritic-insensitive, tolerating Turkish suffixes: "Tosya'da", "Tosyalı"). It returns "" when none.
// Villages and ambiguous names only count together with "kastamonu" in the text.
func (e *Engine) ResolveRegionByName(ctx context.Context, message string) (id, name string, err error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT id, name, kind FROM geographic_regions WHERE kind IN ('province','district','village')`)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	toks := tokenize(foldTR(message))
	hasKast := false
	for _, t := range toks {
		if strings.HasPrefix(t, "kastamonu") {
			hasKast = true
		}
	}
	rank := map[string]int{"village": 3, "district": 2, "province": 1}
	bestRank, bestLen := 0, 0
	for rows.Next() {
		var rid, rname, kind string
		if err := rows.Scan(&rid, &rname, &kind); err != nil {
			return "", "", err
		}
		pt := tokenize(foldTR(rname))
		if len(pt) == 0 || len(strings.Join(pt, "")) < 4 {
			continue
		}
		if kind == "village" && !hasKast {
			continue
		}
		if matchPhrase(toks, pt) {
			l := len(strings.Join(pt, ""))
			if rank[kind] > bestRank || (rank[kind] == bestRank && l > bestLen) {
				id, name, bestRank, bestLen = rid, rname, rank[kind], l
			}
		}
	}
	return id, name, rows.Err()
}

func matchPhrase(toks, phrase []string) bool {
	n := len(phrase)
	for i := 0; i+n <= len(toks); i++ {
		ok := true
		for j := 0; j < n; j++ {
			a, b := toks[i+j], phrase[j]
			if a == b || (j == n-1 && len(b) >= 5 && strings.HasPrefix(a, b) && len(a)-len(b) <= 4) {
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

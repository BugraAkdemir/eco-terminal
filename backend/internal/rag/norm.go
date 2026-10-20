package rag

import (
	"strings"
	"unicode"
)

// lowerTR lower-cases with Turkish rules (İ→i, I→ı) and keeps all other letters (diacritics preserved).
func lowerTR(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'İ':
			return 'i'
		case 'I':
			return 'ı'
		}
		return unicode.ToLower(r)
	}, s)
}

// foldTR lower-cases and strips Turkish diacritics (ç→c ğ→g ı→i ö→o ş→s ü→u). Used for rule matching on
// user questions, never for storage.
func foldTR(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'İ', 'I', 'ı', 'i':
			return 'i'
		case 'Ç', 'ç':
			return 'c'
		case 'Ğ', 'ğ':
			return 'g'
		case 'Ö', 'ö':
			return 'o'
		case 'Ş', 'ş':
			return 's'
		case 'Ü', 'ü', 'Û', 'û':
			return 'u'
		case 'Â', 'â':
			return 'a'
		case 'Î', 'î':
			return 'i'
		}
		return unicode.ToLower(r)
	}, s)
}

func tokenize(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// stopwords (folded) removed from retrieval queries and from "subject term" extraction.
var stopwords = toSet(
	// Turkish question / glue words
	"ne", "nedir", "neler", "nasil", "nerede", "neden", "hangi", "kim", "kac", "kactir", "kadar", "var", "yok", "mi", "mu",
	"bir", "bu", "su", "o", "ve", "ile", "ya", "da", "de", "icin", "gibi", "daha", "en", "cok", "az", "olan", "olarak",
	"oldu", "olur", "olmus", "yapildi", "yapilmis", "bulunuyor", "bulunan", "var", "mıdır", "midir", "burada", "burasi",
	"bolgede", "bolge", "alanda", "cevrede", "ilgili", "hakkinda", "bana", "bize", "soyler", "misin", "musun", "lutfen",
	"son", "gecen", "yil", "yilda", "yildir", "ay", "ayda", "gun", "gunde", "hafta", "haftada", "bugun", "toplam", "sayisi", "sayi", "adet",
	"tane", "gozlem", "gozlemler", "gozlemi", "kayit", "kayitlar", "kaydi", "bildirim", "rapor", "tur", "turu", "turler",
	"bir", "iki", "uc", "dort", "bes", "alti", "yedi", "sekiz", "dokuz", "on",
	// English
	"the", "a", "an", "is", "are", "was", "were", "of", "in", "on", "at", "to", "for", "and", "or", "how", "many", "much",
	"what", "which", "where", "when", "who", "why", "do", "does", "did", "there", "here", "this", "that", "these", "those",
	"me", "tell", "about", "last", "past", "year", "years", "month", "months", "day", "days", "week", "weeks", "total",
	"count", "number", "observation", "observations", "record", "records", "report", "reports", "species", "have", "has", "been",
	"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "with", "from", "by", "any", "all",
)

func toSet(ws ...string) map[string]bool {
	m := make(map[string]bool, len(ws))
	for _, w := range ws {
		m[w] = true
	}
	return m
}

// suffixes are common Turkish inflections (plural, case, possessive) stripped to produce a recall-oriented
// stem variant. Longest first. The stem is used as an additional FTS *prefix* term, never instead of the word.
var suffixes = []string{
	"larının", "lerinin", "larında", "lerinde", "ların", "lerin", "ların", "larda", "lerde", "lardan", "lerden",
	"ndan", "nden", "nın", "nin", "nun", "nün", "ları", "leri", "lar", "ler", "dan", "den", "tan", "ten",
	"da", "de", "ta", "te", "ın", "in", "un", "ün", "ya", "ye", "yı", "yi", "yu", "yü", "ı", "i", "u", "ü", "a", "e",
}

func stem(t string) string {
	for pass := 0; pass < 2; pass++ {
		changed := false
		for _, s := range suffixes {
			if len([]rune(t))-len([]rune(s)) >= 3 && strings.HasSuffix(t, s) {
				t = strings.TrimSuffix(t, s)
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	return t
}

func ftsQuote(s string) string { return `"` + strings.ReplaceAll(s, `"`, ``) + `"` }

// BuildFTSQuery turns a free-text question into an FTS5 MATCH expression:
// stopwords dropped, each remaining term expanded into an OR group of
//   - the term itself, plus its ı↔i variants (the unicode61 tokenizer folds İ/I→i but leaves dotless ı),
//   - a prefix term on the term's stem (handles Turkish inflection: "ayıların" → ayı*).
//
// Terms are OR-ed; BM25 ranks chunks matching more/rarer terms higher. Every term is double-quoted so user text
// can never inject FTS operators. Returns "" when nothing searchable remains.
func BuildFTSQuery(q string) string {
	seen := map[string]bool{}
	var groups []string
	for _, tk := range tokenize(lowerTR(q)) {
		if len([]rune(tk)) < 2 || stopwords[foldTR(tk)] {
			continue
		}
		if seen[tk] {
			continue
		}
		seen[tk] = true
		variants := []string{ftsQuote(tk)}
		if strings.ContainsRune(tk, 'ı') {
			variants = append(variants, ftsQuote(strings.ReplaceAll(tk, "ı", "i")))
		} else if strings.ContainsRune(tk, 'i') {
			variants = append(variants, ftsQuote(strings.ReplaceAll(tk, "i", "ı")))
		}
		if st := stem(tk); st != tk && len([]rune(st)) >= 3 {
			variants = append(variants, ftsQuote(st)+"*")
			if strings.ContainsRune(st, 'ı') {
				variants = append(variants, ftsQuote(strings.ReplaceAll(st, "ı", "i"))+"*")
			}
		}
		groups = append(groups, "("+strings.Join(variants, " OR ")+")")
		if len(groups) >= 12 {
			break
		}
	}
	return strings.Join(groups, " OR ")
}

// isEnglish guesses the question language: Turkish letters or Turkish function words win; otherwise English
// when common English function words appear. Default is Turkish (the product's main language).
func isEnglish(q string) bool {
	if strings.ContainsAny(q, "çğışöüÇĞİŞÖÜ") {
		return false
	}
	en, tr := 0, 0
	for _, tk := range tokenize(foldTR(q)) {
		switch tk {
		case "how", "many", "what", "which", "where", "when", "the", "is", "are", "there", "last", "years", "year", "observations", "species", "here", "of", "in", "total", "number", "tell", "about":
			en++
		case "kac", "ne", "nedir", "nasil", "hangi", "var", "mi", "son", "yil", "yilda", "gozlem", "bu", "ve", "burada", "icin", "ile", "tur", "sayisi":
			tr++
		}
	}
	return en > tr && en > 0
}

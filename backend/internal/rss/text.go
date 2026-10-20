package rss

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style|noscript|iframe|object|embed|template)\b.*?</(script|style|noscript|iframe|object|embed|template)\s*>`)
	reComment     = regexp.MustCompile(`(?s)<!--.*?-->`)
	reBlockClose  = regexp.MustCompile(`(?i)</?(p|div|br|li|ul|ol|tr|table|h[1-6]|blockquote|section|article|header|footer)\b[^>]*>`)
	reTag         = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpaces      = regexp.MustCompile(`[ \t\f\v]+`)
	reBlankLines  = regexp.MustCompile(`\n\s*\n+`)
	reEntity      = regexp.MustCompile(`&(#[0-9]+|#[xX][0-9a-fA-F]+|[a-zA-Z][a-zA-Z0-9]{1,8});`)
)

// boilerplateLabels are labels some CMSs (SharePoint publishing sites such as ogm.gov.tr) inject in front of
// field values inside the feed description.
var boilerplateLabels = []string{"Rollup Image:", "Page Content:", "Sayfa İçeriği:"}

// PlainText converts feed-provided HTML or text into sanitized plain text: scripts/styles/comments dropped,
// tags stripped, entities decoded (also double-escaped ones), zero-width/control characters removed, whitespace
// collapsed. The result is never interpreted as HTML again, so it is safe to store and to show verbatim.
func PlainText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Up to 3 passes handle &lt;p&gt; (escaped markup) and &amp;nbsp; (double escaping).
	for i := 0; i < 3; i++ {
		prev := s
		s = reComment.ReplaceAllString(s, " ")
		s = reScriptStyle.ReplaceAllString(s, " ")
		s = reBlockClose.ReplaceAllString(s, "\n")
		s = reTag.ReplaceAllString(s, " ")
		if reEntity.MatchString(s) {
			s = html.UnescapeString(s)
		}
		if s == prev || (!strings.ContainsAny(s, "<>") && !reEntity.MatchString(s)) {
			break
		}
	}
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\u00a0' || r == '\u2007' || r == '\u202f' || r == '\u3000':
			return ' '
		case r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\ufeff' || r == '\u2060' || r == '\u00ad':
			return -1
		case r == '\n':
			return r
		case unicode.IsControl(r):
			return ' '
		}
		return r
	}, s)
	for _, l := range boilerplateLabels {
		s = strings.ReplaceAll(s, l, " ")
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimSpace(reSpaces.ReplaceAllString(ln, " "))
	}
	s = strings.Join(lines, "\n")
	s = reBlankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// SingleLine is PlainText collapsed to one line (titles).
func SingleLine(s string) string {
	return strings.Join(strings.Fields(PlainText(s)), " ")
}

// Truncate shortens s to at most max runes on a word boundary, appending an ellipsis.
func Truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := max
	for cut > max*2/3 && !unicode.IsSpace(r[cut]) {
		cut--
	}
	if cut <= max*2/3 {
		cut = max
	}
	return strings.TrimSpace(string(r[:cut])) + "…"
}

// foldTR lower-cases with Turkish rules and removes diacritics (ç→c ğ→g ı→i İ→i ö→o ş→s ü→u),
// so "KASTAMONU", "Kastamonu" and "kastamonu'da" compare equal after tokenization.
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

// tokens splits folded text into alphanumeric tokens (apostrophes and punctuation separate tokens,
// so "Tosya'da" yields "tosya", "da").
func tokens(folded string) []string {
	return strings.FieldsFunc(folded, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

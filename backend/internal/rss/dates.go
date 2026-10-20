package rss

import (
	"regexp"
	"strings"
	"time"
)

var timeLayouts = []string{
	time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339Nano, time.RFC3339,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04 -0700",
	"Mon, 2 Jan 2006 15:04 MST",
	"Mon, 2 Jan 06 15:04:05 -0700",
	"Mon, 2 Jan 2006",
	"2 Jan 2006 15:04:05 -0700",
	"2 Jan 2006 15:04:05 MST",
	"2 Jan 2006 15:04 -0700",
	"2 Jan 2006 15:04 MST",
	"2 Jan 2006",
	"2006-01-02T15:04:05-0700",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"2006/01/02",
	"02.01.2006 15:04:05",
	"02.01.2006 15:04",
	"02.01.2006",
	"2.1.2006",
	"Monday, 02-Jan-06 15:04:05 MST",
	"Monday, January 2, 2006",
	"January 2, 2006",
	"Jan 2, 2006",
	"2 January 2006",
}

var trMonths = strings.NewReplacer(
	"Ocak", "Jan", "Şubat", "Feb", "Mart", "Mar", "Nisan", "Apr", "Mayıs", "May", "Haziran", "Jun",
	"Temmuz", "Jul", "Ağustos", "Aug", "Eylül", "Sep", "Ekim", "Oct", "Kasım", "Nov", "Aralık", "Dec",
	"Pazartesi", "Mon", "Salı", "Tue", "Çarşamba", "Wed", "Perşembe", "Thu", "Cuma", "Fri", "Cumartesi", "Sat", "Pazar", "Sun",
)

var zoneOffsets = map[string]string{
	"UT": "+0000", "UTC": "+0000", "GMT": "+0000", "Z": "+0000",
	"EST": "-0500", "EDT": "-0400", "CST": "-0600", "CDT": "-0500",
	"MST": "-0700", "MDT": "-0600", "PST": "-0800", "PDT": "-0700",
	"CET": "+0100", "CEST": "+0200", "EET": "+0200", "EEST": "+0300",
	"TRT": "+0300", "MSK": "+0300", "BST": "+0100",
}

var (
	reZoneTail = regexp.MustCompile(`\s([A-Z]{1,4})$`)
	reMultiWS  = regexp.MustCompile(`\s+`)
)

// ParseFeedTime parses the many date formats found in feeds (RFC 822/1123 with or without weekday or
// seconds, RFC 3339, ISO date-only such as csb.gov.tr's "2026-10-20", dotted Turkish dates, Turkish month
// names, common US zone abbreviations). Date-only values become midnight UTC. ok=false when unparseable.
func ParseFeedTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	s = reMultiWS.ReplaceAllString(trMonths.Replace(s), " ")
	// Replace a trailing zone abbreviation with a numeric offset so the result is correct, not fabricated.
	if m := reZoneTail.FindStringSubmatch(s); m != nil {
		if off, ok := zoneOffsets[m[1]]; ok {
			s = strings.TrimSuffix(s, m[0]) + " " + off
		}
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

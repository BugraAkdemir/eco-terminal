package rss

import (
	"os"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseCSBDateOnlyAndCDATAWhitespace(t *testing.T) {
	f, err := Parse(fixture(t, "csb.xml"), "https://csb.gov.tr/rss", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != "rss2" || len(f.Items) != 3 {
		t.Fatalf("format=%s items=%d", f.Format, len(f.Items))
	}
	it := f.Items[0]
	// link inside CDATA with surrounding whitespace/newlines must be trimmed
	if !strings.HasPrefix(it.Link, "https://csb.gov.tr/haberler/") || strings.ContainsAny(it.Link, " \n") {
		t.Fatalf("link = %q", it.Link)
	}
	if it.GUID != it.Link {
		t.Fatalf("guid %q", it.GUID)
	}
	// date-only pubDate "2026-10-20"
	if it.Published == nil || it.Published.Format("2006-01-02") != "2026-10-20" {
		t.Fatalf("published = %v", it.Published)
	}
	// &rsquo; &ouml; entities inside CDATA decoded to real characters
	if strings.Contains(it.Summary, "&") || !strings.Contains(it.Summary, "İstanbul’da") {
		t.Fatalf("summary = %q", it.Summary)
	}
}

func TestParseOGMBOMStylesheetHTMLInCDATA(t *testing.T) {
	f, err := Parse(fixture(t, "ogm_haber.xml"), "https://www.ogm.gov.tr/x", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d", len(f.Items))
	}
	it := f.Items[0]
	if strings.ContainsAny(it.Summary, "<>") || strings.Contains(it.Summary, "Rollup Image") || strings.Contains(it.Summary, "Page Content") {
		t.Fatalf("summary not sanitized: %q", it.Summary[:min(200, len(it.Summary))])
	}
	if strings.ContainsRune(it.Summary, '​') {
		t.Fatal("zero-width space survived")
	}
	if it.Published == nil || it.Published.Year() != 2026 {
		t.Fatalf("published = %v", it.Published)
	}
}

func TestParseWordPressWithCreatorAndCategory(t *testing.T) {
	f, err := Parse(fixture(t, "kastamonu_bel.xml"), "https://www.kastamonu.bel.tr/feed/", testNow)
	if err != nil {
		t.Fatal(err)
	}
	it := f.Items[0]
	if it.Author != "kenanerkan" || len(it.Categories) == 0 || it.Categories[0] != "Duyurular" {
		t.Fatalf("author=%q cats=%v", it.Author, it.Categories)
	}
	if it.GUID != "https://www.kastamonu.bel.tr/?p=14041" {
		t.Fatalf("guid = %q", it.GUID)
	}
}

func TestParseMalformedNASARootAttributes(t *testing.T) {
	// science.nasa.gov really emits `xmlns:apod="..."xmlns:media="..."` (no space).
	f, err := Parse(fixture(t, "nasa.xml"), "https://science.nasa.gov/feed/", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Items) == 0 || f.Items[0].Title == "" || f.Items[0].Link == "" {
		t.Fatalf("items = %+v", f.Items)
	}
}

func TestParseAtomUSGS(t *testing.T) {
	f, err := Parse(fixture(t, "usgs.xml"), "https://earthquake.usgs.gov/x", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != "atom" || len(f.Items) != 3 {
		t.Fatalf("format=%s items=%d", f.Format, len(f.Items))
	}
	if f.Items[0].Published == nil || f.Items[0].Link == "" || f.Items[0].GUID == "" {
		t.Fatalf("item = %+v", f.Items[0])
	}
}

func TestParseNatureEmptyDescriptionAndTubitak(t *testing.T) {
	f, err := Parse(fixture(t, "nature.xml"), "https://www.nature.com/subjects/biodiversity.rss", testNow)
	if err != nil || len(f.Items) != 3 || f.Items[0].Summary != "" || f.Items[0].Title == "" {
		t.Fatalf("err=%v f=%+v", err, f)
	}
	f, err = Parse(fixture(t, "tubitak.xml"), "https://bilimgenc.tubitak.gov.tr/rss.xml", testNow)
	if err != nil || len(f.Items) == 0 {
		t.Fatalf("err=%v", err)
	}
}

func TestParseRejectsHTMLAndGarbage(t *testing.T) {
	for _, doc := range []string{"<!DOCTYPE html><html><body>404</body></html>", "", "not xml at all", "{\"json\":true}"} {
		if _, err := Parse([]byte(doc), "https://x.example/", testNow); err == nil {
			t.Fatalf("expected error for %q", doc)
		}
	}
}

func TestParseRDFAndLegacyCharsetAndEntities(t *testing.T) {
	rdf := `<?xml version="1.0" encoding="ISO-8859-9"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/" xmlns:dc="http://purl.org/dc/elements/1.1/">
<channel><title>Orman Haberleri</title><link>http://example.org/</link></channel>
<item><title>` + "\xdd\xfeLEM" + ` &amp; &nbsp;Ormanc&#305;l&#305;k</title><link>/h/1</link><dc:date>2026-10-01T08:00:00+03:00</dc:date>
<description>&lt;p&gt;Merhaba&lt;/p&gt;&lt;script&gt;alert(1)&lt;/script&gt;</description></item></rdf:RDF>`
	f, err := Parse([]byte(rdf), "http://example.org/feed", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != "rss1" || len(f.Items) != 1 {
		t.Fatalf("%+v", f)
	}
	it := f.Items[0]
	if it.Title != "İşLEM & Ormancılık" {
		t.Fatalf("title = %q", it.Title)
	}
	if it.Link != "http://example.org/h/1" {
		t.Fatalf("relative link not resolved: %q", it.Link)
	}
	if it.Summary != "Merhaba" {
		t.Fatalf("summary = %q", it.Summary)
	}
	if it.Published == nil || it.Published.Hour() != 5 {
		t.Fatalf("published = %v", it.Published)
	}
}

func TestFutureDatesAreDropped(t *testing.T) {
	doc := `<rss version="2.0"><channel><title>t</title><item><title>a</title><link>https://x.example/a</link><pubDate>Mon, 01 Jan 2035 10:00:00 GMT</pubDate></item></channel></rss>`
	f, err := Parse([]byte(doc), "https://x.example/f", testNow)
	if err != nil || f.Items[0].Published != nil {
		t.Fatalf("err=%v item=%+v", err, f.Items[0])
	}
}

func TestUnsafeLinksRejected(t *testing.T) {
	doc := `<rss version="2.0"><channel><title>t</title><item><title>a</title><link>javascript:alert(1)</link><guid>g1</guid></item></channel></rss>`
	f, err := Parse([]byte(doc), "https://x.example/f", testNow)
	if err != nil || f.Items[0].Link != "" || f.Items[0].GUID != "g1" {
		t.Fatalf("err=%v item=%+v", err, f.Items[0])
	}
}

func TestParseFeedTimeFormats(t *testing.T) {
	cases := map[string]string{
		"Mon, 19 Oct 2026 13:17:02 GMT":     "2026-10-19T13:17:02Z",
		"20 Oct 2026 08:00:28 +0000":        "2026-10-20T08:00:28Z",
		"2026-10-20":                        "2026-10-20T00:00:00Z",
		"2026-10-20T10:31:39Z":              "2026-10-20T10:31:39Z",
		"2026-10-20T06:28:59.943Z":          "2026-10-20T06:28:59Z",
		"Tue, 20 Oct 2026 12:00:00 EST":     "2026-10-20T17:00:00Z",
		"Tue, 20 Oct 2026 12:00:00 +0300":   "2026-10-20T09:00:00Z",
		"20.10.2026 14:30":                  "2026-10-20T14:30:00Z",
		"20 Ekim 2026":                      "2026-10-20T00:00:00Z",
		"Salı, 20 Ekim 2026 09:00:00 +0300": "2026-10-20T06:00:00Z",
	}
	for in, want := range cases {
		got, ok := ParseFeedTime(in)
		if !ok || got.Format(time.RFC3339) != want {
			t.Errorf("%q -> %v ok=%v, want %s", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "yesterday", "32 Foo 2026"} {
		if _, ok := ParseFeedTime(bad); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func TestPlainTextSanitizer(t *testing.T) {
	cases := map[string]string{
		`<p>Merhaba <b>dünya</b></p><p>İkinci</p>`: "Merhaba dünya\n\nİkinci",
		`a<script>alert(1)</script>b`:              "a b",
		`<style>.x{}</style>metin<!-- gizli -->`:   "metin",
		`&lt;p&gt;kaçak&lt;/p&gt;`:                 "kaçak",
		`x&amp;nbsp;y`:                             "x y",
		"  çok   boşluk ​  var ":                   "çok boşluk var",
		`<img src=x onerror=alert(1)>yazı`:         "yazı",
		`<a href="javascript:alert(1)">tıkla</a>`:  "tıkla",
		`AT&T 5 < 6`: "AT&T 5 < 6",
	}
	for in, want := range cases {
		if got := PlainText(in); got != want {
			t.Errorf("PlainText(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("kelime ", 100)
	if got := Truncate(long, 50); len([]rune(got)) > 51 || !strings.HasSuffix(got, "…") {
		t.Errorf("Truncate = %q", got)
	}
}

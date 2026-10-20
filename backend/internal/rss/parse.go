package rss

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ErrNotFeed means the document is not an RSS/Atom/RDF feed (typically an HTML page or an error page).
var ErrNotFeed = errors.New("rss: document is not an RSS or Atom feed")

// Feed is the format-independent result of parsing.
type Feed struct {
	Format      string // rss2 | rss1 | atom
	Title       string
	Link        string
	Description string
	Items       []Item
}

// Item is one normalized entry. Title/Summary are sanitized plain text; Link is an absolute http(s) URL or "".
type Item struct {
	GUID         string
	Title        string
	Link         string
	Summary      string
	Published    *time.Time // nil when the feed gave no parseable date (or a date in the far future)
	PublishedRaw string
	Categories   []string
	Author       string
	ContentHash  string
}

const maxSummaryRunes = 2000

// Parse decodes RSS 2.0, RSS 1.0 (RDF) and Atom 1.0 feeds. It tolerates BOMs, legacy Turkish charsets,
// stray control bytes, HTML entities outside CDATA, and a few real-world defects (see fixAttributeSpacing).
// now is used to discard publication dates unreasonably far in the future.
func Parse(data []byte, baseURL string, now time.Time) (*Feed, error) {
	data = cleanXMLBytes(data)
	if len(data) == 0 {
		return nil, ErrNotFeed
	}
	root, err := rootName(data)
	if err != nil {
		if fixed := fixAttributeSpacing(data); !bytes.Equal(fixed, data) {
			data = fixed
			root, err = rootName(data)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNotFeed, err)
		}
	}
	var feed *Feed
	switch root {
	case "rss":
		feed, err = parseRSS2(data)
		if err != nil {
			if fixed := fixAttributeSpacing(data); !bytes.Equal(fixed, data) {
				feed, err = parseRSS2(fixed)
			}
		}
	case "RDF":
		feed, err = parseRDF(data)
	case "feed":
		feed, err = parseAtom(data)
	default:
		return nil, ErrNotFeed
	}
	if err != nil {
		return nil, fmt.Errorf("rss: parsing %s feed: %w", root, err)
	}
	feed.Title = SingleLine(feed.Title)
	feed.Description = Truncate(PlainText(feed.Description), 500)
	base := resolveBase(baseURL, feed.Link)
	for i := range feed.Items {
		finalizeItem(&feed.Items[i], base, now)
	}
	return feed, nil
}

func newDecoder(data []byte) *xml.Decoder {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	d.CharsetReader = charsetReader
	return d
}

func rootName(data []byte) (string, error) {
	d := newDecoder(data)
	for {
		tok, err := d.Token()
		if err != nil {
			if err == io.EOF {
				return "", errors.New("no root element")
			}
			return "", err
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local, nil
		}
	}
}

// fixAttributeSpacing repairs "attr="v"attr2="v2"" (no space between attributes) which appears in the
// root element of science.nasa.gov feeds and makes strict-ish parsers fail. Only the first start tag is touched.
var reGlued = regexp.MustCompile(`("|')([A-Za-z_][\w.:-]*=)`)

func fixAttributeSpacing(data []byte) []byte {
	i := bytes.Index(data, []byte("<rss"))
	if i < 0 {
		i = bytes.Index(data, []byte("<feed"))
	}
	if i < 0 {
		return data
	}
	j := bytes.IndexByte(data[i:], '>')
	if j < 0 {
		return data
	}
	tag := data[i : i+j]
	fixed := reGlued.ReplaceAll(tag, []byte("$1 $2"))
	// the regex also matches `="` boundaries inside values only when glued, which is the defect we repair.
	out := make([]byte, 0, len(data)+8)
	out = append(out, data[:i]...)
	out = append(out, fixed...)
	out = append(out, data[i+j:]...)
	return out
}

// ─── RSS 2.0 ───────────────────────────────────────────────────────

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	Description string   `xml:"description"`
	Encoded     string   `xml:"encoded"`
	GUID        string   `xml:"guid"`
	PubDate     string   `xml:"pubDate"`
	DCDate      string   `xml:"date"`
	Categories  []string `xml:"category"`
	Creator     string   `xml:"creator"`
	Author      string   `xml:"author"`
}

func (r rssItem) toItem() Item {
	it := Item{GUID: strings.TrimSpace(r.GUID), Title: r.Title, Link: r.Link}
	desc, full := r.Description, r.Encoded
	it.Summary = desc
	if len(PlainText(full)) > len(PlainText(desc)) {
		it.Summary = full
	}
	it.PublishedRaw = strings.TrimSpace(r.PubDate)
	if it.PublishedRaw == "" {
		it.PublishedRaw = strings.TrimSpace(r.DCDate)
	}
	it.Categories = r.Categories
	it.Author = r.Creator
	if it.Author == "" {
		it.Author = r.Author
	}
	return it
}

func parseRSS2(data []byte) (*Feed, error) {
	var doc struct {
		Channel struct {
			Title       string    `xml:"title"`
			Link        string    `xml:"link"`
			Description string    `xml:"description"`
			Items       []rssItem `xml:"item"`
		} `xml:"channel"`
	}
	if err := newDecoder(data).Decode(&doc); err != nil {
		return nil, err
	}
	f := &Feed{Format: "rss2", Title: doc.Channel.Title, Link: doc.Channel.Link, Description: doc.Channel.Description}
	for _, r := range doc.Channel.Items {
		f.Items = append(f.Items, r.toItem())
	}
	return f, nil
}

func parseRDF(data []byte) (*Feed, error) {
	var doc struct {
		Channel struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
		} `xml:"channel"`
		Items []rssItem `xml:"item"`
	}
	if err := newDecoder(data).Decode(&doc); err != nil {
		return nil, err
	}
	f := &Feed{Format: "rss1", Title: doc.Channel.Title, Link: doc.Channel.Link, Description: doc.Channel.Description}
	for _, r := range doc.Items {
		f.Items = append(f.Items, r.toItem())
	}
	return f, nil
}

// ─── Atom ──────────────────────────────────────────────────────────

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomEntry struct {
	ID         string     `xml:"id"`
	Title      string     `xml:"title"`
	Links      []atomLink `xml:"link"`
	Summary    string     `xml:"summary"`
	Content    string     `xml:"content"`
	Updated    string     `xml:"updated"`
	Published  string     `xml:"published"`
	Categories []struct {
		Term string `xml:"term,attr"`
	} `xml:"category"`
	Author struct {
		Name string `xml:"name"`
	} `xml:"author"`
}

func atomAlternate(ls []atomLink) string {
	for _, l := range ls {
		if (l.Rel == "" || l.Rel == "alternate") && l.Href != "" {
			return l.Href
		}
	}
	for _, l := range ls {
		if l.Href != "" && l.Rel != "self" {
			return l.Href
		}
	}
	return ""
}

func parseAtom(data []byte) (*Feed, error) {
	var doc struct {
		Title    string      `xml:"title"`
		Subtitle string      `xml:"subtitle"`
		Links    []atomLink  `xml:"link"`
		Entries  []atomEntry `xml:"entry"`
	}
	if err := newDecoder(data).Decode(&doc); err != nil {
		return nil, err
	}
	f := &Feed{Format: "atom", Title: doc.Title, Link: atomAlternate(doc.Links), Description: doc.Subtitle}
	for _, e := range doc.Entries {
		it := Item{GUID: strings.TrimSpace(e.ID), Title: e.Title, Link: atomAlternate(e.Links), Author: e.Author.Name}
		it.Summary = e.Summary
		if len(PlainText(e.Content)) > len(PlainText(e.Summary)) {
			it.Summary = e.Content
		}
		it.PublishedRaw = strings.TrimSpace(e.Published)
		if it.PublishedRaw == "" {
			it.PublishedRaw = strings.TrimSpace(e.Updated)
		}
		for _, c := range e.Categories {
			it.Categories = append(it.Categories, c.Term)
		}
		f.Items = append(f.Items, it)
	}
	return f, nil
}

// ─── normalization ─────────────────────────────────────────────────

func resolveBase(feedURL, channelLink string) *url.URL {
	if u, err := url.Parse(strings.TrimSpace(feedURL)); err == nil && u.Host != "" {
		return u
	}
	if u, err := url.Parse(strings.TrimSpace(channelLink)); err == nil && u.Host != "" {
		return u
	}
	return nil
}

// safeLink returns an absolute http(s) URL (resolving relative links against base) or "".
func safeLink(raw string, base *url.URL) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if !u.IsAbs() && base != nil {
		u = base.ResolveReference(u)
	}
	// feeds sometimes omit the scheme ("csb.gov.tr/feed"): do not guess; only accept http(s).
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.String()
}

func finalizeItem(it *Item, base *url.URL, now time.Time) {
	it.Title = SingleLine(it.Title)
	it.Summary = Truncate(PlainText(it.Summary), maxSummaryRunes)
	it.Link = safeLink(it.Link, base)
	it.Author = SingleLine(it.Author)
	cats := make([]string, 0, len(it.Categories))
	for _, c := range it.Categories {
		if c = SingleLine(c); c != "" {
			cats = append(cats, c)
		}
	}
	it.Categories = cats
	if t, ok := ParseFeedTime(it.PublishedRaw); ok && !t.After(now.Add(48*time.Hour)) {
		it.Published = &t
	}
	if it.Title == "" {
		it.Title = Truncate(it.Summary, 90)
	}
	it.GUID = strings.TrimSpace(it.GUID)
	if it.GUID == "" {
		it.GUID = it.Link
	}
	if it.GUID == "" {
		sum := sha256.Sum256([]byte(it.Title + "\x00" + it.PublishedRaw))
		it.GUID = "sha256:" + hex.EncodeToString(sum[:16])
	}
	it.ContentHash = contentHash(it.Title, it.Summary, it.Link)
}

func contentHash(title, summary, link string) string {
	h := sha256.New()
	h.Write([]byte(strings.Join(strings.Fields(title), " ")))
	h.Write([]byte{0})
	h.Write([]byte(strings.Join(strings.Fields(summary), " ")))
	h.Write([]byte{0})
	h.Write([]byte(link))
	return hex.EncodeToString(h.Sum(nil))
}

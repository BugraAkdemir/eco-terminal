package rss

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
)

const (
	// MaxFeedBytes caps a downloaded feed document.
	MaxFeedBytes = 5 << 20
	maxRedirects = 5
)

var (
	// ErrBlockedAddress is returned when a feed URL resolves to a loopback/private/link-local/etc. address.
	ErrBlockedAddress = errors.New("rss: address is not allowed (private, loopback, link-local or otherwise non-public)")
	// ErrInvalidURL is returned for URLs that are not plain http(s) URLs.
	ErrInvalidURL = errors.New("rss: invalid feed URL")
	// ErrTooLarge is returned when a feed exceeds MaxFeedBytes.
	ErrTooLarge = errors.New("rss: feed is larger than the allowed size")
)

// HTTPStatusError is a non-200/304 answer.
type HTTPStatusError struct{ Code int }

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("rss: feed server answered HTTP %d", e.Code)
}

var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24",
	"203.0.113.0/24", "240.0.0.0/4", "255.255.255.255/32", "2001:db8::/32", "64:ff9b:1::/48",
)

func mustPrefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// IsBlockedIP reports whether ip must never be contacted on behalf of an admin-supplied URL:
// loopback, RFC1918/ULA private, link-local (incl. cloud metadata 169.254.169.254), multicast, unspecified,
// CGNAT, documentation/benchmark/reserved ranges, and IPv4 addresses embedded in IPv6 (mapped, NAT64, 6to4).
func IsBlockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return true
	}
	if ip.Is6() {
		b := ip.As16()
		// NAT64 well-known prefix 64:ff9b::/96 and 6to4 2002::/16 embed an IPv4 address.
		if b[0] == 0x00 && b[1] == 0x64 && b[2] == 0xff && b[3] == 0x9b && allZero(b[4:12]) {
			return IsBlockedIP(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
		}
		if b[0] == 0x20 && b[1] == 0x02 {
			return IsBlockedIP(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}))
		}
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

// ValidateURL performs the synchronous checks for an admin-supplied feed URL: http/https only, no credentials,
// a host, no obviously internal names, and (unless allowPrivate) every resolved address must be public.
// The dialer re-checks the real connection address, so DNS rebinding cannot bypass this.
func ValidateURL(ctx context.Context, raw string, allowPrivate bool) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, ErrInvalidURL
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: credentials in URL are not allowed", ErrInvalidURL)
	}
	if len(raw) > 2048 {
		return nil, fmt.Errorf("%w: too long", ErrInvalidURL)
	}
	if allowPrivate {
		return u, nil
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || !strings.Contains(host, ".") && net.ParseIP(host) == nil && !strings.Contains(host, ":") {
		return nil, ErrBlockedAddress
	}
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if IsBlockedIP(addr) {
			return nil, ErrBlockedAddress
		}
		return u, nil
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("rss: cannot resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if IsBlockedIP(ip) {
			return nil, ErrBlockedAddress
		}
	}
	return u, nil
}

// Fetcher downloads feeds with conditional GET, size cap, redirect limit and SSRF protection.
type Fetcher struct {
	Client    *http.Client
	UserAgent string
	// AllowPrivate disables the address guard. Only honoured outside production (see NewFetcher).
	AllowPrivate bool
}

// NewFetcher builds a Fetcher from config. allowPrivate is ignored in production.
func NewFetcher(cfg config.Config, allowPrivate bool) *Fetcher {
	allow := allowPrivate && !cfg.IsProduction()
	timeout := cfg.RSS.FetchTimeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ua := cfg.RSS.UserAgent
	if ua == "" {
		ua = "EcoTerminal/1.0"
	}
	f := &Fetcher{UserAgent: ua, AllowPrivate: allow}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if !allow {
		dialer.Control = func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			addr, err := netip.ParseAddr(host)
			if err != nil || IsBlockedIP(addr) {
				return ErrBlockedAddress
			}
			return nil
		}
	}
	tr := &http.Transport{
		Proxy:                 nil, // never route admin-supplied URLs through an environment proxy
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          10,
		IdleConnTimeout:       60 * time.Second,
		DisableCompression:    false,
	}
	f.Client = &http.Client{
		Transport: tr,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("rss: stopped after %d redirects", maxRedirects)
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return ErrInvalidURL
			}
			return nil
		},
	}
	return f
}

// FetchResult is the outcome of one conditional GET.
type FetchResult struct {
	NotModified  bool
	Body         []byte
	ETag         string
	LastModified string
	FinalURL     string
}

// Fetch downloads rawURL. etag / lastModified (from a previous fetch) make it a conditional request.
func (f *Fetcher) Fetch(ctx context.Context, rawURL, etag, lastModified string) (FetchResult, error) {
	u, err := ValidateURL(ctx, rawURL, f.AllowPrivate)
	if err != nil {
		return FetchResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return FetchResult{}, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.9, */*;q=0.5")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		if errors.Is(err, ErrBlockedAddress) {
			return FetchResult{}, ErrBlockedAddress
		}
		return FetchResult{}, err
	}
	defer resp.Body.Close()
	res := FetchResult{
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		FinalURL:     resp.Request.URL.String(),
	}
	switch resp.StatusCode {
	case http.StatusNotModified:
		res.NotModified = true
		return res, nil
	case http.StatusOK:
	default:
		return res, &HTTPStatusError{Code: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxFeedBytes+1))
	if err != nil {
		return res, err
	}
	if len(body) > MaxFeedBytes {
		return res, ErrTooLarge
	}
	res.Body = body
	return res, nil
}

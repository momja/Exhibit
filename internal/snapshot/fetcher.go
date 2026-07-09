// Package snapshot vendors a URL-imported artifact's external assets into the
// stored document so the artifact stays a self-contained file (spec §1,
// exhibit-lwb). This file holds the bounded resolver/fetcher: every asset
// fetch the snapshot pipeline performs goes through Fetcher, which owns all
// fetch policy in one place — reference resolution against the source base,
// per-asset and total size budgets, an asset-count cap, timeouts, a redirect
// limit, and a dial-time guard against non-public (SSRF-prone) addresses.
// Callers never touch the network directly.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Limits bounds a snapshot run so a runaway page cannot balloon storage or
// hang ingest. A zero value disables that individual limit; use
// DefaultLimits for the standard budget.
type Limits struct {
	MaxAssetBytes int64         // per-asset size cap
	MaxTotalBytes int64         // cumulative cap across all fetched assets
	MaxAssets     int           // cap on distinct URLs fetched over the network
	Timeout       time.Duration // per-fetch timeout, covering connect through body read
	MaxRedirects  int           // redirect hops allowed per fetch (0 = Go's default of 10)
}

// DefaultLimits returns the standard budget for snapshotting one artifact.
func DefaultLimits() Limits {
	return Limits{
		MaxAssetBytes: 5 << 20,  // 5 MiB
		MaxTotalBytes: 20 << 20, // 20 MiB — a snapshot must stay "just a file"
		MaxAssets:     100,
		Timeout:       10 * time.Second,
		MaxRedirects:  5,
	}
}

// ErrorKind classifies a FetchError so callers can decide how to record it
// (e.g. report a residual origin vs. drop the reference) without string
// matching.
type ErrorKind string

const (
	ErrBadRef      ErrorKind = "bad-ref"            // unparseable ref, or resolves to a non-http(s) URL
	ErrBlockedAddr ErrorKind = "blocked-address"    // destination is loopback/private/link-local
	ErrTooLarge    ErrorKind = "too-large"          // asset exceeds the per-asset size cap
	ErrBudget      ErrorKind = "budget-exhausted"   // total-bytes or asset-count budget spent
	ErrHTTPStatus  ErrorKind = "http-status"        // non-200 response
	ErrRedirect    ErrorKind = "too-many-redirects" // redirect chain exceeded MaxRedirects
	ErrNetwork     ErrorKind = "network"            // transport failure, including timeouts
)

// FetchError is the typed failure for a single asset. It is recordable
// per-asset: one bad reference never aborts the whole snapshot.
type FetchError struct {
	Ref  string    // the reference as it appeared in the document
	URL  string    // resolved absolute URL, "" if resolution failed
	Kind ErrorKind // classification for the caller's failure report
	Err  error     // underlying cause, may be nil
}

func (e *FetchError) Error() string {
	msg := fmt.Sprintf("fetch %q", e.Ref)
	if e.URL != "" && e.URL != e.Ref {
		msg += " (" + e.URL + ")"
	}
	msg += ": " + string(e.Kind)
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *FetchError) Unwrap() error { return e.Err }

// Asset is one successfully fetched resource.
type Asset struct {
	URL         string // resolved absolute URL the bytes came from
	ContentType string // raw Content-Type header value, may be empty
	Body        []byte
}

// Fetcher resolves references against a source base URL and fetches them
// under Limits. One Fetcher serves one snapshot run: it accumulates the
// byte/count budget and dedupes identical URLs across calls. Not safe for
// concurrent use.
type Fetcher struct {
	base       *url.URL
	limits     Limits
	client     *http.Client
	totalBytes int64
	fetched    int // distinct URLs that reached the network
	cache      map[string]cached
}

// cached memoizes the outcome per resolved URL so repeated references cost
// one network fetch. Budget errors are never cached — they describe the
// fetcher's state, not the URL.
type cached struct {
	asset *Asset
	err   *FetchError
}

// NewFetcher returns a Fetcher resolving against baseURL (the imported
// document's own URL), which must be absolute http(s).
func NewFetcher(baseURL string, limits Limits) (*Fetcher, error) {
	return newFetcher(baseURL, limits, guardControl)
}

// newFetcher exists so tests can drop the dial guard and talk to loopback
// httptest servers.
func newFetcher(baseURL string, limits Limits, control func(network, address string, c syscall.RawConn) error) (*Fetcher, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL %q: %w", baseURL, err)
	}
	if (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("base URL %q must be absolute http(s)", baseURL)
	}

	dialer := &net.Dialer{Control: control}
	client := &http.Client{
		// The guard runs at dial time on the literal address being connected
		// to, so it also covers redirect targets and every DNS answer —
		// there is no resolve-then-check race. Proxying is disabled because
		// a proxy would carry the request past the guard.
		Transport: &http.Transport{DialContext: dialer.DialContext},
	}
	if limits.MaxRedirects > 0 {
		maxRedirects := limits.MaxRedirects
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects: %w", maxRedirects, errTooManyRedirects)
			}
			return nil
		}
	}

	return &Fetcher{
		base:   base,
		limits: limits,
		client: client,
		cache:  make(map[string]cached),
	}, nil
}

// NewFetcherForTests returns a Fetcher with the non-public-address dial guard
// disabled so tests in other packages can fetch from loopback httptest fixture
// servers. It refuses to run outside a test binary: ingest fetches URLs taken
// from untrusted documents, and skipping the guard there would reopen the SSRF
// hole guardControl exists to close.
func NewFetcherForTests(baseURL string, limits Limits) (*Fetcher, error) {
	if !testing.Testing() {
		panic("snapshot.NewFetcherForTests called outside a test binary")
	}
	return newFetcher(baseURL, limits, nil)
}

// Vendored reports the assets successfully fetched in this run: their resolved
// URLs (sorted, for stable output) and their cumulative size in bytes. It backs
// the ingest report's "what got inlined" summary.
func (f *Fetcher) Vendored() (urls []string, totalBytes int64) {
	urls = make([]string, 0, len(f.cache))
	for u, c := range f.cache {
		if c.asset != nil {
			urls = append(urls, u)
		}
	}
	sort.Strings(urls)
	return urls, f.totalBytes
}

// Resolve resolves a document reference — relative, root-relative,
// protocol-relative, or absolute — against the source base and returns the
// absolute URL, without fetching. The error, if any, is a *FetchError of
// kind ErrBadRef.
func (f *Fetcher) Resolve(ref string) (string, error) {
	u, ferr := f.resolve(ref)
	if ferr != nil {
		return "", ferr
	}
	return u.String(), nil
}

func (f *Fetcher) resolve(ref string) (*url.URL, *FetchError) {
	fail := func(err error) (*url.URL, *FetchError) {
		return nil, &FetchError{Ref: ref, Kind: ErrBadRef, Err: err}
	}
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return fail(errors.New("empty reference"))
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fail(err)
	}
	abs := f.base.ResolveReference(u)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return fail(fmt.Errorf("unsupported scheme %q", abs.Scheme))
	}
	if abs.Host == "" {
		return fail(errors.New("no host after resolution"))
	}
	abs.Fragment = "" // fragments never reach the network; dropping them dedupes a.css#x / a.css#y
	return abs, nil
}

// Fetch resolves ref against the base and returns the asset bytes, enforcing
// Limits. Failures come back as a *FetchError the caller can record while
// continuing with the remaining assets. Identical resolved URLs are fetched
// once and served from cache afterwards.
func (f *Fetcher) Fetch(ctx context.Context, ref string) (*Asset, error) {
	target, ferr := f.resolve(ref)
	if ferr != nil {
		return nil, ferr
	}
	targetURL := target.String()

	if c, ok := f.cache[targetURL]; ok {
		if c.err != nil {
			e := *c.err
			e.Ref = ref // report the caller's own reference, not the first one seen
			return nil, &e
		}
		return c.asset, nil
	}

	fail := func(kind ErrorKind, err error) (*Asset, error) {
		return nil, &FetchError{Ref: ref, URL: targetURL, Kind: kind, Err: err}
	}
	if f.limits.MaxAssets > 0 && f.fetched >= f.limits.MaxAssets {
		return fail(ErrBudget, fmt.Errorf("asset count limit %d reached", f.limits.MaxAssets))
	}
	if f.limits.MaxTotalBytes > 0 && f.totalBytes >= f.limits.MaxTotalBytes {
		return fail(ErrBudget, fmt.Errorf("total byte budget %d exhausted", f.limits.MaxTotalBytes))
	}

	f.fetched++
	asset, ferr := f.doFetch(ctx, ref, target)
	if ferr != nil {
		if ferr.Kind != ErrBudget {
			f.cache[targetURL] = cached{err: ferr}
		}
		slog.DebugContext(ctx, "snapshot asset fetch failed",
			slog.String("ref", ref), slog.String("url", targetURL),
			slog.String("kind", string(ferr.Kind)), slog.String("err", ferr.Error()))
		return nil, ferr
	}
	f.totalBytes += int64(len(asset.Body))
	f.cache[targetURL] = cached{asset: asset}
	slog.DebugContext(ctx, "snapshot asset fetched",
		slog.String("url", targetURL),
		slog.String("content_type", asset.ContentType),
		slog.Int("bytes", len(asset.Body)),
		slog.Int64("budget_used", f.totalBytes),
	)
	return asset, nil
}

func (f *Fetcher) doFetch(ctx context.Context, ref string, target *url.URL) (*Asset, *FetchError) {
	fail := func(kind ErrorKind, err error) (*Asset, *FetchError) {
		return nil, &FetchError{Ref: ref, URL: target.String(), Kind: kind, Err: err}
	}

	if f.limits.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, f.limits.Timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fail(ErrBadRef, err)
	}
	req.Header.Set("User-Agent", "artifact-viewer-snapshot/1")

	resp, err := f.client.Do(req)
	if err != nil {
		switch {
		case errors.Is(err, errBlockedAddress):
			return fail(ErrBlockedAddr, err)
		case errors.Is(err, errTooManyRedirects):
			return fail(ErrRedirect, err)
		default:
			return fail(ErrNetwork, err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fail(ErrHTTPStatus, fmt.Errorf("unexpected status %s", resp.Status))
	}
	if f.limits.MaxAssetBytes > 0 && resp.ContentLength > f.limits.MaxAssetBytes {
		return fail(ErrTooLarge, fmt.Errorf("declared size %d exceeds per-asset limit %d", resp.ContentLength, f.limits.MaxAssetBytes))
	}

	reader := io.Reader(resp.Body)
	if f.limits.MaxAssetBytes > 0 {
		reader = io.LimitReader(reader, f.limits.MaxAssetBytes+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return fail(ErrNetwork, err)
	}
	if f.limits.MaxAssetBytes > 0 && int64(len(body)) > f.limits.MaxAssetBytes {
		return fail(ErrTooLarge, fmt.Errorf("asset exceeds per-asset limit %d", f.limits.MaxAssetBytes))
	}
	if f.limits.MaxTotalBytes > 0 && f.totalBytes+int64(len(body)) > f.limits.MaxTotalBytes {
		return fail(ErrBudget, fmt.Errorf("total byte budget %d exhausted", f.limits.MaxTotalBytes))
	}

	return &Asset{
		URL:         target.String(),
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,
	}, nil
}

// errBlockedAddress marks dial attempts rejected by the address guard;
// doFetch maps it to ErrBlockedAddr via errors.Is.
var errBlockedAddress = errors.New("destination address is not publicly routable")

// errTooManyRedirects marks CheckRedirect aborts; doFetch maps it to ErrRedirect.
var errTooManyRedirects = errors.New("too many redirects")

// guardControl rejects connections to non-public addresses. Ingest fetches
// URLs taken from untrusted documents, so without this an artifact could
// point the server at localhost services or cloud metadata endpoints (SSRF).
func guardControl(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		// Fail closed: never dial an address we cannot classify.
		return fmt.Errorf("unparseable dial address %q: %w", address, errBlockedAddress)
	}
	if !publiclyRoutable(ap.Addr()) {
		return fmt.Errorf("dial %s: %w", ap.Addr(), errBlockedAddress)
	}
	return nil
}

// publiclyRoutable reports whether a is a public unicast address — i.e. not
// loopback, RFC 1918/4193 private, link-local (including the 169.254.169.254
// metadata range), multicast, broadcast, or unspecified.
func publiclyRoutable(a netip.Addr) bool {
	a = a.Unmap() // classify ::ffff:10.0.0.1 as its embedded IPv4 self
	return a.IsValid() && a.IsGlobalUnicast() && !a.IsPrivate()
}

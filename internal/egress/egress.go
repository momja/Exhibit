// Package egress is the network boundary for requests made on an artifact's
// behalf. Guest headers and every redirect are checked before trusted connector
// credentials are added. No artifact code runs here.
package egress

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/origin"
)

type Limits struct {
	MaxRedirects         int
	MaxRequestBytes      int64
	MaxResponseBytes     int64
	Timeout              time.Duration
	UserAgent            string
	OperatorAllowOrigins []string
	OperatorDenyOrigins  []string
	// BeforeRequest is trusted operator policy, run for every checked hop
	// before authorization or network I/O. It must not mutate the destination.
	BeforeRequest func(*url.URL) error
}

func (l Limits) defaults() (Limits, error) {
	if l.MaxRedirects == 0 {
		l.MaxRedirects = 5
	}
	if l.MaxRequestBytes == 0 {
		l.MaxRequestBytes = 1 << 20
	}
	if l.MaxResponseBytes == 0 {
		l.MaxResponseBytes = 4 << 20
	}
	if l.Timeout == 0 {
		l.Timeout = 15 * time.Second
	}
	if l.UserAgent == "" {
		l.UserAgent = "Exhibit-Relay/1"
	}
	if l.MaxRedirects < 0 || l.MaxRequestBytes < 0 || l.MaxResponseBytes < 0 || l.Timeout < 0 {
		return l, errors.New("egress limits must be positive")
	}
	if strings.ContainsAny(l.UserAgent, "\r\n") {
		return l, errors.New("invalid relay user agent")
	}
	return l, nil
}

// DeniedError has an Origin only for refusals an artifact owner can approve.
// Private destinations and operator policy refusals are never approvable.
type DeniedError struct{ Origin, Reason string }

func (e *DeniedError) Error() string {
	if e.Origin == "" {
		return "egress denied: " + e.Reason
	}
	return fmt.Sprintf("egress denied for %s: %s", e.Origin, e.Reason)
}
func (e *DeniedError) DeniedOrigin() string { return e.Origin }

type Response struct {
	Status     int
	StatusText string
	Header     http.Header
	Body       []byte
	URL        string
}

type Gate struct {
	allowed, operatorAllow, operatorDeny map[string]bool
	limits                               Limits
	transport                            *http.Transport
	client                               *http.Client
	test                                 bool
	lookup                               func(context.Context, string) ([]netip.Addr, error)
	dial                                 func(context.Context, string, string) (net.Conn, error)
}

func New(allowlist []string, limits Limits) (*Gate, error) { return newGate(allowlist, limits, false) }

// NewForTests allows HTTP and private fixture servers. It panics outside a Go
// test binary, so production callers cannot switch off the address boundary.
func NewForTests(allowlist []string, limits Limits) (*Gate, error) {
	if !testing.Testing() {
		panic("egress.NewForTests outside a test binary")
	}
	return newGate(allowlist, limits, true)
}

func newGate(allowlist []string, limits Limits, test bool) (*Gate, error) {
	limits, err := limits.defaults()
	if err != nil {
		return nil, err
	}
	sets := make([]map[string]bool, 0, 3)
	for _, entries := range [][]string{allowlist, limits.OperatorAllowOrigins, limits.OperatorDenyOrigins} {
		list, err := origin.NormalizeOrigins(entries)
		if err != nil {
			return nil, err
		}
		set := make(map[string]bool, len(list))
		for _, entry := range list {
			set[entry] = true
		}
		sets = append(sets, set)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	g := &Gate{allowed: sets[0], operatorAllow: sets[1], operatorDeny: sets[2], limits: limits, test: test,
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}, dial: dialer.DialContext}
	g.transport = &http.Transport{
		// Proxy stays nil, even if HTTP(S)_PROXY is set in the environment.
		DialContext: g.dialContext, TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: limits.Timeout, MaxResponseHeaderBytes: 64 << 10,
		MaxIdleConns: 4, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second,
		DisableCompression: true,
	}
	if test {
		// Fixture-only trust. NewForTests cannot be invoked outside a test
		// binary; production always uses the system trust store.
		g.transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	g.client = &http.Client{Transport: g.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return g, nil
}

// Close releases pooled connections when a per-artifact gate is discarded.
func (g *Gate) Close()                 { g.transport.CloseIdleConnections() }
func (g *Gate) Allows(u *url.URL) bool { return g.check(u) == nil }

func (g *Gate) check(u *url.URL) error {
	if u == nil || u.Host == "" || u.Opaque != "" || u.User != nil || u.Fragment != "" {
		return &DeniedError{Reason: "request requires an absolute URL without credentials or fragment"}
	}
	if u.Scheme != "https" && !(g.test && u.Scheme == "http") {
		return &DeniedError{Reason: "only https requests are allowed"}
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if !g.test {
		if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.Contains(host, "%") {
			return &DeniedError{Reason: "destination address is not publicly routable"}
		}
		if ip, err := netip.ParseAddr(host); err == nil {
			if !publicAddress(ip) {
				return &DeniedError{Reason: "destination address is not publicly routable"}
			}
		} else if numericHost(host) {
			return &DeniedError{Reason: "ambiguous numeric destination is not allowed"}
		}
	}
	o, err := origin.NormalizeOrigin(u.Scheme + "://" + u.Host)
	if err != nil {
		return &DeniedError{Reason: "invalid request origin"}
	}
	if g.operatorDeny[o] || (len(g.operatorAllow) != 0 && !g.operatorAllow[o]) {
		return &DeniedError{Reason: "destination is blocked by operator policy"}
	}
	if !g.allowed[o] {
		return &DeniedError{Origin: o, Reason: "origin is not on this artifact's network allowlist"}
	}
	return nil
}

// numericHost rejects legacy inet_aton spellings (decimal, octal, hexadecimal,
// shortened IPv4). URL implementations disagree about these; none is a DNS name
// a relay should interpret on an artifact's behalf.
func numericHost(host string) bool {
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return true
		}
		if strings.HasPrefix(label, "0x") {
			if _, err := strconv.ParseUint(label[2:], 16, 64); err != nil {
				return false
			}
			continue
		}
		for _, r := range label {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, prefix := range nonPublic {
		if prefix.Contains(ip) {
			return false
		}
	}
	// Currently assigned global IPv6 unicast space. Other ranges fail closed.
	return ip.Is4() || netip.MustParsePrefix("2000::/3").Contains(ip)
}

func (g *Gate) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, &DeniedError{Reason: "invalid dial address"}
	}
	if g.test {
		return g.dial(ctx, network, address)
	}
	var addresses []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{ip}
	} else {
		addresses, err = g.lookup(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("relay DNS lookup failed: %w", err)
		}
	}
	if len(addresses) == 0 {
		return nil, &DeniedError{Reason: "destination has no public addresses"}
	}
	for _, ip := range addresses {
		if !publicAddress(ip) {
			return nil, &DeniedError{Reason: "destination address is not publicly routable"}
		}
	}
	var last error
	for _, ip := range addresses {
		// Dial the checked literal: there is no second resolution to rebind.
		conn, err := g.dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

var requestHeaders = map[string]bool{
	"Accept": true, "Accept-Language": true, "Content-Type": true,
	"If-Match": true, "If-None-Match": true, "If-Modified-Since": true,
	"If-Unmodified-Since": true, "Range": true, "If-Range": true,
	"Cache-Control": true,
}
var responseHeaders = map[string]bool{
	"Content-Type": true, "Content-Language": true, "Cache-Control": true,
	"Etag": true, "Last-Modified": true, "Expires": true, "Date": true,
	"Retry-After": true, "Accept-Ranges": true, "Content-Range": true, "Vary": true,
}

func selectedHeaders(src http.Header, allow map[string]bool) http.Header {
	dst := make(http.Header)
	// A peer may nominate any field as hop-by-hop with Connection.
	hop := make(map[string]bool)
	for _, entry := range src.Values("Connection") {
		for _, name := range strings.Split(entry, ",") {
			hop[http.CanonicalHeaderKey(strings.TrimSpace(name))] = true
		}
	}
	for name, values := range src {
		name = http.CanonicalHeaderKey(name)
		if allow[name] && !hop[name] {
			dst[name] = append([]string(nil), values...)
		}
	}
	return dst
}

func (g *Gate) Do(ctx context.Context, req *http.Request) (*Response, error) {
	return g.DoAuthorized(ctx, req, nil)
}

// DoAuthorized adds first-party credentials after checking each destination.
// authorize must decide from the hop's URL, and must only mutate its headers.
// Each hop starts with sanitized guest headers: previous credentials never
// carry over, even between allowlisted origins. Never pass an artifact callback.
func (g *Gate) DoAuthorized(ctx context.Context, req *http.Request, authorize func(*http.Request) error) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, g.limits.Timeout)
	defer cancel()
	prepared, err := g.prepareRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	method, body, baseHeaders := prepared.method, prepared.body, prepared.headers
	target := *req.URL
	for hop := 0; ; hop++ {
		if err := g.check(&target); err != nil {
			return nil, err
		}
		if g.limits.BeforeRequest != nil {
			checked := target
			if err := g.limits.BeforeRequest(&checked); err != nil {
				return nil, err
			}
			if checked.String() != target.String() {
				return nil, &DeniedError{Reason: "operator request hook changed destination"}
			}
		}
		current, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
		if err != nil {
			return nil, &DeniedError{Reason: "invalid request"}
		}
		current.Header = baseHeaders.Clone()
		current.Header.Set("User-Agent", g.limits.UserAgent)
		current.Header.Set("Accept-Encoding", "gzip, deflate")
		expectedHost := current.Host
		if authorize != nil {
			if err := authorize(current); err != nil {
				return nil, err
			}
			if current.URL.String() != target.String() || current.Method != method || current.Host != expectedHost {
				return nil, &DeniedError{Reason: "connector authorization changed request destination"}
			}
		}
		// Even trusted recipes cannot impersonate browser/session context or
		// change transport framing. Only credentials belong to the hook.
		for name := range current.Header {
			lower := strings.ToLower(name)
			if lower == "host" || lower == "cookie" || lower == "origin" || lower == "referer" || lower == "forwarded" || lower == "connection" || lower == "keep-alive" || lower == "te" || lower == "trailer" || lower == "transfer-encoding" || lower == "upgrade" || lower == "content-length" || strings.HasPrefix(lower, "proxy-") || strings.HasPrefix(lower, "x-forwarded-") || strings.HasPrefix(lower, "sec-") {
				delete(current.Header, name)
			}
		}
		current.Header.Set("User-Agent", g.limits.UserAgent)
		current.Header.Set("Accept-Encoding", "gzip, deflate")
		resp, err := g.client.Do(current)
		if err != nil {
			var denied *DeniedError
			if errors.As(err, &denied) {
				return nil, denied
			}
			var urlError *url.Error
			if errors.As(err, &urlError) {
				return nil, fmt.Errorf("relay request failed: %w", urlError.Err)
			}
			return nil, fmt.Errorf("relay request failed: %w", err)
		}
		if isRedirect(resp.StatusCode) && resp.Header.Get("Location") != "" {
			resp.Body.Close()
			if hop >= g.limits.MaxRedirects {
				return nil, &DeniedError{Reason: "too many redirects"}
			}
			next, err := target.Parse(resp.Header.Get("Location"))
			if err != nil {
				return nil, &DeniedError{Reason: "invalid redirect destination"}
			}
			// Fragments have no network meaning and must not affect the gate.
			next.Fragment, next.RawFragment = "", ""
			if resp.StatusCode == 303 && method != "HEAD" || (resp.StatusCode == 301 || resp.StatusCode == 302) && method == "POST" {
				method, body = "GET", nil
				baseHeaders.Del("Content-Type")
			}
			target = *next
			continue
		}
		data, err := g.readResponse(resp)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		return &Response{Status: resp.StatusCode, StatusText: http.StatusText(resp.StatusCode), Header: selectedHeaders(resp.Header, responseHeaders), Body: data, URL: target.String()}, nil
	}
}

func isRedirect(status int) bool {
	return status == 301 || status == 302 || status == 303 || status == 307 || status == 308
}

func (g *Gate) readResponse(resp *http.Response) ([]byte, error) {
	if resp.Request.Method == http.MethodHead || resp.StatusCode == 204 || resp.StatusCode == 304 {
		return nil, nil
	}
	var reader io.Reader = resp.Body
	var decoder io.ReadCloser
	var err error
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		decoder, err = gzip.NewReader(resp.Body)
	case "deflate":
		// RFC deflate uses zlib framing. Raw deflate is not accepted ambiguously.
		decoder, err = zlib.NewReader(resp.Body)
	default:
		return nil, &DeniedError{Reason: "unsupported response content encoding"}
	}
	if err != nil {
		return nil, fmt.Errorf("invalid compressed response: %w", err)
	}
	if decoder != nil {
		defer decoder.Close()
		reader = decoder
	}
	data, err := io.ReadAll(io.LimitReader(reader, g.limits.MaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > g.limits.MaxResponseBytes {
		return nil, &DeniedError{Reason: "decoded response exceeds relay byte limit"}
	}
	return data, nil
}

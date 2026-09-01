// Package origin defines what an allowlist entry *is*: a CSP origin — scheme,
// host, and an explicit non-default port, and nothing else.
//
// The allowlist is the input to the render CSP (docs/architecture.md §3.2), so
// an entry that is not an origin is not a cosmetic problem. A path-bearing
// value is path-matched by CSP, which means it silently grants something other
// than what the allowlist editor shows the user; a trailing-dot or mixed-case
// spelling of a host that is already approved defeats the "one decision per
// (artifact, origin)" invariant (§3.3) by letting the same origin appear under
// several names. Both were observed in live data (av-i7hd).
//
// NormalizeOrigin is therefore applied at the single write path (PRD §4.1) so
// no client can put a non-origin into the store, and defensively in the store
// itself so no future caller can either.
package origin

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// cspKeywords are source expressions the CSP builder owns. The no-egress
// sources (data:, blob:, 'unsafe-inline', …) are already unconditional in
// buildCSP, and the keywords that widen a policy ('self', '*', …) are not the
// user's to type into a per-artifact allowlist. Either way they are never
// stored as allowlist rows.
var cspKeywords = map[string]struct{}{
	"'self'":           {},
	"'none'":           {},
	"'unsafe-inline'":  {},
	"'unsafe-eval'":    {},
	"'strict-dynamic'": {},
	"data:":            {},
	"blob:":            {},
	"filesystem:":      {},
	"mediastream:":     {},
	"*":                {},
}

// NormalizeOrigin reduces s to its canonical origin — lowercase scheme and
// host, a trailing dot stripped from the host, and the port kept only when it
// is not the scheme's default.
//
// The two return values are deliberately independent, because two callers want
// different things from the same rule:
//
//   - A non-nil error means s was not already an origin. The write path rejects
//     the request on it (400, naming the entry) rather than storing something
//     the user did not see — silently truncating https://cdn.example.com/lib.js
//     to https://cdn.example.com would grant a whole host from an entry that
//     read as one file.
//   - A non-empty first return means an origin could still be *derived* from s.
//     That is what the legacy-row repair (store migration 23) salvages through
//     SalvageOrigin, so an already-stored path-bearing row collapses onto the
//     host it always effectively named instead of being dropped from a working
//     artifact.
//
// Values with no origin in them at all — empty, padded with or containing
// whitespace, relative, wildcarded, a CSP keyword, or a non-http(s) scheme —
// return ("", err).
//
// Scheme policy: https always; http only for loopback hosts (localhost,
// 127.0.0.0/8, ::1), which is how a self-hosted dev artifact talks to a service
// on the same machine. Any other plaintext http origin is rejected — over the
// network it is egress the visitor cannot verify, and approving it in a
// per-artifact allowlist would read as safer than it is.
func NormalizeOrigin(s string) (string, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed != s {
		// As typed this is not the origin it would normalize to; trimming
		// silently would store a spelling the user never saw.
		return "", fmt.Errorf("origin has surrounding whitespace")
	}
	if trimmed == "" {
		return "", fmt.Errorf("empty origin")
	}
	if strings.ContainsFunc(trimmed, unicode.IsSpace) {
		return "", fmt.Errorf("origin contains whitespace")
	}
	if _, isKeyword := cspKeywords[strings.ToLower(trimmed)]; isKeyword {
		return "", fmt.Errorf("CSP keyword is not an allowlist origin")
	}
	if strings.Contains(trimmed, "*") {
		return "", fmt.Errorf("wildcards are not allowed; list each origin explicitly")
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("not a URL: %w", err)
	}
	if !u.IsAbs() || u.Host == "" {
		return "", fmt.Errorf("must be an absolute origin like https://example.com")
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return "", fmt.Errorf("scheme %q is not allowed; use https", scheme)
	}

	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return "", fmt.Errorf("missing host")
	}
	if !validHost(host) {
		return "", fmt.Errorf("invalid host %q", host)
	}
	if scheme == "http" && !isLoopback(host) {
		return "", fmt.Errorf("plaintext http is only allowed for loopback hosts; use https")
	}

	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("invalid port %q", port)
		}
		// Canonicalize the spelling (":0443" is ":443") so a default port
		// with leading zeros still collapses onto the defaultless form.
		port = strconv.Itoa(n)
		if port == defaultPort(scheme) {
			port = ""
		}
	}

	if strings.Contains(host, ":") { // IPv6 literal — CSP needs the brackets back
		host = "[" + host + "]"
	}
	normalized := scheme + "://" + host
	if port != "" {
		normalized += ":" + port
	}

	// Everything below is recoverable: the origin above is real, but s carried
	// more than an origin, so the write path must reject it.
	switch {
	case u.User != nil:
		return normalized, fmt.Errorf("credentials are not part of an origin; use %s", normalized)
	case u.Path != "" && u.Path != "/":
		return normalized, fmt.Errorf("an allowlist entry is an origin, not a URL with a path; use %s", normalized)
	case u.RawQuery != "" || u.ForceQuery:
		return normalized, fmt.Errorf("a query string is not part of an origin; use %s", normalized)
	case u.Fragment != "":
		return normalized, fmt.Errorf("a fragment is not part of an origin; use %s", normalized)
	}
	return normalized, nil
}

// SalvageOrigin is the legacy-row repair's (store migration 23) entry point —
// and nothing else's. It returns the origin s always effectively named, even
// when s itself is not an origin (a path-bearing row collapses onto its host),
// or "" when s has no origin in it at all and the row must be dropped. Every
// other caller goes through NormalizeOrigin and honors its error: anywhere but
// the repair, a non-origin is rejected, never silently truncated.
func SalvageOrigin(s string) string {
	normalized, _ := NormalizeOrigin(s)
	return normalized
}

// NormalizeOrigins normalizes a whole allowlist, de-duplicating the result
// while preserving the caller's order (the order the allowlist editor shows).
// The first invalid entry fails the whole list and the error names it, so the
// caller can point at the value rather than reporting that "something" was
// wrong.
func NormalizeOrigins(origins []string) ([]string, error) {
	out := make([]string, 0, len(origins))
	seen := make(map[string]struct{}, len(origins))
	for _, raw := range origins {
		normalized, err := NormalizeOrigin(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid allowlist entry %q: %w", raw, err)
		}
		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}

// validHost accepts a DNS name (letters, digits, '-', '.') or an IP literal.
// url.Parse is permissive about hosts; the allowlist is not, because the value
// is pasted verbatim into a CSP header.
func validHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	if strings.Contains(host, ":") { // a colon outside an IP literal is not a host
		return false
	}
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.', r == '_':
		default:
			return false
		}
	}
	return !strings.HasPrefix(host, ".") && !strings.Contains(host, "..")
}

func isLoopback(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

package egress

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

type preparedRequest struct {
	method  string
	body    []byte
	headers http.Header
}

// prepareRequest is shared by direct and vendor-forwarded requests. Artifact
// headers never become trusted headers merely because they travel inside JSON.
func (g *Gate) prepareRequest(ctx context.Context, req *http.Request) (preparedRequest, error) {
	var prepared preparedRequest
	if req == nil {
		return prepared, &DeniedError{Reason: "missing request"}
	}
	if err := g.check(req.URL); err != nil {
		return prepared, err
	}
	prepared.method = req.Method
	if prepared.method == "" {
		prepared.method = http.MethodGet
	}
	switch prepared.method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
	default:
		return prepared, &DeniedError{Reason: "request method is not supported"}
	}
	if req.Body != nil {
		defer req.Body.Close()
		stopClose := context.AfterFunc(ctx, func() { _ = req.Body.Close() })
		defer stopClose()
		body, err := io.ReadAll(io.LimitReader(req.Body, g.limits.MaxRequestBytes+1))
		if err != nil {
			return prepared, err
		}
		if int64(len(body)) > g.limits.MaxRequestBytes {
			return prepared, &DeniedError{Reason: "request exceeds relay byte limit"}
		}
		prepared.body = body
	}
	prepared.headers = selectedHeaders(req.Header, requestHeaders)
	if !validHeaders(prepared.headers) {
		return prepared, &DeniedError{Reason: "request headers are invalid or exceed relay byte limit"}
	}
	return prepared, ctx.Err()
}

func validHeaders(header http.Header) bool {
	total := 0
	for name, values := range header {
		if name == "" {
			return false
		}
		for i := 0; i < len(name); i++ {
			c := name[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c))) {
				return false
			}
		}
		total += len(name) + 4
		if total > 64<<10 {
			return false
		}
		for _, value := range values {
			total += len(name) + len(value) + 4
			if total > 64<<10 {
				return false
			}
			for i := 0; i < len(value); i++ {
				if value[i] == 127 || value[i] < 32 && value[i] != '\t' {
					return false
				}
			}
		}
	}
	return true
}

// DoForwarded validates an initial provider request before a trusted vendor
// engine sends it. The vendor performs the actual dial and any redirects, so
// this preflight cannot provide direct mode's DNS pinning or per-hop checks.
// Only first-party engines may be passed as forward; never artifact callbacks.
func (g *Gate) DoForwarded(ctx context.Context, req *http.Request, forward func(context.Context, *http.Request) (*Response, error)) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, g.limits.Timeout)
	defer cancel()
	prepared, err := g.prepareRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	if forward == nil {
		return nil, &DeniedError{Reason: "missing vendor engine"}
	}
	target := *req.URL
	if g.limits.BeforeRequest != nil {
		checked := target
		if err := g.limits.BeforeRequest(&checked); err != nil {
			return nil, err
		}
		if checked.String() != target.String() {
			return nil, &DeniedError{Reason: "operator request hook changed destination"}
		}
	}
	if err := g.checkForwardAddresses(ctx, &target); err != nil {
		return nil, err
	}
	current, err := http.NewRequestWithContext(ctx, prepared.method, target.String(), bytes.NewReader(prepared.body))
	if err != nil {
		return nil, &DeniedError{Reason: "invalid request"}
	}
	current.Header = prepared.headers
	current.Header.Set("User-Agent", g.limits.UserAgent)
	response, err := forward(ctx, current)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if response == nil || response.Status < 200 || response.Status > 599 {
		return nil, &DeniedError{Reason: "vendor returned an invalid HTTP response"}
	}
	if int64(len(response.Body)) > g.limits.MaxResponseBytes {
		return nil, &DeniedError{Reason: "decoded response exceeds relay byte limit"}
	}
	if !validHeaders(response.Header) || len(response.URL)+len(response.StatusText) > 64<<10 {
		return nil, &DeniedError{Reason: "response metadata exceeds relay byte limit"}
	}
	// A vendor URL is neither redirect proof nor public metadata. Return the
	// exact checked URL and only the same safe headers as the direct path.
	result := &Response{Status: response.Status, StatusText: http.StatusText(response.Status), Header: selectedHeaders(response.Header, responseHeaders), Body: response.Body, URL: target.String()}
	if prepared.method == http.MethodHead || result.Status == http.StatusNoContent || result.Status == http.StatusNotModified {
		result.Body = nil
	}
	return result, nil
}

func (g *Gate) checkForwardAddresses(ctx context.Context, target *url.URL) error {
	if g.test {
		return ctx.Err()
	}
	host := strings.TrimSuffix(target.Hostname(), ".")
	var addresses []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{ip}
	} else {
		var err error
		addresses, err = g.lookup(ctx, host)
		if err != nil {
			return &DeniedError{Reason: "destination DNS lookup failed"}
		}
	}
	if len(addresses) == 0 {
		return &DeniedError{Reason: "destination has no public addresses"}
	}
	for _, ip := range addresses {
		if !publicAddress(ip) {
			return &DeniedError{Reason: "destination address is not publicly routable"}
		}
	}
	return ctx.Err()
}

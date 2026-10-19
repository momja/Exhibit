// Package chromium connects the service to the headless Chromium an operator
// runs beside it (CHROMIUM_CDP_URL), so that no server feature has to carry a
// browser in the service image (technical_stack.md §8). It owns reaching the
// browser and isolating each use of it. What a feature loads, what it may
// reach, and what it does with the page are that feature's decisions; link
// previews (internal/ogsnap) are the first.
//
// Every Tab runs in a browser context of its own, so features sharing the
// browser share nothing inside it: not cookies, not storage, not service
// workers, and not network restrictions. A restriction is a property of the
// context it was given to (ReachOnly), never of the browser, so one feature's
// confinement cannot reach another feature's pages.
package chromium

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

// Browser is a remote headless Chromium reached over the DevTools protocol.
type Browser struct {
	endpoint string
	// discovery asks the browser for its websocket address. A field so a test
	// can see the request without a browser on a default port.
	discovery *http.Client
}

// New returns the browser at endpoint: a DevTools address such as
// ws://chrome:9222, http://chrome:9222, https://chrome.example, or a full
// ws(s)://…/devtools/browser/… address. It refuses anything that cannot be
// one, without contacting the browser.
func New(endpoint string) (*Browser, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%q is not a URL with a host", endpoint)
	}
	switch u.Scheme {
	case "ws", "wss", "http", "https":
	default:
		return nil, fmt.Errorf("%q must be a ws://, wss://, http:// or https:// DevTools endpoint", endpoint)
	}
	return &Browser{endpoint: endpoint, discovery: http.DefaultClient}, nil
}

// ContextOption configures the browser context a Tab runs in.
type ContextOption = chromedp.CreateBrowserContextOption

// Tab opens a blank page in a new browser context, over a connection of its
// own, and returns the chromedp context to drive it with. Cancelling it
// disposes the browser context, and everything the page started in it, and
// then closes the connection; the browser itself stays up.
//
// A connection per tab costs a dial, which is small next to a page load, and
// means a restarted browser is simply reached again by the next tab.
func (b *Browser) Tab(ctx context.Context, opts ...ContextOption) (context.Context, context.CancelFunc, error) {
	wsURL, err := b.webSocketURL(ctx)
	if err != nil {
		return nil, nil, err
	}
	allocCtx, cancelAlloc := remote.NewAllocator(ctx, wsURL, remote.NoModifyURL)
	// The first context is only the connection. The tab's context below owns
	// the browser context, so cancelling it disposes that and nothing else.
	connCtx, cancelConn := chromedp.NewContext(allocCtx)
	// disposeOnDetach is the backstop for a connection that drops before the
	// cancel below runs: the browser then disposes the context itself, rather
	// than keep whatever the page left running.
	dispose := true
	disposeOnDetach := func(p *target.CreateBrowserContextParams) { p.DisposeOnDetach = &dispose }
	tabCtx, cancelTab := chromedp.NewContext(connCtx,
		chromedp.WithNewBrowserContext(append([]ContextOption{disposeOnDetach}, opts...)...))
	cancel := func() {
		// Cancel waits until the tab is closed and its context disposed, which
		// cancelling the context alone only starts.
		_ = chromedp.Cancel(tabCtx)
		cancelTab()
		cancelConn()
		cancelAlloc()
	}
	// Running anything creates the tab, so a browser that cannot be reached is
	// an error here rather than at the caller's first action.
	noop := chromedp.Func(func(context.Context, *chromedp.Target) error { return nil })
	if _, err := chromedp.Run(tabCtx, noop); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("chromium: open a tab at %s: %w", b.endpoint, err)
	}
	return tabCtx, cancel, nil
}

// unreachableProxy is where a confined context sends every request it may not
// make directly. Nothing can listen on port 0, so each one fails at once.
const unreachableProxy = "http://127.0.0.1:0"

// ReachOnly confines a browser context's network to origins. Everything else
// is sent to a proxy that does not exist, so the browser's network stack
// refuses it: fetches and subresources, navigations, WebSockets, and requests
// from popups, workers and service workers the page starts, since they all
// live in the same context. WebRTC is the exception, as it does not use the
// proxy; it can send packets but reads back nothing an HTTP service says.
//
// The origins are matched by scheme, host and port, the port made explicit so
// that a default-port origin does not admit every other port on its host.
// Chromium sends loopback addresses around any proxy unless told otherwise,
// and that exemption would hand a confined page the browser's own DevTools
// port, so "<-loopback>" removes it.
func ReachOnly(origins ...string) (ContextOption, error) {
	rules := []string{"<-loopback>"}
	for _, raw := range origins {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") ||
			strings.TrimSuffix(u.Path, "/") != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return nil, fmt.Errorf("chromium: %q is not an http(s) origin", raw)
		}
		port := u.Port()
		if port == "" {
			port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
		}
		rules = append(rules, u.Scheme+"://"+net.JoinHostPort(u.Hostname(), port))
	}
	bypass := strings.Join(rules, ";")
	return func(p *target.CreateBrowserContextParams) {
		p.ProxyServer = unreachableProxy
		p.ProxyBypassList = bypass
	}, nil
}

// webSocketURL turns the configured endpoint into the browser's websocket
// address. A full .../devtools/browser/... address is used as given; anything
// else is asked for it at /json/version. chromedp's remote allocator does this
// too, but always over plain HTTP and only with an explicit port.
//
// The scheme decides TLS for both steps: https and wss stay encrypted, with the
// hostname kept for certificate checks, and a missing port means the scheme's
// default. Chromium builds the address it reports from the request's Host
// header, so only its path is kept and is joined to the endpoint as configured.
//
// A plaintext endpoint's hostname is resolved to an IP address first, because
// Chromium answers a DevTools request only when its Host header is an IP
// address or localhost. A TLS endpoint is a proxy the operator put in front of
// the browser, and the Host header it forwards is the proxy's business.
func (b *Browser) webSocketURL(ctx context.Context) (string, error) {
	u, err := url.Parse(b.endpoint)
	if err != nil {
		return "", err
	}
	var wsScheme, httpScheme string
	switch u.Scheme {
	case "ws", "http":
		wsScheme, httpScheme = "ws", "http"
		if u.Host, err = hostAsIP(ctx, u); err != nil {
			return "", fmt.Errorf("chromium: resolve %s: %w", u.Hostname(), err)
		}
	default:
		wsScheme, httpScheme = "wss", "https"
	}

	if strings.Contains(u.Path, "/devtools/browser/") {
		u.Scheme = wsScheme
		return u.String(), nil
	}

	versionURL := (&url.URL{Scheme: httpScheme, Host: u.Host, Path: "/json/version"}).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := b.discovery.Do(req)
	if err != nil {
		return "", fmt.Errorf("chromium: discover the browser at %s: %w", versionURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chromium: discover the browser at %s: %s", versionURL, resp.Status)
	}
	var version struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&version); err != nil {
		return "", fmt.Errorf("chromium: discover the browser at %s: %w", versionURL, err)
	}
	reported, err := url.Parse(version.WebSocketDebuggerURL)
	if err != nil || !strings.Contains(reported.Path, "/devtools/browser/") {
		return "", fmt.Errorf("chromium: the browser at %s reported no browser address", versionURL)
	}
	return (&url.URL{Scheme: wsScheme, Host: u.Host, Path: reported.Path}).String(), nil
}

// hostAsIP returns u's host with its name resolved to an IP address, keeping
// the port when there is one.
func hostAsIP(ctx context.Context, u *url.URL) (string, error) {
	name, port := u.Hostname(), u.Port()
	ip := name
	if name != "localhost" && net.ParseIP(name) == nil {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, name)
		if err != nil {
			return "", err
		}
		if len(addrs) == 0 {
			return "", errors.New("no addresses")
		}
		ip = addrs[0].IP.String()
	}
	if port != "" {
		return net.JoinHostPort(ip, port), nil
	}
	if strings.Contains(ip, ":") {
		return "[" + ip + "]", nil
	}
	return ip, nil
}

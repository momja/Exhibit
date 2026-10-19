// Package ogsnap renders a share's widget into the PNG a link preview shows
// (av-rclm). It drives a headless Chromium the operator runs beside the
// service — the optional sidecar technical_stack.md §8 describes — over the
// DevTools protocol, so the service image never carries a browser.
//
// The one property worth knowing before reading the code: a snapshot reaches
// RENDER_ORIGIN and nothing else, whatever the artifact's allowlist says. The
// allowlist is a decision about the *visitor's* browser. This browser sits
// inside the operator's network and its pixels are published to anyone
// holding the link, so honouring the allowlist here would let one owner point
// it at an internal address and read the answer back out of a screenshot. A
// widget whose numbers come from the network renders without them in its
// preview; that is the whole cost.
//
// The confinement is browser-wide, not per tab, because a page is not the only
// thing a widget can start. A popup, a worker or a service worker is a target
// of its own, and each one was a way out while only the snapshot's tab was
// guarded. So the interception is enabled on the browser session itself, and it
// does two things to every request from every target:
//
//   - A request outside RENDER_ORIGIN is failed before it is sent.
//   - A response from RENDER_ORIGIN that could become a document or a worker
//     gets one more Content-Security-Policy, limiting connect-src to the render
//     origin. A browser applies every policy a response carries, so this one can
//     only narrow the artifact's own. It is what stops a WebSocket, which the
//     interception never sees, and documents with no response of their own
//     (about:blank, srcdoc, blob: and data: workers) inherit it from their
//     creator.
//
// Because the interception covers the whole browser, the sidecar must be
// dedicated to Exhibit: anything else using it is confined in the same way.
package ogsnap

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
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

// The viewport is the gallery card's tile: 132px tall, as in
// web/gallery/components.css, at the width of a card in a typical grid, with
// the 2:1 shape summary_large_image previews expect. Captured at scale 4, the
// PNG is 1056x528 — large enough for a preview card, while the widget lays
// itself out exactly as it does on the shelf.
const (
	ViewportWidth  = 264
	ViewportHeight = 132
	Scale          = 4
	ImageWidth     = ViewportWidth * Scale
	ImageHeight    = ViewportHeight * Scale
)

const (
	// settle is how long the page runs after its load event before the
	// capture. A widget reads its state synchronously from the inlined cache,
	// so most draw in their first frame; this covers a chart that animates in.
	settle = 400 * time.Millisecond
	// timeout bounds one snapshot end to end, dial included. A preview that
	// takes longer than this is not worth the crawler's wait.
	timeout = 15 * time.Second
	// connectPolicy is the policy added to every render-origin response that
	// can run script. data: and blob: are bytes the page already holds.
	connectPolicy = "connect-src 'self' data: blob:"
)

// Chromium snapshots pages in a remote headless Chromium.
type Chromium struct {
	cdpURL string
	// allowedPrefix is RENDER_ORIGIN plus "/": the only URLs a snapshot may
	// load. The slash is what stops https://render.test.evil.example passing
	// as https://render.test.
	allowedPrefix string
	// discovery asks the browser for its websocket address. A field so a test
	// can see the request without a browser on a default port.
	discovery *http.Client
}

// NewChromium returns a snapshotter for the browser at cdpURL — a DevTools
// endpoint such as ws://chrome:9222, http://chrome:9222, https://chrome.example
// or a full ws(s)://…/devtools/browser/… address — that will load nothing
// outside renderOrigin.
func NewChromium(cdpURL, renderOrigin string) *Chromium {
	return &Chromium{
		cdpURL:        cdpURL,
		allowedPrefix: strings.TrimSuffix(renderOrigin, "/") + "/",
		discovery:     http.DefaultClient,
	}
}

// Snapshot loads pageURL and returns a PNG of its viewport.
//
// Each call dials the browser afresh and works in a browser context of its
// own, disposed with it. The dial is cheap next to a page load, and it means a
// restarted sidecar is simply reached again on the next call, while nothing one
// artifact leaves behind in the browser — cookies, storage, a service worker
// registered on the render origin — is there when the next one loads.
func (c *Chromium) Snapshot(ctx context.Context, pageURL string) ([]byte, error) {
	if !c.allowed(pageURL) {
		return nil, fmt.Errorf("ogsnap: %q is outside the render origin", pageURL)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	wsURL, err := c.browserWebSocketURL(ctx)
	if err != nil {
		return nil, err
	}
	allocCtx, cancelAlloc := remote.NewAllocator(ctx, wsURL, remote.NoModifyURL)
	defer cancelAlloc()
	// The first context is the connection, and the confinement is enabled on
	// its browser session before any tab exists. Cancelling it closes the
	// connection, which ends the interception, never the browser.
	connCtx, cancelConn := chromedp.NewContext(allocCtx)
	defer cancelConn()
	if err := c.confine(connCtx); err != nil {
		return nil, fmt.Errorf("ogsnap: connect to %s: %w", c.cdpURL, err)
	}
	tabCtx, cancelTab := chromedp.NewContext(connCtx, chromedp.WithNewBrowserContext())
	defer cancelTab()

	png, err := chromedp.Run(tabCtx, chromedp.Action[[]byte](func(ctx context.Context, t *chromedp.Target) ([]byte, error) {
		steps := chromedp.Steps(
			chromedp.EmulateViewport(ViewportWidth, ViewportHeight, chromedp.EmulateScale(Scale)),
			chromedp.Navigate(pageURL),
			chromedp.Sleep(settle),
		)
		if _, err := steps(ctx, t); err != nil {
			return nil, err
		}
		return chromedp.CaptureScreenshot()(ctx, t)
	}))
	if err != nil {
		return nil, fmt.Errorf("ogsnap: snapshot %s: %w", pageURL, err)
	}
	if len(png) == 0 {
		return nil, errors.New("ogsnap: empty screenshot")
	}
	return png, nil
}

// confine enables request interception on the browser session, so it applies
// to every target the browser creates from here on: the snapshot's tab and
// whatever that tab starts. It intercepts everything rather than blocking a
// list, so a scheme or host nobody thought of is refused by default.
func (c *Chromium) confine(ctx context.Context) error {
	// Subscribe before enabling, so no paused request is missed.
	paused := chromedp.BrowserEvents(ctx, fetch.RequestPaused)
	go func() {
		for ev, err := range paused {
			if err != nil {
				return
			}
			// One at a time would make a large response hold up every request
			// behind it.
			go c.decide(ctx, ev)
		}
	}()
	_, err := chromedp.CallBrowser(ctx, fetch.Enable, fetch.EnableParams{})
	return err
}

// decide answers one paused request: refused when it leaves the render origin,
// let through when it cannot run script, and otherwise let through and paused
// again at its response, which then gets connectPolicy.
func (c *Chromium) decide(ctx context.Context, ev fetch.EventRequestPaused) {
	atResponse := ev.ResponseStatusCode != 0 || ev.ResponseErrorReason != ""
	switch {
	case ev.Request == nil || !c.allowed(ev.Request.URL):
		c.fail(ctx, ev.RequestID)
	case !atResponse:
		stampResponse := !inert(ev.ResourceType)
		_, _ = chromedp.CallBrowser(ctx, fetch.ContinueRequest, fetch.ContinueRequestParams{
			RequestID: ev.RequestID, InterceptResponse: &stampResponse})
	case ev.ResponseErrorReason != "" || isRedirect(ev.ResponseStatusCode):
		// Nothing to stamp: an error has no body, and a redirect's next hop is
		// paused as a request of its own.
		_, _ = chromedp.CallBrowser(ctx, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: ev.RequestID})
	default:
		c.stamp(ctx, ev)
	}
}

// stamp re-issues a render-origin response with connectPolicy added to its
// headers. The body has to be read and sent back: Chromium does not enforce a
// policy added by Fetch.continueResponse. If the body cannot be read, the
// response is refused rather than let through unstamped.
func (c *Chromium) stamp(ctx context.Context, ev fetch.EventRequestPaused) {
	body, err := chromedp.CallBrowser(ctx, fetch.GetResponseBody, fetch.GetResponseBodyParams{RequestID: ev.RequestID})
	if err != nil {
		c.fail(ctx, ev.RequestID)
		return
	}
	// The body comes back decoded, so the headers that described the encoded
	// one would now misdescribe it.
	var headers []*fetch.HeaderEntry
	for _, h := range ev.ResponseHeaders {
		if !strings.EqualFold(h.Name, "Content-Encoding") && !strings.EqualFold(h.Name, "Content-Length") {
			headers = append(headers, h)
		}
	}
	headers = append(headers, &fetch.HeaderEntry{Name: "Content-Security-Policy", Value: connectPolicy})
	if _, err := chromedp.CallBrowser(ctx, fetch.FulfillRequest, fetch.FulfillRequestParams{
		RequestID:       ev.RequestID,
		ResponseCode:    ev.ResponseStatusCode,
		ResponsePhrase:  ev.ResponseStatusText,
		ResponseHeaders: headers,
		Body:            body.Body,
	}); err != nil {
		c.fail(ctx, ev.RequestID)
	}
}

func (c *Chromium) fail(ctx context.Context, id fetch.RequestID) {
	_, _ = chromedp.CallBrowser(ctx, fetch.FailRequest, fetch.FailRequestParams{
		RequestID: id, ErrorReason: network.ErrorReasonBlockedByClient})
}

// inert reports whether a response of this type can never become a document
// or a worker, and so needs no policy of its own. Everything else is stamped,
// so a type nobody listed is confined by default. The list exists for cost:
// a stamped response travels through the protocol twice.
func inert(t network.ResourceType) bool {
	switch t {
	case network.ResourceTypeImage, network.ResourceTypeMedia, network.ResourceTypeFont,
		network.ResourceTypeStylesheet, network.ResourceTypeFetch, network.ResourceTypeXHR:
		return true
	}
	return false
}

func isRedirect(status int64) bool { return status >= 300 && status < 400 }

// browserWebSocketURL turns the configured DevTools endpoint into the
// browser's websocket address. A full .../devtools/browser/... address is used
// as given; anything else is asked for it at /json/version.
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
func (c *Chromium) browserWebSocketURL(ctx context.Context) (string, error) {
	u, err := url.Parse(c.cdpURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("ogsnap: DevTools endpoint %q is not a URL with a host", c.cdpURL)
	}
	var wsScheme, httpScheme string
	switch u.Scheme {
	case "ws", "http":
		wsScheme, httpScheme = "ws", "http"
		if u.Host, err = hostAsIP(ctx, u); err != nil {
			return "", fmt.Errorf("ogsnap: resolve %s: %w", u.Hostname(), err)
		}
	case "wss", "https":
		wsScheme, httpScheme = "wss", "https"
	default:
		return "", fmt.Errorf("ogsnap: DevTools endpoint %q is not ws, wss, http or https", c.cdpURL)
	}

	if strings.Contains(u.Path, "/devtools/browser/") {
		u.Scheme = wsScheme
		return u.String(), nil
	}

	versionURL := url.URL{Scheme: httpScheme, Host: u.Host, Path: "/json/version"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.discovery.Do(req)
	if err != nil {
		return "", fmt.Errorf("ogsnap: discover the browser at %s: %w", versionURL.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ogsnap: discover the browser at %s: %s", versionURL.String(), resp.Status)
	}
	var version struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&version); err != nil {
		return "", fmt.Errorf("ogsnap: discover the browser at %s: %w", versionURL.String(), err)
	}
	reported, err := url.Parse(version.WebSocketDebuggerURL)
	if err != nil || !strings.Contains(reported.Path, "/devtools/browser/") {
		return "", fmt.Errorf("ogsnap: the browser at %s reported no browser address", versionURL.String())
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

func (c *Chromium) allowed(u string) bool {
	return strings.HasPrefix(u, c.allowedPrefix)
}

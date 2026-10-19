// Package ogsnap renders a share's widget into the PNG a link preview shows
// (av-rclm). It drives a headless Chromium the operator runs beside the
// service — the optional sidecar technical_stack.md §8 describes — over the
// DevTools protocol, so the service image never carries a browser.
//
// The one property worth knowing before reading the code: a snapshot reaches
// RENDER_ORIGIN and nothing else. Every other request the page makes is failed
// at the protocol layer before it leaves the browser, whatever the artifact's
// allowlist says. The allowlist is a decision about the *visitor's* browser.
// This browser sits inside the operator's network and its pixels are published
// to anyone holding the link, so honouring the allowlist here would let one
// owner point it at an internal address and read the answer back out of a
// screenshot. A widget whose numbers come from the network renders without
// them in its preview; that is the whole cost.
package ogsnap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
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
)

// Chromium snapshots pages in a remote headless Chromium.
type Chromium struct {
	cdpURL string
	// allowedPrefix is RENDER_ORIGIN plus "/": the only URLs a snapshot may
	// load. The slash is what stops https://render.test.evil.example passing
	// as https://render.test.
	allowedPrefix string
}

// NewChromium returns a snapshotter for the browser at cdpURL — a DevTools
// endpoint such as ws://chrome:9222 or http://chrome:9222 — that will load
// nothing outside renderOrigin.
func NewChromium(cdpURL, renderOrigin string) *Chromium {
	return &Chromium{cdpURL: cdpURL, allowedPrefix: strings.TrimSuffix(renderOrigin, "/") + "/"}
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

	allocCtx, cancelAlloc := remote.NewAllocator(ctx, c.cdpURL)
	defer cancelAlloc()
	// The first context only connects. CallBrowser attaches without opening a
	// tab, which is what lets the next context own a browser context of its
	// own; cancelling this one closes the connection, never the browser.
	connCtx, cancelConn := chromedp.NewContext(allocCtx)
	defer cancelConn()
	if _, err := chromedp.CallBrowser(connCtx, browser.GetVersion, cdp.Empty{}); err != nil {
		return nil, fmt.Errorf("ogsnap: connect to %s: %w", c.cdpURL, err)
	}
	tabCtx, cancelTab := chromedp.NewContext(connCtx, chromedp.WithNewBrowserContext())
	defer cancelTab()

	png, err := chromedp.Run(tabCtx, chromedp.Action[[]byte](func(ctx context.Context, t *chromedp.Target) ([]byte, error) {
		if err := c.confine(ctx, t); err != nil {
			return nil, err
		}
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

// confine pauses every request the tab makes and lets through only those
// under the render origin. It intercepts everything rather than blocking a
// list, so a scheme or host nobody thought of is refused by default.
//
// Fetch interception does not see WebSockets, so those are blocked by pattern
// as well: a socket is a request the guard above would otherwise never be
// asked about. data: and blob: URLs never reach the network and need neither.
func (c *Chromium) confine(ctx context.Context, t *chromedp.Target) error {
	// Subscribe before enabling, so no paused request is missed.
	paused := cdp.Events(ctx, t, fetch.RequestPaused)
	go func() {
		for ev, err := range paused {
			if err != nil {
				return
			}
			if ev.Request != nil && c.allowed(ev.Request.URL) {
				_, _ = cdp.Call(ctx, t, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: ev.RequestID})
				continue
			}
			_, _ = cdp.Call(ctx, t, fetch.FailRequest, fetch.FailRequestParams{
				RequestID: ev.RequestID, ErrorReason: network.ErrorReasonBlockedByClient})
		}
	}()
	if _, err := cdp.Call(ctx, t, network.Enable, network.EnableParams{}); err != nil {
		return err
	}
	if _, err := cdp.Call(ctx, t, network.SetBlockedURLs, network.SetBlockedURLsParams{
		URLPatterns: []*network.BlockPattern{
			{URLPattern: "ws://*:*/*", Block: true},
			{URLPattern: "wss://*:*/*", Block: true},
		},
	}); err != nil {
		return err
	}
	_, err := cdp.Call(ctx, t, fetch.Enable, fetch.EnableParams{})
	return err
}

func (c *Chromium) allowed(u string) bool {
	return strings.HasPrefix(u, c.allowedPrefix)
}

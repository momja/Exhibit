// Package ogsnap renders a share's widget into the PNG a link preview shows
// (av-rclm), in the operator's headless Chromium (internal/chromium). It holds
// what is particular to link previews: the picture's size, and that the page
// may reach the render origin and nothing else.
//
// That second rule is the one worth knowing before reading the code. It holds
// whatever the artifact's allowlist says, because the allowlist is a decision
// about the *visitor's* browser. This browser sits inside the operator's
// network and its pixels are published to anyone holding the link, so
// honouring the allowlist here would let one owner point it at an internal
// address and read the answer back out of a screenshot. A widget whose numbers
// come from the network renders without them in its preview; that is the
// whole cost.
//
// The rule is enforced on the snapshot's browser context, not its tab, because
// a page is not the only thing a widget can start: a popup, a worker and a
// service worker are all targets of their own, and all live in the context.
package ogsnap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
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

// Snapshotter draws link-preview images in a browser.
type Snapshotter struct {
	browser *chromium.Browser
	// pagePrefix is RENDER_ORIGIN plus "/": the only pages a snapshot is
	// taken of. The slash is what stops https://render.test.evil.example
	// passing as https://render.test.
	pagePrefix string
	// renderOriginOnly confines each snapshot's browser context.
	renderOriginOnly chromium.ContextOption
}

// New returns a Snapshotter that draws in browser and lets each page reach
// renderOrigin and nothing else.
func New(browser *chromium.Browser, renderOrigin string) (*Snapshotter, error) {
	renderOrigin = strings.TrimSuffix(renderOrigin, "/")
	reach, err := chromium.ReachOnly(renderOrigin)
	if err != nil {
		return nil, fmt.Errorf("ogsnap: %w", err)
	}
	return &Snapshotter{browser: browser, pagePrefix: renderOrigin + "/", renderOriginOnly: reach}, nil
}

// Snapshot loads pageURL and returns a PNG of its viewport.
//
// Each snapshot runs in a browser context of its own, disposed with it, so
// nothing one artifact leaves behind in the browser — cookies, storage, a
// service worker registered on the render origin — is there when the next one
// loads.
func (s *Snapshotter) Snapshot(ctx context.Context, pageURL string) ([]byte, error) {
	if !strings.HasPrefix(pageURL, s.pagePrefix) {
		return nil, fmt.Errorf("ogsnap: %q is outside the render origin", pageURL)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tab, closeTab, err := s.browser.Tab(ctx, s.renderOriginOnly)
	if err != nil {
		return nil, fmt.Errorf("ogsnap: %w", err)
	}
	defer closeTab()

	png, err := chromedp.Run(tab, chromedp.Action[[]byte](func(ctx context.Context, t *chromedp.Target) ([]byte, error) {
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

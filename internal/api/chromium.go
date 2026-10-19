package api

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/ogsnap"
	"github.com/momja/Exhibit/internal/render"
)

// The headless browser.
//
// Some server features need a real browser: the first is the picture in a
// share's link preview (av-rclm). It is a headless Chromium the operator runs
// beside the service — never inside the image, which would gain several
// hundred megabytes (technical_stack.md §8). CHROMIUM_CDP_URL names its
// DevTools endpoint, and every feature uses that one browser, each in browser
// contexts of its own (internal/chromium). Unset, the default, means there is
// no browser, and each feature falls back on its own terms.
const envChromiumCDPURL = "CHROMIUM_CDP_URL"

// ChromiumFromEnv reads CHROMIUM_CDP_URL. A value that is not a ws(s) or
// http(s) URL with a host is an error the caller makes fatal: the operator
// asked for a browser, and features silently falling back would leave them
// looking for a fault in it.
func ChromiumFromEnv() (*chromium.Browser, error) {
	raw := strings.TrimSpace(os.Getenv(envChromiumCDPURL))
	if raw == "" {
		return nil, nil
	}
	b, err := chromium.New(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", envChromiumCDPURL, err)
	}
	return b, nil
}

// previewSnapshotter is the link-preview consumer of the browser: it draws a
// live share's widget, letting the page reach the render origin and nothing
// else (internal/ogsnap). nil when the instance runs no browser, which shows
// every preview as the generic card.
func (ro *Router) previewSnapshotter() render.Snapshotter {
	if ro.cfg.Chromium == nil {
		return nil
	}
	snap, err := ogsnap.New(ro.cfg.Chromium, ro.cfg.RenderOrigin)
	if err != nil {
		// RENDER_ORIGIN is checked at startup, so this is not expected. The
		// generic card is the safe answer either way.
		slog.Error("link previews will use the generic card", slog.String("err", err.Error()))
		return nil
	}
	return snap
}

// previewImagePNG is the generic link-preview card the asset build rasterized
// from the logo (web/pwa-icons). Missing means previews carry no image, not
// that the render surface fails to start.
func previewImagePNG() []byte {
	b, err := fs.ReadFile(embeddedAssets, "assets/icons/og-default.png")
	if err != nil {
		return nil
	}
	return b
}

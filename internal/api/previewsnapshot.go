package api

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"

	"github.com/momja/Exhibit/internal/ogsnap"
	"github.com/momja/Exhibit/internal/render"
)

// Link-preview snapshots (av-rclm).
//
// A live share's og:image is a picture of its widget, drawn by a headless
// Chromium the operator runs beside the service — never inside the image,
// which would gain several hundred megabytes for a nicety (technical_stack.md
// §8). OG_SNAPSHOT_CDP_URL names that browser's DevTools endpoint. Unset, the
// default, means there is no browser: every preview shows the generic card,
// and nothing else about sharing changes.
//
// The browser must be able to reach RENDER_ORIGIN, and should be able to reach
// nothing it does not need to: ogsnap refuses every request off the render
// origin, but a browser that cannot route to the operator's internal network
// in the first place is the better wall.
const envPreviewSnapshotCDPURL = "OG_SNAPSHOT_CDP_URL"

// PreviewSnapshotterFromEnv reads OG_SNAPSHOT_CDP_URL. A value that is not a
// ws(s) or http(s) URL with a host is an error the caller makes fatal: the
// operator asked for previews, and silently showing the generic card instead
// would leave them looking for a fault in the browser.
func PreviewSnapshotterFromEnv(renderOrigin string) (render.Snapshotter, error) {
	raw := strings.TrimSpace(os.Getenv(envPreviewSnapshotCDPURL))
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%s: %q is not a URL with a host", envPreviewSnapshotCDPURL, raw)
	}
	switch u.Scheme {
	case "ws", "wss", "http", "https":
	default:
		return nil, fmt.Errorf("%s: %q must be a ws://, wss://, http:// or https:// DevTools endpoint", envPreviewSnapshotCDPURL, raw)
	}
	return ogsnap.NewChromium(raw, renderOrigin), nil
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

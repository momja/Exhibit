package render

import (
	"context"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/ogsnap"
	"github.com/momja/Exhibit/internal/store"
	"golang.org/x/sync/singleflight"
)

// Link previews for shares (av-rclm).
//
// A chat app that unfurls a pasted /s/:id fetches the URL with no credential
// and reads OpenGraph tags out of the <head>. Those tags are therefore the
// share link's own claim about what it shows, and they follow the same rule
// as everything else on the share route: the anonymous link row is the whole
// authorization. A live link gets the artifact's title and a picture of its
// widget; anything else — a grant's id, a revoked link, an id never issued —
// gets Exhibit's name and logo and nothing that came from an artifact, and
// gets the same bytes in all three cases, so a preview is no oracle over ids.
//
// The picture is of the *widget*, through the shared widget document, never of
// the full artifact. The widget is the face the owner chose for the artifact
// on their shelf, the share already publishes it at /s/:id/widget, and a tile
// is drawn for exactly this size. The full tool is an interactive document
// whose first screen may be a form, a splash, or whatever the owner was in the
// middle of.

// Snapshotter renders a page on the render origin into a PNG of a widget
// tile's shape. *ogsnap.Chromium is the production implementation; it is an
// interface so the render surface does not need a browser to be tested.
type Snapshotter interface {
	Snapshot(ctx context.Context, pageURL string) ([]byte, error)
}

const (
	// previewDescription is the same for every artifact on purpose. The title
	// is the one piece of artifact text a preview carries: the owner named the
	// thing they shared, and nothing else in the artifact was written to be
	// read aloud by a chat client.
	previewDescription = "An interactive tool, shared from Exhibit."
	// genericDescription describes the product, for a link that resolves to
	// nothing and must say nothing about why.
	genericDescription = "A library for self-contained web tools."
)

// previewTags is the <head> markup a live share link carries: the artifact's
// title, the widget snapshot, and where both live. Every value is escaped, the
// title above all, since it is the owner's (or a fetched page's) own text.
func (rd *Renderer) previewTags(a *store.Artifact, shareID string) string {
	title := strings.TrimSpace(a.Title)
	if title == "" {
		title = "Untitled artifact"
	}
	shareURL := rd.cfg.RenderOrigin + "/s/" + shareID
	return metaTags([][2]string{
		{"og:type", "website"},
		{"og:site_name", "Exhibit"},
		{"og:title", title},
		{"og:description", previewDescription},
		{"og:url", shareURL},
		{"og:image", shareURL + "/og.png"},
		{"og:image:type", "image/png"},
		{"og:image:width", strconv.Itoa(ogsnap.ImageWidth)},
		{"og:image:height", strconv.Itoa(ogsnap.ImageHeight)},
		{"og:image:alt", "A preview of " + title},
		{"twitter:card", "summary_large_image"},
	})
}

// genericTags is what a link that resolves to no live share carries: the
// product, not any artifact.
func (rd *Renderer) genericTags() string {
	tags := [][2]string{
		{"og:type", "website"},
		{"og:site_name", "Exhibit"},
		{"og:title", "Exhibit"},
		{"og:description", genericDescription},
	}
	if len(rd.cfg.PreviewImagePNG) > 0 {
		tags = append(tags,
			[2]string{"og:image", rd.cfg.RenderOrigin + "/og.png"},
			[2]string{"og:image:type", "image/png"},
			[2]string{"og:image:width", strconv.Itoa(ogsnap.ImageWidth)},
			[2]string{"og:image:height", strconv.Itoa(ogsnap.ImageHeight)},
			[2]string{"og:image:alt", "The Exhibit logo"},
			[2]string{"twitter:card", "summary_large_image"},
		)
	}
	return metaTags(tags)
}

// metaTags writes one <meta> per pair. OpenGraph keys are `property`
// attributes and Twitter's are `name`, which is the one distinction the two
// vocabularies insist on.
func metaTags(tags [][2]string) string {
	var b strings.Builder
	for _, kv := range tags {
		attr := "property"
		if strings.HasPrefix(kv[0], "twitter:") {
			attr = "name"
		}
		b.WriteString(`<meta ` + attr + `="` + html.EscapeString(kv[0]) + `" content="` + html.EscapeString(kv[1]) + `">` + "\n")
	}
	return b.String()
}

// serveShareNotFound answers /s/:id for every id that is not a live link. It
// is a 404, as it always was, and identical for every such id; what changed is
// that its <head> names Exhibit, so a preview of a revoked link shows the
// product rather than a blank card.
func (rd *Renderer) serveShareNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("<!doctype html>\n<html><head><meta charset=\"utf-8\">\n<title>Exhibit</title>\n" +
		rd.genericTags() +
		"</head><body><p>This link is not available.</p></body></html>\n"))
}

// ServeSharePreviewImage serves the PNG a live share link's og:image names:
// its widget, snapshotted, or the generic card when the instance runs no
// snapshot browser or the snapshot failed. Any id that is not a live link is
// the plain 404 every share sub-route answers.
//
// The share row is resolved before the cache is consulted, never after, so a
// revoked link stops yielding its picture on the very next request however
// recently it was taken. no-store asks the crawler for the same; what a chat
// app already unfurled is out of anybody's reach.
func (rd *Renderer) ServeSharePreviewImage(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "shareID")
	if _, ok := rd.resolveShare(w, r, shareID); !ok {
		return
	}
	png := rd.previews.get(r.Context(), shareID, rd.cfg.RenderOrigin+"/s/"+shareID+"/widget")
	if png == nil {
		png = rd.cfg.PreviewImagePNG
	}
	if len(png) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// ServePreviewImage serves the generic card at /og.png: Exhibit's logo, the
// same for every visitor and every link, so cacheable like the favicon.
func (rd *Renderer) ServePreviewImage(w http.ResponseWriter, r *http.Request) {
	if len(rd.cfg.PreviewImagePNG) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(rd.cfg.PreviewImagePNG)
}

const (
	// previewTTL is how long one snapshot answers for its link. Long enough
	// that a link pasted into a busy channel, and unfurled by every client in
	// it, costs one browser render; short enough that the picture follows the
	// widget within minutes.
	previewTTL = 10 * time.Minute
	// previewConcurrency is how many snapshots may render at once. Holding a
	// public link must not be a way to drive the operator's browser harder
	// than this, whatever the request rate.
	previewConcurrency = 2
	// previewCacheEntries bounds memory: a few hundred PNGs of a tile.
	previewCacheEntries = 256
)

// previewCache memoizes snapshots per share id. Concurrent requests for one id
// share a single render, renders are bounded, and a failure is remembered as
// briefly as a success, so a link whose widget cannot be drawn is not retried
// on every unfurl.
type previewCache struct {
	snap  Snapshotter
	slots chan struct{}
	group singleflight.Group
	now   func() time.Time

	mu      sync.Mutex
	entries map[string]previewEntry
}

type previewEntry struct {
	png []byte // nil when the render failed
	at  time.Time
}

func newPreviewCache(snap Snapshotter) *previewCache {
	return &previewCache{
		snap:    snap,
		slots:   make(chan struct{}, previewConcurrency),
		now:     time.Now,
		entries: map[string]previewEntry{},
	}
}

// get returns the snapshot of pageURL for key, or nil when there is no
// snapshotter or the render failed — the caller's cue for the generic card.
func (c *previewCache) get(ctx context.Context, key, pageURL string) []byte {
	if c.snap == nil {
		return nil
	}
	c.mu.Lock()
	e, ok := c.entries[key]
	c.mu.Unlock()
	if ok && c.now().Sub(e.at) < previewTTL {
		return e.png
	}

	v, _, _ := c.group.Do(key, func() (any, error) {
		// One render serves every waiter, so it must not die with whichever
		// request happened to start it. The snapshotter bounds its own time.
		png, err := c.render(context.WithoutCancel(ctx), pageURL)
		if err != nil {
			slog.WarnContext(ctx, "share preview snapshot failed",
				slog.String("err", err.Error()))
		}
		c.store(key, png)
		return png, nil
	})
	png, _ := v.([]byte)
	return png
}

func (c *previewCache) render(ctx context.Context, pageURL string) ([]byte, error) {
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-time.After(30 * time.Second):
		return nil, errors.New("no snapshot slot free")
	}
	return c.snap.Snapshot(ctx, pageURL)
}

func (c *previewCache) store(key string, png []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for k, e := range c.entries {
		if now.Sub(e.at) >= previewTTL {
			delete(c.entries, k)
		}
	}
	for k := range c.entries {
		if len(c.entries) < previewCacheEntries {
			break
		}
		delete(c.entries, k)
	}
	c.entries[key] = previewEntry{png: png, at: now}
}

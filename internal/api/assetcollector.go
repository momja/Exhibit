package api

// The ingest-side half of out-of-line markup assets (av-oz40).
//
// The markup walker has to rewrite each reference as it goes — an `<img src>`
// is not fetch-loaded, so unlike the runtime pass there is nothing to intercept
// at render — which means it needs the asset's final URL while it is still
// walking, before anything has been written to the database. This collector
// supplies that: it mints the id, remembers the bytes, and hands back the URL
// the document should carry. Persistence happens later, once the artifact row
// those assets hang off actually exists.

import (
	"sync"

	"github.com/momja/Exhibit/internal/snapshot"
	"github.com/momja/Exhibit/internal/store"
)

// assetCollector mints asset URLs during a snapshot walk and accumulates the
// bytes for the caller to store afterwards.
type assetCollector struct {
	renderOrigin string
	artifactID   string

	mu     sync.Mutex
	byURL  map[string]string // source URL -> minted asset URL
	assets []snapshot.RuntimeAsset
}

func newAssetCollector(renderOrigin, artifactID string) *assetCollector {
	return &assetCollector{
		renderOrigin: renderOrigin,
		artifactID:   artifactID,
		byURL:        map[string]string{},
	}
}

// sink is the snapshot.AssetSink the markup walker calls. Returning an error
// makes the walker keep the original reference, so a failure here degrades to
// the un-vendored page rather than to a broken one.
//
// The same source URL always yields the same asset URL: a page that shows one
// image in three places is one stored asset referenced three times, not three
// copies. The fetcher already dedupes the *fetch*; this dedupes the *storage*.
func (c *assetCollector) sink(a snapshot.RuntimeAsset) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.byURL[a.SourceURL]; ok {
		return existing, nil
	}
	id, err := store.NewAssetID()
	if err != nil {
		return "", err
	}
	url := c.renderOrigin + "/a/" + c.artifactID + "/assets/" + id
	c.byURL[a.SourceURL] = url

	a.AssetID = id
	c.assets = append(c.assets, a)
	return url, nil
}

// merge returns everything to persist: the markup assets this collector minted
// ids for, followed by the runtime payloads, which take their ids later because
// nothing in the document refers to them by id at all.
func (c *assetCollector) merge(runtime []snapshot.RuntimeAsset) []snapshot.RuntimeAsset {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append(append([]snapshot.RuntimeAsset{}, c.assets...), runtime...)
}

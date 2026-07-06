package snapshot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testFetcher builds a Fetcher with the dial guard disabled so tests can
// reach loopback httptest servers. Guard behavior itself is covered by
// TestFetchBlocksLoopback / TestPubliclyRoutable.
func testFetcher(t *testing.T, base string, limits Limits) *Fetcher {
	t.Helper()
	f, err := newFetcher(base, limits, nil)
	require.NoError(t, err)
	return f
}

func fetchErr(t *testing.T, err error) *FetchError {
	t.Helper()
	require.Error(t, err)
	var fe *FetchError
	require.ErrorAs(t, err, &fe)
	return fe
}

func TestNewFetcherRejectsBadBase(t *testing.T) {
	for _, base := range []string{"", "not a url\x7f", "ftp://example.com/x", "/just/a/path", "data:text/html,hi"} {
		_, err := NewFetcher(base, DefaultLimits())
		assert.Error(t, err, "base %q", base)
	}
}

func TestResolve(t *testing.T) {
	f := testFetcher(t, "https://example.com/tools/page.html", DefaultLimits())

	tests := []struct {
		name     string
		ref      string
		expected string
	}{
		{"relative", "js/app.js", "https://example.com/tools/js/app.js"},
		{"relative with dot", "./js/app.js", "https://example.com/tools/js/app.js"},
		{"relative with parent", "../shared/x.css", "https://example.com/shared/x.css"},
		{"root-relative", "/assets/x.png", "https://example.com/assets/x.png"},
		{"protocol-relative", "//cdn.example.com/lib.js", "https://cdn.example.com/lib.js"},
		{"absolute", "http://other.example.com/a.js", "http://other.example.com/a.js"},
		{"query preserved", "img.png?v=2", "https://example.com/tools/img.png?v=2"},
		{"fragment stripped", "style.css#section", "https://example.com/tools/style.css"},
		{"whitespace trimmed", "  a.css  ", "https://example.com/tools/a.css"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := f.Resolve(tt.ref)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, resolved)
		})
	}

	for _, ref := range []string{"", "data:image/png;base64,AAAA", "javascript:alert(1)", "mailto:a@b.c"} {
		t.Run("rejects "+ref, func(t *testing.T) {
			_, err := f.Resolve(ref)
			assert.Equal(t, ErrBadRef, fetchErr(t, err).Kind)
		})
	}
}

func TestFetchAndDedupe(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		fmt.Fprint(w, "body { color: red }")
	}))
	defer srv.Close()

	f := testFetcher(t, srv.URL+"/dir/page.html", DefaultLimits())

	asset, err := f.Fetch(context.Background(), "a.css")
	require.NoError(t, err)
	assert.Equal(t, srv.URL+"/dir/a.css", asset.URL)
	assert.Equal(t, "text/css; charset=utf-8", asset.ContentType)
	assert.Equal(t, "body { color: red }", string(asset.Body))

	// Different spellings of the same URL are served from cache.
	for _, ref := range []string{"a.css", "./a.css", "/dir/a.css", "a.css#frag"} {
		again, err := f.Fetch(context.Background(), ref)
		require.NoError(t, err)
		assert.Same(t, asset, again, "ref %q", ref)
	}
	assert.Equal(t, 1, hits)
}

func TestFetchCachesHTTPErrors(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	defer srv.Close()

	f := testFetcher(t, srv.URL+"/page.html", DefaultLimits())

	_, err := f.Fetch(context.Background(), "missing.css")
	assert.Equal(t, ErrHTTPStatus, fetchErr(t, err).Kind)

	// The failure is remembered per-URL; the ref in the error tracks the caller.
	_, err = f.Fetch(context.Background(), "./missing.css")
	fe := fetchErr(t, err)
	assert.Equal(t, ErrHTTPStatus, fe.Kind)
	assert.Equal(t, "./missing.css", fe.Ref)
	assert.Equal(t, 1, hits)
}

func TestFetchPerAssetSizeCap(t *testing.T) {
	big := strings.Repeat("x", 2048)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chunked" {
			// Suppress Content-Length so the cap is enforced while reading.
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, big)
	}))
	defer srv.Close()

	f := testFetcher(t, srv.URL+"/page.html", Limits{MaxAssetBytes: 1024})

	// Declared via Content-Length: rejected before reading the body.
	_, err := f.Fetch(context.Background(), "declared")
	assert.Equal(t, ErrTooLarge, fetchErr(t, err).Kind)

	// No Content-Length: rejected once the read passes the cap.
	_, err = f.Fetch(context.Background(), "chunked")
	assert.Equal(t, ErrTooLarge, fetchErr(t, err).Kind)
}

func TestFetchTotalByteBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Repeat("x", 600))
	}))
	defer srv.Close()

	f := testFetcher(t, srv.URL+"/page.html", Limits{MaxTotalBytes: 1000})

	_, err := f.Fetch(context.Background(), "a.bin")
	require.NoError(t, err)

	_, err = f.Fetch(context.Background(), "b.bin")
	assert.Equal(t, ErrBudget, fetchErr(t, err).Kind)

	// Budget failure poisons neither the cache nor already-fetched assets.
	cachedAsset, err := f.Fetch(context.Background(), "a.bin")
	require.NoError(t, err)
	assert.Len(t, cachedAsset.Body, 600)
}

func TestFetchAssetCountCap(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	f := testFetcher(t, srv.URL+"/page.html", Limits{MaxAssets: 2})

	for _, ref := range []string{"a.js", "b.js"} {
		_, err := f.Fetch(context.Background(), ref)
		require.NoError(t, err)
	}
	_, err := f.Fetch(context.Background(), "c.js")
	assert.Equal(t, ErrBudget, fetchErr(t, err).Kind)
	assert.Equal(t, 2, hits, "over-cap fetch must not reach the network")
}

func TestFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	f := testFetcher(t, srv.URL+"/page.html", Limits{Timeout: 50 * time.Millisecond})

	_, err := f.Fetch(context.Background(), "slow.js")
	assert.Equal(t, ErrNetwork, fetchErr(t, err).Kind)
}

func TestFetchRedirectLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer srv.Close()

	f := testFetcher(t, srv.URL+"/page.html", Limits{MaxRedirects: 3})

	_, err := f.Fetch(context.Background(), "loop")
	assert.Equal(t, ErrRedirect, fetchErr(t, err).Kind)
}

func TestFetchFailureDoesNotAbortRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad.css" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	f := testFetcher(t, srv.URL+"/page.html", DefaultLimits())

	_, err := f.Fetch(context.Background(), "bad.css")
	assert.Equal(t, ErrHTTPStatus, fetchErr(t, err).Kind)

	asset, err := f.Fetch(context.Background(), "good.css")
	require.NoError(t, err)
	assert.Equal(t, "ok", string(asset.Body))
}

// TestFetchBlocksLoopback exercises the real dial guard end to end: the
// httptest server lives on 127.0.0.1, exactly the kind of address an
// untrusted document must not be able to make the server fetch. The guard
// runs at dial time on the literal connected address, so the same rejection
// applies to redirect targets and every resolved DNS answer.
func TestFetchBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("guarded fetch must never reach the server")
	}))
	defer srv.Close()

	f, err := NewFetcher(srv.URL+"/page.html", DefaultLimits())
	require.NoError(t, err)

	_, err = f.Fetch(context.Background(), "secret.txt")
	assert.Equal(t, ErrBlockedAddr, fetchErr(t, err).Kind)
}

func TestPubliclyRoutable(t *testing.T) {
	tests := []struct {
		addr    string
		allowed bool
	}{
		{"127.0.0.1", false},
		{"::1", false},
		{"10.1.2.3", false},
		{"172.16.0.1", false},
		{"172.31.255.255", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false}, // cloud metadata endpoint
		{"fe80::1", false},
		{"fd00::1", false},
		{"0.0.0.0", false},
		{"255.255.255.255", false},
		{"224.0.0.1", false},
		{"::ffff:10.0.0.1", false}, // IPv4-mapped private
		{"::ffff:127.0.0.1", false},
		{"93.184.216.34", true},
		{"172.32.0.1", true}, // just past the RFC 1918 172.16/12 block
		{"2606:4700::1111", true},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			assert.Equal(t, tt.allowed, publiclyRoutable(netip.MustParseAddr(tt.addr)))
		})
	}
}

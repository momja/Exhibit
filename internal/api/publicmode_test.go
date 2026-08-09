package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPublicModeRouter is newTestRouter with a public-mode configuration
// supplied, so a test can vary that one field and hold everything else equal.
func newPublicModeRouter(t *testing.T, public PublicMode) *Router {
	t.Helper()

	f, err := os.CreateTemp("", "test-public-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	st, err := store.OpenSQLite(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })

	blobDir, err := os.MkdirTemp("", "test-public-blobs-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(blobDir) })

	bl, err := blob.NewFSStore(blobDir)
	require.NoError(t, err)

	box, err := secrets.Load("test-secret", "")
	require.NoError(t, err)

	return NewRouter(Config{
		Store:        st,
		Blob:         bl,
		AppOrigin:    "http://app.test",
		RenderOrigin: "http://render.test",
		AuthToken:    "secret",
		Secrets:      box,
		Public:       public,
	})
}

func getPublicSettings(t *testing.T, r *Router, header string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/settings/public", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The endpoint's reason for existing: an anonymous visitor can read the
// instance's name without a credential.
func TestPublicSettingsUnauthenticated(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{
		Enabled:     true,
		Name:        "Max's Exhibit",
		Description: "A shelf of small tools.",
		OwnerID:     defaultOwnerID,
	})

	w := getPublicSettings(t, r, "")
	require.Equal(t, http.StatusOK, w.Code)

	var got publicSettingsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "Max's Exhibit", got.Name)
	assert.Equal(t, "A shelf of small tools.", got.Description)

	// The response says what the instance calls itself and nothing else — in
	// particular not which owner it publishes.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.ElementsMatch(t, []string{"name", "description"}, keysOf(raw))
}

// Public but unnamed is a real state, and it answers 200 with empty strings —
// which is exactly why "not public" cannot also answer 200.
func TestPublicSettingsEnabledButUnset(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{Enabled: true})

	w := getPublicSettings(t, r, "")
	require.Equal(t, http.StatusOK, w.Code)

	var got publicSettingsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "", got.Name)
	assert.Equal(t, "", got.Description)
}

// A private instance does not name itself — to a stranger or to its own
// operator. The route is indistinguishable from one that was never registered.
func TestPublicSettingsDisabled404s(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{
		Name:        "Not published",
		Description: "Nor is this.",
	})

	for _, header := range []string{"", authHeader()} {
		w := getPublicSettings(t, r, header)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.NotContains(t, w.Body.String(), "Not published")
	}
}

// The claim this ticket has to make good on: configuration alone changes no
// authentication behaviour. With public mode off, every authenticated route
// still rejects a request with no credential and still accepts one with the
// token — as TestAuthMiddleware asserts for the default router, held here
// against a router that merely knows about public mode.
func TestPublicModeOffLeavesAuthUnchanged(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{})

	authed := []string{
		"/api/artifacts",
		"/api/collections",
		"/api/tags",
		"/api/agent/key",
	}
	for _, path := range authed {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "%s must still require auth", path)

		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", authHeader())
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusUnauthorized, w.Code, "%s must still accept the token", path)
	}
}

// Enabling public mode opens exactly two reads and nothing else (av-wmp6).
// This was TestPublicModeOnStillGuardsTheAPI, which asserted that turning
// public mode on changed nothing at all: av-wmp6 is the change that makes the
// read half of that false, so the read assertion is gone and everything it
// covered besides is spelled out here instead. A mutation is refused whatever
// the configuration says, and so is a GET of anything but the library.
func TestPublicModeOnStillGuardsWrites(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{Enabled: true, Name: "Public"})
	id := seedPublicArtifact(t, r, defaultOwnerID)

	// AC#3: every mutating method, including on the routes whose GET is now
	// public. The method is what decides, not the path.
	writes := []struct{ method, path string }{
		{http.MethodPost, "/api/artifacts"},
		{http.MethodPatch, "/api/artifacts/" + id},
		{http.MethodDelete, "/api/artifacts/" + id},
		{http.MethodPost, "/api/artifacts/" + id + "/refetch"},
		{http.MethodPut, "/api/artifacts/" + id + "/state"},
		{http.MethodDelete, "/api/artifacts/" + id + "/state"},
		{http.MethodPut, "/api/artifacts/" + id + "/widget"},
		{http.MethodDelete, "/api/artifacts/" + id + "/widget"},
		{http.MethodPost, "/api/artifacts/" + id + "/widget/generate"},
		{http.MethodPost, "/api/collections"},
		{http.MethodPost, "/api/tags"},
		{http.MethodPost, "/api/shares"},
		{http.MethodPut, "/api/agent/key"},
	}
	for _, tc := range writes {
		w := anon(t, r, tc.method, tc.path, []byte(`{}`))
		assert.Equal(t, http.StatusUnauthorized, w.Code,
			"%s %s must stay authenticated in public mode", tc.method, tc.path)
	}

	// And the reads that are not the library. A public instance publishes the
	// artifacts; it does not publish the owner's state, their agent
	// conversations, or the rest of their account.
	reads := []string{
		"/api/artifacts/" + id + "/state",
		"/api/artifacts/" + id + "/widget",
		"/api/artifacts/" + id + "/transcripts",
		"/api/collections",
		"/api/tags",
		"/api/agent/key",
	}
	for _, path := range reads {
		w := anon(t, r, http.MethodGet, path, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code,
			"GET %s must stay authenticated in public mode", path)
	}
}

// AC#1 and AC#2 — the point of the ticket. A visitor with no credential reads
// the published library and one artifact in it.
func TestPublicModeOpensTheLibraryToAnonymousReads(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{Enabled: true, OwnerID: defaultOwnerID})
	id := seedPublicArtifact(t, r, defaultOwnerID)

	list := anon(t, r, http.MethodGet, "/api/artifacts", nil)
	require.Equal(t, http.StatusOK, list.Code)
	assert.Contains(t, list.Body.String(), id)

	one := anon(t, r, http.MethodGet, "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, one.Code)
	assert.Contains(t, one.Body.String(), "A published tool")
}

// The same two requests against a private instance, which is AC#4 stated where
// it is easiest to get wrong: the routes public mode opens are the ones that
// must stay shut without it.
func TestPublicReadsAreClosedWhenPublicModeIsOff(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{})
	id := seedPublicArtifact(t, r, defaultOwnerID)

	for _, path := range []string{"/api/artifacts", "/api/artifacts/" + id} {
		w := anon(t, r, http.MethodGet, path, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "GET %s", path)
	}
}

// Which library a public instance publishes is a configured answer, not "the
// only one there is" (av-4ac9's PUBLIC_OWNER_ID). Owner scoping is a real query
// predicate since av-ep8k, so an anonymous read that resolved to the default
// owner would show a stranger the wrong tenant's shelf — or, on a hosted
// instance, somebody's private one.
func TestPublicReadsResolveTheConfiguredOwner(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{Enabled: true, OwnerID: otherOwner})
	published := seedPublicArtifact(t, r, otherOwner)
	private := seedPublicArtifact(t, r, defaultOwnerID)

	w := anon(t, r, http.MethodGet, "/api/artifacts", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), published)
	assert.NotContains(t, w.Body.String(), private,
		"a public instance publishes PUBLIC_OWNER_ID's library and no other")

	// And the artifact route agrees: the unpublished owner's artifact is as
	// absent as one that never existed.
	assert.Equal(t, http.StatusNotFound, anon(t, r, http.MethodGet, "/api/artifacts/"+private, nil).Code)
	assert.Equal(t, http.StatusOK, anon(t, r, http.MethodGet, "/api/artifacts/"+published, nil).Code)
}

// AC#5. The flag is what lets a handler render a page with no edit controls and
// mint render tokens that carry no principal, so both halves are asserted: an
// anonymous read is marked and resolves to the published owner, and a
// credentialed request on the very same instance is not marked at all — the
// operator browsing their own public library is still themselves.
func TestPublicVisitorIsMarkedInTheRequestContext(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{Enabled: true, OwnerID: otherOwner})

	var marked bool
	var owner int64
	handler := r.authMiddleware(r.ownerMiddleware(http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			marked = publicVisitor(req.Context())
			owner = ownerIDFromCtx(req.Context())
		})))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/artifacts", nil))
	assert.True(t, marked, "an anonymous public read must be marked for handlers to branch on")
	assert.Equal(t, otherOwner, owner)

	authed := httptest.NewRequest(http.MethodGet, "/api/artifacts", nil)
	authed.Header.Set("Authorization", authHeader())
	handler.ServeHTTP(httptest.NewRecorder(), authed)
	assert.False(t, marked, "a credentialed request is not a public visitor")
	assert.Equal(t, defaultOwnerID, owner)
}

// The privacy half of the ticket, at the seam where it is decided: every render
// URL a public visitor's page points a frame at is minted for nobody, so the
// render surface inlines no state into it (the render side of the same fact is
// internal/render/public_render_test.go). An authenticated render of the same
// artifact still names its owner.
func TestPublicVisitorsRenderURLsCarryNoPrincipal(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{Enabled: true, OwnerID: defaultOwnerID})

	// Both requests are run through ownerMiddleware rather than hand-built,
	// because that is what decides an owner: PUBLIC_OWNER_ID for a public
	// visitor, the single-user default for anyone else. ownerIDFromCtx no
	// longer guesses one for a context that never met the chain (av-syug), so
	// a bare request would only assert that nobody is nobody.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	visitor := r.renderURLs(withOwnerResolved(r,
		req.WithContext(context.WithValue(req.Context(), publicVisitorKey, true))))
	owner := r.renderURLs(withOwnerResolved(r, req))

	for name, u := range map[string]string{"artifact": visitor.artifact("abc"), "widget": visitor.widget("abc")} {
		claims, err := r.tokens.Verify(tokenOf(t, u), "abc")
		require.NoError(t, err, "%s URL must still carry a valid token", name)
		assert.True(t, claims.Anonymous, "%s URL for a public visitor must render for nobody", name)
		assert.Equal(t, defaultOwnerID, claims.OwnerID, "%s URL must still name the owner it renders", name)
	}

	claims, err := r.tokens.Verify(tokenOf(t, owner.artifact("abc")), "abc")
	require.NoError(t, err)
	assert.False(t, claims.Anonymous, "an ordinary render must still be rendered for its owner")
}

// withOwnerResolved runs req through ownerMiddleware and returns it as the
// handler behind that middleware would receive it — with the owner decided.
func withOwnerResolved(ro *Router, req *http.Request) *http.Request {
	var resolved *http.Request
	ro.ownerMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		resolved = r
	})).ServeHTTP(httptest.NewRecorder(), req)
	return resolved
}

// tokenOf pulls the render token back out of a minted URL.
func tokenOf(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return u.Query().Get(rendertoken.Param)
}

// anon issues a request with no credential of any kind — the public visitor's
// request, and the one every assertion in this file turns on.
func anon(t *testing.T, r *Router, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// seedPublicArtifact puts an artifact directly into the store under a named
// owner. It goes in through the Store rather than the API because the owner is
// the variable under test, and the API only ever creates for the request's own.
func seedPublicArtifact(t *testing.T, r *Router, owner int64) string {
	t.Helper()
	ctx := context.Background()
	id := fmt.Sprintf("artifact-of-owner-%d", owner)
	blobID := id + "-blob"
	require.NoError(t, r.cfg.Blob.Put(ctx, blobID, strings.NewReader("<html><body>published</body></html>")))
	require.NoError(t, r.cfg.Store.PutArtifact(ctx, &store.Artifact{
		ID: id, OwnerID: owner, Title: "A published tool",
		SourceBlobID: blobID, Tier: store.Tier1,
	}))
	return id
}

func TestPublicModeFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want PublicMode
	}{
		{
			name: "unset is a private instance owned by the default owner",
			want: PublicMode{OwnerID: defaultOwnerID},
		},
		{
			name: "fully configured",
			env: map[string]string{
				envPublicModeEnabled:         "true",
				envPublicInstanceName:        "Max's Exhibit",
				envPublicInstanceDescription: "A shelf of small tools.",
				envPublicOwnerID:             "7",
			},
			want: PublicMode{
				Enabled:     true,
				Name:        "Max's Exhibit",
				Description: "A shelf of small tools.",
				OwnerID:     7,
			},
		},
		{
			name: "1 enables",
			env:  map[string]string{envPublicModeEnabled: "1"},
			want: PublicMode{Enabled: true, OwnerID: defaultOwnerID},
		},
		{
			name: "yes enables",
			env:  map[string]string{envPublicModeEnabled: "yes"},
			want: PublicMode{Enabled: true, OwnerID: defaultOwnerID},
		},
		{
			name: "on enables, case-insensitively",
			env:  map[string]string{envPublicModeEnabled: "ON"},
			want: PublicMode{Enabled: true, OwnerID: defaultOwnerID},
		},
		{
			name: "off disables",
			env:  map[string]string{envPublicModeEnabled: "off"},
			want: PublicMode{OwnerID: defaultOwnerID},
		},
		// The misreading a "any non-empty value" rule would make, and the
		// reason this knob does not use one.
		{
			name: "the word false disables",
			env:  map[string]string{envPublicModeEnabled: "false"},
			want: PublicMode{OwnerID: defaultOwnerID},
		},
		{
			name: "nonsense fails closed",
			env:  map[string]string{envPublicModeEnabled: "maybe"},
			want: PublicMode{OwnerID: defaultOwnerID},
		},
		{
			name: "an unusable owner id falls back rather than failing the boot",
			env:  map[string]string{envPublicOwnerID: "not-a-number"},
			want: PublicMode{OwnerID: defaultOwnerID},
		},
		{
			name: "so does a nonsensical one",
			env:  map[string]string{envPublicOwnerID: "0"},
			want: PublicMode{OwnerID: defaultOwnerID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{
				envPublicModeEnabled, envPublicInstanceName,
				envPublicInstanceDescription, envPublicOwnerID,
			} {
				t.Setenv(key, tt.env[key])
			}
			assert.Equal(t, tt.want, PublicModeFromEnv())
		})
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

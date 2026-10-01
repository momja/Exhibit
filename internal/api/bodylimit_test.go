package api

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testBodyLimit is small so the over-limit bodies these tests send are cheap.
// What is being tested is the mechanism, which does not care about the number;
// the default's own boundary is pinned separately below.
const testBodyLimit = 4 << 10

func TestMaxRequestBodyBytesFromEnv(t *testing.T) {
	t.Run("unset is the default, never unbounded", func(t *testing.T) {
		t.Setenv("MAX_REQUEST_BODY_BYTES", "")
		n, err := MaxRequestBodyBytesFromEnv()
		require.NoError(t, err)
		assert.Equal(t, DefaultMaxRequestBodyBytes, n)
		assert.Equal(t, int64(32<<20), n, "32 MiB; the measurements behind it are in bodylimit.go")
	})
	t.Run("a number of bytes is the limit", func(t *testing.T) {
		t.Setenv("MAX_REQUEST_BODY_BYTES", " 1048576 ")
		n, err := MaxRequestBodyBytesFromEnv()
		require.NoError(t, err)
		assert.Equal(t, int64(1<<20), n)
	})
	for _, bad := range []string{"0", "-1", "32MiB", "lots", "1.5"} {
		t.Run("rejects "+bad, func(t *testing.T) {
			t.Setenv("MAX_REQUEST_BODY_BYTES", bad)
			_, err := MaxRequestBodyBytesFromEnv()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "MAX_REQUEST_BODY_BYTES", "the failure names the variable that would fix it")
		})
	}
}

// writeRoutes is every route on the app surface that takes a request body,
// meaning every POST, PUT, PATCH and DELETE. TestEveryWriteRouteRefusesAnOversizedBody
// fails when the router's set and this one differ. So a new write route cannot
// land without somebody deciding, here, what it does with a body.
//
// The limit is the same for all of them, and that is the decision this table
// records rather than an oversight it hides: limitRequestBody sits on the root
// of the router, so no route can be registered outside it. The per-route
// decision is the one in readsBody: whether the handler reads the body before
// refusing for any other reason, given the fixtures seedBodyLimitFixtures sets
// up. Those are the routes where an over-limit body of undeclared length
// actually reaches a decoder, and so where decodeJSON (or the login form's
// bodyReadStatus) must turn the overrun into a 413 rather than a 400.
var writeRoutes = []struct {
	method, route string
	readsBody     bool
}{
	{"POST", "/api/artifacts/", true},
	{"PATCH", "/api/artifacts/{artifactID}/", true},
	{"DELETE", "/api/artifacts/{artifactID}/", false},
	{"POST", "/api/artifacts/{artifactID}/refetch", false},
	{"PUT", "/api/artifacts/{artifactID}/state", true},
	{"DELETE", "/api/artifacts/{artifactID}/state", false},
	{"DELETE", "/api/artifacts/{artifactID}/assets/{assetID}", false},
	{"POST", "/api/artifacts/{artifactID}/origins", true},
	{"DELETE", "/api/artifacts/{artifactID}/origins", false},
	{"PUT", "/api/artifacts/{artifactID}/widget", true},
	{"DELETE", "/api/artifacts/{artifactID}/widget", false},
	{"POST", "/api/artifacts/{artifactID}/widget/generate", false},
	{"POST", "/api/artifacts/{artifactID}/collections/{collectionID}", false},
	{"DELETE", "/api/artifacts/{artifactID}/collections/{collectionID}", false},
	{"POST", "/api/artifacts/{artifactID}/tags/{tagID}", false},
	{"DELETE", "/api/artifacts/{artifactID}/tags/{tagID}", false},
	{"PUT", "/api/agent/key", true},
	{"DELETE", "/api/agent/key", false},
	{"POST", "/api/agent/sessions", true},
	{"POST", "/api/agent/sessions/{sessionID}/ticket", false},
	{"POST", "/api/agent/sessions/{sessionID}/prompt", false},
	{"POST", "/api/agent/sessions/{sessionID}/abort", false},
	{"DELETE", "/api/agent/sessions/{sessionID}", false},
	{"POST", "/api/collections/", true},
	{"POST", "/api/collections/{collectionID}/artifacts/{artifactID}", false},
	{"DELETE", "/api/collections/{collectionID}/artifacts/{artifactID}", false},
	{"POST", "/api/tags/", true},
	{"PATCH", "/api/tags/{tagID}", true},
	{"DELETE", "/api/tags/{tagID}", false},
	{"POST", "/api/tags/{tagID}/artifacts/{artifactID}", false},
	{"DELETE", "/api/tags/{tagID}/artifacts/{artifactID}", false},
	{"POST", "/api/admin/users/", true},
	{"PATCH", "/api/admin/users/{userID}", true},
	{"DELETE", "/api/account", false},
	{"POST", "/api/shares/", true},
	{"DELETE", "/api/shares/{shareID}", false},
	{"POST", "/auth/local", true},
	{"POST", "/auth/logout", false},
}

// bodyLimitRouter is the router these tests walk: the test router with a
// small limit, and with a local login configured so the /auth routes, which
// only exist on an instance that has one, are registered too.
func bodyLimitRouter(t *testing.T) *Router {
	t.Helper()
	ro := newTestRouter(t)
	cfg := ro.cfg
	cfg.MaxRequestBodyBytes = testBodyLimit
	cfg.LocalCredential = newTestCredential(t, "admin", "pw")
	return NewRouter(cfg)
}

func TestEveryWriteRouteRefusesAnOversizedBody(t *testing.T) {
	ro := bodyLimitRouter(t)

	var registered []string
	require.NoError(t, chi.Walk(ro.Mux, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		switch method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			return nil
		}
		// The embedded static files, mounted with Handle and so registered
		// for every method. A file server reads no body.
		if route == "/assets/*" {
			return nil
		}
		registered = append(registered, method+" "+route)
		return nil
	}))
	var listed []string
	for _, wr := range writeRoutes {
		listed = append(listed, wr.method+" "+wr.route)
	}
	sort.Strings(registered)
	sort.Strings(listed)
	require.Equal(t, listed, registered,
		"the router's write routes and writeRoutes differ: add the new route to the table "+
			"(bodylimit_test.go), deciding whether its handler reads a body")

	for _, wr := range writeRoutes {
		t.Run(wr.method+" "+wr.route, func(t *testing.T) {
			// A router per row, because a DELETE row really deletes and must
			// not leave the next row a 404 in place of a decoder.
			ro := bodyLimitRouter(t)
			path := seedBodyLimitFixtures(t, ro).Replace(wr.route)
			contentType := "application/json"
			if wr.route == "/auth/local" {
				// A form post: ParseForm reads no body of any other type.
				contentType = "application/x-www-form-urlencoded"
			}

			// Declared too large: refused before anything is read.
			declared := &countingBody{remaining: testBodyLimit + 1}
			rec := serveBody(ro, wr.method, path, contentType, declared, testBodyLimit+1)
			assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
			assert.Zero(t, declared.read, "a body declared over the limit is never read")
			var refusal bodyTooLargeResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refusal))
			assert.Equal(t, int64(testBodyLimit), refusal.LimitBytes)
			assert.Contains(t, refusal.Error, "4.0 KiB")

			// Undeclared (chunked): read no further than the limit, and a
			// handler that reads it answers 413.
			chunked := &countingBody{remaining: 16 * testBodyLimit}
			rec = serveBody(ro, wr.method, path, contentType, chunked, -1)
			assert.LessOrEqual(t, chunked.read, int64(testBodyLimit+1),
				"no route reads past the limit, whatever it does with the body")
			if wr.readsBody {
				assert.Positive(t, chunked.read, "this handler reads its body; if it no longer does, update readsBody")
				assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
			} else {
				assert.Zero(t, chunked.read,
					"this handler refuses or ignores the body without reading it; if it reads one now, set readsBody")
			}
		})
	}
}

func serveBody(ro *Router, method, path, contentType string, body io.Reader, contentLength int64) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	req.ContentLength = contentLength
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	ro.ServeHTTP(rec, req)
	return rec
}

// seedBodyLimitFixtures gives each route's path real ids to name, so a handler
// that looks its target up before reading the body finds one, and the chunked
// case reaches the decoder wherever it would in production.
func seedBodyLimitFixtures(t *testing.T, ro *Router) *strings.Replacer {
	t.Helper()
	return strings.NewReplacer(
		"{artifactID}", postForID(t, ro, "/api/artifacts", `{"title":"t","body":"<p>x</p>"}`, "artifact"),
		"{tagID}", postForID(t, ro, "/api/tags", `{"name":"t"}`, ""),
		"{collectionID}", postForID(t, ro, "/api/collections", `{"name":"c"}`, ""),
		"{userID}", "1",
		"{assetID}", "missing",
		"{sessionID}", "missing",
		"{shareID}", "missing",
	)
}

// postForID creates a fixture through the API and returns its id, read from
// the top level or from under nest when the response wraps the resource.
func postForID(t *testing.T, ro *Router, path, body, nest string) string {
	t.Helper()
	rec := serveBody(ro, http.MethodPost, path, "application/json", strings.NewReader(body), int64(len(body)))
	require.Less(t, rec.Code, 300, rec.Body.String())
	dec := json.NewDecoder(rec.Body)
	dec.UseNumber()
	var out map[string]any
	require.NoError(t, dec.Decode(&out))
	if nest != "" {
		out, _ = out[nest].(map[string]any)
	}
	switch id := out["id"].(type) {
	case string:
		return id
	case json.Number:
		return id.String()
	}
	t.Fatalf("no id in the response to POST %s", path)
	return ""
}

// countingBody is a request body that yields `remaining` bytes of a JSON
// object that never closes, so a decoder keeps reading for as long as it is
// allowed to, and counts what was taken from it.
type countingBody struct {
	remaining, read int64
	started         bool
}

func (b *countingBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > b.remaining {
		n = b.remaining
	}
	fill := p[:n]
	for i := range fill {
		fill[i] = 'a'
	}
	if !b.started {
		b.started = true
		copy(fill, `{"x":"`)
	}
	b.remaining -= n
	b.read += n
	return int(n), nil
}

// The default is what an instance that sets nothing enforces, and Config's
// zero value must mean it rather than "unbounded". Pinned at the boundary
// with a declared length, so the test sends a few bytes, not 32 MiB.
func TestTheDefaultLimitAppliesWhenNoneIsConfigured(t *testing.T) {
	ro := newTestRouter(t)
	for _, tc := range []struct {
		declared int64
		want     int
	}{
		{DefaultMaxRequestBodyBytes + 1, http.StatusRequestEntityTooLarge},
		// At the limit the body is let through to the handler, which finds it
		// short of what was declared: a 400, not a 413.
		{DefaultMaxRequestBodyBytes, http.StatusBadRequest},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/artifacts", strings.NewReader(`{"title":`))
		req.ContentLength = tc.declared
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		ro.ServeHTTP(rec, req)
		assert.Equal(t, tc.want, rec.Code, "declared %d bytes: %s", tc.declared, rec.Body.String())
	}
}

// The limit refuses what is over it and nothing else: a document just under
// it is stored whole.
func TestAWriteUnderTheLimitIsStored(t *testing.T) {
	ro := bodyLimitRouter(t)
	doc := "<p>" + strings.Repeat("a", testBodyLimit-100) + "</p>"
	payload, err := json.Marshal(map[string]string{"title": "t", "body": doc})
	require.NoError(t, err)
	require.LessOrEqual(t, len(payload), testBodyLimit)

	req := httptest.NewRequest(http.MethodPost, "/api/artifacts", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ro.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

// A malformed body is still a 400: decodeJSON only turns an overrun into a 413.
func TestDecodeJSONKeepsMalformedBodiesA400(t *testing.T) {
	ro := bodyLimitRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/tags", strings.NewReader(`{"name":`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	ro.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid request body")
}

// decodeJSON is what turns an overrun into a 413 for a body of undeclared
// length. A handler that read r.Body itself would answer such a body with
// whatever its own decode error path says (a 400 claiming the JSON was bad),
// and nothing would notice. So the rule is held by the parser: no code in this
// package outside bodylimit.go touches r.Body.
//
// It matches the shape the handlers use, a request named `r`. The login form
// reads its body through r.ParseForm instead, which this does not see; those
// two call sites use bodyReadStatus, and the route walk above covers them.
func TestOnlyDecodeJSONReadsARequestBody(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go") && fi.Name() != "bodylimit.go"
	}, 0)
	require.NoError(t, err)

	var offenders []string
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Body" {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "r" {
					offenders = append(offenders, fset.Position(sel.Pos()).String())
				}
				return true
			})
		}
	}
	assert.Empty(t, offenders,
		"read request bodies with decodeJSON (internal/api/bodylimit.go): it answers an "+
			"over-limit body 413, where a handler's own decode error path would say 400 (av-ombn)")
}

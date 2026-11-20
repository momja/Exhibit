package api

// Exports turn artifact AI off (Exh-i902): both the standalone HTML and the ZIP
// app bundle carry the unavailable window.exhibit.ai stand-in ahead of the
// artifact's own scripts, keep the artifact's AI call sites as written, and
// carry nothing that could reach an instance's inference — no endpoint, no
// credential, no INFERENCE_* configuration — even on an instance that has AI
// and relay connectors configured.

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/momja/Exhibit/internal/bundle"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/inference"
	"github.com/momja/Exhibit/internal/inference/standin"
	"github.com/momja/Exhibit/internal/store"
)

// aiArtifactScript is the artifact's own first script. It records what the
// AI API answered so the browser tests can read it back, and keeps a non-AI
// feature working beside it.
const aiArtifactScript = `<script>
window.aiResults = (async () => {
  const kinds = await window.exhibit.ai.available();
  const codes = {};
  try { await window.exhibit.ai.complete({ prompt: 'Summarize my notes' }); } catch (e) { codes.complete = e.code; }
  try { await window.exhibit.ai.decide({ question: 'Spam?', options: ['yes', 'no'] }); } catch (e) { codes.decide = e.code; }
  return { kinds, codes };
})();
window.counter = 1 + 1;
</script>`

const aiArtifactSource = `<!DOCTYPE html><html><head><title>AI notes</title>` + aiArtifactScript + `</head><body>notes</body></html>`

// Strings an export must never carry: the instance's AI configuration and
// credential, and the route the hosted bridge calls.
const (
	exportTestInferenceKey   = "sk-export-test-secret-8841"
	exportTestInferenceModel = "export-test-model-8841"
	exportTestConnectorKey   = "connector-secret-8841"
)

// newAIExportRouter is an instance with AI configured (platform key and
// models) and an unrelated relay connector granted to the artifact.
func newAIExportRouter(t *testing.T) (*Router, string) {
	t.Helper()
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	svc, err := inference.New(inference.Config{
		Platform:      &inference.Credential{Provider: "anthropic", APIKey: exportTestInferenceKey},
		ChatModel:     exportTestInferenceModel,
		DecisionModel: exportTestInferenceModel,
	})
	require.NoError(t, err)
	ro.cfg.Inference = svc

	_, resp := postArtifact(t, ro, map[string]any{"title": "AI notes", "body": aiArtifactSource, "network_allowlist": []string{}})
	id := resp.Artifact.ID
	require.NotEmpty(t, id)

	ctx := context.Background()
	const origin = "https://api.example.test"
	require.NoError(t, ro.cfg.Store.SetOriginDecision(ctx, 1, id, origin, store.DecisionAllow, "test"))
	ro.cfg.Connections = connectors.New(connectors.Config{CustomEnabled: true, Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	connection, err := ro.cfg.Connections.Create(ctx, 1, connectors.CreateRequest{Name: "Notes API", Recipe: "bearer", Origin: origin, Secret: exportTestConnectorKey}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = ro.cfg.Connections.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
	return ro, id
}

// zipFiles reads a relay bundle response into name (without the top
// directory) → contents.
func zipFiles(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	require.NoError(t, err)
	files := map[string]string{}
	for _, file := range zr.File {
		reader, err := file.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(reader)
		require.NoError(t, reader.Close())
		require.NoError(t, err)
		name := file.Name
		if i := strings.IndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		files[name] = string(body)
	}
	return files
}

func assertAIDisabledDocument(t *testing.T, doc string) {
	t.Helper()
	at := strings.Index(doc, standin.Tag)
	require.GreaterOrEqual(t, at, 0, "the stand-in is installed")
	assert.Equal(t, 1, strings.Count(doc, standin.Tag))
	firstScript := strings.Index(strings.Replace(doc, standin.Tag, strings.Repeat(" ", len(standin.Tag)), 1), "<script")
	if firstScript >= 0 {
		assert.Less(t, at, firstScript, "the stand-in precedes every other script")
	}
	assert.Contains(t, doc, aiArtifactScript, "the artifact's AI call sites are kept verbatim")
	assertNoInstanceAI(t, "document", doc)
}

func assertNoInstanceAI(t *testing.T, name, content string) {
	t.Helper()
	for _, forbidden := range []string{"/inference", "INFERENCE_", exportTestInferenceKey, exportTestInferenceModel, exportTestConnectorKey} {
		assert.NotContains(t, content, forbidden, name)
	}
}

func TestHTMLExportDisablesAI(t *testing.T) {
	ro, id := newAIExportRouter(t)
	w := exportGet(t, ro, id)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertAIDisabledDocument(t, w.Body.String())
}

func TestRelayBundleExportDisablesAI(t *testing.T) {
	ro, id := newAIExportRouter(t)
	files := zipFiles(t, doJSON(t, ro, http.MethodGet, "/api/artifacts/"+id+"/relay-bundle", nil))

	assertAIDisabledDocument(t, files["index.html"])
	for name, content := range files {
		assertNoInstanceAI(t, name, content)
	}
	// The runners inject shim.js ahead of index.html, and it freezes
	// window.exhibit; it carries the stand-in so the frozen object keeps ai.
	assert.True(t, strings.HasPrefix(files["shim.js"], standin.Script))
	assert.Contains(t, files["exhibit.json"], `"connections"`, "connector export is unchanged")

	readme := files["README.md"]
	assert.Contains(t, readme, "AI features (window.exhibit.ai) are turned off in exported copies, even when\nonline")
	assert.Contains(t, readme, "This bundle has no AI route and needs no AI\nruntime.")
	assert.Contains(t, readme, "wire up\nyour own AI integration")
}

// exportAIResults loads a page in Chromium and returns what the artifact's
// own script learned from window.exhibit.ai.
func exportAIResults(t *testing.T, url string) map[string]any {
	t.Helper()
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = chromedp.Run(ctx, chromedp.Navigate(url))
	require.NoError(t, err)
	results, err := chromedp.Run(ctx, chromedp.Evaluate[map[string]any](`window.aiResults`, chromedp.EvalAwaitPromise))
	require.NoError(t, err)
	counter, err := chromedp.Run(ctx, chromedp.Evaluate[int](`window.counter`))
	require.NoError(t, err)
	assert.Equal(t, 2, counter, "the artifact's non-AI code still runs")
	return results
}

func assertUnavailableResults(t *testing.T, results map[string]any) {
	t.Helper()
	assert.Equal(t, map[string]any{"complete": false, "decide": false}, results["kinds"])
	assert.Equal(t, map[string]any{"complete": "unavailable", "decide": "unavailable"}, results["codes"])
}

// requestLog counts every request a page makes apart from the ones the test
// expects, so an export that reached for AI (or anything else) shows up.
type requestLog struct {
	mu    sync.Mutex
	extra []string
}

func (l *requestLog) wrap(expected map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !expected[r.URL.Path] && r.URL.Path != "/favicon.ico" {
			l.mu.Lock()
			l.extra = append(l.extra, r.Method+" "+r.URL.Path)
			l.mu.Unlock()
		}
		next.ServeHTTP(w, r)
	})
}

func (l *requestLog) requests() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.extra...)
}

func TestHTMLExportDisablesAIBrowser(t *testing.T) {
	ro, id := newAIExportRouter(t)
	w := exportGet(t, ro, id)
	require.Equal(t, http.StatusOK, w.Code)
	doc := w.Body.String()

	// The page is served beside a live mock of the hosted inference route,
	// online, so a stand-in that reached anywhere would be seen.
	var log requestLog
	mux := http.NewServeMux()
	mux.HandleFunc("GET /export.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, doc)
	})
	mux.HandleFunc("/api/artifacts/{id}/inference", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"text":"mock"}`)
	})
	srv := httptest.NewServer(log.wrap(map[string]bool{"/export.html": true}, mux))
	t.Cleanup(srv.Close)

	assertUnavailableResults(t, exportAIResults(t, srv.URL+"/export.html"))
	assert.Empty(t, log.requests(), "the export makes no requests")
}

func TestRelayBundleExportDisablesAIBrowser(t *testing.T) {
	ro, id := newAIExportRouter(t)
	w := doJSON(t, ro, http.MethodGet, "/api/artifacts/"+id+"/relay-bundle", nil)
	require.Equal(t, http.StatusOK, w.Code)
	path := filepath.Join(t.TempDir(), "app.zip")
	require.NoError(t, os.WriteFile(path, w.Body.Bytes(), 0o600))
	app, err := bundle.Read(path)
	require.NoError(t, err)

	// Run the bundle in the Go runner, with the unrelated connector's
	// requirement in place, and open the artifact page it serves.
	var log requestLog
	var runner http.Handler
	srv := httptest.NewServer(log.wrap(map[string]bool{"/__exhibit/render": true}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runner.ServeHTTP(w, r)
	})))
	t.Cleanup(srv.Close)
	server, err := bundle.NewServer(app, bundle.ServerOptions{AllowedHosts: []string{strings.TrimPrefix(srv.URL, "http://")}, StatePath: filepath.Join(t.TempDir(), "state.json")})
	require.NoError(t, err)
	t.Cleanup(server.Close)
	runner = server

	assertUnavailableResults(t, exportAIResults(t, srv.URL+"/__exhibit/render"))
	assert.Empty(t, log.requests(), "the bundled page makes no requests")
}

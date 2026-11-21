// Package inferencetriage tests the example artifact in this directory: a
// feedback triage tool that uses window.exhibit.ai.decide() to triage and
// complete() to summarise, and keeps working with either kind, or both,
// missing (Exh-15tf).
package inferencetriage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
)

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(b)
}

// The source contract, checked without a browser: the tool asks before it
// calls, answers every error code, and reaches nothing over the network.
func TestExampleSourceHandlesEveryAIOutcome(t *testing.T) {
	tool := readFile(t, "index.html")
	assert.Contains(t, tool, ".available()")
	assert.Contains(t, tool, "window.exhibit.ai.decide(")
	assert.Contains(t, tool, "window.exhibit.ai.complete(")
	assert.Contains(t, tool, "schema: SUMMARY_SCHEMA")
	for _, code := range []string{"unavailable", "not_approved", "no_key", "limit_reached", "invalid_request", "rate_limited", "provider_error", "invalid_output"} {
		assert.Contains(t, tool, "'"+code+"'", "error code %s", code)
	}
	assert.Contains(t, tool, "exported copies always have AI turned off")
	assert.Contains(t, tool, "color-scheme:light dark")

	widget := readFile(t, "widget.html")
	assert.NotContains(t, widget, "exhibit.ai", "a widget never calls AI")
	assert.Contains(t, widget, "triage.v1", "the widget reads the tool's key")
	assert.Contains(t, widget, "color-scheme:light dark")

	external := regexp.MustCompile(`(?i)(src|href)\s*=\s*["']?(https?:)?//|url\(\s*["']?(https?:)?//|fetch\(|XMLHttpRequest|import\s*\(`)
	for name, src := range map[string]string{"index.html": tool, "widget.html": widget} {
		assert.Empty(t, external.FindAllString(src, -1), "%s must load nothing over the network", name)
	}
}

// stub stands in for Exhibit's bridge, chosen by ?ai= on the page URL:
// a comma list of offered kinds, "absent" for no window.exhibit at all (the
// file opened outside Exhibit), and "reject" to offer both kinds but reject
// every call as unavailable. It records calls and every uncaught error.
const stub = `(() => {
  const mode = new URLSearchParams(location.search).get('ai') || '';
  window.__calls = []; window.__errors = [];
  addEventListener('error', (e) => __errors.push('error: ' + e.message));
  addEventListener('unhandledrejection', (e) => __errors.push('unhandled: ' + ((e.reason && e.reason.message) || e.reason)));
  if (!location.pathname.endsWith('widget.html')) localStorage.clear();
  if (mode === 'absent') return;
  const kinds = mode.split(',');
  const reject = kinds.includes('reject');
  const offered = { complete: reject || kinds.includes('complete'), decide: reject || kinds.includes('decide') };
  const unavailable = () => Object.assign(new Error('AI is not available here.'), { name: 'ExhibitAIError', code: 'unavailable' });
  window.exhibit = { ai: {
    available: async () => ({ ...offered }),
    decide: async (request) => {
      __calls.push({ kind: 'decide', request });
      if (reject || !offered.decide) throw unavailable();
      const crash = /crash/i.test(request.state.feedback);
      const unsure = /maybe/i.test(request.state.feedback);
      return {
        category: { type: 'choice', choice: crash ? 'bug' : 'praise', probabilities: {}, confidence: unsure ? 0.4 : 0.9 },
        urgent: { type: 'bool', probability: crash ? 0.95 : 0.05 },
        severity: { type: 'score', score: crash ? 3.6 : 0.2, confidence: 0.8 },
      };
    },
    complete: async (request) => {
      __calls.push({ kind: 'complete', request });
      if (reject || !offered.complete) throw unavailable();
      return { headline: 'One crash needs attention', summary: 'A login crash is urgent.', top_actions: ['Fix the login crash'] };
    },
  } };
})();`

// observe adds feedback and reports which controls are usable.
const observe = `(async () => {
  const until = async (ok) => { for (let i = 0; i < 200 && !ok(); i++) await new Promise((r) => setTimeout(r, 10)); return ok(); };
  await until(() => !document.getElementById('ai-status').textContent.startsWith('Checking'));
  document.getElementById('new-items').value = 'App crashes on login\nLove the new theme\nmaybe fine';
  document.getElementById('add-items').click();
  const $ = (id) => document.getElementById(id);
  return {
    status: $('ai-status').textContent,
    triageDisabled: $('triage-ai').disabled,
    triageNote: $('triage-note').hidden ? '' : $('triage-note').textContent,
    summaryDisabled: $('summary-ai').disabled,
    summaryNote: $('summary-note').hidden ? '' : $('summary-note').textContent,
    manualEnabled: !document.querySelector('#items [data-field=category]').disabled && !$('summary-save').disabled,
    items: document.querySelectorAll('#items li.item').length,
  };
})()`

type observation struct {
	Status          string `json:"status"`
	TriageDisabled  bool   `json:"triageDisabled"`
	TriageNote      string `json:"triageNote"`
	SummaryDisabled bool   `json:"summaryDisabled"`
	SummaryNote     string `json:"summaryNote"`
	ManualEnabled   bool   `json:"manualEnabled"`
	Items           int    `json:"items"`
}

func TestExampleDegradesPerKindBrowser(t *testing.T) {
	ctx, base := openExample(t)
	cases := map[string]struct{ triage, summary bool }{
		"complete,decide": {true, true},
		"decide":          {true, false},
		"complete":        {false, true},
		"none":            {false, false}, // what an export's stand-in reports
		"absent":          {false, false},
	}
	for mode, want := range cases {
		t.Run(mode, func(t *testing.T) {
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(base+"/index.html?ai="+mode)))
			got, err := chromedp.Run(ctx, chromedp.Evaluate[observation](observe, chromedp.EvalAwaitPromise))
			require.NoError(t, err)
			assert.Equal(t, 3, got.Items)
			assert.True(t, got.ManualEnabled, "manual triage and summary always work")
			assert.Equal(t, !want.triage, got.TriageDisabled, "triage button")
			assert.Equal(t, !want.summary, got.SummaryDisabled, "summary button")
			if !want.triage {
				assert.Contains(t, got.TriageNote, "AI triage is off")
			}
			if !want.summary {
				assert.Contains(t, got.SummaryNote, "AI drafting is off")
			}
			if !want.triage && !want.summary {
				assert.Contains(t, got.Status, "exported copies always have AI turned off")
			}
			assertNoAICallsOrErrors(t, ctx, 0)
		})
	}
}

// The full path with both kinds: nothing is sent until the person has seen it,
// thresholds turn an unsure answer into "needs a person", and the widget shows
// what was saved.
func TestExampleTriagesAndSummarisesBrowser(t *testing.T) {
	ctx, base := openExample(t)
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(base+"/index.html?ai=complete,decide")))
	_, err := chromedp.Run(ctx, chromedp.Evaluate[observation](observe, chromedp.EvalAwaitPromise))
	require.NoError(t, err)

	type previewed struct {
		Visible bool   `json:"visible"`
		Text    string `json:"text"`
		Calls   int    `json:"calls"`
	}
	openPreview := func(button string) previewed {
		got, err := chromedp.Run(ctx, chromedp.Evaluate[previewed](`(() => {
			document.getElementById('`+button+`').click();
			return { visible: !document.getElementById('preview').hidden, text: document.getElementById('preview-text').textContent, calls: __calls.length };
		})()`))
		require.NoError(t, err)
		return got
	}
	shown := openPreview("triage-ai")
	assert.True(t, shown.Visible)
	assert.Contains(t, shown.Text, "App crashes on login")
	assert.Zero(t, shown.Calls, "nothing is sent before the person confirms")

	type triaged struct {
		Calls   int               `json:"calls"`
		Saved   []map[string]any  `json:"saved"`
		Chips   []string          `json:"chips"`
		ByText  map[string]string `json:"byText"`
		Persons int               `json:"persons"`
	}
	got, err := chromedp.Run(ctx, chromedp.Evaluate[triaged](`(async () => {
		document.getElementById('preview-send').click();
		for (let i = 0; i < 200 && __calls.length < 3; i++) await new Promise((r) => setTimeout(r, 10));
		await new Promise((r) => setTimeout(r, 50));
		const saved = JSON.parse(localStorage.getItem('triage.v1')).items;
		const visible = displayedItems();
		const byText = {};
		for (const item of visible) byText[item.text] = (item.category || '') + (item.urgent ? '+urgent' : '');
		return { calls: __calls.length, saved, byText, persons: visible.filter((i) => i.needsPerson).length,
			chips: [...document.querySelectorAll('#items .chip')].map((c) => c.textContent) };
	})()`, chromedp.EvalAwaitPromise))
	require.NoError(t, err)
	assert.Equal(t, 3, got.Calls)
	assert.Equal(t, "bug+urgent", got.ByText["App crashes on login"])
	assert.Equal(t, "praise", got.ByText["Love the new theme"])
	assert.Equal(t, "", got.ByText["maybe fine"], "a low-confidence category is left for a person")
	assert.Equal(t, 1, got.Persons)
	assert.Contains(t, got.Chips, "Needs a person")
	for _, saved := range got.Saved {
		assert.Nil(t, saved["category"], "the selected category is a private suggestion")
		assert.NotContains(t, saved, "ai", "raw judgments stay out of saved state")
		assert.Equal(t, false, saved["urgent"])
	}

	shown = openPreview("summary-ai")
	assert.True(t, shown.Visible)
	assert.Contains(t, shown.Text, "[Bug, urgent, severity Critical] App crashes on login")
	assert.Equal(t, 3, shown.Calls)

	headline, err := chromedp.Run(ctx, chromedp.Evaluate[string](`(async () => {
		document.getElementById('preview-send').click();
		for (let i = 0; i < 200 && !document.getElementById('summary-headline').value; i++) await new Promise((r) => setTimeout(r, 10));
		const call = __calls[3];
		if (!call || call.kind !== 'complete' || !call.request.schema) return 'complete() was not called with a schema';
		return document.getElementById('summary-headline').value;
	})()`, chromedp.EvalAwaitPromise))
	require.NoError(t, err)
	assert.Equal(t, "One crash needs attention", headline)
	assertNoAICallsOrErrors(t, ctx, 4)
	draftStored, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`JSON.parse(localStorage.getItem('triage.v1')).summary !== null`))
	require.NoError(t, err)
	assert.False(t, draftStored, "the draft stays private until Save")
	require.NoError(t, chromedp.Do(ctx, chromedp.EmulateViewport(390, 844)))
	fitsPhone, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`document.documentElement.scrollWidth <= innerWidth`))
	require.NoError(t, err)
	assert.True(t, fitsPhone, "draft disclosure and save controls fit on a phone")
	require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`#items [data-action="save-triage"]:not([hidden])`))))
	require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS("#summary-save"))))

	// The widget reads what the tool saved, and calls nothing.
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(base+"/widget.html?ai=complete,decide")))
	tile, err := chromedp.Run(ctx, chromedp.Evaluate[string](`[...document.body.querySelectorAll('#total,#label,#legend,#headline')].map((e) => e.textContent).join(' | ')`))
	require.NoError(t, err)
	assert.Equal(t, "3 | items · 1 urgent · 2 untriaged | 1 bugs | One crash needs attention", tile)
	assertNoAICallsOrErrors(t, ctx, 0)
}

// Offered but refused at call time (say, the tool was exported between the
// check and the click): the error is explained and the control switched off.
func TestExampleExplainsUnavailableAtCallTimeBrowser(t *testing.T) {
	ctx, base := openExample(t)
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(base+"/index.html?ai=reject")))
	_, err := chromedp.Run(ctx, chromedp.Evaluate[observation](observe, chromedp.EvalAwaitPromise))
	require.NoError(t, err)
	type after struct {
		Error          string `json:"error"`
		TriageDisabled bool   `json:"triageDisabled"`
	}
	got, err := chromedp.Run(ctx, chromedp.Evaluate[after](`(async () => {
		document.getElementById('triage-ai').click();
		document.getElementById('preview-send').click();
		for (let i = 0; i < 200 && document.getElementById('error').hidden; i++) await new Promise((r) => setTimeout(r, 10));
		return { error: document.getElementById('error').textContent, triageDisabled: document.getElementById('triage-ai').disabled };
	})()`, chromedp.EvalAwaitPromise))
	require.NoError(t, err)
	assert.Contains(t, got.Error, "AI isn’t available here")
	assert.True(t, got.TriageDisabled)
	assertNoAICallsOrErrors(t, ctx, 1)
}

func assertNoAICallsOrErrors(t *testing.T, ctx context.Context, calls int) {
	t.Helper()
	type report struct {
		Calls  int      `json:"calls"`
		Errors []string `json:"errors"`
	}
	got, err := chromedp.Run(ctx, chromedp.Evaluate[report](`({ calls: __calls.length, errors: __errors })`))
	require.NoError(t, err)
	assert.Equal(t, calls, got.Calls, "AI calls")
	assert.Empty(t, got.Errors, "uncaught errors")
}

func openExample(t *testing.T) (context.Context, string) {
	t.Helper()
	server := httptest.NewServer(http.FileServer(http.Dir(".")))
	t.Cleanup(server.Close)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	t.Cleanup(cancel)
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	t.Cleanup(cancelTimeout)
	_, err = chromedp.Call(ctx, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: stub})
	require.NoError(t, err)
	return ctx, strings.TrimSuffix(server.URL, "/")
}

package api

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Use the shipped page, Stimulus, htmx, real Pi RPC and deterministic provider.
// The form must not merely look complete: only an explicit send/skip may resume
// its waiting tool, on desktop and in the phone's scrollable chat pane.
func TestAgentQuestionsBrowser(t *testing.T) {
	h := newPiHarness(t)
	app := httptest.NewServer(h.router)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, closeTab, err := browser.Tab(context.Background())
	require.NoError(t, err)
	ctx, timeout := context.WithTimeout(ctx, 90*time.Second)
	defer func() { timeout(); closeTab(); app.CloseClientConnections(); app.Close() }()
	run := func(actions ...chromedp.Action[chromedp.Void]) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, actions...))
	}
	check := func(expression string) {
		t.Helper()
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](expression))
		require.NoError(t, err)
		require.True(t, ok, expression)
	}
	for _, width := range []int64{1280, 390, 320} {
		run(chromedp.EmulateViewport(width, 900), chromedp.Navigate(app.URL+"/agent"),
			chromedp.Poll[chromedp.Void](`document.querySelector('#send-btn') && !document.querySelector('#key-modal[open]')`),
			chromedp.SendKeys(chromedp.CSS(`#input`), "mock-ask-questions-mixed"),
			chromedp.Click(chromedp.CSS(`#send-btn`)),
			chromedp.Poll[chromedp.Void](`document.querySelectorAll('[data-question-id]').length === 3`))
		// Loading a long set reveals its beginning rather than its footer.
		check(`document.querySelector('.question-heading').getBoundingClientRect().top >= document.querySelector('#messages').getBoundingClientRect().top - 1`)
		run(chromedp.Click(chromedp.CSS(`[data-question-id="view"] [data-question-choice]`)),
			chromedp.Click(chromedp.CSS(`[data-question-id="priorities"] [value="Quick to cook"]`)),
			chromedp.Click(chromedp.CSS(`[data-question-id="priorities"] [value="Budget friendly"]`)),
			chromedp.SendKeys(chromedp.CSS(`[data-question-id="goal"] textarea`), "Meals for my family"))
		check(`document.querySelector('.question-progress').textContent === '3 of 3 answered' && document.querySelector('[data-question-form]') !== null`)
		check(`document.documentElement.scrollWidth <= innerWidth`)
		run(chromedp.Click(chromedp.CSS(`.question-submit`)),
			chromedp.Poll[chromedp.Void](`!document.querySelector('[data-question-form]') && document.querySelector('.question-completed')?.textContent.includes('Answers sent')`),
			chromedp.Poll[chromedp.Void](`getComputedStyle(document.querySelector('#stop-btn')).display === 'none'`))
		check(`document.querySelector('.question-summary-answer').textContent === 'Weekly calendar'`)
		check(`document.activeElement.matches('[data-interaction-id]') && document.querySelector('.question-completed').getAttribute('role') === 'status'`)

		run(chromedp.SendKeys(chromedp.CSS(`#input`), "mock-ask-questions"), chromedp.Click(chromedp.CSS(`#send-btn`)),
			chromedp.Poll[chromedp.Void](`!!document.querySelector('[data-question-form]')`),
			chromedp.Click(chromedp.CSS(`[data-question-form] [data-question-choice]`)),
			chromedp.Click(chromedp.CSS(`[data-agent-questions-target="skipTrigger"]`)),
			chromedp.Poll[chromedp.Void](`!document.querySelector('.question-skip-confirmation').hidden`))
		check(`document.querySelector('[data-question-form]') !== null`)
		run(chromedp.Click(chromedp.CSS(`[data-action="agent-questions#keepAnswering"]`)))
		check(`document.querySelector('.question-skip-confirmation').hidden && document.querySelector('[data-question-form] [aria-pressed="true"]') !== null`)
		run(chromedp.Click(chromedp.CSS(`[data-agent-questions-target="skipTrigger"]`)),
			chromedp.Click(chromedp.CSS(`[data-action="agent-questions#confirmSkip"]`)),
			chromedp.Poll[chromedp.Void](`!document.querySelector('[data-question-form]') && document.querySelector('.question-skipped')?.textContent.includes('Questions skipped')`))
		check(`document.activeElement.matches('[data-interaction-id]') && document.querySelector('.question-skipped').getAttribute('role') === 'status'`)
	}
}

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

const markdownBrowserOverview = "## Your reading list is ready\n\nUse **tags** to organize it, *search* to find a title, and `localStorage` to keep changes. ~~No setup needed.~~\n\n### What is included\n\n- [x] Save titles and links\n- [ ] Add your first book\n- Organize the collection\n  - Filter by tag\n  - Sort by title\n\n> Your list stays with this artifact.\n\n1. Open the preview.\n2. Add a book and try [search](https://example.com/search).\n\n```js\nconst title = \"The Left Hand of Darkness\";\nlocalStorage.setItem(\"book\", title);\n```"
const markdownBrowserFence = "### Streaming an HTML example\n\nAn unfinished code fence still displays as code:\n\n```html\n<div class=\"reading-list\" data-description=\"a deliberately long line that should scroll inside this code block on a narrow phone\">\n  <script>window.pwned = true</script>"
const markdownBrowserEdges = markdownBrowserFence + "\n</div>\n```\n\n| Feature | Desktop | Small phone | Status |\n| --- | --- | --- | --- |\n| Wide content | Scrolls within the message | Keeps the composer visible | Ready |\n\nA long link: https://example.com/reading/averylongunbrokentitlethatmustwrapwithoutwideningthechatpaneorhidingthecopybutton\n\n**HTML stays disabled.** Raw tags are omitted; fenced examples above remain readable.\n\n<script>window.pwned = true</script>\n\n<style>body{display:none}</style>\n\n<img src=x onerror=\"window.pwned=true\">\n\n<div hx-get=\"/should-not-run\" hx-trigger=\"load\">raw HTML</div>\n\n![Book cover](https://example.com/cover.png?conversation=private)\n\n[Unsafe link](javascript&colon;alert%281%29) · [Safe link](https://example.com)"

// Use the actual page, assets, history routes and htmx. Only the Pi subprocess
// boundary is replaced by a controllable stream of its delta-only wire events.
func markdownBrowserFixture(t *testing.T) (http.Handler, string, chan []byte) {
	t.Helper()
	router := newTestRouter(t)
	enablePlatformMode(router)
	router.cfg.Agent = &agent.Manager{}
	id := createArtifact(t, router, map[string]any{"title": "Reading list", "body": "<html><body>Reading list</body></html>"})
	var transcript strings.Builder
	transcript.WriteString("{\"type\":\"session\",\"version\":3,\"id\":\"markdown\",\"cwd\":\"/\"}\n")
	for i, message := range []map[string]any{
		{"role": "user", "content": "Show **Markdown** and <b>HTML</b> examples."},
		{"role": "assistant", "content": []map[string]string{{"type": "text", "text": markdownBrowserOverview}}},
		{"role": "assistant", "content": []map[string]string{{"type": "text", "text": markdownBrowserEdges}}},
	} {
		line, err := json.Marshal(map[string]any{"type": "message", "id": fmt.Sprint(i), "message": message})
		require.NoError(t, err)
		transcript.Write(line)
		transcript.WriteByte('\n')
	}
	keepConversation(t, router, id, "markdown", "Markdown examples", transcript.String())
	events := make(chan []byte, 32)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/agent/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusCreated, map[string]string{"id": "markdown", "sse_ticket": "fixture"})
	})
	mux.HandleFunc("POST /api/agent/sessions/markdown/prompt", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("DELETE /api/agent/sessions/markdown", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /api/agent/sessions/markdown/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		var markdown agentMarkdownStream
		ticker := time.NewTicker(agentMarkdownInterval)
		defer ticker.Stop()
		for {
			select {
			case line := <-events:
				fmt.Fprintf(w, "data: %s\n\n", markdown.renderEvent(line))
				w.(http.Flusher).Flush()
			case <-ticker.C:
				if line := markdown.flush(); line != nil {
					fmt.Fprintf(w, "data: %s\n\n", line)
					w.(http.Flusher).Flush()
				}
			case <-r.Context().Done():
				return
			}
		}
	})
	mux.Handle("/", router)
	return mux, id, events
}

func TestAgentMarkdownBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var imageRequests atomic.Int64
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imageRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer imageServer.Close()
	liveEdges := markdownBrowserEdges + "\n\n![Network probe](" + imageServer.URL + "/cover.png?conversation=private)"
	handler, id, events := markdownBrowserFixture(t)
	srv := httptest.NewServer(handler)
	defer func() {
		cancel()
		srv.CloseClientConnections()
		srv.Close()
	}()
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
	capture := func(name string) {
		if dir := os.Getenv("AGENT_MARKDOWN_SCREENSHOT_DIR"); dir != "" {
			require.NoError(t, os.MkdirAll(dir, 0755))
			shot, err := chromedp.Run(ctx, chromedp.Screenshot(chromedp.CSS(`body`)))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), shot, 0644))
		}
	}
	run(chromedp.EmulateViewport(1366, 900), chromedp.Navigate(srv.URL+"/agent?artifact="+id),
		chromedp.Poll[chromedp.Void](`typeof window.send === 'function'`),
		chromedp.Evaluate[chromedp.Void](`Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText: async text => {window.copied = text}}})`),
		chromedp.SendKeys(chromedp.CSS(`#input`), "Show **Markdown** and <b>HTML</b> examples."),
		chromedp.Click(chromedp.CSS(`#send-btn`)))
	events <- []byte(`{"type":"agent_start"}`)
	events <- markdownWireEvent("text_delta", "## Your reading list is ready\n\nUse **ta", 0)
	run(chromedp.Poll[chromedp.Void](`document.querySelector('#messages h2')?.textContent === 'Your reading list is ready'`))
	check(`document.querySelector('#messages .assistant').textContent.includes('**ta')`)
	events <- markdownWireEvent("text_delta", "gs**", 0)
	run(chromedp.Poll[chromedp.Void](`document.querySelector('#messages strong')?.textContent === 'tags'`))
	events <- markdownWireEvent("text_end", markdownBrowserOverview, 0)
	run(chromedp.Poll[chromedp.Void](`document.querySelector('#messages pre code')?.textContent.includes('localStorage')`))
	check(`document.querySelectorAll('#messages input[type=checkbox]').length === 2 && document.querySelector('#messages input[type=checkbox]').disabled`)
	check(`document.querySelector('#messages .user').textContent === 'Show **Markdown** and <b>HTML</b> examples.' && !document.querySelector('#messages .user b')`)
	check(`getComputedStyle(document.querySelector('#messages .assistant')).backgroundColor === 'rgba(0, 0, 0, 0)' && getComputedStyle(document.querySelector('#messages .assistant')).padding === '0px'`)
	run(chromedp.Click(chromedp.CSS(`#messages [data-message-action=copy]`)), chromedp.Poll[chromedp.Void](`window.copied != null`))
	check(fmt.Sprintf(`window.copied === %q`, markdownBrowserOverview))
	capture("01-desktop-markdown.png")
	run(chromedp.EmulateViewport(390, 844))
	check(`document.documentElement.scrollWidth <= innerWidth && document.querySelector('#messages').scrollWidth <= document.querySelector('#messages').clientWidth`)
	capture("02-mobile-markdown.png")
	// An open fence containing executable-looking HTML must remain inert, while
	// its long line scrolls inside the code block instead of widening the pane.
	events <- markdownWireEvent("text_delta", markdownBrowserFence, 1)
	run(chromedp.Poll[chromedp.Void](`document.querySelectorAll('#messages pre').length === 2`),
		chromedp.Evaluate[chromedp.Void](`document.querySelector('#messages .agent-message:last-child').scrollIntoView({block:'start'})`))
	check(`!window.pwned && document.querySelectorAll('#messages script, #messages iframe, #messages style').length === 0`)
	check(`document.querySelector('#messages .agent-message:last-child pre').scrollWidth > document.querySelector('#messages .agent-message:last-child pre').clientWidth`)
	capture("03-mobile-streaming-fence.png")
	events <- markdownWireEvent("text_end", liveEdges, 1)
	events <- []byte(`{"type":"agent_settled"}`)
	run(chromedp.Poll[chromedp.Void](`document.querySelector('#messages table') != null`),
		chromedp.Evaluate[chromedp.Void](`document.querySelector('#messages .agent-message:last-child').scrollIntoView({block:'start'})`))
	check(`!window.pwned && document.querySelectorAll('#messages script, #messages iframe, #messages style, #messages [hx-get], #messages img').length === 0`)
	check(`document.querySelector('#messages .agent-message:last-child').textContent.includes('Book cover') && document.querySelector('#messages .agent-message:last-child').textContent.includes('Network probe')`)
	check(`!Array.from(document.querySelectorAll('#messages a')).some(a => a.textContent === 'Unsafe link')`)
	run(chromedp.Evaluate[chromedp.Void](`window.pageMarker = 'still here'; Array.from(document.querySelectorAll('#messages p')).find(p => p.textContent.includes('Unsafe link')).click()`))
	check(`window.pageMarker === 'still here' && document.querySelector('#messages table') != null`)
	check(`document.documentElement.scrollWidth <= innerWidth && document.querySelector('#messages').scrollWidth <= document.querySelector('#messages').clientWidth`)
	check(`document.querySelector('#messages table').scrollWidth > document.querySelector('#messages table').clientWidth`)
	capture("04-mobile-edge-cases.png")
	run(chromedp.EmulateViewport(1366, 900), chromedp.Evaluate[chromedp.Void](`document.querySelector('#messages .agent-message:last-child').scrollIntoView({block:'start'})`))
	capture("05-desktop-edge-cases.png")
	// Real htmx history swaps and continuation preserve both markup and source.
	run(chromedp.Click(chromedp.CSS(`#history-btn`)), chromedp.WaitVisible(chromedp.CSS(`.history-item`)), chromedp.Click(chromedp.CSS(`.history-item`)), chromedp.WaitVisible(chromedp.CSS(`#history-messages table`)))
	check(`document.querySelectorAll('#history-messages .agent-message').length === 2 && !window.pwned`)
	check(`!document.querySelector('#history-messages img') && document.querySelector('#history-messages').textContent.includes('Book cover')`)
	require.Zero(t, imageRequests.Load(), "Markdown images must never trigger automatic requests")
	run(chromedp.Click(chromedp.CSS(`#history-messages [data-message-action=copy]`)))
	check(fmt.Sprintf(`window.copied === %q`, markdownBrowserOverview))
	run(chromedp.Click(chromedp.CSS(`[data-resume=markdown]`)), chromedp.Poll[chromedp.Void](`!document.querySelector('#pane-chat').classList.contains('history-open') && document.querySelector('#messages table') != null`),
		chromedp.EmulateViewport(320, 740))
	check(`document.documentElement.scrollWidth <= innerWidth && document.querySelector('#messages').scrollWidth <= document.querySelector('#messages').clientWidth`)
	// Insecure/self-hosted clipboard fallback copies source and restores focus.
	run(chromedp.Evaluate[chromedp.Void](`Object.defineProperty(navigator, 'clipboard', {configurable:true, value:undefined}); document.execCommand = () => { const field = document.querySelector('textarea[readonly]'); window.copied = field.value.slice(field.selectionStart, field.selectionEnd); return true; }; document.querySelector('#messages [data-message-action=copy]').focus(); document.activeElement.click()`),
		chromedp.Poll[chromedp.Void](`document.querySelector('#messages .message-action-status').textContent === 'Copied'`))
	copied, err := chromedp.Run(ctx, chromedp.Evaluate[string](`window.copied ?? 'missing'`))
	require.NoError(t, err)
	require.Equal(t, markdownBrowserOverview, copied)
	active, err := chromedp.Run(ctx, chromedp.Evaluate[string](`document.activeElement.outerHTML`))
	require.NoError(t, err)
	require.Contains(t, active, `data-message-action="copy"`)
	run(chromedp.Evaluate[chromedp.Void](`Array.from(document.querySelectorAll('#messages .agent-message')).at(-1).scrollIntoView({block:'start'})`))
	capture("06-narrow-mobile-resumed.png")
}

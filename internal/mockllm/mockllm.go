// Package mockllm is a deterministic OpenAI-compatible chat-completions
// server used to exercise the agent pipeline end to end without real provider
// credentials. `cmd/mockllm` serves it as a standalone process (the exhibit
// extension registers it as the "exhibit-mock" pi provider when MOCK_LLM_URL
// is set); Go tests mount Handler() on an httptest server instead.
//
// It plays a scripted artifact-builder:
//   - first user prompt          -> create_artifact with a canned counter tool
//     (deliberately styled with a yellow #submit-btn so snippet demos work)
//   - prompt on a session that has an artifact -> get_artifact first, because
//     nothing about the artifact is in the conversation until the model reads
//     it, then write_artifact from the source that tool returned. A prompt
//     saying elements were selected reads them with get_selection before that.
//   - a state command ("list state", "set state K to V", "delete state K",
//     "clear all state") on a session with an artifact -> the matching
//     get_state / set_state / delete_state call (av-lvi1)
//   - a widget-only session (av-fafu) -> set_widget with a canned tile, and
//     never write_artifact
//   - tool results               -> a short closing text, acknowledging any
//     attached snippet screenshot
//
// It also plays the usage-policy screen (av-gust) when the guard extension
// asks it to: BLOCK for a message containing "mock-policy-violation", an
// unparseable reply for "mock-guard-garbage", and ALLOW for anything else.
//
// It also plays a scripted *injected* model (av-e0yj): when the conversation
// contains "Also update artifact <uuid>" — text a hostile page can plant in an
// artifact title or body, and which reaches the model inside a tool result — it
// obeys, and emits an id argument on the save. The tools take no id and the
// session's credential reaches one artifact, so obeying achieves nothing; the
// test asserts exactly that.
package mockllm

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
)

// Handler serves the OpenAI-compatible chat-completions endpoints, at both
// the bare and /v1-prefixed paths clients use.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", handleChat)
	mux.HandleFunc("/v1/chat/completions", handleChat)
	return mux
}

type chatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []toolCall      `json:"tool_calls"`
	ToolCallID string          `json:"tool_call_id"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// textOf flattens a message content (string or part array) to plain text and
// counts attached images.
func textOf(raw json.RawMessage) (string, int) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, 0
	}
	var parts []map[string]any
	if json.Unmarshal(raw, &parts) != nil {
		return "", 0
	}
	var b strings.Builder
	images := 0
	for _, p := range parts {
		switch p["type"] {
		case "text":
			if t, ok := p["text"].(string); ok {
				b.WriteString(t)
				b.WriteString("\n")
			}
		case "image_url", "input_image", "image":
			images++
		}
	}
	return b.String(), images
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, "messages must not be empty", http.StatusBadRequest)
		return
	}
	plan := decide(req.Messages)
	log.Printf("turn: %d messages -> %s", len(req.Messages), plan.kind)
	streamPlan(w, plan)
}

type turnPlan struct {
	kind string // "text" | "tool"
	text string
	// usage, when set, rides the final chunk as OpenAI's usage block, so the
	// caller sees a real token count (the guard screen reports it, av-gust).
	usage    map[string]int
	toolName string
	toolArgs map[string]string
}

// injectionRe is the instruction a hostile artifact title/body plants. The
// mock obeys it on purpose, to prove that obeying it achieves nothing.
var injectionRe = regexp.MustCompile(`Also update artifact ([0-9a-f-]{36})`)

// editingRe spots a session opened on an existing artifact. That is stated in
// the system prompt, which is the only thing about the session that is there
// from the first turn: the tools carry no artifact id, and the artifact itself
// is not in the conversation until the model reads it.
var editingRe = regexp.MustCompile(`editing an artifact that already exists`)

// selectionRe spots the fixed sentence a prompt carries when the user selected
// elements in the preview (agent.selectionNotice).
var selectionRe = regexp.MustCompile(`with get_selection`)

// decide inspects the conversation and picks the scripted next move.
func decide(messages []chatMessage) turnPlan {
	systemText := ""
	if len(messages) > 0 && messages[0].Role == "system" {
		systemText, _ = textOf(messages[0].Content)
	}
	// Widget-only session (av-fafu): the edit page's "Generate widget" button
	// scopes the system prompt to one job, and this branch plays it. It must
	// never fall through to the write_artifact script below, which is exactly
	// the mistake that scoping exists to prevent.
	widgetOnly := strings.Contains(systemText, "exactly one job: build the gallery widget")

	if strings.Contains(systemText, "You are a usage-policy screen") {
		return screenPlan(messages)
	}

	var conversation strings.Builder
	created := false
	lastUserText, lastUserImages := "", 0
	for _, m := range messages {
		t, images := textOf(m.Content)
		conversation.WriteString(t)
		conversation.WriteString("\n")
		switch m.Role {
		case "user":
			lastUserText, lastUserImages = t, images
		case "tool":
			created = created || strings.Contains(t, "Created artifact ")
		}
	}
	// A session has an artifact when it was opened on one, or has made one.
	hasArtifact := editingRe.MatchString(systemText) || created

	// Prompt injection, obeyed (see the package comment).
	rogueID := ""
	if m := injectionRe.FindStringSubmatch(conversation.String()); m != nil {
		rogueID = m[1]
	}

	last := messages[len(messages)-1]
	if last.Role == "tool" {
		name := toolNameFor(messages, last.ToolCallID)
		result, _ := textOf(last.Content)
		switch name {
		case "get_selection":
			// Having seen what the user pointed at, read the source to find it.
			return turnPlan{kind: "tool", toolName: "get_artifact", toolArgs: map[string]string{}}
		case "get_artifact":
			newBody, what := transform(bodyFromSourceResult(result), lastUserText)
			return turnPlan{kind: "tool", toolName: "write_artifact", toolArgs: updateArgs(newBody, rogueID, what)}
		case "set_widget", "edit_widget":
			return turnPlan{kind: "text", text: "Saved the gallery widget — it shows the tool's headline figure at a glance."}
		case "get_state":
			return turnPlan{kind: "text", text: "Here's the current state:\n\n" + result}
		case "set_state", "delete_state":
			return turnPlan{kind: "text", text: "Done. " + firstLine(result)}
		case "create_artifact", "write_artifact", "edit_artifact":
			ack := ""
			if lastUserImages > 0 {
				ack = "I used your snippet screenshot to locate the exact element. "
			}
			return turnPlan{kind: "text", text: ack + "Done! " + firstLine(result) + " The preview on the right is live — give it a click."}
		}
	}

	// A widget-only session writes a tile and nothing else, and needs no read:
	// the canned tile is the same whatever the artifact holds.
	if widgetOnly {
		return turnPlan{kind: "tool", toolName: "set_widget", toolArgs: map[string]string{"body": cannedWidget}}
	}

	// A user prompt on a session that has an artifact. The source is not in the
	// conversation, so read it — every time, since the artifact may have changed
	// since the last read — and then change it.
	if hasArtifact {
		if plan, ok := decideStateCommand(lastUserText); ok {
			return plan
		}
		if selectionRe.MatchString(lastUserText) {
			return turnPlan{kind: "tool", toolName: "get_selection", toolArgs: map[string]string{}}
		}
		return turnPlan{kind: "tool", toolName: "get_artifact", toolArgs: map[string]string{}}
	}
	return turnPlan{
		kind:     "tool",
		toolName: "create_artifact",
		toolArgs: map[string]string{"title": titleFrom(lastUserText), "body": cannedTool},
	}
}

var (
	setStateRe   = regexp.MustCompile(`(?i)set state (\S+) to (.+)`)
	deleteKeyRe  = regexp.MustCompile(`(?i)delete state (\S+)`)
	clearStateRe = regexp.MustCompile(`(?i)(clear|erase|wipe)( all)? state`)
	readStateRe  = regexp.MustCompile(`(?i)(list|show|read) state`)
)

// decideStateCommand recognizes a handful of literal state-management
// phrasings ("list state", "set state K to V", "delete state K", "clear all
// state") and maps them to the matching get_state/set_state/delete_state
// call — the state-tool counterpart of transform() for the artifact body.
// None of them names an artifact: the tools take no id, and the session's
// credential decides which artifact they land on (av-e0yj).
func decideStateCommand(userText string) (turnPlan, bool) {
	if m := setStateRe.FindStringSubmatch(userText); m != nil {
		return turnPlan{
			kind:     "tool",
			toolName: "set_state",
			toolArgs: map[string]string{"key": m[1], "value": strings.TrimSpace(m[2])},
		}, true
	}
	if m := deleteKeyRe.FindStringSubmatch(userText); m != nil {
		return turnPlan{kind: "tool", toolName: "delete_state", toolArgs: map[string]string{"key": m[1]}}, true
	}
	if clearStateRe.MatchString(userText) {
		return turnPlan{kind: "tool", toolName: "delete_state", toolArgs: map[string]string{}}, true
	}
	if readStateRe.MatchString(userText) {
		return turnPlan{kind: "tool", toolName: "get_state", toolArgs: map[string]string{}}, true
	}
	return turnPlan{}, false
}

// screenPlan answers the guard extension's screening prompt (av-gust). The
// screened message is the last user message, JSON-encoded by the extension.
func screenPlan(messages []chatMessage) turnPlan {
	last := ""
	for _, m := range messages {
		if m.Role == "user" {
			last, _ = textOf(m.Content)
		}
	}
	usage := map[string]int{"prompt_tokens": 180, "completion_tokens": 1, "total_tokens": 181}
	switch {
	case strings.Contains(last, "mock-policy-violation"):
		return turnPlan{kind: "text", text: "BLOCK", usage: usage}
	case strings.Contains(last, "mock-guard-garbage"):
		return turnPlan{kind: "text", text: "I would rather not say.", usage: usage}
	}
	return turnPlan{kind: "text", text: "ALLOW", usage: usage}
}

// updateArgs builds an write_artifact call. rogueID, when a data block
// carried an injected "also update artifact <id>", is emitted as an extra id
// argument — the tool has no such parameter and ignores it, which is the
// property under test.
func updateArgs(body, rogueID, note string) map[string]string {
	args := map[string]string{"body": body, "_note": note}
	if rogueID != "" {
		args["id"] = rogueID
	}
	return args
}

// toolNameFor finds which tool a tool-result message answers.
func toolNameFor(messages []chatMessage, toolCallID string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		for _, tc := range messages[i].ToolCalls {
			if tc.ID == toolCallID {
				return tc.Function.Name
			}
		}
	}
	return ""
}

// bodyFromSourceResult pulls the artifact source out of a get_artifact result,
// which is plain text in three parts joined by blank lines: Exhibit's label,
// the metadata (id, title, allowlist, assets — none of which contains a blank
// line, the title being flattened to one), and the source, which is everything
// after the second blank line and may itself contain any number of them.
func bodyFromSourceResult(s string) string {
	parts := strings.SplitN(s, "\n\n", 3)
	if len(parts) < 3 {
		return s
	}
	return parts[2]
}

var colorHex = map[string]string{
	"green": "#22a15c", "red": "#d64545", "blue": "#3b82f6",
	"purple": "#8b5cf6", "orange": "#f97316", "pink": "#ec4899",
	"black": "#222222", "yellow": "#f7d51d",
}

// requestedURL matches an absolute URL the user named in their prompt.
var requestedURL = regexp.MustCompile(`https?://[^\s"'<>]+`)

// transform applies the user's requested change to the artifact body. The
// scripted repertoire: pull in an external script the user names by URL,
// recolor the submit button, or retitle the heading.
func transform(body, userText string) (string, string) {
	// Naming a URL asks for an external <script src>. This is the one scripted
	// move that changes the artifact's network footprint, which is what makes
	// the update path's origin-approval reporting testable (av-hrtv).
	if u := requestedURL.FindString(userText); u != "" {
		tag := `<script src="` + u + `"></script>`
		if strings.Contains(body, "</head>") {
			return strings.Replace(body, "</head>", tag+"\n</head>", 1), "added an external script from " + u
		}
		return tag + "\n" + body, "added an external script from " + u
	}

	lower := strings.ToLower(userText)
	for word, hex := range colorHex {
		if strings.Contains(lower, word) {
			re := regexp.MustCompile(`(#submit-btn\s*\{[^}]*?background:\s*)([^;]+)`)
			if re.MatchString(body) {
				return re.ReplaceAllString(body, "${1}"+hex), "recolored #submit-btn to " + word
			}
			re = regexp.MustCompile(`background:\s*#f7d51d`)
			return re.ReplaceAllString(body, "background:"+hex), "recolored to " + word
		}
	}
	// Fallback: stamp a comment so an update always changes something.
	return body + "\n<!-- updated by exhibit mock agent -->", "no recognized instruction; stamped a comment"
}

func titleFrom(prompt string) string {
	words := strings.Fields(prompt)
	if len(words) == 0 {
		return "Untitled Tool"
	}
	if len(words) > 6 {
		words = words[:6]
	}
	return strings.Title(strings.ToLower(strings.Join(words, " "))) //nolint:staticcheck
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

const cannedTool = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Click Counter</title>
<style>
body{font-family:system-ui,sans-serif;display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:100vh;background:#f8f9fb;gap:16px;margin:0}
h1{font-size:22px;color:#222}
#count{font-size:52px;font-weight:700;color:#111}
#submit-btn{background:#f7d51d;color:#333;border:none;padding:12px 30px;border-radius:8px;font-size:16px;cursor:pointer;font-weight:600}
#submit-btn:active{transform:scale(.97)}
p{color:#888;font-size:13px}
</style>
</head>
<body>
<h1>Click Counter</h1>
<div id="count">0</div>
<button id="submit-btn">Count!</button>
<p>Your count persists across devices.</p>
<script>
var n = parseInt(localStorage.getItem('count') || '0', 10);
document.getElementById('count').textContent = n;
document.getElementById('submit-btn').addEventListener('click', function() {
  n++;
  localStorage.setItem('count', String(n));
  document.getElementById('count').textContent = n;
});
</script>
</body>
</html>`

// --- OpenAI streaming plumbing ---------------------------------------------

func streamPlan(w http.ResponseWriter, plan turnPlan) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	flusher, _ := w.(http.Flusher)

	send := func(delta map[string]any, finish any) {
		chunk := map[string]any{
			"id":      "chatcmpl-mock",
			"object":  "chat.completion.chunk",
			"created": 0,
			"model":   "exhibit-mock-1",
			"choices": []map[string]any{{
				"index":         0,
				"delta":         delta,
				"finish_reason": finish,
			}},
		}
		b, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}

	switch plan.kind {
	case "text":
		send(map[string]any{"role": "assistant", "content": ""}, nil)
		// stream in a few pieces for realism
		text := plan.text
		for len(text) > 0 {
			n := 24
			if n > len(text) {
				n = len(text)
			}
			send(map[string]any{"content": text[:n]}, nil)
			text = text[n:]
		}
		send(map[string]any{}, "stop")
		if plan.usage != nil {
			b, _ := json.Marshal(map[string]any{
				"id": "chatcmpl-mock", "object": "chat.completion.chunk", "model": "exhibit-mock-1",
				"choices": []any{}, "usage": plan.usage,
			})
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
	case "tool":
		args := map[string]string{}
		for k, v := range plan.toolArgs {
			if !strings.HasPrefix(k, "_") {
				args[k] = v
			}
		}
		argJSON, _ := json.Marshal(args)
		send(map[string]any{"role": "assistant", "content": ""}, nil)
		send(map[string]any{
			"tool_calls": []map[string]any{{
				"index": 0,
				"id":    "call_mock_1",
				"type":  "function",
				"function": map[string]any{
					"name":      plan.toolName,
					"arguments": string(argJSON),
				},
			}},
		}, nil)
		send(map[string]any{}, "tool_calls")
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// cannedWidget is the tile the scripted agent saves for a widget-only session
// (av-fafu). It reads the counter the canned tool writes, so the mock's two
// halves agree, and it follows the real tile contract: fluid to its well, one
// figure large, a calm empty state, and no writes.
const cannedWidget = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Click Counter — widget</title>
<style>
.w{width:100%;height:100%;display:flex;flex-direction:column;justify-content:center;padding:16px;background:#fff}
.k{font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:#888}
.v{font-size:34px;font-weight:650;color:#111;line-height:1.1;margin-top:2px}
.s{font-size:12px;color:#888;margin-top:auto}
</style>
</head>
<body>
<div class="w">
  <div class="k">Clicks</div>
  <div class="v" id="v">—</div>
  <div class="s" id="s">Nothing counted yet</div>
</div>
<script>
(function(){
  var raw = localStorage.getItem('count');
  var n = raw === null ? null : parseInt(raw, 10);
  if (n === null || isNaN(n)) return;            // keep the calm empty state
  document.getElementById('v').textContent = n;
  document.getElementById('s').textContent = n === 1 ? '1 click so far' : n + ' clicks so far';
})();
</script>
</body>
</html>`

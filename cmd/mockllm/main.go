// mockllm is a deterministic OpenAI-compatible chat-completions server used
// to test the agent pipeline end to end without real provider credentials.
// The exhibit extension registers it as the "exhibit-mock" pi provider when
// MOCK_LLM_URL is set on the exhibit server.
//
// It plays a scripted artifact-builder:
//   - first user prompt          -> create_artifact with a canned counter tool
//     (deliberately styled with a yellow #submit-btn so snippet demos work)
//   - prompt on a bound artifact -> get_artifact, then update_artifact with a
//     color transformation derived from the user's words
//   - tool results              -> a short closing text, acknowledging any
//     attached snippet screenshot
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

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

func main() {
	addr := ":9095"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}
	handler := http.HandlerFunc(handleChat)
	http.Handle("/chat/completions", handler)
	http.Handle("/v1/chat/completions", handler)
	log.Printf("mock LLM listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
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
	plan := decide(req.Messages)
	log.Printf("turn: %d messages -> %s", len(req.Messages), plan.kind)
	streamPlan(w, plan)
}

type turnPlan struct {
	kind     string // "text" | "tool"
	text     string
	toolName string
	toolArgs map[string]string
}

// decide inspects the conversation and picks the scripted next move.
func decide(messages []chatMessage) turnPlan {
	systemText := ""
	if len(messages) > 0 && messages[0].Role == "system" {
		systemText, _ = textOf(messages[0].Content)
	}
	// Artifact bound to the session: either declared in the system prompt
	// (modify mode) or announced by an earlier create_artifact result.
	artifactID := ""
	if m := regexp.MustCompile(`artifact id "([^"]+)"`).FindStringSubmatch(systemText); m != nil {
		artifactID = m[1]
	}
	lastUserText, lastUserImages := "", 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserText, lastUserImages = textOf(messages[i].Content)
			break
		}
	}
	for _, m := range messages {
		if m.Role == "tool" {
			t, _ := textOf(m.Content)
			if mm := regexp.MustCompile(`(?:Created|Updated) artifact ([0-9a-f-]{36})`).FindStringSubmatch(t); mm != nil {
				artifactID = mm[1]
			}
		}
	}

	last := messages[len(messages)-1]
	if last.Role == "tool" {
		name := toolNameFor(messages, last.ToolCallID)
		result, _ := textOf(last.Content)
		switch name {
		case "get_artifact":
			body := bodyFromGetResult(result)
			newBody, what := transform(body, lastUserText)
			return turnPlan{
				kind:     "tool",
				toolName: "update_artifact",
				toolArgs: map[string]string{"id": artifactID, "body": newBody, "_note": what},
			}
		case "create_artifact", "update_artifact":
			ack := ""
			if lastUserImages > 0 {
				ack = "I used your snippet screenshot to locate the exact element. "
			}
			return turnPlan{kind: "text", text: ack + "Done! " + firstLine(result) + " The preview on the right is live — give it a click."}
		}
	}

	// A user prompt. Bound artifact -> read it first; otherwise create.
	if artifactID != "" {
		return turnPlan{kind: "tool", toolName: "get_artifact", toolArgs: map[string]string{"id": artifactID}}
	}
	return turnPlan{
		kind:     "tool",
		toolName: "create_artifact",
		toolArgs: map[string]string{"title": titleFrom(lastUserText), "body": cannedTool},
	}
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

// bodyFromGetResult strips the metadata line the get_artifact tool prefixes.
func bodyFromGetResult(s string) string {
	if i := strings.Index(s, "\n\n"); i >= 0 {
		return s[i+2:]
	}
	return s
}

var colorHex = map[string]string{
	"green": "#22a15c", "red": "#d64545", "blue": "#3b82f6",
	"purple": "#8b5cf6", "orange": "#f97316", "pink": "#ec4899",
	"black": "#222222", "yellow": "#f7d51d",
}

// transform applies the user's requested change to the artifact body. The
// scripted repertoire: recolor the submit button, or retitle the heading.
func transform(body, userText string) (string, string) {
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
	flusher := w.(http.Flusher)

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
		flusher.Flush()
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

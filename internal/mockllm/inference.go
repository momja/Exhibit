package mockllm

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Artifact inference (Exh-zue3). The mock serves the provider HTTP side only:
// requests still go through Pi's registry and its real OpenRouter adapters, so
// what is exercised is the wire format, usage reporting and error paths.
//
// A chat request is an inference request when its system prompt carries the
// inference preamble (InferencePreambleMarker) or its last user message
// carries the explicit "mock-inference" marker. Agent chat keeps its tool
// script either way.
//
// Answers are deterministic functions of the request so tests and demos can
// rely on them:
//   - prose: "Summary: " and the start of the last user message, or "OK" plus
//     any tenant-A / tenant-B identity for credential-pairing tests;
//   - structured output: when the system prompt carries "JSON Schema:", a
//     value generated from that schema;
//   - classification: an option chosen by word overlap between its criterion
//     and the state, and probabilities that follow from it.
//
// Fault markers anywhere in the inference request text:
//
//	mock-inference-slow             answer after 300ms (cancellation tests)
//	mock-inference-http-error       401 with a body echoing a fake key
//	mock-inference-bad-json         first structured answer is not JSON
//	mock-inference-always-bad-json  every structured answer is not JSON
//	mock-inference-malformed        classifier answers nothing (billed)

// InferencePreambleMarker is the phrase the inference service's fixed system
// preamble contains. Matching on server-authored prompt text is how the mock
// already recognizes the guard screen and widget-only sessions.
const InferencePreambleMarker = "a request from a small web tool"

func isInferenceChat(messages []chatMessage) bool {
	if len(messages) == 0 {
		return false
	}
	if messages[0].Role == "system" {
		if s, _ := textOf(messages[0].Content); strings.Contains(s, InferencePreambleMarker) {
			return true
		}
	}
	last := messages[len(messages)-1]
	text, _ := textOf(last.Content)
	return last.Role == "user" && strings.Contains(text, "mock-inference")
}

func handleInferenceChat(w http.ResponseWriter, r *http.Request, messages []chatMessage) {
	var all strings.Builder
	system, lastUser := "", ""
	users := 0
	for _, m := range messages {
		t, _ := textOf(m.Content)
		all.WriteString(t)
		all.WriteString("\n")
		switch m.Role {
		case "system":
			system = t
		case "user":
			lastUser = t
			users++
		}
	}
	text := all.String()
	if !inferenceReady(w, r, text) {
		return
	}
	var answer string
	if _, schemaText, ok := strings.Cut(system, "JSON Schema:"); ok {
		badFirst := strings.Contains(text, "mock-inference-bad-json") && users == 1
		if badFirst || strings.Contains(text, "mock-inference-always-bad-json") {
			answer = "Sure! Here is what you asked for."
		} else {
			var schema map[string]any
			_ = json.Unmarshal([]byte(strings.TrimSpace(schemaText)), &schema)
			out, _ := json.Marshal(valueForSchema(schema, lastUser, 0))
			answer = string(out)
		}
	} else if strings.Contains(lastUser, "mock-inference") {
		answer = "OK"
		for _, tenant := range []string{"tenant-A", "tenant-B"} {
			if strings.Contains(lastUser, tenant) {
				answer += " " + tenant
			}
		}
	} else {
		answer = "Summary: " + clip(strings.Join(strings.Fields(lastUser), " "), 160)
	}
	streamPlan(w, turnPlan{kind: "text", text: answer, usage: map[string]any{
		"prompt_tokens": 100, "completion_tokens": 12, "total_tokens": 112,
		"cost": 0.0000104, "is_byok": false,
	}})
}

// valueForSchema produces a value that satisfies the subset of JSON Schema
// the inference service accepts. seed text fills strings so answers differ
// with their input.
func valueForSchema(schema map[string]any, seed string, depth int) any {
	if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
		return enum[int(hash(seed))%len(enum)]
	}
	if c, ok := schema["const"]; ok {
		return c
	}
	typ := schema["type"]
	if list, ok := typ.([]any); ok && len(list) > 0 {
		typ = list[0]
	}
	switch typ {
	case "object":
		out := map[string]any{}
		props, _ := schema["properties"].(map[string]any)
		for name, sub := range props {
			if s, ok := sub.(map[string]any); ok && depth < 8 {
				out[name] = valueForSchema(s, seed+name, depth+1)
			}
		}
		return out
	case "array":
		n := 2
		if v, ok := schema["minItems"].(float64); ok && int(v) > n {
			n = int(v)
		}
		if v, ok := schema["maxItems"].(float64); ok && int(v) < n {
			n = int(v)
		}
		items, _ := schema["items"].(map[string]any)
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, valueForSchema(items, fmt.Sprint(seed, i), depth+1))
		}
		return out
	case "integer", "number":
		v := 1.0
		if m, ok := schema["minimum"].(float64); ok && v < m {
			v = m
		}
		if m, ok := schema["maximum"].(float64); ok && v > m {
			v = m
		}
		return v
	case "boolean":
		return hash(seed)%2 == 0
	case "null":
		return nil
	default:
		s := "Mock " + clip(strings.Join(strings.Fields(seed), " "), 40)
		if m, ok := schema["maxLength"].(float64); ok && len(s) > int(m) {
			s = s[:int(m)]
		}
		for m, ok := schema["minLength"].(float64); ok && len(s) < int(m); {
			s += "."
		}
		return s
	}
}

func handleClassify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State     map[string]any `json:"state"`
		Questions map[string]struct {
			Type     string          `json:"type"`
			Criteria json.RawMessage `json:"criteria"`
		} `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.State == nil || len(req.Questions) == 0 {
		http.Error(w, "state and questions required", http.StatusBadRequest)
		return
	}
	state, _ := json.Marshal(req.State)
	text := strings.ToLower(string(state))
	answers := map[string]any{}
	for id, q := range req.Questions {
		switch q.Type {
		case "noul": // Pi normalizes bool to this System One wire type.
			var c map[string]string
			_ = json.Unmarshal(q.Criteria, &c)
			p := 0.5 + 0.45*(overlap(text, c["true"])-overlap(text, c["false"]))
			if p < 0.02 {
				p = 0.02
			}
			if p > 0.98 {
				p = 0.98
			}
			if c == nil { // criteria not on the wire: the historical fixed answer
				p = 0.98
			}
			answers[id] = map[string]any{"type": "noul", "noul": round3(p)}
		case "choice":
			var c map[string]string
			_ = json.Unmarshal(q.Criteria, &c)
			if len(c) == 0 {
				c = map[string]string{"positive": "positive", "negative": "negative"}
			}
			labels := make([]string, 0, len(c))
			for k := range c {
				labels = append(labels, k)
			}
			sort.Strings(labels)
			best := labels[int(hash(text))%len(labels)]
			bestScore := -1.0
			for _, l := range labels {
				if s := overlap(text, l+" "+c[l]); s > bestScore {
					best, bestScore = l, s
				}
			}
			probs := map[string]float64{}
			rest := 0.1 / float64(max(1, len(labels)-1))
			for _, l := range labels {
				probs[l] = round3(rest)
			}
			probs[best] = 0.9
			if len(labels) == 1 {
				probs[best] = 1
			}
			answers[id] = map[string]any{"type": "choice", "choice": best, "probabilities": probs, "confidence": 0.9}
		case "score":
			var levels []string
			_ = json.Unmarshal(q.Criteria, &levels)
			n := max(len(levels), 3)
			best, bestScore := float64(n-1), -1.0
			for i, l := range levels {
				if s := overlap(text, l); s > bestScore {
					best, bestScore = float64(i), s
				}
			}
			answers[id] = map[string]any{"type": "score", "score": best, "confidence": 0.85}
		default:
			http.Error(w, "unsupported wire question type", http.StatusBadRequest)
			return
		}
	}
	if !inferenceReady(w, r, string(state)) {
		return
	}
	if strings.Contains(text, "mock-inference-malformed") {
		answers = map[string]any{} // A billed response the real adapter must reject.
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers, "usage": map[string]int{"input_tokens": 100, "output_tokens": 0}})
}

// overlap is the share of a criterion's words (four letters or more) that
// appear in text: a crude, deterministic stand-in for judgment.
func overlap(text, criterion string) float64 {
	words := strings.FieldsFunc(strings.ToLower(criterion), func(r rune) bool {
		return !(r >= 'a' && r <= 'z')
	})
	n, hit := 0, 0
	for _, w := range words {
		if len(w) < 4 {
			continue
		}
		n++
		if strings.Contains(text, w) {
			hit++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(hit) / float64(n)
}

func hash(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

func round3(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}

// Explicit fault markers apply only to inference requests. A slow request
// waits on the real HTTP context, so aborting Pi's fetch cancels the handler.
func inferenceReady(w http.ResponseWriter, r *http.Request, text string) bool {
	if strings.Contains(text, "mock-inference-slow") {
		timer := time.NewTimer(300 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return false
		case <-timer.C:
		}
	}
	if strings.Contains(text, "mock-inference-http-error") {
		w.WriteHeader(http.StatusUnauthorized)
		// Deliberate fake-key echo exercises the host's error redaction.
		fmt.Fprint(w, `{"error":{"message":"test-key-A must never escape"}}`)
		return false
	}
	return true
}

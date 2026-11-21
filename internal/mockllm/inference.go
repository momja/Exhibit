package mockllm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// The inference script is intentionally small: OK text and positive-sentiment
// bool/choice/score answers with fixed usage. It mocks the HTTP provider, not
// Pi's registry, adapters or catalog. Agent chat keeps its usual tool script
// unless the last user message explicitly contains "mock-inference".
func handleInferenceChat(w http.ResponseWriter, r *http.Request, text string) {
	if !inferenceReady(w, r, text) {
		return
	}
	answer := "OK"
	for _, tenant := range []string{"tenant-A", "tenant-B"} {
		if strings.Contains(text, tenant) {
			answer += " " + tenant
		}
	}
	streamPlan(w, turnPlan{kind: "text", text: answer, usage: map[string]any{
		"prompt_tokens": 100, "completion_tokens": 1, "total_tokens": 101,
		"cost": 0.0000104, "is_byok": false,
	}})
}

func handleClassify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State     map[string]any `json:"state"`
		Questions map[string]struct {
			Type string `json:"type"`
		} `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.State == nil || len(req.Questions) == 0 {
		http.Error(w, "state and questions required", http.StatusBadRequest)
		return
	}
	answers := map[string]any{}
	for id, q := range req.Questions {
		switch q.Type {
		case "noul": // Pi normalizes bool to this System One wire type.
			answers[id] = map[string]any{"type": "noul", "noul": 0.98}
		case "choice": // Canned sentiment criteria: positive / negative.
			answers[id] = map[string]any{"type": "choice", "choice": "positive", "probabilities": map[string]float64{"positive": 0.98, "negative": 0.02}, "confidence": 0.96}
		case "score":
			answers[id] = map[string]any{"type": "score", "score": 0.9, "confidence": 0.95}
		default:
			http.Error(w, "unsupported wire question type", http.StatusBadRequest)
			return
		}
	}
	state, _ := json.Marshal(req.State)
	text := string(state)
	if !inferenceReady(w, r, text) {
		return
	}
	if strings.Contains(text, "mock-inference-malformed") {
		answers = map[string]any{} // A billed response the real adapter must reject.
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers, "usage": map[string]int{"input_tokens": 100, "output_tokens": 0}})
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

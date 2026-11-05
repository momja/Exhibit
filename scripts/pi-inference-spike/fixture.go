package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// Loopback HTTP fixture, not a provider measurement. Exercises Pi's real adapters.
type fixture struct {
	server                        *httptest.Server
	mu                            sync.Mutex
	active, peak, canceled, calls int
	leaks                         int
}

func newFixture() *fixture {
	f := &fixture{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		encoded, _ := json.Marshal(body)
		text := string(encoded)
		f.mu.Lock()
		f.calls++
		f.active++
		if f.active > f.peak {
			f.peak = f.active
		}
		// Check pairing against independent payload identity; never return/print the key.
		if strings.Contains(text, "tenant-A") && r.Header.Get("Authorization") != "Bearer test-key-A" ||
			strings.Contains(text, "tenant-B") && r.Header.Get("Authorization") != "Bearer test-key-B" {
			f.leaks++
		}
		f.mu.Unlock()
		defer func() { f.mu.Lock(); f.active--; f.mu.Unlock() }()
		delay := time.Millisecond
		if strings.Contains(text, "slow") {
			delay = 300 * time.Millisecond
		}
		select {
		case <-r.Context().Done():
			f.mu.Lock()
			f.canceled++
			f.mu.Unlock()
			return
		case <-time.After(delay):
		}
		if strings.Contains(text, "http-error") {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":{"message":"test-key-A must never escape"}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "systemone") {
			answers := map[string]any{}
			questions, _ := body["questions"].(map[string]any)
			for id, raw := range questions {
				q := raw.(map[string]any)
				switch q["type"] {
				case "noul":
					answers[id] = map[string]any{"type": "noul", "noul": 0.98}
				case "choice":
					answers[id] = map[string]any{"type": "choice", "choice": "positive", "probabilities": map[string]float64{"positive": 0.98, "negative": 0.02}, "confidence": 0.96}
				case "score":
					answers[id] = map[string]any{"type": "score", "score": 0.9, "confidence": 0.95}
				}
			}
			if strings.Contains(text, "malformed") {
				answers = map[string]any{}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers, "usage": map[string]int{"input_tokens": 100, "output_tokens": 0}})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		answer := "OK"
		if strings.Contains(text, "tenant-A") {
			answer += " tenant-A"
		}
		if strings.Contains(text, "tenant-B") {
			answer += " tenant-B"
		}
		chunk, _ := json.Marshal(map[string]any{"id": "fake-1", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": answer}, "finish_reason": nil}}})
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		fmt.Fprint(w, "data: {\"id\":\"fake-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":1,\"total_tokens\":101,\"cost\":0.0000104,\"is_byok\":false}}\n\ndata: [DONE]\n\n")
	}))
	return f
}

func sampleRequest(kind, model, key, identity string) request {
	if kind == "complete" {
		return request{Op: kind, Model: model, APIKey: key, Context: map[string]any{"messages": []any{map[string]any{"role": "user", "content": "Reply only OK. " + identity, "timestamp": int64(0)}}}}
	}
	return request{Op: kind, Model: model, APIKey: key, Context: map[string]any{
		"state": map[string]any{"text": "A wonderful sunny day. " + identity},
		"questions": map[string]any{
			"positive":  map[string]any{"type": "bool", "instructions": "Is the sentiment positive?", "criteria": map[string]string{"true": "Positive sentiment", "false": "Negative sentiment"}},
			"sentiment": map[string]any{"type": "choice", "instructions": "Choose the sentiment.", "criteria": map[string]string{"positive": "Positive sentiment", "negative": "Negative sentiment"}},
			"rating":    map[string]any{"type": "score", "instructions": "Rate positive sentiment.", "criteria": []string{"Negative", "Neutral", "Positive"}},
		},
	}}
}

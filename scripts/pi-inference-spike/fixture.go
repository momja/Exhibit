package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/momja/Exhibit/internal/mockllm"
)

// Instrument the shared mockLLM HTTP handler, not Pi's registry/adapters.
// Only credential/overlap assertions and a 1 ms benchmark delay live here.
type fixture struct {
	server                        *httptest.Server
	mu                            sync.Mutex
	active, peak, canceled, calls int
	leaks                         int
}

func newFixture() *fixture {
	f := &fixture{}
	provider := mockllm.Handler()
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
		defer func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.active--
			if r.Context().Err() != nil {
				f.canceled++
			}
		}()
		timer := time.NewTimer(time.Millisecond)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
		// Restore the body after inspecting the independent tenant identity.
		r.Body = io.NopCloser(bytes.NewReader(encoded))
		provider.ServeHTTP(w, r)
	}))
	return f
}

func sampleRequest(kind, model, key, identity string) request {
	if kind == "complete" {
		return request{Op: kind, Model: model, APIKey: key, Context: map[string]any{"messages": []any{map[string]any{"role": "user", "content": "mock-inference Reply only OK. " + identity, "timestamp": int64(0)}}}}
	}
	return request{Op: kind, Model: model, APIKey: key, Context: map[string]any{
		"state": map[string]any{"text": "mock-inference A wonderful sunny day. " + identity},
		"questions": map[string]any{
			"positive":  map[string]any{"type": "bool", "instructions": "Is the sentiment positive?", "criteria": map[string]string{"true": "Positive sentiment", "false": "Negative sentiment"}},
			"sentiment": map[string]any{"type": "choice", "instructions": "Choose the sentiment.", "criteria": map[string]string{"positive": "Positive sentiment", "negative": "Negative sentiment"}},
			"rating":    map[string]any{"type": "score", "instructions": "Rate positive sentiment.", "criteria": []string{"Negative", "Neutral", "Positive"}},
		},
	}}
}

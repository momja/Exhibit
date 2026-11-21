package mockllm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Keep route and script selection covered even when native Pi tests are not
// configured. Real-adapter answer/usage, cancellation and error assertions live
// in scripts/pi-inference-spike/worker_test.go rather than being duplicated here.
func TestHandlerScriptSelection(t *testing.T) {
	for _, prefix := range []string{"", "/v1", "/api/v1"} {
		for _, tc := range []struct {
			name, messages, want string
		}{
			{"agent", `[{"role":"user","content":"Build a counter"}]`, `"name":"create_artifact"`},
			{"guard", `[{"role":"system","content":"You are a usage-policy screen"},{"role":"user","content":"mock-policy-violation"}]`, `"content":"BLOCK"`},
			{"inference", `[{"role":"user","content":"mock-inference Reply OK"}]`, `"content":"OK"`},
			{"system marker", `[{"role":"system","content":"mock-inference"},{"role":"user","content":"Build a counter"}]`, `"name":"create_artifact"`},
		} {
			t.Run(prefix+"/"+tc.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, prefix+"/chat/completions", strings.NewReader(`{"messages":`+tc.messages+`}`)))
				if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.want) {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
			})
		}
		// Pi's normalized bool belongs at the registry boundary; the HTTP mock
		// expects System One's noul and refuses unsupported wire types.
		for _, body := range []string{`null`, `{}`, `{"state":{},"questions":{"q":null}}`, `{"state":{},"questions":{"q":{"type":"bool"}}}`} {
			w := httptest.NewRecorder()
			Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, prefix+"/systemone", strings.NewReader(body)))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("invalid classifier request %s: status=%d", body, w.Code)
			}
		}
	}
}

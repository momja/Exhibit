package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run the real Pi provider normalizer, message events and stats RPC. The mock
// supplies fixed usage on every actual provider request. Prepared source reads
// make no provider request and must contribute no synthetic usage;
// cached prompt tokens must be separated from input and counted only once.
func TestPiUsageIsMeteredOnceAcrossTurnsAndResume(t *testing.T) {
	h := newPlatformPiHarness(t)
	r := h.router
	inner := h.llm.inner
	var requests atomic.Int64
	h.llm.inner = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		response := httptest.NewRecorder()
		inner.ServeHTTP(response, req)
		for name, values := range response.Header() {
			w.Header()[name] = values
		}
		w.WriteHeader(response.Code)
		usage := `data: {"id":"metered","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":240,"completion_tokens":60,"total_tokens":300,"prompt_tokens_details":{"cached_tokens":40}}}`
		_, _ = w.Write([]byte(strings.Replace(response.Body.String(), "data: [DONE]", usage+"\n\ndata: [DONE]", 1)))
	})
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": counterBody})
	session := conversation(t, h, id, "make the button purple")

	check := func() {
		t.Helper()
		count := requests.Load()
		require.Positive(t, count)
		want := store.UsageTotals{InputTokens: 200 * count, OutputTokens: 60 * count, CacheReadTokens: 40 * count}
		totals, err := r.cfg.Store.SessionAgentSpend(context.Background(), defaultOwnerID, session)
		require.NoError(t, err)
		assert.Equal(t, want, totals, "meter each actual provider request once, excluding prepared source")
		owner, err := r.cfg.Store.OwnerAgentSpend(context.Background(), defaultOwnerID, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, want, owner)
		instance, err := r.cfg.Store.InstanceAgentSpend(context.Background(), time.Time{})
		require.NoError(t, err)
		assert.Equal(t, want, instance)
	}
	check()
	prompt(t, r, session, "now make it red")
	keptConversation(t, r, id, session, 2)
	check()
	closeAndWait(t, r, session)
	session = resume(t, r, id, session)
	prompt(t, r, session, "now make it green")
	keptConversation(t, r, id, session, 3)
	check()
	// The durable meter also survives closing the resumed process.
	closeAndWait(t, r, session)
	check()
}

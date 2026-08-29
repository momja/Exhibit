package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The update path's origin-approval reporting (av-hrtv): a modification must
// filter the raw scan against the artifact's already-approved origins before
// reporting it — a false alarm the create path never has to worry about,
// since a brand-new artifact starts with nothing approved — and it must never
// disturb those approvals. Driven through a real `pi` sidecar and the
// scripted internal/mockllm (newPiHarness, agent_pipeline_test.go) so the
// read that mattered — what the extension actually does with the PATCH
// response — is exercised for real, not assumed from a server-side read.

// sessionEvents is one session's broadcast event stream, decoded.
type sessionEvents []map[string]any

// modify opens a session bound to artifactID, sends one prompt, and returns
// every event the session broadcast during that turn. It reads through to
// agent_settled rather than stopping at the save, so the tool result the
// model was shown is in the stream too.
func modify(t *testing.T, h *piHarness, artifactID, prompt string) sessionEvents {
	t.Helper()
	r := h.router

	w := doJSON(t, r, "POST", "/api/agent/sessions", map[string]string{"artifact_id": artifactID})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	t.Cleanup(func() { r.cfg.Agent.Close(defaultOwnerID, created.ID) })

	s := r.cfg.Agent.Get(defaultOwnerID, created.ID)
	require.NotNil(t, s)
	events, unsubscribe := s.Subscribe()
	defer unsubscribe()

	w = doJSON(t, r, "POST", "/api/agent/sessions/"+created.ID+"/prompt", map[string]any{"message": prompt})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	var seen sessionEvents
	deadline := time.After(90 * time.Second)
	for {
		select {
		case raw := <-events:
			var ev map[string]any
			if json.Unmarshal(raw, &ev) != nil {
				continue
			}
			seen = append(seen, ev)
			if ev["type"] == "agent_settled" {
				return seen
			}
		case <-deadline:
			t.Fatalf("agent never settled after prompt %q (saw %d events)", prompt, len(seen))
		}
	}
}

// saved returns the synthetic artifact-saved event — the one the chat page
// turns into a preview swap.
func (evs sessionEvents) saved(t *testing.T) map[string]any {
	t.Helper()
	for _, ev := range evs {
		if ev["type"] == "exhibit_artifact_saved" {
			return ev
		}
	}
	t.Fatal("no exhibit_artifact_saved event in the session events")
	return nil
}

// toolResult is a finished tool call's reported text and details, decoded
// once so a test asserting on both needs one lookup.
type toolResult struct {
	text    string
	details map[string]any
}

// toolResultFor finds the finished call to toolName and decodes what it
// reported: the text the model was actually shown, and the structured details
// (exhibit.ts's second ok() argument) the session's event handling reads.
func (evs sessionEvents) toolResultFor(t *testing.T, toolName string) toolResult {
	t.Helper()
	for _, ev := range evs {
		if ev["type"] != "tool_execution_end" || ev["toolName"] != toolName {
			continue
		}
		result, _ := ev["result"].(map[string]any)
		details, _ := result["details"].(map[string]any)
		require.NotNil(t, details, "tool result carried no details")
		content, _ := result["content"].([]any)
		require.NotEmpty(t, content, "tool result carried no content")
		first, _ := content[0].(map[string]any)
		text, _ := first["text"].(string)
		return toolResult{text: text, details: details}
	}
	t.Fatalf("no finished %s tool call in the session events", toolName)
	return toolResult{}
}

// jsonStrings converts a decoded JSON array to a string slice.
func jsonStrings(t *testing.T, v any) []string {
	t.Helper()
	raw, ok := v.([]any)
	require.True(t, ok, "expected a JSON array, got %T", v)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		require.True(t, ok, "expected a string element, got %T", item)
		out = append(out, s)
	}
	return out
}

// allowlistOf reads an artifact's approved origins back through the API.
func allowlistOf(t *testing.T, r *Router, id string) []string {
	t.Helper()
	w := doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var art struct {
		NetworkAllowlist []string `json:"network_allowlist"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &art))
	return art.NetworkAllowlist
}

// AC3: when a modification pulls in an origin the user has not approved, the
// saved event carries it, and the tool told the model in words — the same
// pending-approval note a creation gets. Already-approved origins are left
// out: they are not blocked, and asking for approval again would be a false
// alarm. footprintChanged, threaded alongside, reflects that this edit really
// did change the scanned footprint.
func TestAgentUpdateReportsOriginsAwaitingApproval(t *testing.T) {
	h := newPiHarness(t)
	const body = `<html><head><title>Charty</title>` +
		`<script src="https://approved.example.com/lib.js"></script></head><body><h1>Charty</h1></body></html>`
	id := createArtifact(t, h.router, map[string]any{
		"title":             "Charty",
		"body":              body,
		"network_allowlist": []string{"https://approved.example.com"},
	})

	events := modify(t, h, id, "add the confetti library from https://cdn.example.com/confetti.js")
	footprint := jsonStrings(t, events.saved(t)["footprint"])

	assert.Contains(t, footprint, "https://cdn.example.com",
		"a newly referenced, unapproved origin must reach the chat so the user can approve it")
	assert.NotContains(t, footprint, "https://approved.example.com",
		"an already-approved origin is not blocked, so it must not be reported as pending approval")

	result := events.toolResultFor(t, "update_artifact")
	assert.Contains(t, result.text, "https://cdn.example.com",
		"the model must be told in words, the way create_artifact already is")
	assert.Equal(t, true, result.details["footprintChanged"], "a newly referenced origin did change the footprint")
}

// AC4 through the real agent path: a rewrite must not disturb the artifact's
// approved origins, and the render CSP built from them must still reach the
// origin afterward. Approval is per (artifact, origin), not per body version
// — see docs/security.md, "An approved origin outlives the code it was
// approved for" — so re-gating on this rewrite is deliberately not attempted.
func TestAgentUpdateKeepsApprovedOriginsReachable(t *testing.T) {
	h := newPiHarness(t)
	const origin = "https://approved.example.com"
	const body = `<html><head><title>Charty</title>` +
		`<script src="` + origin + `/lib.js"></script></head><body><h1>Charty</h1></body></html>`
	id := createArtifact(t, h.router, map[string]any{
		"title":             "Charty",
		"body":              body,
		"network_allowlist": []string{origin},
	})

	modify(t, h, id, "make the heading bigger")

	assert.Equal(t, []string{origin}, allowlistOf(t, h.router, id),
		"an agent rewrite must not drop or re-gate origins the user already approved")

	// The approval is only meaningful if it still reaches the wall: the
	// per-artifact CSP built at render time must still carry the origin.
	tok := h.router.tokens.Mint(id, defaultOwnerID)
	req := httptest.NewRequest("GET", "/a/"+id+"?"+rendertoken.Param+"="+tok, nil)
	rr := httptest.NewRecorder()
	h.router.RenderHandler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Security-Policy"), origin)
}

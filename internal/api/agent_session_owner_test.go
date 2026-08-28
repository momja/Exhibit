package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Agent sessions live in an in-memory registry, not in SQLite, so av-ep8k's
// sweep — which made owner_id a predicate inside every Store query — did not
// reach them. `Manager.Get` took an id and nothing else, and the four routes
// that resolve a session by id compared nothing: any authenticated user holding
// somebody else's session id could steer their agent, read their event stream,
// and kill their subprocess.
//
// The prompt case is the sharp one. A session's tool calls run on the *session's*
// av-e0yj credential, so a prompt injected by a stranger writes into the
// victim's artifact under the victim's own scope — the containment security.md
// §5.1 describes, defeated rather than sidestepped.
//
// These tests are the walk that holds it: every session route, reached by the
// owner and refused to a second real account, with a live pi subprocess behind
// them rather than a stub, because the object being protected is the live
// session.

// agentOwnerHarness is a pi harness with two real accounts on it. The session
// under test belongs to owner 1 — the id the static token also resolves to, so
// the same fixture exercises the multi-user and the single-user credential
// against one registry.
type agentOwnerHarness struct {
	*piHarness
	server         *httptest.Server
	ownerCookie    *http.Cookie
	intruderCookie *http.Cookie
	sessionID      string
	artifactID     string
}

func newAgentOwnerHarness(t *testing.T) *agentOwnerHarness {
	t.Helper()
	h := newPiHarness(t)
	r := h.router
	ctx := context.Background()

	// The first account created lands on owner 1, which is where a session
	// opened with the static token also lands — so this account is the session's
	// owner and the intruder is a genuinely different one.
	owner, err := r.cfg.Store.CreateLocalUser(ctx, store.NewLocalUser{
		ExternalID: auth.LocalExternalID("owner"), Email: "owner", PasswordHash: testHash(t, "owner-long-passphrase"),
	})
	require.NoError(t, err)
	require.Equal(t, defaultOwnerID, owner.ID)
	intruder, err := r.cfg.Store.CreateLocalUser(ctx, store.NewLocalUser{
		ExternalID: auth.LocalExternalID("intruder"), Email: "intruder", PasswordHash: testHash(t, "intruder-long-passphrase"),
	})
	require.NoError(t, err)
	require.NotEqual(t, owner.ID, intruder.ID)

	// Accounts exist, so this instance has a login and sessionUser resolves
	// cookies — the state a self-hoster reaches by running `user add`.
	r.cfg.LocalUsers = true

	out := &agentOwnerHarness{
		piHarness:      h,
		server:         httptest.NewServer(r),
		ownerCookie:    sessionCookieFor(t, r.cfg.Store, owner.ID, "session-agent-owner"),
		intruderCookie: sessionCookieFor(t, r.cfg.Store, intruder.ID, "session-agent-intruder"),
	}
	t.Cleanup(out.server.Close)

	out.artifactID = createArtifact(t, r, map[string]any{
		"title": "Counter",
		"body":  "<html><body><button id=\"submit-btn\">Count!</button></body></html>",
	})
	w := doJSON(t, r, "POST", "/api/agent/sessions", map[string]string{"artifact_id": out.artifactID})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var session struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	out.sessionID = session.ID
	t.Cleanup(func() { h.router.cfg.Agent.Close(defaultOwnerID, session.ID) })
	return out
}

func (h *agentOwnerHarness) path(suffix string) string {
	return "/api/agent/sessions/" + h.sessionID + suffix
}

// doAs issues a request as the account whose session cookie is given — no
// bearer token, so the cookie is the only credential and the owner it resolves
// to is the only thing the route can scope on.
func doAs(t *testing.T, r *Router, method, path string, c *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	rdr := bytes.NewReader(nil)
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if c != nil {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// openEventStream opens the SSE route over a real connection and returns the
// status the handler answered with.
//
// A recorder cannot serve here: a stream that opens successfully never returns,
// so the test needs the status the moment the headers are flushed and a way to
// hang up afterwards. Closing the body ends the request, which is what the
// handler's `<-r.Context().Done()` case is waiting for.
func openEventStream(t *testing.T, h *agentOwnerHarness, query string, c *http.Cookie) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.server.URL+h.path("/events")+query, nil)
	require.NoError(t, err)
	if c != nil {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

// The refusal, on every route that resolves a session by id.
//
// It is a 404 and not a 403 for the reason architecture §3.3 gives for the
// store and admin.go gives for the account directory: a permission error
// confirms the id is live, which would make these routes an oracle over which
// session ids exist. So the assertion is not merely "refused" — it is
// "indistinguishable from an id that was never issued", and the invented id
// below is what that is measured against.
func TestAgentSessionRoutesRefuseAnotherOwner(t *testing.T) {
	h := newAgentOwnerHarness(t)
	r := h.router

	// The stream first, and deliberately: the DELETE below ends the session, so
	// a refusal asserted after it would be the right status for the wrong
	// reason.
	t.Run("events", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, openEventStream(t, h, "", h.intruderCookie),
			"the SSE route authenticates outside the auth middleware (EventSource sets no headers), "+
				"which is exactly why it has to resolve an owner of its own rather than settle for "+
				"'this request is authenticated'")
	})

	for _, tc := range []struct {
		name, method, suffix string
		body                 any
	}{
		{"prompt", http.MethodPost, "/prompt", map[string]any{"message": "make the button green"}},
		{"abort", http.MethodPost, "/abort", nil},
		{"close", http.MethodDelete, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := doAs(t, r, tc.method, h.path(tc.suffix), h.intruderCookie, tc.body)
			require.Equal(t, http.StatusNotFound, w.Code,
				"%s %s let a second account act on owner %d's agent session. The session registry is "+
					"in memory, so nothing filters it by owner on the handler's behalf the way the "+
					"Store's SQL does (av-ep8k) — the predicate has to be in Manager.Get.",
				tc.method, h.path(tc.suffix), defaultOwnerID)

			// And the same answer as an id that never existed, byte for byte.
			missing := doAs(t, r, tc.method,
				"/api/agent/sessions/no-such-session-id"+tc.suffix, h.intruderCookie, tc.body)
			assert.Equal(t, w.Code, missing.Code)
			assert.Equal(t, w.Body.String(), missing.Body.String(),
				"a refusal that differs from 'no such session' tells the caller the id is live")
		})
	}

	// The session survived all of it — in particular the DELETE, where an
	// unscoped id costs a subprocess rather than a read.
	assert.NotNil(t, r.cfg.Agent.Get(defaultOwnerID, h.sessionID),
		"a stranger's DELETE killed the owner's live session")
}

// The other half, and the one that makes the refusals mean something: the
// session's own owner still reaches every one of these routes, holding nothing
// but their session cookie.
func TestAgentSessionOwnerReachesEveryRoute(t *testing.T) {
	h := newAgentOwnerHarness(t)
	r := h.router

	assert.Equal(t, http.StatusOK, openEventStream(t, h, "", h.ownerCookie))

	w := doAs(t, r, http.MethodPost, h.path("/prompt"), h.ownerCookie,
		map[string]any{"message": "make the button green"})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	w = doAs(t, r, http.MethodPost, h.path("/abort"), h.ownerCookie, nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	w = doAs(t, r, http.MethodDelete, h.path(""), h.ownerCookie, nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	// Closed is closed: the route answers a gone session the way it answers
	// somebody else's, which is the property that keeps the two indistinguishable.
	w = doAs(t, r, http.MethodDelete, h.path(""), h.ownerCookie, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Nil(t, r.cfg.Agent.Get(defaultOwnerID, h.sessionID))
}

// The single-user instance, which is most of them: no login, no accounts, the
// static token and owner 1. The token is authenticated by authMiddleware and
// attributed by ownerMiddleware, and on the SSE route by authorizeEventStream
// standing in for both — so every session route has to keep working exactly as
// it did before any of this existed.
func TestSingleUserAgentSessionRoutesUnchanged(t *testing.T) {
	h := newPiHarness(t)
	r := h.router
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	id := createArtifact(t, r, map[string]any{
		"title": "Counter",
		"body":  "<html><body><button id=\"submit-btn\">Count!</button></body></html>",
	})
	w := doJSON(t, r, "POST", "/api/agent/sessions", map[string]string{"artifact_id": id})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var session struct {
		ID        string `json:"id"`
		SSETicket string `json:"sse_ticket"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	require.NotEmpty(t, session.SSETicket, "create returns the ticket the stream needs")
	base := "/api/agent/sessions/" + session.ID

	// The stream, with the session's SSE ticket in the query string — the only
	// place an EventSource can carry a credential, and since av-rgp1 the only
	// credential this route takes there. The service token is refused here even
	// on a single-user instance, which is the point: a URL ends up in logs and
	// history, and a ticket is worth a replay of one stream rather than the
	// library.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.URL+base+"/events?ticket="+url.QueryEscape(session.SSETicket), nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	w = doJSON(t, r, "POST", base+"/prompt", map[string]any{"message": "make the button green"})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	w = doJSON(t, r, "POST", base+"/abort", nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	w = doJSON(t, r, "DELETE", base, nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
}

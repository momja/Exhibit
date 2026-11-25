package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/inference"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeInferenceRuntime answers every call and counts them.
type fakeInferenceRuntime struct {
	mu   sync.Mutex
	jobs []inference.Job
}

func (f *fakeInferenceRuntime) Resolve(context.Context, string, inference.Kind, string) (bool, error) {
	return true, nil
}

func (f *fakeInferenceRuntime) Run(_ context.Context, job inference.Job) (inference.Result, inference.Outcome, error) {
	f.mu.Lock()
	f.jobs = append(f.jobs, job)
	f.mu.Unlock()
	return inference.Result{StopReason: "stop", Text: "a summary"},
		inference.Outcome{Dispatched: true, Usage: &inference.Usage{Input: 10, Output: 3, Cost: inference.UsageCost{Total: 0.0005}}}, nil
}

func (f *fakeInferenceRuntime) Screen(context.Context, inference.ScreenJob) (inference.ScreenVerdict, error) {
	return inference.ScreenVerdict{Allowed: true}, nil
}

func (f *fakeInferenceRuntime) calls() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.jobs) }

func withPlatformInference(t *testing.T, ro *Router) *fakeInferenceRuntime {
	t.Helper()
	rt := &fakeInferenceRuntime{}
	svc, err := inference.New(inference.Config{
		Enabled: true, Platform: &inference.Credential{Provider: "openrouter", APIKey: "platform-secret"},
		ChatModel: "chat-model-x", DecisionModel: "judge-model-y", Ledger: ro.cfg.Store, Runtime: rt,
	})
	require.NoError(t, err)
	ro.cfg.Inference = svc
	return rt
}

func postInference(ro *Router, artifactID string, c *http.Cookie, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts/"+artifactID+"/inference",
		strings.NewReader(`{"kind":"complete","payload":{"prompt":"Summarize"}}`))
	req.Header.Set("Content-Type", "application/json")
	if c != nil {
		req.AddCookie(c)
	}
	for _, m := range mutate {
		m(req)
	}
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

func inferenceCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code  string `json:"code"`
			Needs string `json:"needs"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Error.Code + "/" + body.Error.Needs
}

func TestInferenceNeedsTheOwnersApprovalThenEachRecipientsConsent(t *testing.T) {
	in := newGrantedInstance(t)
	rt := withPlatformInference(t, in.ro)
	ctx := context.Background()

	w := postInference(in.ro, in.granted, in.cookieOne)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Equal(t, "not_approved/owner_approval", inferenceCode(t, w))
	w = postInference(in.ro, in.granted, in.cookieTwo)
	assert.Equal(t, "not_approved/owner_approval", inferenceCode(t, w), "a recipient cannot approve for the owner")

	require.NoError(t, in.ro.cfg.Store.UpdateArtifact(ctx, in.ownerOne, in.granted, map[string]any{"inference_approved": true}))
	w = postInference(in.ro, in.granted, in.cookieOne)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"value":"a summary"}`, w.Body.String())

	w = postInference(in.ro, in.granted, in.cookieTwo)
	assert.Equal(t, "not_approved/consent", inferenceCode(t, w), "the recipient pays, so the recipient agrees")
	put := httptest.NewRequest(http.MethodPut, "/api/artifacts/"+in.granted+"/inference/consent", nil)
	put.AddCookie(in.cookieTwo)
	pw := httptest.NewRecorder()
	in.ro.ServeHTTP(pw, put)
	require.Equal(t, http.StatusNoContent, pw.Code, pw.Body.String())
	w = postInference(in.ro, in.granted, in.cookieTwo)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 2, rt.calls())

	// The recipient's call was charged to the recipient; the owner's ledger
	// holds only the owner's own call.
	since := time.Now().Add(-time.Hour)
	ownerSpend, err := in.ro.cfg.Store.OwnerInferenceSpend(ctx, in.ownerOne, since)
	require.NoError(t, err)
	recipientSpend, err := in.ro.cfg.Store.OwnerInferenceSpend(ctx, in.ownerTwo, since)
	require.NoError(t, err)
	assert.Equal(t, int64(500), ownerSpend.CostMicros)
	assert.Equal(t, int64(500), recipientSpend.CostMicros)
	rows, err := in.ro.cfg.Store.InferenceUsageByArtifact(ctx, in.ownerOne, since)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(1), rows[0].Calls)
}

func TestInferenceRefusesPrincipalsThatCannotPay(t *testing.T) {
	in := newGrantedInstance(t)
	rt := withPlatformInference(t, in.ro)
	require.NoError(t, in.ro.cfg.Store.UpdateArtifact(context.Background(), in.ownerOne, in.granted, map[string]any{"inference_approved": true}))

	// No credential at all: what a widget, a public link, an opened-directly
	// tab or another site would send — none of them holds the app session.
	w := postInference(in.ro, in.granted, nil)
	assert.NotEqual(t, http.StatusOK, w.Code)
	// A stranger without a grant learns nothing about the artifact.
	stranger, err := in.ro.cfg.Store.UpsertUser(context.Background(), "sub-stranger", "s@example.test")
	require.NoError(t, err)
	strangerCookie := sessionCookieFor(t, in.ro.cfg.Store, stranger.ID, "session-stranger")
	w = postInference(in.ro, in.granted, strangerCookie)
	assert.Equal(t, http.StatusNotFound, w.Code)
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/"+in.granted+"/inference", nil)
	req.AddCookie(strangerCookie)
	w = httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, "the page state is no oracle either")
	assert.Equal(t, 0, rt.calls())
}

func TestInferenceRefusesPublicVisitorsAndAgents(t *testing.T) {
	ctx := context.Background()
	for _, p := range []Principal{
		{OwnerID: 1, Kind: PrincipalNone},
		{OwnerID: 1, Kind: PrincipalPublic, ReadOnly: true},
		{OwnerID: 1, Kind: PrincipalAgentGrant},
	} {
		assert.False(t, inferencePrincipalAllowed(withPrincipal(ctx, p)), "%+v", p)
	}
	assert.True(t, inferencePrincipalAllowed(withPrincipal(ctx, Principal{OwnerID: 2, Kind: PrincipalSession})))
}

func TestInferenceIsUnavailableWhenOff(t *testing.T) {
	in := newGrantedInstance(t)
	w := postInference(in.ro, in.granted, in.cookieOne)
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "unavailable/", inferenceCode(t, w))
}

func TestInferenceStateNeverNamesProviderOrModel(t *testing.T) {
	in := newGrantedInstance(t)
	withPlatformInference(t, in.ro)
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/"+in.granted+"/inference", nil)
	req.AddCookie(in.cookieOne)
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	for _, secret := range []string{"openrouter", "chat-model-x", "judge-model-y", "platform-secret"} {
		assert.NotContains(t, w.Body.String(), secret)
	}
	var st inferencePageState
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &st))
	assert.True(t, st.Enabled && st.Complete && st.Decide && st.Owner)
	assert.Equal(t, "platform", st.Payer)
}

func TestSettingsListsAndRevokesAIPermissions(t *testing.T) {
	in := newGrantedInstance(t)
	withPlatformInference(t, in.ro)
	ctx := context.Background()
	require.NoError(t, in.ro.cfg.Store.UpdateArtifact(ctx, in.ownerOne, in.granted, map[string]any{"inference_approved": true}))
	require.NoError(t, in.ro.cfg.Store.SetInferenceConsent(ctx, store.ViewerID(in.ownerTwo), in.granted, true))

	page := in.get(t, "/settings", in.cookieTwo)
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, "AI in artifacts")
	assert.Contains(t, body, "Shared with you")
	assert.Contains(t, body, `data-ai-revoke="`+in.granted+`"`)
	assert.NotContains(t, body, "chat-model-x")

	del := httptest.NewRequest(http.MethodDelete, "/api/artifacts/"+in.granted+"/inference/consent", nil)
	del.AddCookie(in.cookieTwo)
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, del)
	require.Equal(t, http.StatusNoContent, w.Code)
	ok, err := in.ro.cfg.Store.HasInferenceConsent(ctx, store.ViewerID(in.ownerTwo), in.granted)
	require.NoError(t, err)
	assert.False(t, ok)
	approved, err := in.ro.cfg.Store.GetArtifact(ctx, in.ownerOne, in.granted)
	require.NoError(t, err)
	assert.True(t, approved.InferenceApproved, "withdrawing my consent leaves the owner's decision alone")
}

// The allowance sentence never says "0% used" about a month that has spent
// something, and says plainly when tools or the agent spent all of it.
func TestAllowanceUsedSentence(t *testing.T) {
	for _, tc := range []struct {
		a    inference.Allowance
		want string
	}{
		{inference.Allowance{}, "Nothing used yet"},
		{inference.Allowance{Spent: true, InferenceShare: 100}, "Less than 1% used, all of it by tools"},
		{inference.Allowance{Spent: true, UsedPercent: 12}, "12% used, all of it by the agent"},
		{inference.Allowance{Spent: true, UsedPercent: 40, InferenceShare: 25}, "40% used, 25% of that by tools and the rest by the agent"},
	} {
		assert.Equal(t, tc.want, allowanceUsed(tc.a))
	}
}

// A browser request that spends or authorizes AI must come from the app's own
// page. SameSite=Lax still carries the session cookie on a POST from a
// sibling subdomain, so the cookie alone does not prove it.
func TestInferenceRefusesRequestsFromAnotherPage(t *testing.T) {
	in := newGrantedInstance(t)
	rt := withPlatformInference(t, in.ro)
	ctx := context.Background()
	require.NoError(t, in.ro.cfg.Store.UpdateArtifact(ctx, in.ownerOne, in.granted, map[string]any{"inference_approved": true}))

	foreign := map[string]func(*http.Request){
		"another origin": func(r *http.Request) { r.Header.Set("Origin", "http://evil.test") },
		"same site":      func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-site") },
		"cross site":     func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
	}
	for name, mutate := range foreign {
		w := postInference(in.ro, in.granted, in.cookieOne, mutate)
		assert.Equal(t, http.StatusForbidden, w.Code, name)
		assert.Equal(t, "not_approved/", inferenceCode(t, w), name)

		put := httptest.NewRequest(http.MethodPut, "/api/artifacts/"+in.granted+"/inference/consent", nil)
		put.AddCookie(in.cookieTwo)
		mutate(put)
		pw := httptest.NewRecorder()
		in.ro.ServeHTTP(pw, put)
		assert.Equal(t, http.StatusForbidden, pw.Code, name+": consent")

		patch := httptest.NewRequest(http.MethodPatch, "/api/artifacts/"+in.granted, strings.NewReader(`{"inference_approved":false}`))
		patch.Header.Set("Content-Type", "application/json")
		patch.AddCookie(in.cookieOne)
		mutate(patch)
		aw := httptest.NewRecorder()
		in.ro.ServeHTTP(aw, patch)
		assert.Equal(t, http.StatusForbidden, aw.Code, name+": approval")
	}
	assert.Zero(t, rt.calls(), "nothing ran for a foreign page")
	a, err := in.ro.cfg.Store.GetArtifact(ctx, in.ownerOne, in.granted)
	require.NoError(t, err)
	assert.True(t, a.InferenceApproved, "a foreign page cannot change the approval")

	w := postInference(in.ro, in.granted, in.cookieOne, func(r *http.Request) {
		r.Header.Set("Origin", in.ro.cfg.AppOrigin)
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set(inferenceChannelHeader, in.ro.tokens.ArtifactChannel(in.granted, in.ownerOne))
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, rt.calls())
}

func TestInferenceRejectsAnArtifactPageFromBeforeAnAccountSwitch(t *testing.T) {
	in := newGrantedInstance(t)
	rt := withPlatformInference(t, in.ro)
	ctx := t.Context()
	require.NoError(t, in.ro.cfg.Store.UpdateArtifact(ctx, in.ownerOne, in.granted, map[string]any{"inference_approved": true}))
	ownerChannel := in.ro.tokens.ArtifactChannel(in.granted, in.ownerOne)
	recipientChannel := in.ro.tokens.ArtifactChannel(in.granted, in.ownerTwo)
	path := "/api/artifacts/" + in.granted

	fromPage := func(channel string) func(*http.Request) {
		return func(r *http.Request) {
			r.Header.Set("Sec-Fetch-Site", "same-origin")
			if channel != "" {
				r.Header.Set(inferenceChannelHeader, channel)
			}
		}
	}
	request := func(method, path, channel, body string, cookie *http.Cookie) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.AddCookie(cookie)
		req.Header.Set("Content-Type", "application/json")
		fromPage(channel)(req)
		w := httptest.NewRecorder()
		in.ro.ServeHTTP(w, req)
		return w.Code
	}
	consent := path + "/inference/consent"
	for _, channel := range []string{ownerChannel, recipientChannel} {
		req := httptest.NewRequest(http.MethodGet, path+"/state", nil)
		req.AddCookie(in.cookieTwo)
		req.Header.Set("X-Exhibit-State-Channel", channel)
		w := httptest.NewRecorder()
		in.ro.ServeHTTP(w, req)
		if channel == ownerChannel {
			assert.Equal(t, http.StatusForbidden, w.Code)
		} else {
			assert.Equal(t, http.StatusOK, w.Code)
		}
	}
	assert.Equal(t, http.StatusForbidden, request(http.MethodGet, path+"/inference", ownerChannel, "", in.cookieTwo))
	assert.Equal(t, http.StatusForbidden, request(http.MethodPut, consent, ownerChannel, "", in.cookieTwo))
	allowed, err := in.ro.cfg.Store.HasInferenceConsent(ctx, store.ViewerID(in.ownerTwo), in.granted)
	require.NoError(t, err)
	assert.False(t, allowed)

	require.Equal(t, http.StatusNoContent, request(http.MethodPut, consent, recipientChannel, "", in.cookieTwo))
	assert.Equal(t, http.StatusForbidden, request(http.MethodDelete, consent, ownerChannel, "", in.cookieTwo))
	assert.Equal(t, http.StatusForbidden, request(http.MethodPatch, path, recipientChannel, `{"inference_approved":false}`, in.cookieOne))
	for _, channel := range []string{ownerChannel, ""} {
		w := postInference(in.ro, in.granted, in.cookieTwo, fromPage(channel))
		assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	}
	assert.Zero(t, rt.calls())

	w := postInference(in.ro, in.granted, in.cookieTwo, fromPage(recipientChannel))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, rt.calls(), "the current page can use both approvals")
}

// Every AI frame carries the channel its document inlines for this viewer, so
// the host can tell the artifact's own document from one navigated into it.
func TestDetailFrameCarriesTheViewersArtifactChannel(t *testing.T) {
	in := newGrantedInstance(t)
	withPlatformInference(t, in.ro)
	for _, viewer := range []struct {
		id     int64
		cookie *http.Cookie
	}{{in.ownerOne, in.cookieOne}, {in.ownerTwo, in.cookieTwo}} {
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+in.granted, nil)
		req.AddCookie(viewer.cookie)
		w := httptest.NewRecorder()
		in.ro.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		channel := in.ro.tokens.ArtifactChannel(in.granted, viewer.id)
		require.NotEmpty(t, channel)
		assert.Contains(t, w.Body.String(), `data-artifact-channel="`+channel+`"`)
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditOriginsPartialRendersSavedAndDraftDecisions(t *testing.T) {
	ro := newTestRouter(t)
	blocked := "https://blocked.example.test"
	fresh := "https://new.example.test"
	id := createArtifact(t, ro, map[string]any{"body": `<script src="` + blocked + `/a.js"></script><script src="` + fresh + `/b.js"></script>`})
	require.Equal(t, http.StatusOK, doJSON(t, ro, "POST", "/api/artifacts/"+id+"/origins", map[string]any{"origin": blocked, "decision": "block"}).Code)
	page := getPage(t, ro, "/partials/edit-origins?artifact="+id)
	assert.Contains(t, page, `id="blocked-rows"`)
	assert.Contains(t, page, `id="unapproved-rows"`)
	assert.Contains(t, page, `data-action="forget"`)
	assert.Contains(t, page, `data-origin="`+fresh+`"`)

	// The partial formats a working copy only. Even arbitrary draft text is
	// escaped by the template, and rendering it never changes saved decisions.
	payload := `https://x"><img src=x onerror=alert(1)>`
	draft, err := json.Marshal(editOriginsView{Allowlist: []string{payload}, Unapproved: []string{blocked, fresh}})
	require.NoError(t, err)
	page = getPage(t, ro, "/partials/edit-origins?"+url.Values{"artifact": {id}, "draft": {string(draft)}}.Encode())
	assert.NotContains(t, page, `id="blocked-rows"`)
	assert.Contains(t, page, `data-origin="https://x&#34;&gt;&lt;img src=x onerror=alert(1)&gt;"`)
	assert.NotContains(t, page, `<img src=x onerror=alert(1)>`)
	decisions, err := ro.cfg.Store.ListOriginDecisions(context.Background(), 1, id)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, "block", decisions[0].Decision)
	assert.Equal(t, blocked, decisions[0].Origin)

	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/partials/edit-origins?artifact="+id+"&draft=invalid", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	in := newGrantedInstance(t)
	assert.Equal(t, http.StatusNotFound, in.get(t, "/partials/edit-origins?artifact="+in.granted, in.cookieTwo).Code)
}

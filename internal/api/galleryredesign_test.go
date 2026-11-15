package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGalleryPersonalStarAPI(t *testing.T) {
	in := newGrantedInstance(t)
	seedOwnedArtifact(t, in.ro, in.ownerOne, "private-tool", "Private", "body", "")
	write := func(cookie *http.Cookie, method, id, resource, body string) int {
		r := httptest.NewRequest(method, "/api/artifacts/"+id+"/"+resource, strings.NewReader(body))
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		in.ro.ServeHTTP(w, r)
		return w.Code
	}
	// Empty Starred needs a route to discover tools rather than a link to itself.
	assert.Contains(t, in.get(t, "/", in.cookieTwo).Body.String(), `href="/search">Browse your artifacts</a>`)
	assert.Equal(t, 204, write(in.cookieTwo, "PUT", in.granted, "starred", `{"starred":true}`))
	body := in.get(t, "/", in.cookieTwo).Body.String()
	assert.Contains(t, body, `data-href="/artifacts/`+in.granted+`"`)
	assert.Contains(t, body, ` Unstar</button>`)
	assert.NotContains(t, body, `gallery-overview`)
	assert.Contains(t, body, `<h1 class="sr-only">Starred</h1>`)
	assert.NotContains(t, body, `gallery-filter-trigger`)
	assert.NotContains(t, body, `gallery-filter-section`)
	assert.NotContains(t, in.get(t, "/", in.cookieOne).Body.String(), `data-href="/artifacts/`+in.granted+`"`)
	assert.Equal(t, 204, write(in.cookieTwo, "POST", in.granted, "opened", ``))
	preferences, err := in.ro.cfg.Store.ArtifactPreferences(t.Context(), store.ViewerID(in.ownerTwo), []string{in.granted})
	require.NoError(t, err)
	assert.False(t, preferences[in.granted].LastOpenedAt.IsZero())
	for _, body := range []string{`{}`, `{"starred":null}`, `{"starred":"true"}`} {
		assert.Equal(t, 400, write(in.cookieTwo, "PUT", in.granted, "starred", body))
	}
	assert.Equal(t, 404, write(in.cookieTwo, "PUT", "private-tool", "starred", `{"starred":true}`))
	assert.Equal(t, 404, write(in.cookieTwo, "POST", "private-tool", "opened", ``))
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(t.Context(), in.ownerOne, in.granted, in.ownerTwo))
	assert.NotContains(t, in.get(t, "/", in.cookieTwo).Body.String(), `data-href="/artifacts/`+in.granted+`"`)
	assert.Equal(t, 404, write(in.cookieTwo, "PUT", in.granted, "starred", `{"starred":false}`))
}

func TestActivityOwnerScopeAndConversationLinks(t *testing.T) {
	in := newGrantedInstance(t)
	for _, item := range []struct {
		owner       int64
		id, session string
	}{{in.ownerOne, in.artifactOne, "foreign-session"}, {in.ownerTwo, in.artifactTwo, "my-session"}} {
		body := "version-body-" + item.session
		_, err := in.ro.cfg.Store.CommitVersion(t.Context(), item.owner, item.id, store.VersionChange{Provenance: store.Provenance{Origin: store.VersionAgent, Message: "Update the chart colors", SessionID: item.session}, BodyBlobID: &body})
		require.NoError(t, err)
		require.NoError(t, in.ro.cfg.Store.SaveTranscript(t.Context(), item.owner, store.Transcript{ArtifactID: item.id, SessionID: item.session, Title: "Update the chart colors", SessionFile: piFile}))
	}
	page := in.get(t, "/activity", in.cookieTwo)
	require.Equal(t, 200, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, "Agent change")
	assert.Contains(t, body, "Update the chart colors")
	assert.Contains(t, body, `activity-version">v3`)
	assert.Contains(t, body, "Data change history")
	assert.Contains(t, body, "Coming soon")
	assert.NotContains(t, body, "foreign-session")
	path := conversationURL(in.artifactTwo, "my-session", "conversation")
	conversation := in.get(t, path, in.cookieTwo)
	require.Equal(t, 200, conversation.Code)
	assert.Contains(t, conversation.Body.String(), `data-agent-conversation="true"`)
	assert.Contains(t, conversation.Body.String(), `class="chat history-open"`)
	assert.Contains(t, conversation.Body.String(), "Done.")
	assert.NotContains(t, conversation.Body.String(), "SECRET-ARTIFACT-SOURCE")
	assert.Equal(t, 404, in.get(t, path, in.cookieOne).Code)
	assert.Equal(t, 404, in.get(t, conversationURL(in.artifactTwo, "missing", "conversation"), in.cookieTwo).Code)
	// An expired live link reads its saved conversation without restarting it.
	assert.Equal(t, 200, in.get(t, conversationURL(in.artifactTwo, "my-session", "live"), in.cookieTwo).Code)
	legacy := in.get(t, "/shared-by-me", in.cookieTwo)
	assert.Equal(t, 302, legacy.Code)
	assert.Equal(t, "/shared", legacy.Header().Get("Location"))
}

func TestPersonalStarsOnANoLoginInstance(t *testing.T) {
	ro := newTestRouter(t)
	id := createTestArtifact(t, ro, "Personal tool")
	require.Equal(t, http.StatusNoContent, doJSON(t, ro, "PUT", "/api/artifacts/"+id+"/starred", map[string]any{"starred": true}).Code)
	require.Equal(t, http.StatusNoContent, doJSON(t, ro, "POST", "/api/artifacts/"+id+"/opened", nil).Code)
	assert.Contains(t, getPage(t, ro, "/"), `data-href="/artifacts/`+id+`"`)
}

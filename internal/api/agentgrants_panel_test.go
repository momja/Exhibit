package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// Read the headers from the rendered element, as htmx does, instead of
// supplying the expected token directly to the fragment request. This catches
// a page whose API writes work but whose refresh loses operator authority.
func agentGrantRefreshRequest(t *testing.T, page string) *http.Request {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page))
	require.NoError(t, err)
	var attributes map[string]string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		attrs := map[string]string{}
		for _, attr := range node.Attr {
			attrs[attr.Key] = attr.Val
		}
		if attrs["id"] == "agent-grants-body" {
			attributes = attrs
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	require.NotNil(t, attributes, "settings must render the refresh region")
	require.Equal(t, "exhibit:agent-grants-changed from:body", attributes["hx-trigger"])
	req := httptest.NewRequest(http.MethodGet, attributes["hx-get"], nil)
	req.Header.Set("HX-Request", "true")
	var headers map[string]string
	require.NoError(t, json.Unmarshal([]byte(attributes["hx-headers"]), &headers))
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	return req
}

func TestAgentGrantsOperatorPanelRefreshKeepsItsCredential(t *testing.T) {
	for _, public := range []bool{false, true} {
		t.Run(map[bool]string{false: "private", true: "public"}[public], func(t *testing.T) {
			base := newTestRouter(t)
			base.cfg.MCPURL = "https://agents.example.test/mcp"
			cfg := base.cfg
			// Quotes and markup characters prove the JSON header survives HTML
			// escaping and still supplies exactly the configured credential.
			cfg.AuthToken = `operator-'"<>&`
			cfg.Public = PublicMode{Enabled: public, OwnerID: defaultOwnerID}
			ro := NewRouter(cfg)
			pageReq := httptest.NewRequest(http.MethodGet, "/settings", nil)
			pageReq.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
			page := httptest.NewRecorder()
			ro.ServeHTTP(page, pageReq)
			require.Equal(t, http.StatusOK, page.Code, page.Body.String())

			assert.Contains(t, page.Body.String(), "Copy token")
			assert.NotContains(t, page.Body.String(), "go install")
			assert.NotContains(t, page.Body.String(), `id="agent-grant-command"`)

			// Change the persisted list after loading the page, then replay
			// the request htmx will send in response to the change event.
			_, err := ro.cfg.Store.CreateAgentGrant(context.Background(), defaultOwnerID,
				"New operator connection", "", "panel-test-digest")
			require.NoError(t, err)
			refresh := agentGrantRefreshRequest(t, page.Body.String())
			assert.Equal(t, "Bearer "+cfg.AuthToken, refresh.Header.Get("Authorization"))
			fragment := httptest.NewRecorder()
			ro.ServeHTTP(fragment, refresh)
			require.Equal(t, http.StatusOK, fragment.Code, fragment.Body.String())
			assert.Contains(t, fragment.Body.String(), "New operator connection")
			assert.NotContains(t, fragment.Body.String(), cfg.AuthToken)

			if public {
				anonymous := httptest.NewRecorder()
				ro.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/partials/agent-grants", nil))
				assert.NotEqual(t, http.StatusOK, anonymous.Code)
				assert.NotContains(t, anonymous.Body.String(), "New operator connection")
			}
		})
	}
}

func TestAgentGrantsSessionPanelRefreshUsesOnlyItsCookie(t *testing.T) {
	in := newProfileInstance(t, "panel-session", "person@example.test")
	in.ro.cfg.MCPURL = "https://agents.example.test/mcp"
	_, err := in.st.CreateAgentGrant(context.Background(), in.user.ID, "Session connection", "", "session-panel-digest")
	require.NoError(t, err)
	page := in.page(t)
	require.NotEmpty(t, in.ro.cfg.AuthToken)
	assert.NotContains(t, page, in.ro.cfg.AuthToken, "a signed-in person must never receive the operator credential")
	refresh := agentGrantRefreshRequest(t, page)
	assert.Empty(t, refresh.Header.Get("Authorization"))
	refresh.AddCookie(in.cookie)
	fragment := httptest.NewRecorder()
	in.ro.ServeHTTP(fragment, refresh)
	require.Equal(t, http.StatusOK, fragment.Code, fragment.Body.String())
	assert.Contains(t, fragment.Body.String(), "Session connection")
	assert.NotContains(t, fragment.Body.String(), in.ro.cfg.AuthToken)
}

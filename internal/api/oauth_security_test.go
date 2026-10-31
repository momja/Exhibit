package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthPostBodiesHaveSmallLimits(t *testing.T) {
	in := newOAuthInstance(t)
	for _, path := range []string{"/oauth/register", "/oauth/token", "/oauth/authorize"} {
		for _, chunked := range []bool{false, true} {
			t.Run(path+map[bool]string{false: "/length", true: "/chunked"}[chunked], func(t *testing.T) {
				body := "ignored=" + strings.Repeat("a", 64<<10)
				contentType := "application/x-www-form-urlencoded"
				if path == "/oauth/register" {
					body = `{"redirect_uris":[` + strings.Repeat(`"",`, 22000) + `""]}`
					contentType = "application/json"
				}
				var reader io.Reader = strings.NewReader(body)
				if chunked {
					reader = struct{ io.Reader }{reader}
				}
				req, err := http.NewRequest("POST", in.srv.URL+path, reader)
				require.NoError(t, err)
				req.Header.Set("Content-Type", contentType)
				if path == "/oauth/authorize" {
					req.AddCookie(in.cookieOne)
				}
				resp, err := noRedirect.Do(req)
				require.NoError(t, err)
				resp.Body.Close()
				assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
			})
		}
	}
}

func TestOAuthRedirectHostsMustBeValidASCII(t *testing.T) {
	in := newOAuthInstance(t)
	for _, uri := range []string{
		"https://сlaude.ai/cb", "https://%D1%81laude.ai/cb", "https://bücher.example/cb", "https://ok\u202eevil.example/cb",
		"https://ok\u200d.example/cb", "https://ok\u00ad.example/cb", "https://ok\u0085.example/cb",
		"https://xn--ngba799q.example/cb", "https://xn--11b2ezcw70k.example/cb",
		"https://xn--.example/cb", "https://bad_host.example/cb", "https://-bad.example/cb",
		"cursor://сlaude.ai/cb",
	} {
		t.Run(uri, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"redirect_uris": []string{uri}})
			require.NoError(t, err)
			resp, out := in.register(t, string(body))
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, out)
			assert.Equal(t, "invalid_redirect_uri", out["error"])
		})
	}
	for _, uri := range []string{"https://xn--bcher-kva.example/cb", "https://CLIENT.example/cb", "http://[::1]:4567/cb", "cursor:/callback"} {
		body, err := json.Marshal(map[string]any{"redirect_uris": []string{uri}})
		require.NoError(t, err)
		resp, out := in.register(t, string(body))
		require.Equal(t, http.StatusCreated, resp.StatusCode, out)
		_, challenge := pkcePair()
		q := in.authorizeQuery(out["client_id"].(string), challenge)
		q.Set("redirect_uri", uri)
		req, _ := http.NewRequest("GET", in.srv.URL+"/oauth/authorize?"+q.Encode(), nil)
		req.AddCookie(in.cookieOne)
		resp, err = noRedirect.Do(req)
		require.NoError(t, err)
		page := readBody(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode, page)
		assert.NotContains(t, page, "bücher")
		if strings.Contains(uri, "xn--") {
			assert.Contains(t, page, "https://xn--bcher-kva.example")
		}
	}
}

func TestOAuthLegacyUnicodeRedirectIsRefused(t *testing.T) {
	in := newOAuthInstance(t)
	id, err := in.ro.mintClientID(oauthClient{Name: "Legacy", RedirectURIs: []string{"https://сlaude.ai/cb"}})
	require.NoError(t, err)
	_, challenge := pkcePair()
	q := in.authorizeQuery(id, challenge)
	q.Del("redirect_uri")
	req := httptest.NewRequest("GET", "/oauth/authorize?"+q.Encode(), nil)
	req.AddCookie(in.cookieOne)
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, w.Header().Get("Location"))
}

func TestOAuthAuthorizationErrorsStayLocal(t *testing.T) {
	for _, login := range []bool{true, false} {
		in := newOAuthInstance(t)
		if !login {
			cfg := in.ro.cfg
			cfg.Identity = nil
			cfg.LocalCredential = nil
			in.ro = NewRouter(cfg)
			// Use a separate router for the no-login requests without changing a live server.
		}
		id, err := in.ro.mintClientID(oauthClient{Name: "Untrusted", RedirectURIs: []string{testRedirect}})
		require.NoError(t, err)
		_, challenge := pkcePair()
		for _, field := range []string{"response_type", "code_challenge", "code_challenge_method", "resource", "decision", "missing decision"} {
			for _, method := range []string{"GET", "POST"} {
				if strings.Contains(field, "decision") && method == "GET" {
					continue
				}
				q := in.authorizeQuery(id, challenge)
				q.Set("decision", "allow")
				q.Set(field, "invalid")
				if field == "missing decision" {
					q.Del("decision")
				}
				target, body := "/oauth/authorize", ""
				if method == "GET" {
					target += "?" + q.Encode()
				} else {
					body = q.Encode()
				}
				req := httptest.NewRequest(method, target, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if login {
					req.AddCookie(in.cookieOne)
				}
				w := httptest.NewRecorder()
				in.ro.ServeHTTP(w, req)
				assert.Equal(t, http.StatusBadRequest, w.Code, "%t %s %s", login, method, field)
				assert.Empty(t, w.Header().Get("Location"))
				assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
			}
		}
	}
}

func TestOAuthClientNamesRejectControlsAndIsolateDisplay(t *testing.T) {
	in := newOAuthInstance(t)
	for _, name := range []string{"Agent\u202eevil", "Agent\u2066evil", "Agent\u200bevil", "Agent\x00evil", "Agent\x7fevil", "Agent\nevil"} {
		body, err := json.Marshal(map[string]any{"client_name": name, "redirect_uris": []string{testRedirect}})
		require.NoError(t, err)
		resp, out := in.register(t, string(body))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, name)
		assert.Equal(t, "invalid_client_metadata", out["error"])
	}
	name := "وكيل Agent"
	body, _ := json.Marshal(map[string]any{"client_name": name, "redirect_uris": []string{testRedirect}})
	resp, out := in.register(t, string(body))
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	id := out["client_id"].(string)
	verifier, challenge := pkcePair()
	q := in.authorizeQuery(id, challenge)
	req, _ := http.NewRequest("GET", in.srv.URL+"/oauth/authorize?"+q.Encode(), nil)
	req.AddCookie(in.cookieOne)
	resp, err := noRedirect.Do(req)
	require.NoError(t, err)
	page := readBody(t, resp)
	assert.Contains(t, page, "<bdi>"+name+"</bdi>")
	assert.Contains(t, page, "<bdi>http://127.0.0.1:4567</bdi>")
	code := in.code(t, id, challenge, in.cookieOne)
	status, out := in.exchange(t, in.tokenForm(id, code, verifier))
	require.Equal(t, http.StatusOK, status, out)
	w := doWithCookie(t, in.ro, "GET", "/settings", in.cookieOne, nil)
	assert.Equal(t, 2, strings.Count(w.Body.String(), "<bdi>"+name+"</bdi>"), "label and client are isolated")
}

func TestOAuthPendingCodeLimitIsPerOwner(t *testing.T) {
	in := newOAuthInstance(t)
	id := in.clientID(t)
	verifier, challenge := pkcePair()
	var first string
	for i := 0; i < 16; i++ {
		code := in.code(t, id, challenge, in.cookieOne)
		if i == 0 {
			first = code
		}
	}
	q := in.authorizeQuery(id, challenge)
	q.Set("decision", "allow")
	resp := in.consent(t, in.cookieOne, q)
	page := readBody(t, resp)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, page)
	assert.Empty(t, resp.Header.Get("Location"))
	assert.Contains(t, page, "Too many pending authorizations for your account")
	other := sessionCookieFor(t, in.ro.cfg.Store, in.ownerTwo, "session-code-limit-two")
	require.NotEmpty(t, in.code(t, id, challenge, other))
	status, out := in.exchange(t, in.tokenForm(id, first, verifier))
	require.Equal(t, http.StatusOK, status, out)
	require.NotEmpty(t, in.code(t, id, challenge, in.cookieOne), "redemption frees a slot")
	// Advance the existing store clock without races with HTTP issuance.
	in.ro.oauthCodes.mu.Lock()
	in.ro.oauthCodes.now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	in.ro.oauthCodes.mu.Unlock()
	require.NotEmpty(t, in.code(t, id, challenge, in.cookieOne), "expiry frees slots")
}

func TestOAuthClientIDFitsAuthorizationRequest(t *testing.T) {
	in := newOAuthInstance(t)
	// Serve through the production header limit, not httptest's 1 MiB default.
	srv := httptest.NewUnstartedServer(in.ro)
	srv.Config.MaxHeaderBytes = NewServer("", in.ro).MaxHeaderBytes
	srv.Start()
	defer srv.Close()
	_, challenge := pkcePair()
	for _, char := range []string{"a", "&", "界"} {
		redirects := []string{testRedirect}
		for range 4 {
			redirects = append(redirects, "https://client.example/"+strings.Repeat(char, 2000))
		}
		body, err := json.Marshal(map[string]any{"redirect_uris": redirects})
		require.NoError(t, err)
		resp, out := in.register(t, string(body))
		if char != "a" {
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "oversized minted ID must not register")
			assert.Equal(t, "invalid_client_metadata", out["error"])
			continue
		}
		require.Equal(t, http.StatusCreated, resp.StatusCode, out)
		id := out["client_id"].(string)
		assert.LessOrEqual(t, len(id), 16<<10)
		assert.Greater(t, len(id), 10<<10, "exercise a large accepted ID")
		req, _ := http.NewRequest("GET", srv.URL+"/oauth/authorize?"+in.authorizeQuery(id, challenge).Encode(), nil)
		req.AddCookie(in.cookieOne)
		resp, err = noRedirect.Do(req)
		require.NoError(t, err)
		page := readBody(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode, page)
	}
	// A validly signed legacy ID over the new bound must also be refused on use.
	id, err := in.ro.mintClientID(oauthClient{Name: strings.Repeat("a", 13<<10), RedirectURIs: []string{testRedirect}})
	require.NoError(t, err)
	require.Greater(t, len(id), 16<<10)
	req := httptest.NewRequest("GET", "/oauth/authorize?"+in.authorizeQuery(id, challenge).Encode(), nil)
	req.AddCookie(in.cookieOne)
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, w.Header().Get("Location"))
}

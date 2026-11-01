package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthRejectsNonCanonicalClientIDs(t *testing.T) {
	in := newOAuthInstance(t)
	id := in.clientID(t)
	verifier, challenge := pkcePair()
	payload, signature, _ := strings.Cut(id, ".")
	for name, invalid := range map[string]string{
		"payload CRLF":   payload + "\r\n." + signature,
		"signature CRLF": payload + "." + signature + "\r\n",
		"oversized":      payload + strings.Repeat("a", 16<<10) + "." + signature,
	} {
		t.Run(name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				form := in.authorizeQuery(invalid, challenge)
				form.Set("decision", "allow")
				target := oauthAuthorizePath
				body := ""
				if method == http.MethodGet {
					target += "?" + form.Encode()
				} else {
					body = form.Encode()
				}
				req := httptest.NewRequest(method, target, strings.NewReader(body))
				req.AddCookie(in.cookieOne)
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				w := httptest.NewRecorder()
				in.ro.ServeHTTP(w, req)
				assert.Equal(t, http.StatusBadRequest, w.Code, method)
				assert.Zero(t, len(in.ro.oauthCodes.codes), "invalid client must not mint a code")
				// Clear unexpected codes so each assertion diagnoses its own request.
				clear(in.ro.oauthCodes.codes)
			}
			for _, basic := range []bool{false, true} {
				form := in.authorizeQuery(id, challenge)
				form.Set("decision", "allow")
				resp := in.consent(t, in.cookieOne, form)
				back, err := url.Parse(resp.Header.Get("Location"))
				require.NoError(t, err)
				code := back.Query().Get("code")
				require.NotEmpty(t, code)
				token := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {invalid}, "redirect_uri": {testRedirect}, "resource": {in.ro.cfg.MCPURL}, "code_verifier": {verifier}}
				if basic {
					token.Del("client_id")
				}
				req := httptest.NewRequest(http.MethodPost, oauthTokenPath, strings.NewReader(token.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if basic {
					req.SetBasicAuth(url.QueryEscape(invalid), "")
				}
				w := httptest.NewRecorder()
				in.ro.ServeHTTP(w, req)
				assert.Equal(t, http.StatusBadRequest, w.Code)
				assert.Contains(t, w.Body.String(), `"error":"invalid_client"`)
				assert.Zero(t, len(in.ro.oauthCodes.codes))
			}
		})
	}
}

func TestOAuthAuthorizationRejectsOversizedFields(t *testing.T) {
	in := newOAuthInstance(t)
	id := in.clientID(t)
	_, challenge := pkcePair()
	for _, field := range []string{"redirect_uri", "state", "code_challenge", "resource", "scope"} {
		t.Run(field, func(t *testing.T) {
			q := in.authorizeQuery(id, challenge)
			q.Set("decision", "allow")
			q.Set(field, strings.Repeat("x", 16<<10))
			if field == "resource" {
				original := in.ro.cfg.MCPURL
				in.ro.cfg.MCPURL = q.Get(field)
				defer func() { in.ro.cfg.MCPURL = original }()
			}
			resp := in.consent(t, in.cookieOne, q)
			back, err := url.Parse(resp.Header.Get("Location"))
			require.NoError(t, err)
			assert.Empty(t, back.Query().Get("code"))
			assert.True(t, resp.StatusCode == http.StatusBadRequest || back.Query().Get("error") != "", "oversized field should be refused")
			assert.Zero(t, len(in.ro.oauthCodes.codes))
			clear(in.ro.oauthCodes.codes)
		})
	}
}

// This regression intentionally inspects ownership: equal string contents do
// not reveal that a tiny saved value keeps an entire parsed request alive.
func assertStringOutside(t *testing.T, name, value string, start, end uintptr) {
	t.Helper()
	pointer := uintptr(unsafe.Pointer(unsafe.StringData(value)))
	assert.False(t, pointer >= start && pointer < end, "%s retains request memory", name)
}

func TestOAuthCodeDoesNotRetainIgnoredFormData(t *testing.T) {
	in := newOAuthInstance(t)
	id := in.clientID(t)
	_, challenge := pkcePair()
	// Deliberately unescaped values: ParseForm can then return substrings of
	// its complete input buffer, including the ignored 32 KiB field.
	body := "client_id=" + id + "&redirect_uri=" + testRedirect + "&resource=" + in.ro.cfg.MCPURL +
		"&code_challenge=" + challenge + "&code_challenge_method=S256&response_type=code&decision=allow&padding=" + strings.Repeat("x", 32<<10)
	req := httptest.NewRequest(http.MethodPost, oauthAuthorizePath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(in.cookieOne)
	require.NoError(t, req.ParseForm())
	first, padding := req.PostForm.Get("client_id"), req.PostForm.Get("padding")
	start := uintptr(unsafe.Pointer(unsafe.StringData(first)))
	end := uintptr(unsafe.Pointer(unsafe.StringData(padding))) + uintptr(len(padding))
	require.Greater(t, end-start, uintptr(32<<10), "fixture must share the form allocation")
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusFound, w.Code)
	back, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	code, ok := in.ro.oauthCodes.codes[back.Query().Get("code")]
	require.True(t, ok)
	for name, value := range map[string]string{"client_id": code.clientID, "redirect_uri": code.redirectURI, "resource": code.resource, "challenge": code.challenge, "label": code.label} {
		assertStringOutside(t, name, value, start, end)
	}
	assert.Equal(t, id, code.clientID)
	assert.Equal(t, challenge, code.challenge)
	runtime.KeepAlive(req)
}

func TestOAuthCodeStoreOwnsEveryString(t *testing.T) {
	backing := strings.Repeat("x", 8<<20)
	value := backing[:10]
	store := newOAuthCodeStore()
	key, err := store.issue(oauthCode{clientID: value, label: value, redirectURI: value, challenge: value, resource: value})
	require.NoError(t, err)
	code, ok := store.redeem(key)
	require.True(t, ok)
	start := uintptr(unsafe.Pointer(unsafe.StringData(backing)))
	for name, value := range map[string]string{"client_id": code.clientID, "redirect_uri": code.redirectURI, "resource": code.resource, "challenge": code.challenge, "label": code.label} {
		assertStringOutside(t, name, value, start, start+uintptr(len(backing)))
		assert.Equal(t, "xxxxxxxxxx", value)
	}
	runtime.KeepAlive(backing)
}

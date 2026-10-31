package api

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Consent and token exchange are separate requests. Removing or disabling the
// approving account between them must not create a standing orphan credential.
func TestOAuthCodeCannotOutliveApprovingAccount(t *testing.T) {
	for _, change := range []string{"deleted", "disabled"} {
		t.Run(change, func(t *testing.T) {
			in := newOAuthInstance(t)
			clientID := in.clientID(t)
			verifier, challenge := pkcePair()
			form := in.authorizeQuery(clientID, challenge)
			form.Set("decision", "allow")
			consent := in.consent(t, in.cookieTwo, form)
			require.Equal(t, http.StatusFound, consent.StatusCode)
			redirect, err := url.Parse(consent.Header.Get("Location"))
			require.NoError(t, err)
			code := redirect.Query().Get("code")
			require.NotEmpty(t, code)

			if change == "deleted" {
				_, err = in.ro.cfg.Store.DeleteAccount(t.Context(), in.ownerTwo)
			} else {
				err = in.ro.cfg.Store.SetUserDisabled(t.Context(), in.ownerTwo, true)
			}
			require.NoError(t, err)

			status, response := in.exchange(t, url.Values{
				"grant_type": {"authorization_code"}, "code": {code},
				"client_id": {clientID}, "redirect_uri": {testRedirect},
				"code_verifier": {verifier}, "resource": {in.ro.cfg.MCPURL},
			})
			assert.Equal(t, http.StatusBadRequest, status, response)
			assert.Equal(t, "invalid_grant", response["error"])
			assert.NotContains(t, response, "access_token")
			grants, err := in.ro.cfg.Store.ListAgentGrants(t.Context(), in.ownerTwo)
			require.NoError(t, err)
			assert.Empty(t, grants, "refusal must not persist an orphan grant")
		})
	}
}

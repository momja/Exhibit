package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/stretchr/testify/require"
)

func TestClerkIdentityLifecycle(t *testing.T) {
	email := "old@example.test"
	allowDelete := true
	active := true
	updates := 0
	userGets := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		u := map[string]any{"id": "user_one", "delete_self_enabled": allowDelete, "primary_email_address_id": "email_one", "email_addresses": []any{map[string]any{"id": "email_one", "email_address": email, "verification": map[string]any{"status": "verified"}}}}
		switch {
		case r.URL.Path == "/sessions/sess_one" && r.Method == "GET":
			status := "active"
			if !active {
				status = "revoked"
			}
			json.NewEncoder(w).Encode(map[string]any{"id": "sess_one", "user_id": "user_one", "status": status, "created_at": 1000})
		case r.URL.Path == "/users/user_one" && r.Method == "GET":
			userGets++
			json.NewEncoder(w).Encode(u)
		case r.URL.Path == "/users/user_one" && r.Method == "PATCH":
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, false, body["delete_self_enabled"])
			updates++
			allowDelete = false
			u["delete_self_enabled"] = false
			json.NewEncoder(w).Encode(u)
		case r.URL.Path == "/users" && r.Method == "GET":
			users := []any{}
			if r.URL.Query().Get("email_address") == email {
				users = append(users, u)
			}
			json.NewEncoder(w).Encode(users)
		case r.URL.Path == "/users/count":
			count := 0
			if r.URL.Query().Get("email_address") == email {
				count = 1
			}
			json.NewEncoder(w).Encode(map[string]any{"total_count": count})
		case r.URL.Path == "/sessions" && r.Method == "GET":
			require.Equal(t, "user_one", r.URL.Query().Get("user_id"))
			require.Empty(t, r.URL.Query().Get("status"))
			sessions := []any{}
			if active {
				sessions = append(sessions, map[string]any{"id": "sess_one", "status": "active", "created_at": 1000})
			}
			json.NewEncoder(w).Encode(map[string]any{"data": sessions, "total_count": len(sessions)})
		case r.URL.Path == "/sessions/sess_one/revoke":
			active = false
			json.NewEncoder(w).Encode(map[string]any{"id": "sess_one", "status": "revoked"})
		default:
			t.Errorf("unexpected provider request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	cfg := &clerk.ClientConfig{}
	cfg.Key = clerk.String("sk_test_example")
	cfg.URL = clerk.String(backend.URL)
	c, err := newClerk("pk_test_"+base64.RawStdEncoding.EncodeToString([]byte("example.clerk.accounts.dev$")), "https://app.test", cfg)
	require.NoError(t, err)
	claims := &clerk.SessionClaims{RegisteredClaims: clerk.RegisteredClaims{Issuer: c.FrontendAPI, Subject: "user_one", Expiry: clerk.Int64(time.Now().Add(time.Minute).Unix())}, Claims: clerk.Claims{SessionID: "sess_one"}, Custom: &clerkSessionState{Status: "active"}}
	r := httptest.NewRequest("GET", "https://app.test/", nil)
	r = r.WithContext(clerk.ContextWithSessionClaims(r.Context(), claims))
	identity, err := c.Identity(r)
	require.NoError(t, err)
	require.Equal(t, int64(1000), identity.SessionCreatedAt)
	require.Equal(t, "old@example.test", identity.Email)
	require.Equal(t, 1, updates)
	_, err = c.Identity(r)
	require.NoError(t, err)
	require.Equal(t, 1, userGets, "cached profile avoids a provider round trip on every API call")
	email = "new@example.test"
	subject, err := c.ResolveEmail(context.Background(), "old@example.test")
	require.NoError(t, err)
	require.Empty(t, subject, "historical email must not grant a share")
	subject, err = c.ResolveEmail(context.Background(), email)
	require.NoError(t, err)
	require.Equal(t, "clerk:user_one", subject)
	c.cacheMu.Lock()
	clear(c.identities)
	c.cacheMu.Unlock()
	identity, err = c.Identity(r)
	require.NoError(t, err)
	require.Equal(t, email, identity.Email)
	cutoff, err := c.RevokeSessions(context.Background(), identity.Subject)
	require.NoError(t, err)
	require.Equal(t, int64(1000), cutoff)
	_, err = c.Identity(r)
	require.Error(t, err, "revoked provider session must not authenticate")
}

func TestClerkPrimaryEmailMustBeVerified(t *testing.T) {
	u := &clerk.User{PrimaryEmailAddressID: clerk.String("email_one"), EmailAddresses: []*clerk.EmailAddress{{ID: "email_one", EmailAddress: "unverified@example.test"}}}
	require.Empty(t, primaryVerifiedEmail(u))
	u.EmailAddresses[0].Verification = &clerk.Verification{Status: "unverified"}
	require.Empty(t, primaryVerifiedEmail(u))
	u.EmailAddresses[0].Verification.Status = "verified"
	require.Equal(t, "unverified@example.test", primaryVerifiedEmail(u))
}

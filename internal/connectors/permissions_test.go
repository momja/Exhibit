package connectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdatePermissionsTogglesNativeWritesWithoutReplacingCredentials(t *testing.T) {
	for _, kind := range []string{"api-key", "bearer", "basic"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := testService(t)
			ctx := context.Background()
			const secret = "existing-pat-without-reentry-75319"
			var writes atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "api-key":
					assert.Equal(t, secret, r.Header.Get("X-API-Key"))
				case "bearer":
					assert.Equal(t, "Bearer "+secret, r.Header.Get("Authorization"))
				case "basic":
					username, password, ok := r.BasicAuth()
					assert.True(t, ok)
					assert.Equal(t, "existing-user", username)
					assert.Equal(t, secret, password)
					assert.Equal(t, "/private-base/records", r.URL.Path)
				}
				if r.Method == http.MethodPost {
					writes.Add(1)
					var payload map[string]string
					assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
					assert.Equal(t, map[string]string{"action": "create"}, payload)
				}
				fmt.Fprint(w, `{"ok":true}`)
			}))
			defer provider.Close()
			input := CreateRequest{Name: "Existing account", Recipe: kind, Origin: provider.URL, Header: "X-API-Key", Secret: secret, DailyLimit: 20}
			if kind == "basic" {
				input.Origin = ""
				input.AccessURL = strings.Replace(provider.URL, "https://", "https://existing-user:"+secret+"@", 1) + "/private-base"
			}
			connection, err := s.Create(ctx, 1, input, UnlimitedLimits())
			require.NoError(t, err)
			_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: connection.ID})
			require.NoError(t, err)
			gate := testGate(t, provider.URL)
			request := func(method string) error {
				req, err := http.NewRequest(method, provider.URL+"/records", strings.NewReader(`{"action":"create"}`))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				response, err := s.Do(ctx, 1, "app", req, gate)
				if err == nil {
					assert.JSONEq(t, `{"ok":true}`, string(response.Body))
				}
				return err
			}
			require.NoError(t, request(http.MethodGet))
			assert.ErrorIs(t, request(http.MethodPost), ErrEgress)
			assert.Zero(t, writes.Load())
			for _, allowWrites := range []bool{true, false} {
				before, err := s.cfg.Store.GetConnection(ctx, 1, connection.ID)
				require.NoError(t, err)
				metadata, err := s.UpdatePermissions(ctx, 1, connection.ID, allowWrites)
				require.NoError(t, err)
				assert.Equal(t, connection.ID, metadata.ID)
				assert.Equal(t, connection.Origins, metadata.Origins)
				if allowWrites {
					assert.Equal(t, []string{"read", "write"}, metadata.Capabilities)
				} else {
					assert.Equal(t, []string{"read"}, metadata.Capabilities)
				}
				after, err := s.cfg.Store.GetConnection(ctx, 1, connection.ID)
				require.NoError(t, err)
				assert.Equal(t, before.EncryptedSecret, after.EncryptedSecret, "permission edits must not decrypt and reseal the original PAT")
				before.RecipeJSON, after.RecipeJSON = "", ""
				assert.Equal(t, before, after, "every other connection column must remain unchanged")
				raw, err := json.Marshal(metadata)
				require.NoError(t, err)
				for _, private := range []string{secret, "existing-user", "private-base", before.EncryptedSecret} {
					assert.NotContains(t, string(raw), private)
				}
				if allowWrites {
					require.NoError(t, request(http.MethodPost))
				} else {
					assert.ErrorIs(t, request(http.MethodPost), ErrEgress)
				}
				require.NoError(t, request(http.MethodGet), "normal reads must work before and after either permission change")
			}
			assert.Equal(t, int32(1), writes.Load())
		})
	}
}

func TestUpdatePermissionsRejectsForeignManagedAndInactiveConnections(t *testing.T) {
	s, path := testService(t)
	ctx := context.Background()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer db.Close()
	for _, tc := range []struct {
		id, provider, kind, status string
		owner                      int64
		want                       error
	}{
		{"foreign", "bearer", "native", "active", 2, store.ErrNotFound},
		{"revoked", "bearer", "native", "revoked", 1, store.ErrNotFound},
		{"reconnect", "basic", "native", "reconnect", 1, store.ErrNotFound},
		{"oauth", "github", "oauth", "active", 1, ErrInvalid},
		{"vendor", "bearer", "vendor", "active", 1, ErrInvalid},
		{"unsupported", "custom", "native", "active", 1, ErrInvalid},
	} {
		t.Run(tc.id, func(t *testing.T) {
			record := &store.ConnectionRecord{ID: tc.id, OwnerID: 1, Name: tc.id, Provider: tc.provider, Kind: tc.kind, RecipeJSON: `{"origins":["https://api.example.test"],"allow_writes":false}`, EncryptedSecret: "keep-this-exact-credential"}
			require.NoError(t, s.cfg.Store.CreateConnection(ctx, record, -1))
			if tc.status != "active" {
				_, err := db.ExecContext(ctx, `UPDATE connections SET status=? WHERE id=?`, tc.status, tc.id)
				require.NoError(t, err)
			}
			before, err := s.cfg.Store.GetConnection(ctx, 1, tc.id)
			require.NoError(t, err)
			metadata, err := s.UpdatePermissions(ctx, tc.owner, tc.id, true)
			assert.ErrorIs(t, err, tc.want)
			assert.Nil(t, metadata)
			after, err := s.cfg.Store.GetConnection(ctx, 1, tc.id)
			assert.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
	_, err = s.UpdatePermissions(ctx, 1, "missing", true)
	assert.ErrorIs(t, err, store.ErrNotFound)
	_, err = s.UpdatePermissions(ctx, 0, "foreign", true)
	assert.ErrorIs(t, err, store.ErrNotFound)
	_, err = New(Config{}).UpdatePermissions(ctx, 1, "foreign", true)
	assert.ErrorIs(t, err, ErrDisabled)
	require.NoError(t, s.Revoke(ctx, 1, "foreign"))
	_, err = s.UpdatePermissions(ctx, 1, "foreign", true)
	assert.ErrorIs(t, err, store.ErrNotFound, "a revoked connection must not be recreated")
}

func TestUpdatePermissionsDoesNotNeedCredentialDecryption(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	record := &store.ConnectionRecord{ID: "opaque-secret", OwnerID: 1, Name: "Existing account", Provider: "bearer", Kind: "native", RecipeJSON: `{"origins":["https://api.example.test"],"allow_writes":false,"future":{"keep":true}}`, EncryptedSecret: "opaque-credential-this-service-cannot-decrypt"}
	require.NoError(t, s.cfg.Store.CreateConnection(ctx, record, -1))
	withoutKey := New(Config{Store: s.cfg.Store})
	metadata, err := withoutKey.UpdatePermissions(ctx, 1, record.ID, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"read", "write"}, metadata.Capabilities)
	after, err := s.cfg.Store.GetConnection(ctx, 1, record.ID)
	require.NoError(t, err)
	assert.Equal(t, record.EncryptedSecret, after.EncryptedSecret)
}

func TestUpdatePermissionsWaitsForAuthorizedRequestBeforeDisablingWrites(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var writes atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer original-permission-token", r.Header.Get("Authorization"))
		if r.Method == http.MethodPost {
			writes.Add(1)
			close(entered)
			<-release
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer func() {
		unblock()
		provider.Close()
	}()
	connection, err := s.Create(ctx, 1, CreateRequest{Name: "Existing writer", Recipe: "bearer", Origin: provider.URL, Secret: "original-permission-token", AllowWrites: true}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
	gate := testGate(t, provider.URL)
	requestDone := make(chan error, 1)
	go func() {
		req, _ := http.NewRequest(http.MethodPost, provider.URL+"/records", nil)
		_, err := s.Do(ctx, 1, "app", req, gate)
		requestDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the authorized request did not reach the provider")
	}
	updateDone := make(chan error, 1)
	go func() {
		_, err := s.UpdatePermissions(ctx, 1, connection.ID, false)
		updateDone <- err
	}()
	select {
	case err := <-updateDone:
		t.Fatalf("permission edit completed before the in-flight request: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	require.NoError(t, <-requestDone)
	require.NoError(t, <-updateDone)
	req, err := http.NewRequest(http.MethodPost, provider.URL+"/records", nil)
	require.NoError(t, err)
	_, err = s.Do(ctx, 1, "app", req, gate)
	assert.ErrorIs(t, err, ErrEgress)
	assert.Equal(t, int32(1), writes.Load())
	_, err = doRequest(t, s, 1, gate, provider.URL+"/records")
	require.NoError(t, err)
}

func TestUpdatePermissionsCannotReviveConnectionDeletedWhileWaiting(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	connection, err := s.Create(ctx, 1, CreateRequest{Name: "To revoke", Recipe: "bearer", Origin: "https://api.example.test", Secret: "sealed-before-revocation"}, UnlimitedLimits())
	require.NoError(t, err)
	lock := &sync.Mutex{}
	s.locks.Store(connection.ID, lock)
	lock.Lock()
	updateDone := make(chan error, 1)
	go func() {
		_, err := s.UpdatePermissions(ctx, 1, connection.ID, true)
		updateDone <- err
	}()
	select {
	case err := <-updateDone:
		lock.Unlock()
		t.Fatalf("permission edit bypassed the connection lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	err = s.cfg.Store.DeleteConnection(ctx, 1, connection.ID)
	lock.Unlock()
	require.NoError(t, err)
	assert.ErrorIs(t, <-updateDone, store.ErrNotFound)
	_, err = s.cfg.Store.GetConnection(ctx, 1, connection.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

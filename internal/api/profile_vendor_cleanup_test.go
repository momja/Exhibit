package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountDeletionDrainsUnfinishedManagedAuthorization(t *testing.T) {
	in := newDeleteInstance(t)
	in.seedLibrary(t)
	ctx := context.Background()
	db := in.st.(*store.SQLiteStore)
	request, err := json.Marshal(connectors.VendorFlowRequest{
		ID: "pending-flow", OwnerTag: "pending-owner", Provider: "notes", ExternalRef: "ca_pending",
		Spec: connectors.VendorProvider{Engine: "composio", AuthConfigID: "ac_pending", Toolkit: "notes"},
	})
	require.NoError(t, err)
	require.NoError(t, db.CreatePendingVendorConnection(ctx, store.PendingVendorConnection{
		ID: "pending-flow", OwnerID: in.member.ID, RequestJSON: string(request), ExpiresAt: time.Now().Add(time.Hour),
	}))
	// The account cannot silently discard cleanup merely because connectors
	// were disabled after authorization started.
	w := in.deleteAccount(t)
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	var unavailable atomic.Bool
	unavailable.Store(true)
	var deleted atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			deleted.Add(1)
			w.Write([]byte(`{"success":true}`))
		case strings.HasSuffix(r.URL.Path, "/revoke"):
			if unavailable.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.Write([]byte(`{}`))
			}
		default:
			w.Write([]byte(`{"id":"ca_pending","user_id":"pending-owner","status":"ACTIVE","auth_config":{"id":"ac_pending"},"toolkit":{"slug":"notes"}}`))
		}
	}))
	defer provider.Close()
	in.ro.cfg.Connections = connectors.New(connectors.Config{
		Store: db, Box: in.ro.cfg.Secrets, VendorEnabled: true,
		Vendor:      connectors.VendorConfig{ComposioBaseURL: provider.URL, ComposioAPIKey: "operator-key"},
		GateFactory: func(o []string) (*egress.Gate, error) { return egress.NewForTests(o, egress.Limits{}) },
	})
	w = in.deleteAccount(t)
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	_, err = in.st.GetUser(ctx, in.member.ID)
	require.NoError(t, err)
	pending, err := db.ListPendingVendorConnections(ctx, in.member.ID, time.Time{})
	require.NoError(t, err)
	require.Len(t, pending, 1)
	unavailable.Store(false)
	w = in.deleteAccount(t)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Equal(t, int32(1), deleted.Load())
	pending, err = db.ListPendingVendorConnections(ctx, in.member.ID, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, pending)
	_, err = in.st.GetUser(ctx, in.member.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

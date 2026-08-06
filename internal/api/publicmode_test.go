package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPublicModeRouter is newTestRouter with a public-mode configuration
// supplied, so a test can vary that one field and hold everything else equal.
func newPublicModeRouter(t *testing.T, public PublicMode) *Router {
	t.Helper()

	f, err := os.CreateTemp("", "test-public-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	st, err := store.OpenSQLite(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })

	blobDir, err := os.MkdirTemp("", "test-public-blobs-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(blobDir) })

	bl, err := blob.NewFSStore(blobDir)
	require.NoError(t, err)

	box, err := secrets.Load("test-secret", "")
	require.NoError(t, err)

	return NewRouter(Config{
		Store:        st,
		Blob:         bl,
		AppOrigin:    "http://app.test",
		RenderOrigin: "http://render.test",
		AuthToken:    "secret",
		Secrets:      box,
		Public:       public,
	})
}

func getPublicSettings(t *testing.T, r *Router, header string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/settings/public", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The endpoint's reason for existing: an anonymous visitor can read the
// instance's name without a credential.
func TestPublicSettingsUnauthenticated(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{
		Enabled:     true,
		Name:        "Max's Exhibit",
		Description: "A shelf of small tools.",
		OwnerID:     defaultOwnerID,
	})

	w := getPublicSettings(t, r, "")
	require.Equal(t, http.StatusOK, w.Code)

	var got publicSettingsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "Max's Exhibit", got.Name)
	assert.Equal(t, "A shelf of small tools.", got.Description)

	// The response says what the instance calls itself and nothing else — in
	// particular not which owner it publishes.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.ElementsMatch(t, []string{"name", "description"}, keysOf(raw))
}

// Public but unnamed is a real state, and it answers 200 with empty strings —
// which is exactly why "not public" cannot also answer 200.
func TestPublicSettingsEnabledButUnset(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{Enabled: true})

	w := getPublicSettings(t, r, "")
	require.Equal(t, http.StatusOK, w.Code)

	var got publicSettingsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "", got.Name)
	assert.Equal(t, "", got.Description)
}

// A private instance does not name itself — to a stranger or to its own
// operator. The route is indistinguishable from one that was never registered.
func TestPublicSettingsDisabled404s(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{
		Name:        "Not published",
		Description: "Nor is this.",
	})

	for _, header := range []string{"", authHeader()} {
		w := getPublicSettings(t, r, header)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.NotContains(t, w.Body.String(), "Not published")
	}
}

// The claim this ticket has to make good on: configuration alone changes no
// authentication behaviour. With public mode off, every authenticated route
// still rejects a request with no credential and still accepts one with the
// token — as TestAuthMiddleware asserts for the default router, held here
// against a router that merely knows about public mode.
func TestPublicModeOffLeavesAuthUnchanged(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{})

	authed := []string{
		"/api/artifacts",
		"/api/collections",
		"/api/tags",
		"/api/agent/key",
	}
	for _, path := range authed {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "%s must still require auth", path)

		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", authHeader())
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusUnauthorized, w.Code, "%s must still accept the token", path)
	}
}

// Enabling public mode is likewise not, on its own, a change to authentication:
// that is av-wmp6's job. Until it lands, an anonymous request to a mutating or
// authenticated route is refused exactly as before.
func TestPublicModeOnStillGuardsTheAPI(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{Enabled: true, Name: "Public"})

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPublicModeFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want PublicMode
	}{
		{
			name: "unset is a private instance owned by the default owner",
			want: PublicMode{OwnerID: defaultOwnerID},
		},
		{
			name: "fully configured",
			env: map[string]string{
				envPublicModeEnabled:         "true",
				envPublicInstanceName:        "Max's Exhibit",
				envPublicInstanceDescription: "A shelf of small tools.",
				envPublicOwnerID:             "7",
			},
			want: PublicMode{
				Enabled:     true,
				Name:        "Max's Exhibit",
				Description: "A shelf of small tools.",
				OwnerID:     7,
			},
		},
		{
			name: "1 enables",
			env:  map[string]string{envPublicModeEnabled: "1"},
			want: PublicMode{Enabled: true, OwnerID: defaultOwnerID},
		},
		{
			name: "yes enables",
			env:  map[string]string{envPublicModeEnabled: "yes"},
			want: PublicMode{Enabled: true, OwnerID: defaultOwnerID},
		},
		{
			name: "on enables, case-insensitively",
			env:  map[string]string{envPublicModeEnabled: "ON"},
			want: PublicMode{Enabled: true, OwnerID: defaultOwnerID},
		},
		{
			name: "off disables",
			env:  map[string]string{envPublicModeEnabled: "off"},
			want: PublicMode{OwnerID: defaultOwnerID},
		},
		// The misreading a "any non-empty value" rule would make, and the
		// reason this knob does not use one.
		{
			name: "the word false disables",
			env:  map[string]string{envPublicModeEnabled: "false"},
			want: PublicMode{OwnerID: defaultOwnerID},
		},
		{
			name: "nonsense fails closed",
			env:  map[string]string{envPublicModeEnabled: "maybe"},
			want: PublicMode{OwnerID: defaultOwnerID},
		},
		{
			name: "an unusable owner id falls back rather than failing the boot",
			env:  map[string]string{envPublicOwnerID: "not-a-number"},
			want: PublicMode{OwnerID: defaultOwnerID},
		},
		{
			name: "so does a nonsensical one",
			env:  map[string]string{envPublicOwnerID: "0"},
			want: PublicMode{OwnerID: defaultOwnerID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{
				envPublicModeEnabled, envPublicInstanceName,
				envPublicInstanceDescription, envPublicOwnerID,
			} {
				t.Setenv(key, tt.env[key])
			}
			assert.Equal(t, tt.want, PublicModeFromEnv())
		})
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

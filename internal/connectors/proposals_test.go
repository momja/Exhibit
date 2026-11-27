package connectors

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func proposalSpec() APIProposalSpec {
	return APIProposalSpec{Name: "Weather", Origin: "https://api.weather.com", Auth: "api-key", Header: "X-API-Key", Reason: "Read your weather observations"}
}

func TestValidateAPIProposal(t *testing.T) {
	spec := proposalSpec()
	spec.Origin = "https://API.Weather.COM.:0443"
	spec.Name = " Weather "
	spec.Header = "x-api-key"
	got, err := ValidateAPIProposal(spec)
	require.NoError(t, err)
	assert.Equal(t, "Weather", got.Name)
	assert.Equal(t, "https://api.weather.com", got.Origin)
	assert.Equal(t, "X-Api-Key", got.Header)

	for _, origin := range []string{
		"http://api.weather.com", "https://api.weather.com/", "https://api.weather.com/data", "https://api.weather.com/%2f",
		"https://user:password@api.weather.com", "https://api.weather.com?key=private", "https://api.weather.com?", "https://api.weather.com#", "https://api.weather.com#fragment",
		"https://127.0.0.1", "https://10.0.0.1", "https://8.8.8.8", "https://[::1]", "https://[2606:4700:4700::1111]",
		"https://127.1", "https://2130706433", "https://0x7f.0.0.1", "https://0177.0.0.1", "https://foo.123",
		"https://localhost", "https://api.localhost", "https://database", "https://db.internal", "https://db.local", "https://db.home.arpa", "https://service.onion", "https://api.example.test",
		"https://-bad.weather.com", "https://bad-.weather.com", "https://bad_name.weather.com", "https://api.weather.com:", " https://api.weather.com", "https://*.weather.com",
	} {
		t.Run(origin, func(t *testing.T) {
			bad := proposalSpec()
			bad.Origin = origin
			_, err := ValidateAPIProposal(bad)
			assert.ErrorIs(t, err, ErrInvalid)
		})
	}
	for _, header := range []string{"Cookie", "Host", "Proxy-Authorization", "Sec-Fetch-Site", "X-Forwarded-For", "X-Key\r\nBad", strings.Repeat("A", 101)} {
		bad := proposalSpec()
		bad.Header = header
		_, err := ValidateAPIProposal(bad)
		assert.ErrorIs(t, err, ErrInvalid, header)
	}
	spec = proposalSpec()
	spec.Auth, spec.Header = "bearer", ""
	got, err = ValidateAPIProposal(spec)
	require.NoError(t, err)
	assert.Equal(t, "Authorization", got.Header)
	spec.Header = "X-Api-Key"
	_, err = ValidateAPIProposal(spec)
	assert.ErrorIs(t, err, ErrInvalid)
	for _, auth := range []string{"basic", "oauth", "Bearer", "", "header-template"} {
		spec = proposalSpec()
		spec.Auth = auth
		_, err = ValidateAPIProposal(spec)
		assert.ErrorIs(t, err, ErrInvalid)
	}
	spec = proposalSpec()
	spec.Reason = "reason\x00control"
	_, err = ValidateAPIProposal(spec)
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestAPIProposalPrivateApprovalAndDurableRetry(t *testing.T) {
	s, path := testService(t)
	ctx := context.Background()
	p, err := s.ProposeAPI(ctx, 1, "app", proposalSpec())
	require.NoError(t, err)
	assert.Equal(t, "pending", p.State)
	assert.Empty(t, p.ConnectionID)
	const key = "private-weather-key-4532"
	for _, trustedOrigin := range []string{"", "https://evil.com", "https://api.weather.com/data", "https://API.weather.com"} {
		_, err = s.ResolveAPIProposal(ctx, 1, "app", p.ID, true, trustedOrigin, key, UnlimitedLimits())
		assert.ErrorIs(t, err, ErrInvalid)
	}
	_, err = s.ResolveAPIProposal(ctx, 1, "app", p.ID, true, p.Spec.Origin, "sk-proj-wrong-service", UnlimitedLimits())
	var mismatch *CredentialOriginError
	assert.ErrorAs(t, err, &mismatch)
	for _, secret := range []string{"", "   ", "key\r\nInjected: bad", "\x00bad", "\xffbad", strings.Repeat("x", 16385)} {
		_, err = s.ResolveAPIProposal(ctx, 1, "app", p.ID, true, p.Spec.Origin, secret, UnlimitedLimits())
		assert.ErrorIs(t, err, ErrInvalid)
	}
	connections, err := s.List(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, connections)
	p, err = s.ResolveAPIProposal(ctx, 1, "app", p.ID, true, p.Spec.Origin, key, UnlimitedLimits())
	require.NoError(t, err)
	assert.Equal(t, "approved", p.State)
	require.NotEmpty(t, p.ConnectionID)
	rec, err := s.cfg.Store.GetConnection(ctx, 1, p.ConnectionID)
	require.NoError(t, err)
	_, credential, err := s.readCredential(rec)
	require.NoError(t, err)
	assert.Equal(t, key, credential.Value)
	assert.NotContains(t, rec.EncryptedSecret, key)
	grants, err := s.cfg.Store.ListConnectionGrants(ctx, 1, "app")
	require.NoError(t, err)
	assert.Empty(t, grants)
	a, err := s.cfg.Store.GetArtifact(ctx, 1, "app")
	require.NoError(t, err)
	assert.Empty(t, a.NetworkAllowlist)
	// A restarted service can deliver the recorded result without the key,
	// even when the pending agent interaction has not recorded its answer yet.
	s = New(s.cfg)
	retry, err := s.ResolveAPIProposal(ctx, 1, "app", p.ID, true, "", "", Limits{})
	require.NoError(t, err)
	assert.Equal(t, p, retry)
	deniedRetry, err := s.ResolveAPIProposal(ctx, 1, "app", p.ID, false, "", "", Limits{})
	require.NoError(t, err)
	assert.Equal(t, p, deniedRetry)
	encoded, err := json.Marshal(p)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), key)
	disk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(disk), key)
}

func TestAPIProposalOwnerScopeAndDenialCannotBeReopened(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	p, err := s.ProposeAPI(ctx, 1, "app", proposalSpec())
	require.NoError(t, err)
	for _, ownerID := range []int64{0, 2} {
		_, err := s.GetAPIProposal(ctx, ownerID, "app", p.ID)
		assert.ErrorIs(t, err, store.ErrNotFound)
		_, err = s.ResolveAPIProposal(ctx, ownerID, "app", p.ID, true, p.Spec.Origin, "private", UnlimitedLimits())
		assert.ErrorIs(t, err, store.ErrNotFound)
		_, err = s.ProposeAPI(ctx, ownerID, "app", proposalSpec())
		assert.ErrorIs(t, err, store.ErrNotFound)
	}
	_, err = s.ResolveAPIProposal(ctx, 1, "other", p.ID, false, "", "", UnlimitedLimits())
	assert.ErrorIs(t, err, store.ErrNotFound)
	p, err = s.ResolveAPIProposal(ctx, 1, "app", p.ID, false, "", "", Limits{})
	require.NoError(t, err)
	assert.Equal(t, "denied", p.State)
	spec := proposalSpec()
	spec.Name, spec.Reason = "Repeated prompt", "Try again"
	spec.Origin, spec.Header = "https://API.WEATHER.COM:443", "x-api-key"
	repeat, err := s.ProposeAPI(ctx, 1, "app", spec)
	require.NoError(t, err)
	assert.Equal(t, p, repeat)
	repeat, err = s.ResolveAPIProposal(ctx, 1, "app", p.ID, true, p.Spec.Origin, "private", UnlimitedLimits())
	require.NoError(t, err)
	assert.Equal(t, p, repeat)
	connections, err := s.List(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, connections)
	s.cfg.CustomEnabled = false
	_, err = s.ProposeAPI(ctx, 1, "app", proposalSpec())
	assert.ErrorIs(t, err, ErrDisabled)
	_, err = s.ResolveAPIProposal(ctx, 1, "app", p.ID, true, p.Spec.Origin, "private", UnlimitedLimits())
	assert.ErrorIs(t, err, ErrDisabled)
}

func TestAPIProposalReusesConnectionWithoutReplacingKey(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	spec := proposalSpec()
	spec.Auth, spec.Header = "bearer", ""
	p, err := s.ProposeAPI(ctx, 1, "app", spec)
	require.NoError(t, err)
	existing, err := s.Create(ctx, 1, CreateRequest{Name: "Already connected", Recipe: "api-key", Origin: p.Spec.Origin, Header: "authorization", Secret: "original-key"}, UnlimitedLimits())
	require.NoError(t, err)
	p, err = s.ResolveAPIProposal(ctx, 1, "app", p.ID, true, p.Spec.Origin, "replacement-key", Limits{})
	require.NoError(t, err, "reuse does not consume connection quota")
	assert.Equal(t, existing.ID, p.ConnectionID)
	rec, err := s.cfg.Store.GetConnection(ctx, 1, existing.ID)
	require.NoError(t, err)
	_, credential, err := s.readCredential(rec)
	require.NoError(t, err)
	assert.Equal(t, "original-key", credential.Value)
	require.NoError(t, s.cfg.Store.PutArtifact(ctx, &store.Artifact{ID: "second", OwnerID: 1, Title: "Second", SourceBlobID: "body", Tier: store.Tier1}))
	other, err := s.ProposeAPI(ctx, 1, "second", spec)
	require.NoError(t, err)
	assert.Equal(t, "approved", other.State)
	assert.Equal(t, existing.ID, other.ConnectionID)
	_, err = s.Create(ctx, 1, CreateRequest{Name: "Duplicate", Recipe: "bearer", Origin: p.Spec.Origin, Secret: "another-key"}, UnlimitedLimits())
	assert.ErrorIs(t, err, store.ErrConnectionDuplicate)
}

func TestAPIProposalConcurrentApprovalAndNormalCreate(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	p, err := s.ProposeAPI(ctx, 1, "app", proposalSpec())
	require.NoError(t, err)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if i%2 == 0 {
				approved, err := s.ResolveAPIProposal(ctx, 1, "app", p.ID, true, p.Spec.Origin, "proposal-key", UnlimitedLimits())
				if assert.NoError(t, err) {
					assert.Equal(t, "approved", approved.State)
				}
			} else {
				_, err := s.Create(ctx, 1, CreateRequest{Name: "Concurrent create", Recipe: "api-key", Origin: p.Spec.Origin, Header: p.Spec.Header, Secret: "normal-key"}, UnlimitedLimits())
				if err != nil {
					assert.ErrorIs(t, err, store.ErrConnectionDuplicate)
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	connections, err := s.List(ctx, 1)
	require.NoError(t, err)
	require.Len(t, connections, 1)
	final, err := s.GetAPIProposal(ctx, 1, "app", p.ID)
	require.NoError(t, err)
	assert.Equal(t, connections[0].ID, final.ConnectionID)
	rec, err := s.cfg.Store.GetConnection(ctx, 1, final.ConnectionID)
	require.NoError(t, err)
	_, credential, err := s.readCredential(rec)
	require.NoError(t, err)
	assert.Contains(t, []string{"normal-key", "proposal-key"}, credential.Value)
}

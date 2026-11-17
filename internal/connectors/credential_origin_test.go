package connectors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every recognized format answers with its issuer's service, so the warning
// names the right destination rather than a generic one.
func TestRecognizedCredentialNamesItsIssuer(t *testing.T) {
	for _, tc := range []struct {
		secret   string
		label    string
		domain   string
		expected string
	}{
		{"sk-ant-api03-abc", "Anthropic API key", "anthropic.com", "https://api.anthropic.com"},
		{"sk-proj-abc", "OpenAI API key", "openai.com", "https://api.openai.com"},
		{"sk-abc", "OpenAI API key", "openai.com", "https://api.openai.com"},
		{"ghp_abcdef", "GitHub token", "github.com", "https://api.github.com"},
		{"github_pat_11ABC", "GitHub token", "github.com", "https://api.github.com"},
		{"sk_live_abc", "Stripe secret key", "stripe.com", "https://api.stripe.com"},
		{"xoxb-123", "Slack token", "slack.com", "https://slack.com/api"},
		{"glpat-abc", "GitLab token", "gitlab.com", "https://gitlab.com"},
		{"hf_abc", "Hugging Face token", "huggingface.co", "https://huggingface.co"},
		{"npm_abc", "npm token", "npmjs.org", "https://registry.npmjs.org"},
		{"AIzaSyAbc", "Google API key", "googleapis.com", "https://www.googleapis.com"},
	} {
		label, domain, expected, ok := RecognizedCredential(tc.secret)
		require.True(t, ok, tc.secret)
		assert.Equal(t, tc.label, label, tc.secret)
		assert.Equal(t, tc.domain, domain, tc.secret)
		assert.Equal(t, tc.expected, expected, tc.secret)
	}
	// An opaque token is not guessed at: an unknown format must never produce a
	// warning, or the warning becomes noise people learn to dismiss.
	for _, secret := range []string{"", "exhibit-native-super-secret", "Secret123", "gh_", "sk", "XAIza"} {
		_, _, _, ok := RecognizedCredential(secret)
		assert.False(t, ok, secret)
	}
}

// The comparison is between services (eTLD+1), not host strings: a subdomain of
// the right service is right, and a registrable domain that merely contains the
// right name is not.
func TestCredentialDestinationMismatch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		secret   string
		origins  []string
		mismatch bool
	}{
		{"exact service origin", "sk-ant-abc", []string{"https://api.anthropic.com"}, false},
		{"subdomain of the service", "sk-ant-abc", []string{"https://eu.api.anthropic.com"}, false},
		{"same service, other port", "sk-ant-abc", []string{"https://api.anthropic.com:8443"}, false},
		{"lookalike destination", "sk-ant-abc", []string{"https://api-anthropic.com"}, true},
		{"credential for another provider", "sk-ant-abc", []string{"https://api.openai.com"}, true},
		{"openai key at an unrelated host", "sk-proj-abc", []string{"https://api-openai.co"}, true},
		{"suffix attack", "sk-proj-abc", []string{"https://openai.com.evil.test"}, true},
		{"unknown format at any origin", "some-opaque-token", []string{"https://api-openai.co"}, false},
		{"no origin to compare", "sk-ant-abc", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mismatch := credentialDestinationMismatch(tc.secret, tc.origins)
			if !tc.mismatch {
				assert.Nil(t, mismatch)
				return
			}
			require.NotNil(t, mismatch)
			assert.Equal(t, tc.origins[0], mismatch.Origin)
			assert.NotEmpty(t, mismatch.Label)
			assert.NotContains(t, mismatch.Error(), tc.secret, "the credential never appears in the warning")
		})
	}
}

// The gate is on the save path, so the API cannot skip it, and the override is
// explicit per request: refusing is the default answer.
func TestCreateRefusesAMismatchedDestinationUntilConfirmed(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	request := CreateRequest{Name: "Reading list", Recipe: "bearer", Origin: "https://api-openai.co", Secret: "sk-ant-api03-abc"}

	_, err := s.Create(ctx, 1, request, UnlimitedLimits())
	var mismatch *CredentialOriginError
	require.ErrorAs(t, err, &mismatch)
	assert.Equal(t, "https://api.anthropic.com", mismatch.Expected)
	assert.Equal(t, "https://api-openai.co", mismatch.Origin)

	list, err := s.List(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, list, "a refused create leaves nothing behind")

	request.ConfirmOriginMismatch = true
	connection, err := s.Create(ctx, 1, request, UnlimitedLimits())
	require.NoError(t, err)
	assert.Equal(t, []string{"https://api-openai.co"}, connection.Origins)

	// A credential aimed at its own service needs no confirmation at all.
	matched := CreateRequest{Name: "Anthropic", Recipe: "bearer", Origin: "https://api.anthropic.com", Secret: "sk-ant-api03-abc"}
	_, err = s.Create(ctx, 1, matched, UnlimitedLimits())
	require.NoError(t, err)
}

// Basic credentials are a username and password, not a key format, so the
// check does not apply to them and cannot block a legitimate pairing.
func TestCreateDoesNotGuessAtBasicCredentials(t *testing.T) {
	s, _ := testService(t)
	_, err := s.Create(context.Background(), 1, CreateRequest{
		Name:     "Basic",
		Recipe:   "basic",
		Origin:   "https://api-openai.co",
		Username: "hk_live_person",
		Password: "sk-ant-api03-abc",
	}, UnlimitedLimits())
	require.NoError(t, err)
}

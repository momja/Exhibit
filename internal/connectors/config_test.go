package connectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"CONNECTORS_ENABLED", "CONNECTORS_OAUTH_ENABLED", "CONNECTORS_VENDOR_ENABLED", "CONNECTORS_STANDINS_ENABLED", "CONNECTORS_GITHUB_CLIENT_ID", "CONNECTORS_GITHUB_CLIENT_SECRET", "CONNECTORS_GITHUB_SCOPES", "CONNECTORS_GITHUB_AUTH_URL", "CONNECTORS_GITHUB_TOKEN_URL", "CONNECTORS_GITHUB_REVOKE_URL", "CONNECTORS_GITHUB_REDIRECT_URL", "CONNECTORS_GITHUB_API_ORIGIN", "CONNECTORS_NANGO_SECRET_KEY", "CONNECTORS_NANGO_NAMESPACE", "CONNECTORS_NANGO_PROVIDERS", "CONNECTORS_NANGO_BASE_URL", "CONNECTORS_NANGO_IDLE_DAYS"} {
		t.Setenv(key, "")
	}
}
func TestOperatorConfigFlagsAreIndependentAndFailClosed(t *testing.T) {
	cleanConfigEnv(t)
	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.Nil(t, cfg)
	t.Setenv("CONNECTORS_ENABLED", "true")
	cfg, err = ConfigFromEnv()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.False(t, cfg.OAuthEnabled)
	assert.False(t, cfg.VendorEnabled)
	assert.False(t, cfg.StandinsEnabled)
	t.Setenv("CONNECTORS_OAUTH_ENABLED", "tru")
	_, err = ConfigFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONNECTORS_OAUTH_ENABLED")
	t.Setenv("CONNECTORS_OAUTH_ENABLED", "")
	t.Setenv("CONNECTORS_ENABLED", "false")
	t.Setenv("CONNECTORS_STANDINS_ENABLED", "true")
	_, err = ConfigFromEnv()
	require.Error(t, err)
}
func TestOAuthOperatorConfigRequiresCredentialsAndSafeEndpoints(t *testing.T) {
	cleanConfigEnv(t)
	t.Setenv("CONNECTORS_ENABLED", "true")
	t.Setenv("CONNECTORS_OAUTH_ENABLED", "true")
	_, err := ConfigFromEnv()
	require.Error(t, err)
	t.Setenv("CONNECTORS_GITHUB_CLIENT_ID", "client")
	t.Setenv("CONNECTORS_GITHUB_CLIENT_SECRET", "operator-client-secret-7825")
	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "https://api.github.com", cfg.GitHub.APIOrigin)
	assert.Equal(t, []string{"read:user"}, cfg.GitHub.Scopes)
	t.Setenv("CONNECTORS_GITHUB_TOKEN_URL", "https://operator-client-secret-7825@attacker.test/token")
	_, err = ConfigFromEnv()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "operator-client-secret-7825")
	t.Setenv("CONNECTORS_GITHUB_TOKEN_URL", "")
	t.Setenv("CONNECTORS_GITHUB_API_ORIGIN", "https://api.github.com/private/path")
	_, err = ConfigFromEnv()
	require.Error(t, err)
}
func TestManagedOperatorConfigOnlyAdmitsReviewedOrigins(t *testing.T) {
	cleanConfigEnv(t)
	t.Setenv("CONNECTORS_ENABLED", "true")
	t.Setenv("CONNECTORS_VENDOR_ENABLED", "true")
	t.Setenv("CONNECTORS_NANGO_SECRET_KEY", "vendor-operator-secret-7531")
	t.Setenv("CONNECTORS_NANGO_NAMESPACE", "instance-1")
	t.Setenv("CONNECTORS_NANGO_PROVIDERS", `{"slack":{"name":"Slack","origins":["https://slack.com"]}}`)
	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, 30, cfg.Vendor.IdleDays)
	assert.Equal(t, []string{"https://slack.com"}, cfg.Vendor.Providers["slack"].Origins)
	t.Setenv("CONNECTORS_NANGO_PROVIDERS", `{"slack":{"origins":["https://vendor-operator-secret-7531@attacker.test"]}}`)
	_, err = ConfigFromEnv()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "vendor-operator-secret-7531")
	t.Setenv("CONNECTORS_NANGO_PROVIDERS", `{"slack":{"origins":["https://slack.com"]}}`)
	t.Setenv("CONNECTORS_NANGO_IDLE_DAYS", "0")
	_, err = ConfigFromEnv()
	require.Error(t, err)
}

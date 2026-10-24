package connectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"CONNECTORS_ENABLED", "CONNECTORS_OAUTH_ENABLED", "CONNECTORS_VENDOR_ENABLED", "CONNECTORS_STANDINS_ENABLED", "CONNECTORS_GITHUB_CLIENT_ID", "CONNECTORS_GITHUB_CLIENT_SECRET", "CONNECTORS_GITHUB_SCOPES", "CONNECTORS_GITHUB_AUTH_URL", "CONNECTORS_GITHUB_TOKEN_URL", "CONNECTORS_GITHUB_REVOKE_URL", "CONNECTORS_GITHUB_REDIRECT_URL", "CONNECTORS_GITHUB_API_ORIGIN", "CONNECTORS_NANGO_SECRET_KEY", "CONNECTORS_NANGO_NAMESPACE", "CONNECTORS_NANGO_PROVIDERS", "CONNECTORS_NANGO_BASE_URL", "CONNECTORS_NANGO_IDLE_DAYS", "CONNECTORS_VENDOR_PROVIDERS", "CONNECTORS_VENDOR_NAMESPACE", "CONNECTORS_VENDOR_IDLE_DAYS", "CONNECTORS_COMPOSIO_API_KEY", "CONNECTORS_COMPOSIO_BASE_URL"} {
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

func TestVendorOperatorConfigChoosesEnginePerProvider(t *testing.T) {
	cleanConfigEnv(t)
	t.Setenv("CONNECTORS_ENABLED", "true")
	t.Setenv("CONNECTORS_VENDOR_ENABLED", "true")
	t.Setenv("CONNECTORS_VENDOR_NAMESPACE", "instance-shared")
	t.Setenv("CONNECTORS_NANGO_SECRET_KEY", "nango-private-operator-key")
	t.Setenv("CONNECTORS_COMPOSIO_API_KEY", "composio-private-operator-key")
	t.Setenv("CONNECTORS_NANGO_BASE_URL", "https://nango.selfhost.example/")
	t.Setenv("CONNECTORS_VENDOR_IDLE_DAYS", "45")
	t.Setenv("CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"nango","name":"Work chat","origins":["https://SLACK.com/","https://slack.com"],"daily_limit":12,"cache_minutes":15},"linear":{"engine":"composio","auth_config_id":"ac_reviewed","origins":["https://api.linear.app"]}}`)
	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "https://nango.selfhost.example", cfg.Vendor.BaseURL)
	assert.Equal(t, "https://backend.composio.dev", cfg.Vendor.ComposioBaseURL)
	assert.Equal(t, "instance-shared", cfg.Vendor.UserNamespace)
	assert.Equal(t, 45, cfg.Vendor.IdleDays)
	assert.Equal(t, "inject", cfg.Vendor.Providers["slack"].DeliveryMode)
	assert.Equal(t, []string{"https://slack.com"}, cfg.Vendor.Providers["slack"].Origins)
	assert.Equal(t, 12, cfg.Vendor.Providers["slack"].DailyLimit)
	assert.Equal(t, 15, cfg.Vendor.Providers["slack"].CacheMinutes)
	assert.Equal(t, "forward", cfg.Vendor.Providers["linear"].DeliveryMode)
	assert.Equal(t, "linear", cfg.Vendor.Providers["linear"].Toolkit)
	assert.Equal(t, "ac_reviewed", cfg.Vendor.Providers["linear"].AuthConfigID)
}

func TestComposioOnlyOperatorConfigNeedsNoNangoCredentials(t *testing.T) {
	cleanConfigEnv(t)
	t.Setenv("CONNECTORS_ENABLED", "true")
	t.Setenv("CONNECTORS_VENDOR_ENABLED", "true")
	t.Setenv("CONNECTORS_VENDOR_NAMESPACE", "composio-only")
	t.Setenv("CONNECTORS_COMPOSIO_API_KEY", "composio-operator-key")
	t.Setenv("CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"composio","auth_config_id":"ac_slack","origins":["https://slack.com"]}}`)
	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.Empty(t, cfg.Vendor.SecretKey)
	assert.Equal(t, "composio", cfg.Vendor.Providers["slack"].Engine)
	t.Setenv("CONNECTORS_COMPOSIO_API_KEY", "")
	_, err = ConfigFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONNECTORS_COMPOSIO_API_KEY")
}

func TestVendorOperatorConfigRejectsUnsupportedAndUnreviewedConfiguration(t *testing.T) {
	for _, test := range []struct{ name, key, value string }{
		{"missing engine", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"origins":["https://slack.com"]}}`},
		{"unknown engine", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"unreviewed","origins":["https://slack.com"]}}`},
		{"unknown option", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"nango","origin":["https://slack.com"]}}`},
		{"no providers", "CONNECTORS_VENDOR_PROVIDERS", `{}`},
		{"all providers", "CONNECTORS_VENDOR_PROVIDERS", `{"*":{"engine":"nango","origins":["https://slack.com"]}}`},
		{"wildcard origin", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"nango","origins":["https://*.slack.com"]}}`},
		{"credential origin", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"nango","origins":["https://operator-private-value@slack.com"]}}`},
		{"HTTP origin", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"nango","origins":["http://slack.com"]}}`},
		{"path origin", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"nango","origins":["https://slack.com/api"]}}`},
		{"no auth config", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"composio","origins":["https://slack.com"]}}`},
		{"unsupported inject", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"composio","delivery_mode":"inject","auth_config_id":"ac_slack","origins":["https://slack.com"]}}`},
		{"unsupported forward", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"nango","delivery_mode":"forward","origins":["https://slack.com"]}}`},
		{"negative budget", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"nango","daily_limit":-1,"origins":["https://slack.com"]}}`},
		{"negative reuse", "CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"nango","cache_minutes":-1,"origins":["https://slack.com"]}}`},
		{"secret endpoint", "CONNECTORS_COMPOSIO_BASE_URL", "https://operator-private-value@backend.composio.dev"},
		{"endpoint query", "CONNECTORS_NANGO_BASE_URL", "https://nango.example?key=operator-private-value"},
		{"endpoint path", "CONNECTORS_COMPOSIO_BASE_URL", "https://backend.composio.dev/api/v3"},
		{"invalid key characters", "CONNECTORS_COMPOSIO_API_KEY", "operator-private-value\tbroken"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cleanConfigEnv(t)
			t.Setenv("CONNECTORS_ENABLED", "true")
			t.Setenv("CONNECTORS_VENDOR_ENABLED", "true")
			t.Setenv("CONNECTORS_VENDOR_NAMESPACE", "instance")
			t.Setenv("CONNECTORS_NANGO_SECRET_KEY", "operator-private-value")
			t.Setenv("CONNECTORS_COMPOSIO_API_KEY", "operator-private-value")
			t.Setenv("CONNECTORS_VENDOR_PROVIDERS", `{"slack":{"engine":"nango","origins":["https://slack.com"]}}`)
			t.Setenv(test.key, test.value)
			_, err := ConfigFromEnv()
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "operator-private-value")
		})
	}
}

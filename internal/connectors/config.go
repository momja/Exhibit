package connectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ConfigFromEnv reads only operator configuration. Disabled connectors return
// nil, so callers cannot accidentally expose the endpoints with an empty key.
// Errors name variables, never their potentially secret values.
func ConfigFromEnv() (*Config, error) {
	enabled, err := configSwitch("CONNECTORS_ENABLED")
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	for _, flag := range []struct {
		name  string
		value *bool
	}{{"CONNECTORS_OAUTH_ENABLED", &cfg.OAuthEnabled}, {"CONNECTORS_VENDOR_ENABLED", &cfg.VendorEnabled}, {"CONNECTORS_STANDINS_ENABLED", &cfg.StandinsEnabled}} {
		if *flag.value, err = configSwitch(flag.name); err != nil {
			return nil, err
		}
	}
	if !enabled {
		if cfg.OAuthEnabled || cfg.VendorEnabled || cfg.StandinsEnabled {
			return nil, errors.New("connector phase switches require CONNECTORS_ENABLED=true")
		}
		return nil, nil
	}
	if cfg.OAuthEnabled {
		cfg.GitHub.ClientID = strings.TrimSpace(os.Getenv("CONNECTORS_GITHUB_CLIENT_ID"))
		cfg.GitHub.ClientSecret = os.Getenv("CONNECTORS_GITHUB_CLIENT_SECRET")
		if cfg.GitHub.ClientID == "" || cfg.GitHub.ClientSecret == "" {
			return nil, errors.New("CONNECTORS_OAUTH_ENABLED requires CONNECTORS_GITHUB_CLIENT_ID and CONNECTORS_GITHUB_CLIENT_SECRET")
		}
		if invalidConfigSecret(cfg.GitHub.ClientID) || invalidConfigSecret(cfg.GitHub.ClientSecret) {
			return nil, errors.New("GitHub OAuth client configuration contains invalid characters")
		}
		cfg.GitHub = cfg.GitHub.defaults()
		for _, field := range []struct {
			name  string
			value *string
		}{{"CONNECTORS_GITHUB_AUTH_URL", &cfg.GitHub.AuthURL}, {"CONNECTORS_GITHUB_TOKEN_URL", &cfg.GitHub.TokenURL}, {"CONNECTORS_GITHUB_REVOKE_URL", &cfg.GitHub.RevokeURL}, {"CONNECTORS_GITHUB_REDIRECT_URL", &cfg.GitHub.RedirectURL}} {
			if raw := strings.TrimSpace(os.Getenv(field.name)); raw != "" {
				if !validConfigURL(raw) {
					return nil, fmt.Errorf("%s must be an HTTPS URL without credentials or a fragment", field.name)
				}
				*field.value = raw
			}
		}
		if raw := strings.TrimSpace(os.Getenv("CONNECTORS_GITHUB_API_ORIGIN")); raw != "" {
			normalized, err := normalizeHTTPSOrigin(raw)
			if err != nil {
				return nil, errors.New("CONNECTORS_GITHUB_API_ORIGIN must be an HTTPS origin")
			}
			cfg.GitHub.APIOrigin = normalized
		}
		if raw := strings.TrimSpace(os.Getenv("CONNECTORS_GITHUB_SCOPES")); raw != "" {
			cfg.GitHub.Scopes = strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' })
			for _, scope := range cfg.GitHub.Scopes {
				if strings.ContainsAny(scope, "\r\n\t") {
					return nil, errors.New("CONNECTORS_GITHUB_SCOPES contains invalid characters")
				}
			}
		}
	}
	if cfg.VendorEnabled {
		cfg.Vendor, err = vendorConfigFromEnv()
		if err != nil {
			return nil, err
		}
	}
	return cfg, nil
}
func vendorConfigFromEnv() (VendorConfig, error) {
	cfg := VendorConfig{
		BaseURL: "https://api.nango.dev", SecretKey: os.Getenv("CONNECTORS_NANGO_SECRET_KEY"),
		ComposioBaseURL: "https://backend.composio.dev", ComposioAPIKey: os.Getenv("CONNECTORS_COMPOSIO_API_KEY"),
		UserNamespace: strings.TrimSpace(os.Getenv("CONNECTORS_VENDOR_NAMESPACE")), IdleDays: 30,
	}
	if cfg.UserNamespace == "" {
		cfg.UserNamespace = strings.TrimSpace(os.Getenv("CONNECTORS_NANGO_NAMESPACE"))
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`).MatchString(cfg.UserNamespace) {
		return cfg, errors.New("CONNECTORS_VENDOR_NAMESPACE (or CONNECTORS_NANGO_NAMESPACE) must be a nonempty instance namespace")
	}
	if invalidConfigSecret(cfg.SecretKey) || invalidConfigSecret(cfg.ComposioAPIKey) {
		return cfg, errors.New("vendor operator keys contain invalid characters")
	}
	for _, endpoint := range []struct {
		name  string
		value *string
	}{{"CONNECTORS_NANGO_BASE_URL", &cfg.BaseURL}, {"CONNECTORS_COMPOSIO_BASE_URL", &cfg.ComposioBaseURL}} {
		if raw := strings.TrimSpace(os.Getenv(endpoint.name)); raw != "" {
			normalized, err := normalizeHTTPSOrigin(raw)
			if err != nil {
				return cfg, fmt.Errorf("%s must be an HTTPS origin without credentials, query, or fragment", endpoint.name)
			}
			*endpoint.value = normalized
		}
	}
	idleName := "CONNECTORS_VENDOR_IDLE_DAYS"
	rawIdle := strings.TrimSpace(os.Getenv(idleName))
	if rawIdle == "" {
		idleName, rawIdle = "CONNECTORS_NANGO_IDLE_DAYS", strings.TrimSpace(os.Getenv("CONNECTORS_NANGO_IDLE_DAYS"))
	}
	if rawIdle != "" {
		n, err := strconv.Atoi(rawIdle)
		if err != nil || n < 1 || n > 3650 {
			return cfg, fmt.Errorf("%s must be between 1 and 3650", idleName)
		}
		cfg.IdleDays = n
	}
	providerName := "CONNECTORS_VENDOR_PROVIDERS"
	rawProviders := os.Getenv(providerName)
	legacy := strings.TrimSpace(rawProviders) == ""
	if legacy {
		providerName, rawProviders = "CONNECTORS_NANGO_PROVIDERS", os.Getenv("CONNECTORS_NANGO_PROVIDERS")
	}
	decoder := json.NewDecoder(strings.NewReader(rawProviders))
	decoder.DisallowUnknownFields()
	if len(rawProviders) > 65536 || decoder.Decode(&cfg.Providers) != nil || len(cfg.Providers) == 0 {
		return cfg, fmt.Errorf("%s must be a JSON object of reviewed provider configurations", providerName)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return cfg, fmt.Errorf("%s must contain one JSON object", providerName)
	}
	providerID := regexp.MustCompile(`^[a-z][a-z0-9_-]{0,79}$`)
	remoteID := regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)
	for id, provider := range cfg.Providers {
		if !providerID.MatchString(id) || len(provider.Origins) == 0 || len(provider.Origins) > 8 {
			return cfg, fmt.Errorf("%s contains an invalid provider or origin list", providerName)
		}
		if legacy && provider.Engine == "" {
			provider.Engine = "nango"
		}
		if legacy && provider.Engine != "nango" {
			return cfg, errors.New("CONNECTORS_NANGO_PROVIDERS only configures Nango; use CONNECTORS_VENDOR_PROVIDERS for other engines")
		}
		switch provider.Engine {
		case "nango":
			if strings.TrimSpace(cfg.SecretKey) == "" {
				return cfg, errors.New("Nango providers require CONNECTORS_NANGO_SECRET_KEY")
			}
			if provider.DeliveryMode == "" {
				provider.DeliveryMode = "inject"
			}
			if provider.DeliveryMode != "inject" {
				return cfg, errors.New("Nango providers require inject delivery mode")
			}
		case "composio":
			if strings.TrimSpace(cfg.ComposioAPIKey) == "" {
				return cfg, errors.New("Composio providers require CONNECTORS_COMPOSIO_API_KEY")
			}
			if !remoteID.MatchString(provider.AuthConfigID) {
				return cfg, fmt.Errorf("%s Composio providers require a valid auth_config_id", providerName)
			}
			if provider.Toolkit == "" {
				provider.Toolkit = id
			}
			if !providerID.MatchString(provider.Toolkit) {
				return cfg, fmt.Errorf("%s contains an invalid Composio toolkit", providerName)
			}
			if provider.DeliveryMode == "" {
				provider.DeliveryMode = "forward"
			}
			if provider.DeliveryMode != "forward" {
				return cfg, errors.New("Composio providers require forward delivery mode because connected-account credentials are unavailable")
			}
		default:
			return cfg, fmt.Errorf("%s providers require an explicitly configured nango or composio engine", providerName)
		}
		if provider.Name == "" {
			provider.Name = id
		}
		if len(provider.Name) > 100 || strings.ContainsAny(provider.Name, "\r\n\x00") {
			return cfg, fmt.Errorf("%s contains an invalid display name", providerName)
		}
		if provider.DailyLimit < 0 || provider.CacheMinutes < 0 || provider.CacheMinutes > 525600 {
			return cfg, fmt.Errorf("%s contains invalid request or response reuse limits", providerName)
		}
		var origins []string
		for _, entry := range provider.Origins {
			normalized, err := normalizeHTTPSOrigin(entry)
			if err != nil {
				return cfg, fmt.Errorf("%s contains an invalid HTTPS origin", providerName)
			}
			if !slices.Contains(origins, normalized) {
				origins = append(origins, normalized)
			}
		}
		provider.Origins = origins
		cfg.Providers[id] = provider
	}
	return cfg, nil
}

func configSwitch(name string) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return value, nil
}
func validConfigURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == ""
}
func invalidConfigSecret(raw string) bool {
	return len(raw) > 16384 || strings.ContainsFunc(raw, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

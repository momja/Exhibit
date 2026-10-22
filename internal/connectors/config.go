package connectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
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
		cfg.Vendor.SecretKey = os.Getenv("CONNECTORS_NANGO_SECRET_KEY")
		cfg.Vendor.UserNamespace = strings.TrimSpace(os.Getenv("CONNECTORS_NANGO_NAMESPACE"))
		cfg.Vendor.IdleDays = 30
		if cfg.Vendor.SecretKey == "" || cfg.Vendor.UserNamespace == "" {
			return nil, errors.New("CONNECTORS_VENDOR_ENABLED requires CONNECTORS_NANGO_SECRET_KEY and CONNECTORS_NANGO_NAMESPACE")
		}
		if invalidConfigSecret(cfg.Vendor.SecretKey) || !regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`).MatchString(cfg.Vendor.UserNamespace) {
			return nil, errors.New("Nango operator key or namespace has invalid characters")
		}
		cfg.Vendor.BaseURL = "https://api.nango.dev"
		if raw := strings.TrimSpace(os.Getenv("CONNECTORS_NANGO_BASE_URL")); raw != "" {
			if !validConfigURL(raw) {
				return nil, errors.New("CONNECTORS_NANGO_BASE_URL must be an HTTPS URL without credentials or a fragment")
			}
			u, _ := url.Parse(raw)
			if u.RawQuery != "" || u.Path != "" && u.Path != "/" {
				return nil, errors.New("CONNECTORS_NANGO_BASE_URL must be an origin")
			}
			cfg.Vendor.BaseURL = strings.TrimSuffix(raw, "/")
		}
		if raw := strings.TrimSpace(os.Getenv("CONNECTORS_NANGO_IDLE_DAYS")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 3650 {
				return nil, errors.New("CONNECTORS_NANGO_IDLE_DAYS must be between 1 and 3650")
			}
			cfg.Vendor.IdleDays = n
		}
		raw := os.Getenv("CONNECTORS_NANGO_PROVIDERS")
		if len(raw) > 65536 || json.Unmarshal([]byte(raw), &cfg.Vendor.Providers) != nil || len(cfg.Vendor.Providers) == 0 {
			return nil, errors.New("CONNECTORS_NANGO_PROVIDERS must be a JSON object of reviewed provider names and HTTPS origins")
		}
		for id, p := range cfg.Vendor.Providers {
			if !regexp.MustCompile(`^[a-z][a-z0-9_-]{0,79}$`).MatchString(id) || len(p.Origins) == 0 || len(p.Origins) > 8 {
				return nil, errors.New("CONNECTORS_NANGO_PROVIDERS contains an invalid provider or origin list")
			}
			if p.Name == "" {
				p.Name = id
			}
			if len(p.Name) > 100 {
				return nil, errors.New("CONNECTORS_NANGO_PROVIDERS contains an invalid display name")
			}
			for i, o := range p.Origins {
				normalized, err := normalizeHTTPSOrigin(o)
				if err != nil {
					return nil, errors.New("CONNECTORS_NANGO_PROVIDERS contains an invalid HTTPS origin")
				}
				p.Origins[i] = normalized
			}
			cfg.Vendor.Providers[id] = p
		}
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
	return len(raw) > 16384 || strings.ContainsAny(raw, "\r\n\x00")
}

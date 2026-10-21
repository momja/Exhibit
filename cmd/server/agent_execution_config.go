package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// agentExecutionConfig selects where Pi runs. Provider keys and policy still
// belong to Exhibit; none of these credentials are passed to Pi.
type agentExecutionConfig struct {
	Mode           string
	WorkerURL      string
	WorkerToken    string
	AllowInsecure  bool
	SpriteToken    string
	SpritePrefix   string
	ReleaseURL     string
	ReleaseVersion string
	ReleaseSHA256  string
	StartupTimeout time.Duration
	IdleRetention  time.Duration
	MaxWorkers     int
}

func (agentExecutionConfig) String() string       { return "agentExecutionConfig{credentials redacted}" }
func (cfg agentExecutionConfig) GoString() string { return cfg.String() }

func agentExecutionConfigFromEnv() (agentExecutionConfig, error) {
	cfg := agentExecutionConfig{
		Mode:           strings.TrimSpace(os.Getenv("AGENT_EXECUTION_MODE")),
		StartupTimeout: 2 * time.Minute,
		IdleRetention:  30 * time.Minute,
		MaxWorkers:     32,
	}
	if cfg.Mode == "" {
		cfg.Mode = "local"
	}
	switch cfg.Mode {
	case "off", "local":
	case "remote":
		cfg.WorkerURL = strings.TrimSpace(os.Getenv("AGENT_WORKER_URL"))
		cfg.WorkerToken = strings.TrimSpace(os.Getenv("AGENT_WORKER_TOKEN"))
		if err := validateAgentEndpoint("AGENT_WORKER_URL", cfg.WorkerURL); err != nil {
			return cfg, err
		}
		if len(cfg.WorkerToken) < 32 || strings.ContainsAny(cfg.WorkerToken, "\r\n\x00 ") {
			return cfg, fmt.Errorf("remote execution requires an AGENT_WORKER_TOKEN of at least 32 characters without whitespace")
		}
		if raw := strings.TrimSpace(os.Getenv("AGENT_WORKER_ALLOW_INSECURE")); raw != "" {
			var err error
			cfg.AllowInsecure, err = strconv.ParseBool(raw)
			if err != nil {
				return cfg, fmt.Errorf("AGENT_WORKER_ALLOW_INSECURE must be a boolean")
			}
		}
		u, _ := url.Parse(cfg.WorkerURL)
		if u.Scheme == "http" && u.Hostname() != "localhost" && !net.ParseIP(u.Hostname()).IsLoopback() && !cfg.AllowInsecure {
			return cfg, fmt.Errorf("AGENT_WORKER_URL requires HTTPS; a trusted private HTTP deployment must explicitly set AGENT_WORKER_ALLOW_INSECURE")
		}
	case "sprites":
		cfg.SpriteToken = strings.TrimSpace(os.Getenv("SPRITE_TOKEN"))
		cfg.SpritePrefix = strings.TrimSpace(os.Getenv("AGENT_SPRITE_PREFIX"))
		cfg.ReleaseURL = strings.TrimSpace(os.Getenv("AGENT_WORKER_RELEASE_URL"))
		cfg.ReleaseVersion = strings.TrimSpace(os.Getenv("AGENT_WORKER_RELEASE_VERSION"))
		cfg.ReleaseSHA256 = strings.TrimSpace(os.Getenv("AGENT_WORKER_RELEASE_SHA256"))
		if cfg.SpriteToken == "" {
			return cfg, fmt.Errorf("sprites execution requires SPRITE_TOKEN")
		}
		if cfg.SpritePrefix == "" {
			return cfg, fmt.Errorf("sprites execution requires AGENT_SPRITE_PREFIX")
		}
		for _, ch := range cfg.SpritePrefix {
			if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
				return cfg, fmt.Errorf("AGENT_SPRITE_PREFIX must contain only lowercase letters, digits, and hyphens")
			}
		}
		if len(cfg.SpritePrefix) < 3 || len(cfg.SpritePrefix) > 27 || cfg.SpritePrefix[0] < 'a' || cfg.SpritePrefix[0] > 'z' || strings.HasSuffix(cfg.SpritePrefix, "-") {
			return cfg, fmt.Errorf("AGENT_SPRITE_PREFIX must be 3–27 characters, start with a letter, and end alphanumeric")
		}
		if err := validateAgentEndpoint("AGENT_WORKER_RELEASE_URL", cfg.ReleaseURL); err != nil {
			return cfg, err
		}
		u, _ := url.Parse(cfg.ReleaseURL)
		if u.Scheme != "https" {
			return cfg, fmt.Errorf("AGENT_WORKER_RELEASE_URL must use HTTPS")
		}
		if cfg.ReleaseVersion == "" {
			return cfg, fmt.Errorf("sprites execution requires AGENT_WORKER_RELEASE_VERSION")
		}
		digest, err := hex.DecodeString(cfg.ReleaseSHA256)
		if err != nil || len(digest) != 32 || cfg.ReleaseSHA256 != strings.ToLower(cfg.ReleaseSHA256) {
			return cfg, fmt.Errorf("AGENT_WORKER_RELEASE_SHA256 must be a lowercase SHA256 checksum")
		}
	default:
		return cfg, fmt.Errorf("AGENT_EXECUTION_MODE must be off, local, remote, or sprites")
	}
	var err error
	if cfg.StartupTimeout, err = agentExecutionDuration("AGENT_WORKER_STARTUP_TIMEOUT", cfg.StartupTimeout, false); err != nil {
		return cfg, err
	}
	if cfg.IdleRetention, err = agentExecutionDuration("AGENT_WORKER_IDLE_RETENTION", cfg.IdleRetention, cfg.Mode != "sprites"); err != nil {
		return cfg, err
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_MAX_WORKERS")); raw != "" {
		cfg.MaxWorkers, err = strconv.Atoi(raw)
		if err != nil || cfg.MaxWorkers <= 0 {
			return cfg, fmt.Errorf("AGENT_MAX_WORKERS must be a positive integer")
		}
	}
	return cfg, nil
}

func validateAgentEndpoint(name, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s must be an HTTP(S) URL without user information, query, or fragment", name)
	}
	return nil
}

func agentExecutionDuration(name string, fallback time.Duration, allowZero bool) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration < 0 || (!allowZero && duration == 0) {
		bound := "a positive"
		if allowZero {
			bound = "a nonnegative"
		}
		return 0, fmt.Errorf("%s must be %s duration", name, bound)
	}
	return duration, nil
}

package main

import (
	"fmt"
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
	SpriteToken    string
	SpritePrefix   string
	BinaryURL      string
	StartupTimeout time.Duration
	IdleRetention  time.Duration
	MaxWorkers     int
}

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
		return cfg, nil
	case "remote":
		cfg.WorkerURL = strings.TrimSpace(os.Getenv("AGENT_WORKER_URL"))
		cfg.WorkerToken = strings.TrimSpace(os.Getenv("AGENT_WORKER_TOKEN"))
		if err := validateAgentEndpoint("AGENT_WORKER_URL", cfg.WorkerURL); err != nil {
			return cfg, err
		}
		if cfg.WorkerToken == "" {
			return cfg, fmt.Errorf("remote execution requires AGENT_WORKER_TOKEN")
		}
	case "sprites":
		cfg.SpriteToken = strings.TrimSpace(os.Getenv("SPRITE_TOKEN"))
		cfg.SpritePrefix = strings.TrimSpace(os.Getenv("AGENT_SPRITE_PREFIX"))
		cfg.BinaryURL = strings.TrimSpace(os.Getenv("AGENT_WORKER_BINARY_URL"))
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
		if len(cfg.SpritePrefix) > 24 {
			return cfg, fmt.Errorf("AGENT_SPRITE_PREFIX must be at most 24 characters")
		}
		if err := validateAgentEndpoint("AGENT_WORKER_BINARY_URL", cfg.BinaryURL); err != nil {
			return cfg, err
		}
	default:
		return cfg, fmt.Errorf("AGENT_EXECUTION_MODE must be off, local, remote, or sprites")
	}
	var err error
	if cfg.StartupTimeout, err = agentExecutionDuration("AGENT_WORKER_STARTUP_TIMEOUT", cfg.StartupTimeout, false); err != nil {
		return cfg, err
	}
	if cfg.IdleRetention, err = agentExecutionDuration("AGENT_WORKER_IDLE_RETENTION", cfg.IdleRetention, true); err != nil {
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

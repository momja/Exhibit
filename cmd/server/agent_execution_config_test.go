package main

import (
	"strings"
	"testing"
	"time"
)

func clearAgentExecutionEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"AGENT_EXECUTION_MODE", "AGENT_WORKER_URL", "AGENT_WORKER_TOKEN", "SPRITE_TOKEN", "AGENT_SPRITE_PREFIX", "AGENT_WORKER_BINARY_URL", "AGENT_WORKER_STARTUP_TIMEOUT", "AGENT_WORKER_IDLE_RETENTION", "AGENT_MAX_WORKERS"} {
		t.Setenv(name, "")
	}
}

func TestAgentExecutionModeIsExplicit(t *testing.T) {
	clearAgentExecutionEnv(t)
	cfg, err := agentExecutionConfigFromEnv()
	if err != nil || cfg.Mode != "local" {
		t.Fatalf("default: config=%+v err=%v", cfg, err)
	}
	t.Setenv("AGENT_EXECUTION_MODE", "remote")
	if _, err = agentExecutionConfigFromEnv(); err == nil {
		t.Fatal("remote mode must not silently fall back when unconfigured")
	}
	t.Setenv("AGENT_WORKER_URL", "http://127.0.0.1:8090")
	t.Setenv("AGENT_WORKER_TOKEN", "worker-test-credential")
	t.Setenv("AGENT_WORKER_STARTUP_TIMEOUT", "45s")
	t.Setenv("AGENT_WORKER_IDLE_RETENTION", "0s")
	t.Setenv("AGENT_MAX_WORKERS", "7")
	cfg, err = agentExecutionConfigFromEnv()
	if err != nil || cfg.StartupTimeout != 45*time.Second || cfg.IdleRetention != 0 || cfg.MaxWorkers != 7 {
		t.Fatalf("configured remote mode was rejected: %v", err)
	}
}

func TestAgentExecutionRejectsIncompleteOrMalformedDeployment(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"unknown backend", map[string]string{"AGENT_EXECUTION_MODE": "automatic"}, "AGENT_EXECUTION_MODE"},
		{"missing worker auth", map[string]string{"AGENT_WORKER_TOKEN": ""}, "AGENT_WORKER_TOKEN"},
		{"credentials in URL", map[string]string{"AGENT_WORKER_URL": "https://user:password@worker.example"}, "AGENT_WORKER_URL"},
		{"query token", map[string]string{"AGENT_WORKER_URL": "https://worker.example?token=secret"}, "AGENT_WORKER_URL"},
		{"non HTTP", map[string]string{"AGENT_WORKER_URL": "file:///tmp/worker"}, "AGENT_WORKER_URL"},
		{"zero startup", map[string]string{"AGENT_WORKER_STARTUP_TIMEOUT": "0s"}, "AGENT_WORKER_STARTUP_TIMEOUT"},
		{"negative retention", map[string]string{"AGENT_WORKER_IDLE_RETENTION": "-1s"}, "AGENT_WORKER_IDLE_RETENTION"},
		{"unbounded workers", map[string]string{"AGENT_MAX_WORKERS": "0"}, "AGENT_MAX_WORKERS"},
		{"missing Sprite auth", map[string]string{"AGENT_EXECUTION_MODE": "sprites"}, "SPRITE_TOKEN"},
		{"missing Sprite namespace", map[string]string{"AGENT_EXECUTION_MODE": "sprites", "SPRITE_TOKEN": "test"}, "AGENT_SPRITE_PREFIX"},
		{"invalid Sprite namespace", map[string]string{"AGENT_EXECUTION_MODE": "sprites", "SPRITE_TOKEN": "test", "AGENT_SPRITE_PREFIX": "Project/Owner"}, "AGENT_SPRITE_PREFIX"},
		{"missing runtime artifact", map[string]string{"AGENT_EXECUTION_MODE": "sprites", "SPRITE_TOKEN": "test", "AGENT_SPRITE_PREFIX": "test"}, "AGENT_WORKER_BINARY_URL"},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearAgentExecutionEnv(t)
			t.Setenv("AGENT_EXECUTION_MODE", "remote")
			t.Setenv("AGENT_WORKER_URL", "https://worker.example")
			t.Setenv("AGENT_WORKER_TOKEN", "worker-test-credential")
			for name, value := range test.env {
				t.Setenv(name, value)
			}
			_, err := agentExecutionConfigFromEnv()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want error naming %s; got %v", test.want, err)
			}
			if strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("configuration error reveals credentials: %v", err)
			}
		})
	}
}

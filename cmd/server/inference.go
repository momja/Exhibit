package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/agentguidance"
	"github.com/momja/Exhibit/internal/api"
	"github.com/momja/Exhibit/internal/inference"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
)

// newInferenceService reads the artifact inference configuration (Exh-zue3):
//
//	INFERENCE_ENABLED         off by default; on turns window.exhibit.ai on
//	INFERENCE_MODEL           chat model for complete() (required in platform mode)
//	INFERENCE_DECISION_MODEL  classifier model for decide() (optional)
//	INFERENCE_MOCK_URL        tests and demos only: every provider request
//	                          goes to this mock server
//
// Providers and payers follow the agent: AGENT_API_KEY + AGENT_PROVIDER make
// the instance pay; otherwise each viewer's own stored agent key pays. The
// agent's spend caps bound platform-paid inference too, and an instance that
// enables inference in platform mode without an owner or instance cap does
// not boot: the session and turn caps do not bound an artifact's calls.
//
// Off returns a disabled service, never nil, so every caller can ask it.
func newInferenceService(ctx context.Context, st store.Store, box *secrets.Box, platform *api.PlatformKey,
	caps agent.SpendCaps, guardrail *agent.Guardrail, piBin, workDir string) (*inference.Service, *inference.Worker, error) {
	enabled, err := boolEnv("INFERENCE_ENABLED")
	if err != nil || !enabled {
		svc, _ := inference.New(inference.Config{})
		return svc, nil, err
	}
	if platform != nil && caps.OwnerMicrosPerMonth == nil && caps.InstanceMicrosPerMonth == nil {
		return nil, nil, fmt.Errorf("INFERENCE_ENABLED is set with AGENT_API_KEY but neither AGENT_SPEND_CAP_OWNER_CENTS nor AGENT_SPEND_CAP_INSTANCE_CENTS is: platform-paid artifact AI would be unbounded")
	}
	path, err := exec.LookPath(piBin)
	if err != nil {
		return nil, nil, fmt.Errorf("INFERENCE_ENABLED is set but the Pi executable %q was not found", piBin)
	}
	worker, err := inference.NewWorker(inference.WorkerConfig{PiBin: path, WorkDir: workDir, MockURL: strings.TrimSpace(os.Getenv("INFERENCE_MOCK_URL"))})
	if err != nil {
		return nil, nil, err
	}
	cfg := inference.Config{
		Enabled:       true,
		ChatModel:     strings.TrimSpace(os.Getenv("INFERENCE_MODEL")),
		DecisionModel: strings.TrimSpace(os.Getenv("INFERENCE_DECISION_MODEL")),
		Ledger:        st,
		Caps:          inference.SpendCaps{OwnerMicrosPerMonth: caps.OwnerMicrosPerMonth, InstanceMicrosPerMonth: caps.InstanceMicrosPerMonth},
		Runtime:       worker,
		Keys: func(ctx context.Context, viewerID int64) (inference.Credential, bool, error) {
			k, err := st.GetAgentKey(ctx, viewerID)
			if err != nil || k == nil {
				return inference.Credential{}, false, err
			}
			plain, err := box.Decrypt(k.KeyCiphertext)
			if err != nil {
				return inference.Credential{}, false, err
			}
			return inference.Credential{Provider: k.Provider, APIKey: plain}, true, nil
		},
	}
	if platform != nil {
		cfg.Platform = &inference.Credential{Provider: platform.Provider, APIKey: platform.APIKey}
		if guardrail != nil {
			cfg.Guardrail = &inference.Guardrail{Provider: guardrail.Provider, Model: guardrail.Model, APIKey: guardrail.APIKey}
		}
	}
	svc, err := inference.New(cfg)
	if err != nil {
		worker.Close()
		return nil, nil, err
	}
	startCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := worker.Start(startCtx); err != nil {
		worker.Close()
		return nil, nil, fmt.Errorf("start the inference worker: %w", err)
	}
	if err := svc.CheckStartup(startCtx); err != nil {
		worker.Close()
		return nil, nil, err
	}
	slog.Info("artifact inference enabled", slog.Bool("platform", platform != nil),
		slog.Bool("decide", cfg.DecisionModel != "" || platform == nil), slog.Bool("mock", os.Getenv("INFERENCE_MOCK_URL") != ""))
	return svc, worker, nil
}

func boolEnv(name string) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false, got %q", name, raw)
	}
	return v, nil
}

// inferenceGuidanceNames is what the agent's prompt teaches: the offered
// kinds, plus "images" when complete() can take them (Exh-v81w).
func inferenceGuidanceNames(ctx context.Context, svc *inference.Service) []string {
	names := inferenceKindNames(svc.InstanceKinds())
	if slices.Contains(names, agentguidance.KindComplete) && svc.InstanceImages(ctx) {
		names = append(names, agentguidance.Images)
	}
	return names
}

// inferenceKindNames spells kinds the way agent guidance takes them.
func inferenceKindNames(kinds []inference.Kind) []string {
	names := make([]string, 0, len(kinds))
	for _, k := range kinds {
		names = append(names, string(k))
	}
	return names
}

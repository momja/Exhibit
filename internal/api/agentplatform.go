package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/momja/Exhibit/internal/agent"
)

// Platform mode: the instance supplies the agent credential (av-siqf).
//
// The agent is BYO key by default and that is the right shape for a
// self-hosted library — the operator is the user, and the key is theirs. It is
// the wrong shape for a hosted instance, where asking someone to open a
// provider account and paste a key before they can use the headline feature is
// the step that loses them. Setting AGENT_API_KEY puts the instance in
// platform mode: every session runs on that one credential, the BYOK surface
// is not rendered, and the per-owner key resource does not exist.
//
// One variable chooses between two modes rather than a per-owner key taking
// precedence over an instance-wide fallback. The fallback shape reads like the
// flexible one and is worse in both directions: it silently mixes billing
// models, since an owner who pasted their own key stops being metered while
// the interface says nothing about it, and it leaves a key field on a surface
// whose entire point is that nobody needs one.

// PlatformKey is the credential an instance runs its own agent sessions on.
// A nil *PlatformKey is BYOK — the default, and byte-identical to the
// behaviour before this existed.
//
// The provider and the model are as private as the key itself. Someone using
// AI to build a tool does not need to know what is under the hood, and telling
// them invents a decision they cannot act on; the answer for anyone who wants
// that control is to self-host, where BYOK gives it to them in full. So this
// value reaches exactly one caller — agentSessionOpts, which hands it to the
// subprocess environment — and never a response, a rendered page, or a log
// line.
type PlatformKey struct {
	Provider string
	Model    string
	APIKey   string
}

// PlatformKeyFromEnv reads AGENT_API_KEY / AGENT_PROVIDER / AGENT_MODEL,
// returning nil when the key is unset. Same shape as OIDC_ISSUER: absent means
// the feature does not exist and no existing instance changes.
//
// A key with a missing or unknown provider is an error rather than a value
// fixed up at startup, and the caller makes it fatal — the same posture as a
// malformed LOGIN_PASSWORD_HASH. The alternative is an instance that boots
// looking configured and fails at whichever session happens to be first.
//
// AGENT_MODEL is optional: empty leaves the model to the provider's own
// default, exactly as an empty model does for a stored BYO key.
func PlatformKeyFromEnv() (*PlatformKey, error) {
	key := strings.TrimSpace(os.Getenv("AGENT_API_KEY"))
	if key == "" {
		return nil, nil
	}
	provider := strings.TrimSpace(os.Getenv("AGENT_PROVIDER"))
	if provider == "" {
		return nil, fmt.Errorf("AGENT_API_KEY is set but AGENT_PROVIDER is not")
	}
	if !agent.KnownProvider(provider) {
		return nil, fmt.Errorf("unsupported AGENT_PROVIDER %q", provider)
	}
	pk := &PlatformKey{
		Provider: provider,
		Model:    strings.TrimSpace(os.Getenv("AGENT_MODEL")),
		APIKey:   key,
	}
	pk.logStartup()
	return pk, nil
}

// logStartup announces the mode and the bill that comes with it. The warning
// is here rather than at the call site so that no way of constructing a
// platform-mode instance can omit it.
//
// What used to be warned here — that nothing bounds the spend — is gone:
// usage is metered per owner (av-2yws) and the spend cap bounds it
// (av-99f4), whose RequireSpendCaps refuses to boot a platform credential
// with no cap at all. This line remains because the bill is still real.
//
// Neither the provider nor the model is named, for the same reason no response
// names them: a log line is read by the operator, who set them, but it is also
// the thing most likely to be pasted into a support thread.
func (pk *PlatformKey) logStartup() {
	slog.Warn("platform agent mode enabled: every agent session runs on this instance's own provider credential and bills its account; spend is metered per owner and bounded by the configured spend caps")
}

// RequireSpendCaps is the startup rule av-99f4 makes structural: a platform
// credential with no cap at all must not boot. Absence of the feature is
// unlimited — a BYOK instance with no caps configured is untouched — but
// absence of a limit the feature requires is a refusal to boot, because the
// resource being spent is money and the failure is unbounded rather than
// merely large. This supersedes the startup warning logStartup used to carry
// for exactly this case.
func RequireSpendCaps(pk *PlatformKey, caps agent.SpendCaps) error {
	if pk == nil || caps.AnySet() {
		return nil
	}
	return fmt.Errorf("AGENT_API_KEY is set but no spend cap is: set at least one of " +
		"AGENT_SPEND_CAP_OWNER_CENTS, AGENT_SPEND_CAP_SESSION_CENTS, AGENT_SPEND_CAP_INSTANCE_CENTS " +
		"(AGENT_SPEND_CAP_TURN_SECONDS also bounds a runaway turn) — a platform credential without a cap " +
		"bills the instance without bound")
}

// platformMode reports whether this instance supplies the agent credential.
func (ro *Router) platformMode() bool { return ro.cfg.PlatformAgentKey != nil }

// byokOnly hides the per-owner key resource in platform mode.
//
// The refusal is a 404 rather than a 403 for the reason the rest of this API
// answers 404 (architecture.md §3.3): the resource genuinely does not exist
// here, and a distinct refusal code would be the one place the UI's silence
// about the platform credential is contradicted.
func (ro *Router) byokOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ro.platformMode() {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		next.ServeHTTP(w, r)
	})
}

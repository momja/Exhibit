package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// The usage-policy guardrail (av-gust).
//
// A hosted instance's agent spends the operator's provider account, and a
// provider that sees repeated usage-policy violations can suspend that account
// for every user at once. The guardrail screens each user message on a model
// the operator configures, before the agent model sees it. The screen itself
// lives in ext/guard.ts, loaded as a second Pi extension; this file is the
// host's half: configuration, and turning the extension's signal into the one
// reply a blocked user sees.
//
// It enforces a usage policy, not a topic. Off-topic use is allowed, and this
// neither meters nor caps spend; that is the spend cap (av-99f4), not yet built.

// GuardrailBlockedReply is the only thing a user is told when a message is
// blocked or could not be screened. It is fixed and lives here, in the host,
// so no text the extension or a model produced ever reaches the user on a
// block. A failed screen reads the same as a refusal, so the reply is no
// oracle for probing the screen.
const GuardrailBlockedReply = "This request isn't allowed under this instance's usage policy."

// guardSignal prefixes every notify message ext/guard.ts sends. It must match
// GUARD_SIGNAL there. The word after it says what the signal is: "blocked" and
// "error" mean the extension handled the prompt instead of running it; "usage"
// reports what one screen spent and says nothing about the verdict.
const guardSignal = "exhibit_guard:"

// Guardrail is the screening model's configuration. A nil *Guardrail means no
// guardrail, which is every instance that does not set GUARDRAIL_*.
//
// Provider and Model are Pi's own ids, resolved against Pi's catalog by the
// extension: a classifier model (TypeSafe's Jev, served by several providers)
// or any chat model. They are not checked against providerEnv, because the key
// travels as a per-request option rather than through a provider's
// environment variable, which is also what lets the guardrail and the agent
// share a vendor without sharing a key.
type Guardrail struct {
	Provider string
	Model    string
	APIKey   string
}

// GuardrailFromEnv reads GUARDRAIL_PROVIDER / GUARDRAIL_MODEL /
// GUARDRAIL_API_KEY. All unset is nil: the feature does not exist and no
// instance changes. Some but not all set is an error the caller makes fatal:
// an operator who configured part of a guardrail believes one is running, and
// booting without it is the failure nobody notices.
func GuardrailFromEnv() (*Guardrail, error) {
	g := &Guardrail{
		Provider: strings.TrimSpace(os.Getenv("GUARDRAIL_PROVIDER")),
		Model:    strings.TrimSpace(os.Getenv("GUARDRAIL_MODEL")),
		APIKey:   strings.TrimSpace(os.Getenv("GUARDRAIL_API_KEY")),
	}
	if *g == (Guardrail{}) {
		return nil, nil
	}
	var missing []string
	for _, v := range []struct{ name, value string }{
		{"GUARDRAIL_PROVIDER", g.Provider},
		{"GUARDRAIL_MODEL", g.Model},
		{"GUARDRAIL_API_KEY", g.APIKey},
	} {
		if v.value == "" {
			missing = append(missing, v.name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("a guardrail is partly configured: %s must also be set", strings.Join(missing, ", "))
	}
	return g, nil
}

// env is the guard extension's contract. The data fence tells it where the
// user's own words end and the fenced artifact source begins, so stored
// content is never screened as though the user had just typed it.
func (g *Guardrail) env(nonce string) []string {
	return []string{
		"EXHIBIT_GUARD_PROVIDER=" + g.Provider,
		"EXHIBIT_GUARD_MODEL=" + g.Model,
		"EXHIBIT_GUARD_API_KEY=" + g.APIKey,
		"EXHIBIT_GUARD_DATA_FENCE=" + beginFence(nonce),
	}
}

// guardSignalOf recognizes one of the extension's signals in a line of Pi
// output, returning its verb, the detail after it, and true; false for any
// other line. The verb is matched exactly by the caller: treating every
// guard signal as a block would refuse each allowed message the moment the
// screen reported its usage.
func guardSignalOf(line []byte) (verb, detail string, ok bool) {
	var probe struct {
		Type    string `json:"type"`
		Method  string `json:"method"`
		Message string `json:"message"`
	}
	if json.Unmarshal(line, &probe) != nil {
		return "", "", false
	}
	if probe.Type != "extension_ui_request" || probe.Method != "notify" || !strings.HasPrefix(probe.Message, guardSignal) {
		return "", "", false
	}
	verb, detail, _ = strings.Cut(strings.TrimPrefix(probe.Message, guardSignal), " ")
	return verb, detail, true
}

// handleGuardSignal acts on one guard signal. None of them is forwarded to
// the browser: a block becomes the host's fixed reply, and the rest is
// operator detail.
func (s *Session) handleGuardSignal(verb, detail string) {
	switch verb {
	case "blocked", "error":
		s.noteGuardBlocked(verb + " " + detail)
	case "usage":
		s.noteGuardUsage(detail)
	default:
		slog.Warn("unknown guardrail signal", slog.String("session_id", s.ID), slog.String("verb", verb))
	}
}

// noteGuardUsage receives what one screen spent, as the JSON guard.ts sends:
// {"provider","model","usage"}, usage in Pi's shape. The guardrail runs on the
// instance's key whatever key the session uses, so this is operator spend.
// Metering (av-2yws) records it; until then it is logged for the operator.
func (s *Session) noteGuardUsage(detail string) {
	slog.Debug("guardrail screen usage", slog.String("session_id", s.ID), slog.String("usage", truncate(detail, 300)))
}

// noteGuardBlocked replaces the extension's signal with the event the chat UI
// renders. The signal's detail goes to the operator's log only.
func (s *Session) noteGuardBlocked(detail string) {
	s.mu.Lock()
	s.guardBlocked = true
	s.mu.Unlock()
	slog.Warn("agent prompt blocked by the usage-policy guardrail",
		slog.String("session_id", s.ID),
		slog.Int64("owner_id", s.OwnerID),
		slog.String("detail", detail))
	ev, _ := json.Marshal(map[string]any{
		"type":    "exhibit_guard_blocked",
		"message": GuardrailBlockedReply,
	})
	s.broadcast(ev)
}

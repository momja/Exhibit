package agent

import (
	"bytes"
	"encoding/json"
)

// Model abstraction (av-siqf).
//
// One of this package's seams hands Pi's own protocol to the browser verbatim:
// readLoop broadcasts every line the subprocess emits. It filters nothing, so
// what the browser learns about the model is a property of Pi's protocol rather
// than of any handler — and Pi's protocol is explicit about it. Every assistant
// message it emits carries the triple below:
//
//	{"role":"assistant", "api":"openai-completions",
//	 "provider":"anthropic", "model":"claude-sonnet-4-5", ...}
//
// which reaches a subscriber on message_start, message_end, turn_end and
// agent_end. The stored conversation holds the same triple, but it is not a
// seam: it leaves the server only as a projection (transcript.go) that names
// fields explicitly, none of them these.
//
// On a BYOK instance that is the caller's own credential described back to
// them and nothing is hidden. In platform mode the instance supplies the key
// and reports nothing about it, so the identifiers are stripped from the event
// stream; otherwise "the agent page names no provider or model" would be a
// statement about one page while the network tab said otherwise.
//
// What is deliberately *not* stripped is the `usage` block beside them (token
// counts and cost). It names no model, it is what the metering work will read
// (av-hyo6), and dropping it would cost a future feature to hide nothing.

// modelIdentityFields are the keys removed from a Pi message envelope.
var modelIdentityFields = []string{"api", "provider", "model"}

const platformAgentFailureMessage = "The agent could not complete your request. Please try again."

// redactModelIdentity strips model identifiers and provider failure details
// from one JSON document. Error text can identify the provider or describe its
// credentials, so hosted clients receive only a generic failure message.
//
// The match is structural rather than by key name alone: a field is removed
// only from an object that also carries a "role", which is Pi's message
// envelope and nothing else. A tool argument or a piece of artifact state that
// happens to have a "model" field is left intact, because it is the artifact's
// data and not a fact about this instance.
//
// A document that does not parse comes back unchanged: callers only reach this
// with JSON they have already decoded once, so an unparseable one here would be
// a bug rather than untrusted input, and mangling the stream is the worse
// failure. The Session methods below are what actually gate on platform mode.
func redactModelIdentity(doc []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(doc))
	// Numbers stay verbatim: Pi timestamps are milliseconds since the epoch,
	// and a float64 round-trip would re-marshal them in scientific notation.
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return doc
	}
	changed := stripModelIdentity(v)
	// These fields belong to the event envelope, not arbitrary artifact data
	// nested inside it. Cover retry and compaction failures as well as messages.
	if event, ok := v.(map[string]any); ok {
		switch event["type"] {
		case "auto_retry_start", "compaction_end":
			changed = redactProviderError(event, "errorMessage") || changed
		case "auto_retry_end":
			changed = redactProviderError(event, "finalError") || changed
		case "extension_error":
			changed = redactProviderError(event, "error") || changed
		}
	}
	if !changed {
		return doc
	}
	out, err := json.Marshal(v)
	if err != nil {
		return doc
	}
	return out
}

// stripModelIdentity removes message identifiers and assistant failure details.
// It reports whether anything changed, so untouched lines keep Pi's bytes.
func stripModelIdentity(v any) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		if _, isMessage := t["role"]; isMessage {
			for _, f := range modelIdentityFields {
				if _, ok := t[f]; ok {
					delete(t, f)
					changed = true
				}
			}
			if t["role"] == "assistant" && t["stopReason"] == "error" {
				changed = redactProviderError(t, "errorMessage") || changed
			}
		}
		for _, child := range t {
			if stripModelIdentity(child) {
				changed = true
			}
		}
	case []any:
		for _, child := range t {
			if stripModelIdentity(child) {
				changed = true
			}
		}
	}
	return changed
}

func redactProviderError(fields map[string]any, name string) bool {
	detail, ok := fields[name].(string)
	if !ok || detail == "" || detail == platformAgentFailureMessage {
		return false
	}
	fields[name] = platformAgentFailureMessage
	return true
}

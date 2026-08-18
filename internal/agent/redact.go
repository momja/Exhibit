package agent

import (
	"bytes"
	"encoding/json"
)

// Model abstraction (av-siqf).
//
// Two of this package's seams hand Pi's own protocol to the browser verbatim:
// readLoop broadcasts every line the subprocess emits, and persistTranscript
// stores Pi's message list as the artifact's colophon. Neither filters
// anything, so what the browser learns about the model is a property of Pi's
// protocol rather than of any handler — and Pi's protocol is explicit about it.
// Every assistant message it emits carries the triple below:
//
//	{"role":"assistant", "api":"openai-completions",
//	 "provider":"anthropic", "model":"claude-sonnet-4-5", ...}
//
// which reaches a subscriber on message_start, message_end, turn_end and
// agent_end, and is persisted in agent_transcripts.messages.
//
// On a BYOK instance that is the caller's own credential described back to
// them and nothing is hidden. In platform mode the instance supplies the key
// and reports nothing about it, so the identifiers are stripped at both seams;
// otherwise "the agent page names no provider or model" would be a statement
// about one page while the network tab said otherwise.
//
// What is deliberately *not* stripped is the `usage` block beside them (token
// counts and cost). It names no model, it is what the metering work will read
// (av-hyo6), and dropping it would cost a future feature to hide nothing.

// modelIdentityFields are the keys removed from a Pi message envelope.
var modelIdentityFields = []string{"api", "provider", "model"}

// redactModelIdentity strips the model identifiers from one JSON document —
// an event line or a message list — leaving everything else as it was.
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
	if !stripModelIdentity(v) {
		return doc
	}
	out, err := json.Marshal(v)
	if err != nil {
		return doc
	}
	return out
}

// stripModelIdentity walks a decoded document, removing the identity fields
// from every message envelope it finds. It reports whether anything changed,
// so an untouched line is broadcast as the exact bytes Pi wrote.
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

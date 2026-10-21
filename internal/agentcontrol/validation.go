package agentcontrol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode"
)

func ValidateCheckpoint(p Checkpoint) error {
	if len(p.SessionFile) > MaxSnapshotBytes {
		return ErrTooLarge
	}
	if p.Revision <= 0 || len(p.Title) > 512 || strings.IndexFunc(p.Title, unicode.IsControl) >= 0 {
		return ErrInvalid
	}
	if p.Kind != CheckpointTool && p.Kind != CheckpointSettled && p.Kind != CheckpointInterrupted {
		return ErrInvalid
	}
	// An acknowledged snapshot consists entirely of complete JSON objects. A
	// reader racing Pi's append trims an unfinished tail before calling this API.
	if p.SessionFile == "" || !strings.HasSuffix(p.SessionFile, "\n") {
		return ErrInvalid
	}
	records := 0
	for line := range strings.SplitSeq(p.SessionFile, "\n") {
		if line == "" {
			continue
		}
		b := bytes.TrimSpace([]byte(line))
		if len(b) == 0 || b[0] != '{' || !json.Valid(b) {
			return ErrInvalid
		}
		records++
	}
	if records == 0 {
		return ErrInvalid
	}
	return nil
}
func CheckpointHash(p Checkpoint) string {
	h := sha256.New()
	h.Write([]byte(p.SessionFile))
	h.Write([]byte{0})
	h.Write([]byte(p.Title))
	h.Write([]byte{0})
	h.Write([]byte(p.Kind))
	return hex.EncodeToString(h.Sum(nil))
}
func ValidateUsage(u Usage) error {
	if u.EventID == "" || len(u.EventID) > 200 || len(u.Provider) > 200 || len(u.Model) > 200 {
		return ErrInvalid
	}
	switch u.Source {
	case "model", "tool", "compaction", "guardrail":
	default:
		return ErrInvalid
	}
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.CacheReadTokens < 0 || u.CacheWriteTokens < 0 || u.CostMicros < 0 {
		return ErrInvalid
	}
	return nil
}
func UsageHash(u Usage) string {
	b, _ := json.Marshal(u)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

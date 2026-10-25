package agent

import (
	"fmt"
	"os"
	"strings"
)

// DefaultThinkingLevel is the baseline when the operator leaves it unset.
const DefaultThinkingLevel = "medium"

// ValidThinkingLevel accepts Pi's thinking levels and the empty value used by
// an owner's settings to inherit the instance baseline.
func ValidThinkingLevel(level string) bool {
	switch level {
	case "", "off", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

// ThinkingLevelFromEnv reads the instance-wide AGENT_THINKING_LEVEL baseline.
// Pi clamps the resolved level to the selected model's supported levels.
func ThinkingLevelFromEnv() (string, error) {
	level := strings.TrimSpace(os.Getenv("AGENT_THINKING_LEVEL"))
	if !ValidThinkingLevel(level) {
		return "", fmt.Errorf("invalid AGENT_THINKING_LEVEL %q: want off, minimal, low, medium, high, xhigh, or max", level)
	}
	if level == "" {
		level = DefaultThinkingLevel
	}
	return level, nil
}

package agent

import (
	"fmt"
	"os"
	"strings"
)

// ThinkingLevelFromEnv reads the instance-wide AGENT_THINKING_LEVEL override.
// Empty leaves Pi's default and saved conversation levels in effect. Pi clamps
// a configured level to the selected model's supported thinking levels.
func ThinkingLevelFromEnv() (string, error) {
	level := strings.TrimSpace(os.Getenv("AGENT_THINKING_LEVEL"))
	switch level {
	case "", "off", "minimal", "low", "medium", "high", "xhigh", "max":
		return level, nil
	default:
		return "", fmt.Errorf("invalid AGENT_THINKING_LEVEL %q: want off, minimal, low, medium, high, xhigh, or max", level)
	}
}

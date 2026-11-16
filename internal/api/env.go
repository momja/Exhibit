package api

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// envBool reads a boolean environment variable, treating unknown values as off.
func envBool(key string) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "yes", "on":
		return true
	case "no", "off":
		return false
	}
	v, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		slog.Warn("unrecognized boolean env var; treating as off",
			slog.String("var", key), slog.String("value", raw))
		return false
	}
	return v
}

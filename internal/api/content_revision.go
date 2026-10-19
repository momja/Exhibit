package api

import (
	"net/http"
	"strings"

	"github.com/momja/Exhibit/internal/store"
)

// Legacy owner/API clients may omit a token; the handler still protects its
// own read/commit interval. Agent grants must supply the token they read.
func contentPrecondition(w http.ResponseWriter, r *http.Request, fields map[string]any, name, fallback string, required bool) (*string, bool) {
	value, present := fields[name]
	if !present {
		if required && agentGrantFromCtx(r.Context()) != nil {
			writePreconditionRequired(w, strings.TrimSuffix(strings.TrimPrefix(name, "expected_"), "_revision"))
			return nil, false
		}
		return &fallback, true
	}
	token, ok := value.(string)
	if !ok || token == "" {
		writeError(w, http.StatusBadRequest, name+" must be a non-empty string")
		return nil, false
	}
	// No-op detection and targeted edits use this handler's snapshot. A
	// different token cannot validate those decisions, even if a concurrent
	// restore makes that token current again before the transaction checks it.
	if token != fallback {
		err := store.ErrSourceChanged
		if name == "expected_widget_revision" {
			err = store.ErrWidgetChanged
		}
		writeArtifactError(w, r, "check content snapshot", err)
		return nil, false
	}
	return &token, true
}

func writePreconditionRequired(w http.ResponseWriter, resource string) {
	writeJSON(w, http.StatusPreconditionRequired, map[string]string{
		"error": "read the " + resource + " and retry with expected_" + resource + "_revision; changes were not saved",
		"code":  "precondition_required", "resource": resource,
	})
}

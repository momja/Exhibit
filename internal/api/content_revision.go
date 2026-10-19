package api

import (
	"net/http"
	"strings"
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
	return &token, true
}

func writePreconditionRequired(w http.ResponseWriter, resource string) {
	writeJSON(w, http.StatusPreconditionRequired, map[string]string{
		"error": "read the " + resource + " and retry with expected_" + resource + "_revision; changes were not saved",
		"code":  "precondition_required", "resource": resource,
	})
}

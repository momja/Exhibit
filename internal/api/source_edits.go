package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/momja/Exhibit/internal/store"
)

// sourceEdit is an exact replacement in a versioned source snapshot. The agent
// resolves any fuzzy matches against its retained source before sending these.
type sourceEdit struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

// expandSourceEdits keeps patches on the existing scan/version/save path. The
// immutable body pointer and revision are from the same artifact snapshot; the
// eventual commit checks the revision again inside its write transaction.
func (ro *Router) expandSourceEdits(w http.ResponseWriter, r *http.Request, artifact *store.Artifact, updates map[string]any, widget bool) bool {
	raw, present := updates["edits"]
	if !present {
		return true
	}
	if _, mixed := updates["body"]; mixed {
		writeError(w, http.StatusBadRequest, "send either body or edits, not both")
		return false
	}
	resource, blobID, current := "source", artifact.SourceBlobID, artifact.SourceRevision
	if widget {
		resource, blobID, current = "widget", artifact.WidgetBlobID, artifact.WidgetRevision
	}
	field := "expected_" + resource + "_revision"
	value, present := updates[field]
	if !present {
		writePreconditionRequired(w, resource)
		return false
	}
	expected, ok := value.(string)
	if !ok || expected == "" {
		writeError(w, http.StatusBadRequest, field+" must be a non-empty string")
		return false
	}
	if expected != current {
		err := store.ErrSourceChanged
		if widget {
			err = store.ErrWidgetChanged
		}
		writeArtifactError(w, r, "edit source", err)
		return false
	}
	edits, err := decodeSourceEdits(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	if blobID == "" {
		writeError(w, http.StatusBadRequest, "there is no "+resource+" to edit; save a complete document first")
		return false
	}
	reader, err := ro.cfg.Blob.Get(r.Context(), blobID)
	if err != nil {
		serverError(w, r, "read source for edits", err)
		return false
	}
	source, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		serverError(w, r, "read source for edits", err)
		return false
	}
	body, err := applySourceEdits(string(source), edits)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	if strings.TrimSpace(body) == "" {
		writeError(w, http.StatusBadRequest, "edits must leave a nonempty document")
		return false
	}
	delete(updates, "edits")
	updates["body"] = body
	return true
}

func decodeSourceEdits(raw any) ([]sourceEdit, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var fields []map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil || len(fields) == 0 {
		return nil, fmt.Errorf("edits must be a nonempty array of replacements")
	}
	edits := make([]sourceEdit, len(fields))
	for i, field := range fields {
		for name, output := range map[string]*string{"oldText": &edits[i].OldText, "newText": &edits[i].NewText} {
			value, present := field[name]
			if !present || string(value) == "null" || json.Unmarshal(value, output) != nil {
				return nil, fmt.Errorf("edits[%d].%s must be a string", i, name)
			}
		}
	}
	return edits, nil
}

// applySourceEdits validates every span against the original source before
// applying any replacement. It never guesses which of several matches to use.
func applySourceEdits(source string, edits []sourceEdit) (string, error) {
	if len(edits) == 0 {
		return "", fmt.Errorf("edits must contain at least one replacement")
	}
	type replacement struct {
		start, end int
		text       string
	}
	spans := make([]replacement, 0, len(edits))
	for i, edit := range edits {
		if edit.OldText == "" {
			return "", fmt.Errorf("edits[%d].oldText must not be empty", i)
		}
		start := strings.Index(source, edit.OldText)
		if start < 0 {
			return "", fmt.Errorf("edits[%d] did not match; read the source and retry", i)
		}
		// Advance one byte rather than len(oldText) to detect overlapping matches.
		if strings.Contains(source[start+1:], edit.OldText) {
			return "", fmt.Errorf("edits[%d] matched more than once; include unique surrounding source", i)
		}
		spans = append(spans, replacement{start, start + len(edit.OldText), edit.NewText})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	for i := 1; i < len(spans); i++ {
		if spans[i].start <= spans[i-1].end {
			return "", fmt.Errorf("edits overlap or touch; combine them into one replacement")
		}
	}
	var result strings.Builder
	position := 0
	for _, span := range spans {
		result.WriteString(source[position:span.start])
		result.WriteString(span.text)
		position = span.end
	}
	result.WriteString(source[position:])
	return result.String(), nil
}

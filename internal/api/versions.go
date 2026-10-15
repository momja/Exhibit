package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/momja/Exhibit/internal/store"
)

// Artifact version history (store/versions.go). Two routes and nothing else:
// the history is written as a side effect of every change to an artifact's body
// or widget, so there is no route that creates a version, and the only one that
// changes anything is the restore.
//
// Both are owner-only. An agent session's credential cannot reach either
// (agentSubResources is deny-by-default and has no entry for `versions`):
// returning an artifact to an earlier state is a decision for a person, and a
// session steered by text Exhibit did not author must not be able to make it.

// versionProvenance says what produced the version this request is about to
// write. An agent's writes are labelled with the session its credential was
// minted for and the message it is answering — both read off the grant, which
// the server holds, never off anything the model sent. Anything else is a
// person, and origin says what they were doing.
func versionProvenance(r *http.Request, origin string) store.Provenance {
	if g := agentGrantFromCtx(r.Context()); g != nil {
		return store.Provenance{
			Origin:    store.VersionAgent,
			Message:   versionLabel(g.Prompt()),
			SessionID: g.SessionID(),
		}
	}
	return store.Provenance{Origin: origin}
}

// versionLabelMax bounds a label: it is shown in a list, and a pasted prompt
// can be arbitrarily long.
const versionLabelMax = 200

// versionLabel reduces a message to one line short enough to list.
func versionLabel(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > versionLabelMax {
		s = string(r[:versionLabelMax-1]) + "…"
	}
	return s
}

type versionsResponse struct {
	Versions []store.Version `json:"versions"`
}

// listVersions returns an artifact's history, newest first. Another owner's
// artifact answers 404, like an id that does not exist.
func (ro *Router) listVersions(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "list versions artifact lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	versions, err := ro.cfg.Store.ListVersions(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "list versions", err)
		return
	}
	writeJSON(w, http.StatusOK, versionsResponse{Versions: versions})
}

// restoreVersion returns an artifact to an earlier version: its body and widget
// become the head again and the state that version left behind replaces the
// live state. It is recorded as a new version, so nothing is lost and the
// restore can itself be undone — the state it replaced is snapshotted first.
//
// The caller is expected to have asked a person first: this changes the
// artifact's code and its saved data at once. The pages do.
func (ro *Router) restoreVersion(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")
	seq, err := strconv.Atoi(chi.URLParam(r, "seq"))
	if err != nil || seq < 1 {
		writeError(w, http.StatusBadRequest, "invalid version number")
		return
	}
	ownerID := ownerIDFromCtx(r.Context())

	target, err := ro.cfg.Store.GetVersion(r.Context(), ownerID, id, seq)
	if err != nil {
		serverError(w, r, "restore version lookup", err)
		return
	}
	if target == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// The search shadow of the body being restored. The blob store is not
	// reachable from SQL, so it is read here, which also means a version whose
	// bytes are gone is refused before anything changes.
	body, err := ro.readBlobString(r.Context(), target.BodyBlobID)
	if err != nil {
		writeError(w, http.StatusConflict, "that version's source is no longer stored")
		return
	}

	v, err := ro.cfg.Store.RestoreVersion(r.Context(), ownerID, id, seq, store.Provenance{
		Origin:  store.VersionRestore,
		Message: fmt.Sprintf("Restored v%d", seq),
	}, store.ExtractSearchText(body))
	switch {
	case errors.Is(err, store.ErrAlreadyCurrent):
		writeError(w, http.StatusConflict, "that version is already the current one")
		return
	case err != nil:
		writeArtifactError(w, r, "restore version", err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

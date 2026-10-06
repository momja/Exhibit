package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/momja/Exhibit/internal/store"
)

// The version viewer: an earlier version of an artifact shown running, to be
// looked at before deciding whether to return to it. It is a server-rendered
// fragment (the versionViewer partial) that the page swaps in — the chat's
// preview pane when a conversation is about to be rolled back, the edit page's
// Versions panel when a version is about to be restored — so what a person is
// shown has one definition in both places.
//
// It fetches nothing the pages could not already, and writes nothing: the frame
// it carries is the render surface's version document (render.ServeVersion), which
// reads the version's code and the data it left behind and persists none of what
// is done in it. Owner-only, like every other read of an artifact's history.

// versionViewerData feeds the "versionViewer" partial.
type versionViewerData struct {
	ArtifactID string
	Seq        int
	Title      string
	// Label, Message and When say what the version is, in the Versions panel's
	// words — the same row a person saw before pressing View.
	Label   string
	Message string
	When    string
	WhenISO string
	// FrameURL is the version document, tokened for this version alone.
	FrameURL string
	// Restorable offers the restore beside the view. The edit page asks for it,
	// because the decision to return to a version is made there; the chat's is
	// made in the History card beside the pane, which carries the choice of
	// whether to continue the conversation too, so its pane offers none.
	Restorable bool
}

// versionViewerPartial renders the viewer for one version of one artifact.
//
// Every refusal is a fragment the page swaps in place of what it had, so the
// person learns why nothing opened rather than staring at a stale view (the
// fragmentError partial, as for the other /partials/* routes). Another owner's
// artifact answers as one that is not there, and so does a version that was
// never made. The current version is refused with its own message: it has no
// earlier state to look at, and the artifact's own page already shows it.
func (ro *Router) versionViewerPartial(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	q := r.URL.Query()
	id := q.Get("artifact")

	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		fragmentServerError(w, r, "version viewer artifact lookup", err)
		return
	}
	if a == nil {
		fragmentNotFound(w)
		return
	}
	seq, err := strconv.Atoi(q.Get("seq"))
	if err != nil || seq < 1 {
		writeFragmentMessage(w, http.StatusNotFound, "That version was not found.")
		return
	}
	v, err := ro.cfg.Store.GetVersion(r.Context(), ownerID, id, seq)
	if err != nil {
		fragmentServerError(w, r, "version viewer version lookup", err)
		return
	}
	if v == nil {
		writeFragmentMessage(w, http.StatusNotFound, "That version was not found.")
		return
	}
	if v.Current {
		writeFragmentMessage(w, http.StatusConflict,
			fmt.Sprintf("v%d is the current version — it is what the artifact shows now.", seq))
		return
	}

	vv := newVersionViews([]store.Version{*v})[0]
	ro.writeFragment(w, r, "versionViewer", versionViewerData{
		ArtifactID: a.ID,
		Seq:        v.Seq,
		Title:      a.Title,
		Label:      vv.Label,
		Message:    vv.Message,
		When:       vv.When,
		WhenISO:    vv.WhenISO,
		FrameURL:   ro.renderURLs(r).version(a.ID, v.Seq),
		Restorable: q.Get("restorable") == "1",
	})
}

// writeFragmentMessage answers a /partials/* request the page should be told the
// reason for: the status is the reason's, and the body is the fragmentError
// partial carrying it. fragmentNotFound and fragmentServerError are this with a
// fixed message each.
func writeFragmentMessage(w http.ResponseWriter, status int, message string) {
	fragment, err := renderPage("fragmentError", message)
	if err != nil {
		http.Error(w, message, status)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprint(w, fragment)
}

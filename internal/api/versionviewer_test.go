package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Looking at an earlier version before deciding whether to return to it: the
// fragment the chat's preview pane and the edit page's Versions panel swap in,
// the controls that ask for it, and the URL it mints for the render surface.

// versionedFixture is an artifact with three versions and data that moved:
//
//	v1  "first"   left behind {score: 10}
//	v2  "second"  left behind {score: 50}
//	v3  "third"   live        {score: 99}   (the head)
func versionedFixture(t *testing.T) (*Router, string) {
	t.Helper()
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": "<html><head></head><body>first</body></html>"})
	setState := func(value string) {
		t.Helper()
		w := doJSON(t, r, "PUT", "/api/artifacts/"+id+"/state", map[string]string{"key": "score", "value": value})
		require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	}
	setState("10")
	patchArtifact(t, r, id, map[string]any{"body": "<html><head></head><body>second</body></html>"})
	setState("50")
	patchArtifact(t, r, id, map[string]any{"body": "<html><head></head><body>third</body></html>"})
	setState("99")
	return r, id
}

var versionFrameSrc = regexp.MustCompile(`<iframe class="version-viewer-frame" src="([^"]+)"`)

// versionFrameURL is the URL the fragment points its frame at.
func versionFrameURL(t *testing.T, fragment string) *url.URL {
	t.Helper()
	m := versionFrameSrc.FindStringSubmatch(fragment)
	require.NotNil(t, m, "no version frame in: %s", fragment)
	u, err := url.Parse(strings.ReplaceAll(m[1], "&amp;", "&"))
	require.NoError(t, err)
	return u
}

func TestTheVersionViewerShowsAnEarlierVersionRunning(t *testing.T) {
	r, id := versionedFixture(t)

	frag := getPage(t, r, "/partials/version-viewer?artifact="+id+"&seq=1")

	assert.Contains(t, frag, "Viewing <strong>v1</strong>")
	assert.Contains(t, frag, "Created", "it says what the version is, in the Versions panel's words")
	assert.Contains(t, frag, "A preview — nothing you do here is saved.")
	assert.Contains(t, frag, `sandbox="allow-scripts allow-forms"`)
	assert.NotContains(t, frag, "allow-same-origin")
	assert.Contains(t, frag, `data-action="close-version-viewer"`)

	// The chat page's bridges listen to #pv-frame and nothing else, which is the
	// whole of why a version frame cannot be heard by them: it must never carry
	// that id, whatever else changes about this markup.
	assert.NotContains(t, frag, `id="pv-frame"`)
	assert.NotContains(t, frag, "pv-frame")

	// The restore beside the view is the edit page's to ask for.
	assert.NotContains(t, frag, `data-action="restore"`)
	withRestore := getPage(t, r, "/partials/version-viewer?artifact="+id+"&seq=1&restorable=1")
	assert.Contains(t, withRestore, `data-action="restore" data-seq="1"`)
}

// A tool that tries to save in a preview is told it did not (av-vw7r). The words
// are the page's, written into the fragment and hidden until the frame reports a
// refused write, so what is checked here is that the banner is there, hidden,
// drawn as the capability warning is, and says the true thing for the page that
// asked: that nothing is saved until the version is the active one, and which
// button makes it so.
func TestTheVersionViewerHasAHiddenWarningForAToolThatTriesToSave(t *testing.T) {
	r, id := versionedFixture(t)

	chat := getPage(t, r, "/partials/version-viewer?artifact="+id+"&seq=2")
	edit := getPage(t, r, "/partials/version-viewer?artifact="+id+"&seq=2&restorable=1")

	for name, frag := range map[string]string{"chat": chat, "edit page": edit} {
		// components.js reveals this id, and the id is the whole contract between
		// the two: a template that spells it differently is a notice that never shows.
		assert.Contains(t, frag, `id="version-viewer-unsaved"`, name)
		// The capability warning's own classes and role, so it is the same warning
		// in the same style (and picks up any change to it).
		assert.Contains(t, frag, `class="banner banner-warn" id="version-viewer-unsaved" role="alert" hidden>`, name)
		assert.Contains(t, frag, `<i class="ph ph-warning" aria-hidden="true"></i>`, name)
		assert.Contains(t, frag, "Changes aren't saved while you preview v2.", name)
		assert.Contains(t, frag, "to make it the active version.", name)
		assert.Contains(t, frag, `<details class="banner-details">`, name)
	}

	// Each page names the way back to a version as it offers it: the edit page's
	// Restore sits beside the view, the chat's rollback is its History card's.
	assert.Contains(t, edit, "Restore v2 to make it the active version.")
	assert.NotContains(t, edit, "Roll back to v2")
	assert.Contains(t, chat, "Roll back to v2 to make it the active version.")
	assert.NotContains(t, chat, "Restore v2 to make")

	// The warning sits between the bar and the frame, so it reads as part of what
	// the person is looking at and not as a banner for the whole page.
	assert.Less(t, strings.Index(edit, `class="version-viewer-bar"`), strings.Index(edit, `id="version-viewer-unsaved"`))
	assert.Less(t, strings.Index(edit, `id="version-viewer-unsaved"`), strings.Index(edit, `class="version-viewer-body"`))
}

// The URL the fragment mints is a version document on the render origin, under a
// token for that version alone — which is what keeps the token the live document
// hands the artifact from opening history.
func TestTheVersionViewerMintsATokenForThatVersionAlone(t *testing.T) {
	r, id := versionedFixture(t)

	u := versionFrameURL(t, getPage(t, r, "/partials/version-viewer?artifact="+id+"&seq=2"))
	assert.Equal(t, "/a/"+id+"/versions/2", u.Path)
	assert.Equal(t, r.cfg.RenderOrigin, u.Scheme+"://"+u.Host)
	tok := u.Query().Get(rendertoken.Param)
	require.NotEmpty(t, tok)

	claims, err := r.tokens.Verify(tok, rendertoken.VersionScope(id, 2))
	require.NoError(t, err)
	assert.Equal(t, int64(defaultOwnerID), claims.OwnerID)
	assert.Equal(t, claims.OwnerID, claims.ViewerID, "minted as the owner")
	assert.False(t, claims.Anonymous)

	for name, scope := range map[string]string{
		"the artifact's live document": id,
		"another version":              rendertoken.VersionScope(id, 1),
	} {
		_, err := r.tokens.Verify(tok, scope)
		assert.ErrorIs(t, err, rendertoken.ErrInvalid, name)
	}
}

// Minting and serving have to agree about the scope, and the only thing that
// shows they do is asking the real render handler for what the app minted.
func TestTheMintedVersionURLIsServedByTheRenderSurface(t *testing.T) {
	r, id := versionedFixture(t)
	u := versionFrameURL(t, getPage(t, r, "/partials/version-viewer?artifact="+id+"&seq=1"))

	w := httptest.NewRecorder()
	r.RenderHandler().ServeHTTP(w, httptest.NewRequest("GET", u.RequestURI(), nil))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	assert.Contains(t, body, "first", "version 1's code")
	assert.NotContains(t, body, "third")
	assert.Contains(t, body, `"score":"10"`, "and the data version 1 left behind")
	assert.NotContains(t, body, `"score":"99"`)
	assert.Contains(t, body, "var VERSION_VIEW = true;")
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "sandbox allow-scripts allow-forms")

	// Presented to the live document's route, the same token opens nothing.
	live := httptest.NewRecorder()
	r.RenderHandler().ServeHTTP(live, httptest.NewRequest("GET", "/a/"+id+"?"+rendertoken.Param+"="+u.Query().Get(rendertoken.Param), nil))
	assert.Equal(t, http.StatusNotFound, live.Code)
	assert.NotContains(t, live.Body.String(), "third")
}

// Every refusal is a fragment the page swaps in, saying why; and another owner's
// artifact is indistinguishable from one that is not there, so the route is not an
// oracle over which ids are real or how much history they have.
func TestTheVersionViewerRefusesWhatCannotBeLookedAt(t *testing.T) {
	r, id := versionedFixture(t)
	foreign := seedForeignArtifact(t, r)

	get := func(path string) (int, string) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Code, w.Body.String()
	}

	code, body := get("/partials/version-viewer?artifact=" + id + "&seq=3")
	assert.Equal(t, http.StatusConflict, code)
	assert.Contains(t, body, "v3 is the current version")
	assert.NotContains(t, body, "version-viewer-frame")

	for _, path := range []string{
		"/partials/version-viewer?artifact=" + id + "&seq=99",
		"/partials/version-viewer?artifact=" + id + "&seq=0",
		"/partials/version-viewer?artifact=" + id + "&seq=abc",
		"/partials/version-viewer?artifact=" + id,
	} {
		code, body := get(path)
		assert.Equal(t, http.StatusNotFound, code, path)
		assert.Contains(t, body, `class="frag-error"`, path)
		assert.Contains(t, body, "That version was not found.", path)
	}

	// Somebody else's artifact, with a version that does exist, answers exactly
	// as an artifact that never did.
	theirs, theirsBody := get("/partials/version-viewer?artifact=" + foreign + "&seq=1")
	ghost, ghostBody := get("/partials/version-viewer?artifact=no-such-artifact&seq=1")
	assert.Equal(t, http.StatusNotFound, theirs)
	assert.Equal(t, ghost, theirs)
	assert.Equal(t, ghostBody, theirsBody)
	assert.NotContains(t, theirsBody, "version-viewer-frame")
}

// --- The controls that ask for it -----------------------------------------------

// The History card is where a rollback is decided, so it is where a person can
// look at what they would be returning to first.
func TestTheCardOffersToViewTheVersionItWouldRollBackTo(t *testing.T) {
	r, id := resumeCardFixture(t)
	patchArtifact(t, r, id, map[string]any{"body": "<html><body>two</body></html>"}) // v2: it has moved on

	frag := getPage(t, r, "/partials/agent-transcript?artifact="+id+"&session=sess-1")

	view := `hx-get="/partials/version-viewer?artifact=` + id + `&amp;seq=1" hx-target="#pane-preview" hx-swap="innerHTML"`
	assert.Contains(t, frag, view)
	assert.Contains(t, frag, `onclick="showPane('preview')"`, "on a phone the preview is another screen")
	assert.Contains(t, frag, "View v1 in the preview first")

	// It is offered before the decision it informs, and it is not a way to make
	// that decision: the card's own buttons remain the only things that change
	// the artifact.
	assert.Less(t, strings.Index(frag, "View v1 in the preview first"), strings.Index(frag, "Roll back to v1 and continue"))
	viewButton := frag[strings.Index(frag, `class="resume-view"`):]
	viewButton = viewButton[:strings.Index(viewButton, "</button>")]
	assert.NotContains(t, viewButton, "data-resume", "looking is not continuing")
}

// With the artifact where the conversation left it there is no earlier version
// to look at, and nothing to decide.
func TestTheCardOffersNothingToViewWhileTheArtifactIsWhereTheConversationLeftIt(t *testing.T) {
	r, id := resumeCardFixture(t)
	frag := getPage(t, r, "/partials/agent-transcript?artifact="+id+"&session=sess-1")
	assert.NotContains(t, frag, "version-viewer")
	assert.NotContains(t, frag, "resume-view")
}

func TestTheVersionsPanelOffersToViewEveryVersionButTheCurrent(t *testing.T) {
	r, id := versionedFixture(t)
	page := getPage(t, r, "/artifacts/"+id+"/edit")

	assert.Contains(t, page, `id="version-viewer-slot"`)
	for _, seq := range []string{"1", "2"} {
		assert.Contains(t, page,
			`data-action="view" data-seq="`+seq+`" hx-get="/partials/version-viewer?artifact=`+id+`&amp;seq=`+seq+`&amp;restorable=1" hx-target="#version-viewer-slot" hx-swap="innerHTML"`)
		assert.Contains(t, page, `data-action="restore" data-seq="`+seq+`"`)
	}
	assert.NotContains(t, page, `data-action="view" data-seq="3"`, "the current version is what the artifact shows")
	assert.NotContains(t, page, `data-action="restore" data-seq="3"`)
}

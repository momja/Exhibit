package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-4wyq. Deleting your own account and the library it owns.
//
// Two things are being pinned here and they are not the same thing. One is
// that the erasure is complete — rows *and* bytes, which is why av-7jcq had to
// land first. The other is that the page says what deletion actually means:
// Exhibit cannot remove anyone from their identity provider, so the same
// person signing in again gets a fresh row and an empty library. Someone who
// deletes, finds their login still works, and concludes nothing happened is
// worse off than someone who never had the button.

// deleteInstance is an instance with a login, an admin (the first account, as
// the schema's first-admin rule makes it), and an ordinary member who is the
// subject of these tests. The member matters: an account that is the only
// enabled admin cannot be deleted, and the person av-4wyq is about — signed in
// through a provider, with no shell on the host — is never that account.
type deleteInstance struct {
	ro      *Router
	st      store.Store
	blobDir string
	member  *store.User
	cookie  *http.Cookie
}

func newDeleteInstance(t *testing.T) deleteInstance {
	t.Helper()
	blobDir := t.TempDir()
	bl, err := blob.NewFSStore(blobDir)
	require.NoError(t, err)

	ro, st := newLoginTestRouter(t, &stubProvider{}, nil, func(c *Config) { c.Blob = bl })
	ctx := context.Background()
	admin, err := st.UpsertUser(ctx, "sub-admin", "admin@example.test")
	require.NoError(t, err)
	require.True(t, admin.IsAdmin, "the first account is the instance's admin")

	member, err := st.UpsertUser(ctx, "sub-member", "member@example.test")
	require.NoError(t, err)
	require.False(t, member.IsAdmin)

	return deleteInstance{
		ro: ro, st: st, blobDir: blobDir, member: member,
		cookie: sessionCookieFor(t, st, member.ID, "session-member"),
	}
}

func (in deleteInstance) page(t *testing.T) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req.AddCookie(in.cookie)
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return w.Body.String()
}

// asMember issues a request carrying the member's session cookie.
func (in deleteInstance) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	}
	r.AddCookie(in.cookie)
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, r)
	return w
}

// seedLibrary gives the member one of everything the deletion has to reach,
// and returns the artifact's id.
func (in deleteInstance) seedLibrary(t *testing.T) string {
	t.Helper()
	w := in.do(t, http.MethodPost, "/api/artifacts", map[string]any{
		"title":             "Member's tool",
		"body":              "<html><body>member bytes</body></html>",
		"network_allowlist": []string{"https://api.example.test"},
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	id := resp["artifact"].(map[string]any)["id"].(string)

	require.Equal(t, http.StatusOK, in.do(t, http.MethodPatch, "/api/artifacts/"+id,
		map[string]any{"network_allowlist": []string{"https://api.example.test"}}).Code)
	require.Equal(t, http.StatusOK, in.do(t, http.MethodPut, "/api/artifacts/"+id+"/widget",
		map[string]any{"body": "<b>tile</b>"}).Code)
	require.Equal(t, http.StatusNoContent, in.do(t, http.MethodPut, "/api/artifacts/"+id+"/state",
		map[string]any{"key": "runs", "value": "12"}).Code)

	// A tag and a collection, attached — the two owner-scoped tables no
	// cascade from `artifacts` reaches.
	tag := in.do(t, http.MethodPost, "/api/tags", map[string]any{"name": "fitness"})
	require.Equal(t, http.StatusCreated, tag.Code, tag.Body.String())
	var tagResp store.Tag
	require.NoError(t, json.Unmarshal(tag.Body.Bytes(), &tagResp))
	require.Equal(t, http.StatusNoContent,
		in.do(t, http.MethodPost, "/api/artifacts/"+id+"/tags/"+tagResp.ID, nil).Code)

	col := in.do(t, http.MethodPost, "/api/collections", map[string]any{"name": "Health"})
	require.Equal(t, http.StatusCreated, col.Code, col.Body.String())
	var colResp store.Collection
	require.NoError(t, json.Unmarshal(col.Body.Bytes(), &colResp))
	require.Equal(t, http.StatusNoContent,
		in.do(t, http.MethodPost, "/api/artifacts/"+id+"/collections/"+colResp.ID, nil).Code)

	share := in.do(t, http.MethodPost, "/api/shares", map[string]any{"artifact_id": id})
	require.Equal(t, http.StatusCreated, share.Code, share.Body.String())

	ctx := context.Background()
	require.NoError(t, in.st.SetAgentKey(ctx, &store.AgentKey{
		OwnerID: in.member.ID, Provider: "anthropic", KeyCiphertext: "sealed"}))
	require.NoError(t, in.st.SaveTranscript(ctx, in.member.ID, id, "sess-1", `[{"role":"user"}]`))
	return id
}

func (in deleteInstance) deleteAccount(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	return in.do(t, http.MethodDelete, "/api/account",
		map[string]any{"confirm": deleteAccountConfirmation})
}

// --- the page ----------------------------------------------------------

// The section av-qo05 shipped disabled is live, and the reason it carried
// ("Not available yet") is gone with the limitation it described.
func TestProfileDeleteSectionIsLive(t *testing.T) {
	page := newDeleteInstance(t).page(t)

	assert.Contains(t, page, `id="delete-account"`)
	assert.NotContains(t, page, "Not available yet")
	assert.NotContains(t, page, `id="delete-account" disabled`)
	assert.Contains(t, page, `id="delete-confirm"`, "the confirmation step renders server-side")
	assert.Contains(t, page, "delete my library", "the phrase that has to be typed is shown")
	assert.Contains(t, page, `src="/assets/gallery/profile.js"`)
}

// The wording this ticket is emphatic about. An identity provider account
// survives deletion, so signing in again works — and produces a *new*, empty
// account rather than the one that was deleted. Saying only the first half
// would be worse than saying nothing.
func TestProfileDeleteSectionSaysSigningInAgainGivesAnEmptyAccount(t *testing.T) {
	page := newDeleteInstance(t).page(t)

	// Both halves, in both steps. The claim is not "signing in still works" —
	// on its own that reads as "nothing was deleted" — it is that signing in
	// works *and lands somewhere empty*.
	assert.Contains(t, page, "you will be treated as a new user")
	assert.Contains(t, page, "Signing in again will still work via your identity provider")
	assert.Contains(t, page, "You will arrive at a new account, with an empty library")
}

// A local account gets the opposite sentence, because it has no provider to
// be told about — the distinction av-qo05 drew and this section keeps.
func TestProfileDeleteSectionDistinguishesALocalAccount(t *testing.T) {
	ro, st := newLocalLoginRouter(t)
	ctx := context.Background()
	// The first account is the admin and cannot delete itself; the second is
	// the one this test is about.
	_, err := st.UpsertUser(ctx, "sub-admin", "admin@example.test")
	require.NoError(t, err)
	u, err := st.CreateLocalUser(ctx, store.NewLocalUser{
		ExternalID: "local:dana", Email: "dana", PasswordHash: "$2a$10$notarealhashbutnonempty00000000000000000000000000000"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req.AddCookie(sessionCookieFor(t, st, u.ID, "session-dana"))
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, "If you revisit this site, you will be treated as a new user")
	// And never the provider sentence: naming an identity provider to somebody
	// who signs in with a password here describes an account they do not have.
	assert.NotContains(t, page, "your identity provider")
}

// Shares are the sharp edge: a capability URL somebody else holds, revoked
// silently and with no way to tell them. The confirmation therefore has to
// carry the number, not a shrug.
func TestProfileDeleteSectionCountsLiveShares(t *testing.T) {
	in := newDeleteInstance(t)
	id := in.seedLibrary(t)
	require.Equal(t, http.StatusCreated,
		in.do(t, http.MethodPost, "/api/shares", map[string]any{"artifact_id": id}).Code)

	page := in.page(t)
	assert.Contains(t, page, "<strong>1 artifact</strong>")
	assert.Contains(t, page, "<strong>2 share links</strong>")

	// And again in the confirmation itself, which is the last thing read
	// before the phrase is typed — the count is not left behind on the step
	// somebody has already scrolled past.
	assert.Contains(t, page, "including 1 artifact and everything saved inside")
	assert.Contains(t, page, "2 share links will stop working for whoever is holding them")
}

// An account with no shares is not warned about links that will stop working;
// that sentence is noise on a library nobody has ever linked to.
//
// What it *is* still told is the inventory — "your tags, collections and share
// links" — which describes the operation rather than this account, and is true
// whether or not one was ever minted. The two are different sentences and only
// the second is conditional.
func TestProfileDeleteSectionOmitsSharesWhenThereAreNone(t *testing.T) {
	page := newDeleteInstance(t).page(t)

	assert.Contains(t, page, "<strong>no artifacts</strong>",
		"an empty library still says how much is about to go — as a word, since the sentence is read by a person")
	assert.NotContains(t, page, "Deleting it breaks")
	assert.NotContains(t, page, "will stop working")

	// The confirmation's clauses are each guarded by their own count, so an
	// empty library gets a sentence rather than one built around "your no
	// artifacts" — which is how a confirmation stops being read.
	assert.Contains(t, page, "Everything created or owned by you is permanently inaccessible.")
	assert.NotContains(t, page, "everything saved inside")
}

// The instance's only enabled admin cannot delete itself: the store refuses
// (ErrLastAdmin) and the page says so up front rather than letting the refusal
// arrive after someone typed a confirmation phrase.
func TestProfileDeleteControlIsBlockedForTheLastAdmin(t *testing.T) {
	in := newDeleteInstance(t)
	admin, err := in.st.GetUserByExternalID(context.Background(), "sub-admin")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req.AddCookie(sessionCookieFor(t, in.st, admin.ID, "session-admin"))
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, `id="delete-account" disabled`)
	assert.Contains(t, page, `aria-describedby="delete-account-reason"`,
		"the reason must be attached to the control, not merely printed near it")
	assert.Contains(t, page, "only account that can administer this instance")
	assert.NotContains(t, page, `id="delete-confirm"`,
		"no confirmation panel behind a control that cannot act")
}

// --- the route ---------------------------------------------------------

// The whole point of av-7jcq being a blocker: erasure means rows *and* bytes.
func TestDeleteAccountErasesTheLibraryAndItsBytes(t *testing.T) {
	in := newDeleteInstance(t)
	id := in.seedLibrary(t)

	ctx := context.Background()
	a, err := in.st.GetArtifact(ctx, in.member.ID, id)
	require.NoError(t, err)
	require.NotNil(t, a)
	body := filepath.Join(in.blobDir, a.SourceBlobID)
	widget := filepath.Join(in.blobDir, a.WidgetBlobID)
	require.FileExists(t, body)
	require.FileExists(t, widget)

	require.Equal(t, http.StatusNoContent, in.deleteAccount(t).Code)

	// Bytes.
	for _, p := range []string{body, widget} {
		_, err := os.Stat(p)
		assert.True(t, os.IsNotExist(err), "%s must be gone, got %v", p, err)
	}

	// Rows. Read back through the store as the deleted owner: everything it
	// held is either absent or unreachable, which for a deleted account are
	// the same statement.
	gone, err := in.st.GetArtifact(ctx, in.member.ID, id)
	require.NoError(t, err)
	assert.Nil(t, gone)

	tags, err := in.st.ListTags(ctx, in.member.ID)
	require.NoError(t, err)
	assert.Empty(t, tags)

	cols, err := in.st.ListCollections(ctx, in.member.ID)
	require.NoError(t, err)
	assert.Empty(t, cols)

	key, err := in.st.GetAgentKey(ctx, in.member.ID)
	require.NoError(t, err)
	assert.Nil(t, key, "the BYO provider key goes with the account")

	_, err = in.st.GetUser(ctx, in.member.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// Deleting revokes every share at once, silently, for people who have no
// account here. That is the right behaviour — the alternative is links into a
// library that no longer exists — and it is what the confirmation's share
// count exists to warn about.
func TestDeleteAccountRevokesEveryShare(t *testing.T) {
	in := newDeleteInstance(t)
	id := in.seedLibrary(t)
	w := in.do(t, http.MethodPost, "/api/shares", map[string]any{"artifact_id": id})
	require.Equal(t, http.StatusCreated, w.Code)
	var created createShareResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	share := created.Share
	require.NotNil(t, share)
	require.NotEmpty(t, share.ID)

	// Asked of the surface that answers a share URL, not of the store. The
	// claim is about the link a stranger is holding — served from
	// RENDER_ORIGIN with no credential of any kind — so serving it is what has
	// to change, and a store read would be one layer short of saying so. (It
	// would also be an un-owner-scoped read outside internal/render, which
	// owner_scope_test.go's tripwire refuses on purpose.)
	assert.Equal(t, http.StatusOK, in.share(t, share.ID).Code,
		"the link is live first, or its being dead afterwards would prove nothing")

	require.Equal(t, http.StatusNoContent, in.deleteAccount(t).Code)

	assert.Equal(t, http.StatusNotFound, in.share(t, share.ID).Code,
		"a share URL anyone was holding resolves to nothing")
}

// share fetches a share link the way whoever holds it would: from the render
// origin, carrying no credential.
func (in deleteInstance) share(t *testing.T, shareID string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	in.ro.RenderHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/s/"+shareID, nil))
	return w
}

// The one place a user's data lives outside their own library (av-q0ub):
// state they accumulated as a *viewer* of somebody else's artifact. Migration
// 014's AFTER DELETE trigger on users is what takes it, and this is the test
// that says so.
func TestDeleteAccountTakesStateWrittenOnAnotherOwnersArtifact(t *testing.T) {
	in := newDeleteInstance(t)
	ctx := context.Background()
	admin, err := in.st.GetUserByExternalID(ctx, "sub-admin")
	require.NoError(t, err)

	// The admin's artifact, with state rows from two different viewers on it.
	require.NoError(t, in.st.PutArtifact(ctx, &store.Artifact{
		ID: "admin-artifact", OwnerID: admin.ID, Title: "Shared",
		SourceBlobID: "admin-blob", Tier: store.Tier1,
	}))
	owner, artifact := store.OwnerID(admin.ID), "admin-artifact"
	require.NoError(t, in.st.SetState(ctx, owner, artifact, store.ViewerID(admin.ID), "k", "admin's"))
	require.NoError(t, in.st.SetState(ctx, owner, artifact, store.ViewerID(in.member.ID), "k", "member's"))

	require.Equal(t, http.StatusNoContent, in.deleteAccount(t).Code)

	theirs, err := in.st.GetState(ctx, owner, artifact, store.ViewerID(in.member.ID))
	require.NoError(t, err)
	assert.Empty(t, theirs, "the deleted viewer's rows go, even on somebody else's artifact")

	survived, err := in.st.GetState(ctx, owner, artifact, store.ViewerID(admin.ID))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"k": "admin's"}, survived,
		"and the artifact owner's own state is untouched")
}

// Nothing of another owner's is caught in the blast. The non-vacuity control
// for every emptiness assertion above.
func TestDeleteAccountLeavesTheOtherAccountAlone(t *testing.T) {
	in := newDeleteInstance(t)
	ctx := context.Background()
	admin, err := in.st.GetUserByExternalID(ctx, "sub-admin")
	require.NoError(t, err)
	require.NoError(t, in.st.PutArtifact(ctx, &store.Artifact{
		ID: "admin-artifact", OwnerID: admin.ID, Title: "Admin's",
		SourceBlobID: "admin-blob", Tier: store.Tier1,
	}))
	require.NoError(t, in.st.CreateTag(ctx, &store.Tag{ID: "admin-tag", OwnerID: admin.ID, Name: "theirs"}))
	require.NoError(t, in.cfg().Blob.Put(ctx, "admin-blob", strings.NewReader("<html>admin</html>")))

	in.seedLibrary(t)
	require.Equal(t, http.StatusNoContent, in.deleteAccount(t).Code)

	a, err := in.st.GetArtifact(ctx, admin.ID, "admin-artifact")
	require.NoError(t, err)
	assert.NotNil(t, a, "the other account's artifact survives")
	tags, err := in.st.ListTags(ctx, admin.ID)
	require.NoError(t, err)
	assert.Len(t, tags, 1)
	require.FileExists(t, filepath.Join(in.blobDir, "admin-blob"),
		"and so do its bytes — only the deleted owner's blobs were removed")
}

func (in deleteInstance) cfg() Config { return in.ro.cfg }

// A typed phrase, required by the server as well as by the page. The page's
// interlock is a courtesy to whoever is clicking; this is the one that holds
// for a client that never ran it.
func TestDeleteAccountRequiresTheTypedPhrase(t *testing.T) {
	in := newDeleteInstance(t)
	in.seedLibrary(t)

	for _, body := range []map[string]any{
		{},
		{"confirm": ""},
		{"confirm": "yes"},
		{"confirm": "Delete My Library"}, // the server compares exactly
	} {
		w := in.do(t, http.MethodDelete, "/api/account", body)
		assert.Equal(t, http.StatusBadRequest, w.Code, "confirm=%v", body["confirm"])
	}

	// And nothing was deleted along the way.
	u, err := in.st.GetUser(context.Background(), in.member.ID)
	require.NoError(t, err)
	assert.NotNil(t, u)
}

// The route acts on the signed-in account and on nothing else, which is why it
// takes no id. A credential that is not a person has no account to delete, and
// must not fall back to the single-user default owner's library.
func TestDeleteAccountNeedsASession(t *testing.T) {
	in := newDeleteInstance(t)
	in.seedLibrary(t)

	b, err := json.Marshal(map[string]any{"confirm": deleteAccountConfirmation})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodDelete, "/api/account", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, "the service token is not somebody's account")

	// No credential at all is refused before the handler is reached.
	req = httptest.NewRequest(http.MethodDelete, "/api/account", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// The instance must not be left with nobody able to administer it — the same
// refusal SetUserAdmin and SetUserDisabled give, applied to the stronger act.
func TestDeleteAccountRefusesTheLastEnabledAdmin(t *testing.T) {
	in := newDeleteInstance(t)
	admin, err := in.st.GetUserByExternalID(context.Background(), "sub-admin")
	require.NoError(t, err)
	adminCookie := sessionCookieFor(t, in.st, admin.ID, "session-admin")

	b, err := json.Marshal(map[string]any{"confirm": deleteAccountConfirmation})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodDelete, "/api/account", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie)
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "administer this instance")

	still, err := in.st.GetUser(context.Background(), admin.ID)
	require.NoError(t, err)
	assert.NotNil(t, still, "a refusal writes nothing")
}

// Sessions go with the account, and the browser is told to drop its cookie.
// A deletion a logged-in tab survives is not a deletion.
func TestDeleteAccountSignsTheBrowserOut(t *testing.T) {
	in := newDeleteInstance(t)
	w := in.deleteAccount(t)
	require.Equal(t, http.StatusNoContent, w.Code)

	cookie := cookiesFrom(w)[sessionCookieName]
	require.NotNil(t, cookie, "the session cookie must be cleared")
	assert.Equal(t, "", cookie.Value)
	assert.Less(t, cookie.MaxAge, 0)

	_, err := in.st.GetSession(context.Background(), "session-member")
	assert.ErrorIs(t, err, store.ErrNotFound, "the session row went with the users row")

	// And the cookie really is spent: the same request again is unauthenticated.
	again := in.do(t, http.MethodGet, "/api/artifacts", nil)
	assert.Equal(t, http.StatusUnauthorized, again.Code)
}

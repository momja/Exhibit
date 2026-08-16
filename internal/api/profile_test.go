package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-qo05. /profile is a person's own account, and its whole design is a pair
// of claims about authority: a session is enough for it, and a session is all
// it ever acts on. The tests below are those two, plus the one genuinely
// fiddly thing on the page — the display name, which cannot use admin.go's
// rule because a blank cell in a table is cosmetic and a blank section is the
// page.

// profileInstance is an instance with a login and one signed-in user.
type profileInstance struct {
	ro     *Router
	st     store.Store
	user   *store.User
	cookie *http.Cookie
}

// newProfileInstance seeds a user through the store directly — this page reads
// the row and does not care how it got there, and going through UpsertUser
// lets each test choose the email (including none) that its case is about.
func newProfileInstance(t *testing.T, externalID, email string) profileInstance {
	t.Helper()
	ro, st := newIdentityTestRouter(t, &stubProvider{})
	u, err := st.UpsertUser(context.Background(), externalID, email)
	require.NoError(t, err)
	return profileInstance{
		ro: ro, st: st, user: u,
		cookie: sessionCookieFor(t, st, u.ID, "session-"+externalID),
	}
}

func (in profileInstance) page(t *testing.T) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req.AddCookie(in.cookie)
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return w.Body.String()
}

// The ordinary case: an identity that came with an email.
func TestProfileNamesTheSignedInAccount(t *testing.T) {
	in := newProfileInstance(t, "sub-1", "person@example.test")
	page := in.page(t)

	assert.Contains(t, page, "person@example.test")
	assert.Contains(t, page, "Identity provider",
		"an account with no password here signs in through the provider, and the page says which")
	assert.NotContains(t, page, "subject identifier",
		"the fallback note belongs only to a name that actually came from external_id")
}

// A local account: the login name is in both columns, and the sign-in line
// says password rather than provider.
func TestProfileNamesALocalAccount(t *testing.T) {
	ro, st := newLocalLoginRouter(t)
	hash, err := auth.HashPassword("a-long-enough-password")
	require.NoError(t, err)
	u, err := st.CreateLocalUser(context.Background(), store.NewLocalUser{
		ExternalID: auth.LocalExternalID("dana"), Email: "dana", PasswordHash: hash,
	})
	require.NoError(t, err)
	in := profileInstance{ro: ro, st: st, user: u,
		cookie: sessionCookieFor(t, st, u.ID, "session-dana")}

	page := in.page(t)
	assert.Contains(t, page, "dana")
	assert.Contains(t, page, "Password")
	assert.NotContains(t, page, "Identity provider owns this sign-in")
	// Nor does the danger zone tell someone with a password here that their
	// identity provider is unaffected — they do not have one.
	assert.NotContains(t, page, "your identity provider is theirs")
	assert.Contains(t, page, "Your login name and password are this instance")
}

// The correction this ticket was reopened for. admin.go's rule is `u.Email`
// and nothing else, and users.email is NOT NULL and defaults to the empty
// string — a portable second key, not something a provider guarantees. In a
// table an empty one is a blank cell; here it is the entire Account section,
// so the page falls back to the subject the provider does send, and labels it
// as such.
func TestProfileFallsBackToTheProviderSubjectWhenThereIsNoEmail(t *testing.T) {
	in := newProfileInstance(t, "sub-no-email-9f3c", "")
	page := in.page(t)

	assert.Contains(t, page, "sub-no-email-9f3c",
		"with no email the page must still name the account — this section IS the name")
	assert.Contains(t, page, "subject identifier",
		"an opaque identifier offered as 'you' reads as a bug; the same one labelled reads as an answer")
}

// Both columns empty is not reachable through UpsertUser (external_id is the
// key it upserts on), so the rule is exercised directly. It must still answer
// something a person can read.
func TestProfileNameIsNeverBlank(t *testing.T) {
	name, isSubject := profileName(&store.User{Email: "a@b.test", ExternalID: "sub-1"})
	assert.Equal(t, "a@b.test", name)
	assert.False(t, isSubject, "an email is a name, not a fallback")

	name, isSubject = profileName(&store.User{ExternalID: "sub-1"})
	assert.Equal(t, "sub-1", name)
	assert.True(t, isSubject)

	name, _ = profileName(&store.User{})
	assert.Empty(t, name, "with neither column the template states the sign-in route instead")
}

// The delete section is this ticket's; the deletion is av-4wyq's and is
// blocked on av-7jcq (Blob.Store has no Delete). Until then the control is
// present, disabled, and says why — a button that removed the rows and left
// the artifact files on disk would be worse than one that does nothing.
func TestProfileDeleteControlIsDisabledWithItsReasonVisible(t *testing.T) {
	page := newProfileInstance(t, "sub-1", "person@example.test").page(t)

	assert.Contains(t, page, `id="delete-account"`)
	assert.Contains(t, page, "disabled")
	assert.Contains(t, page, `aria-describedby="delete-account-reason"`,
		"the reason must be attached to the control, not merely printed near it")
	assert.Contains(t, page, "Not available yet")
	assert.Contains(t, page, "permanent")
	assert.Contains(t, page, "identity provider",
		"the section must say that deleting here does not delete the identity at the provider")

	// Nothing on the page can start a deletion, by any route.
	assert.NotContains(t, page, "DELETE")
	assert.NotContains(t, page, "/api/account")
}

// A session is the whole authorization: no admin role, and no session means no
// page. The first half is what distinguishes this route from /admin/users,
// which answers 404 to exactly the user this one must serve.
func TestProfileNeedsASessionAndNothingMore(t *testing.T) {
	// The first user on an instance is its admin, so the account this test is
	// about is the second one — an ordinary member, which is exactly who
	// /admin/users refuses and /profile must serve.
	in := newProfileInstance(t, "sub-first", "first@example.test")
	require.True(t, in.user.IsAdmin, "the first account is the instance's admin")

	member, err := in.st.UpsertUser(context.Background(), "sub-second", "second@example.test")
	require.NoError(t, err)
	require.False(t, member.IsAdmin)
	memberCookie := sessionCookieFor(t, in.st, member.ID, "session-second")

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req.AddCookie(memberCookie)
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "a member with no admin role owns their own account page")
	assert.Contains(t, w.Body.String(), "second@example.test",
		"and it is their account, resolved from the session rather than from the URL")

	// The admin surface, for contrast: same cookie, same page group, 404.
	req = httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	req.AddCookie(memberCookie)
	w = httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// No cookie at all: sessionGate sends them to the login page first, with
	// the destination preserved.
	w = httptest.NewRecorder()
	in.ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/profile", nil))
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "/auth/login")
	assert.Contains(t, w.Header().Get("Location"), "next=%2Fprofile")
}

// An instance with no login configured has no users row behind the single-user
// default owner. That is the self-hoster's default shape, not a missing
// record, so the page says so rather than 500ing or inventing an account.
func TestProfileOnAnInstanceWithNoLogin(t *testing.T) {
	page := getPage(t, newTestRouter(t), "/profile")
	assert.Contains(t, page, "no login configured")
	assert.NotContains(t, page, "Signed in as")
}

// The header entry point. Both controls are icon-only and must therefore
// carry an accessible name, since nothing on screen says one; the admin link
// keeps its label because a third anonymous glyph would say nothing.
func TestGalleryHeaderCarriesBothIconControls(t *testing.T) {
	page := getPage(t, newTestRouter(t), "/")

	assert.Contains(t, page, `href="/profile" aria-label="Your account" title="Your account"`)
	assert.Contains(t, page, `href="/new" aria-label="Add artifact" title="Add artifact"`)
	assert.Contains(t, page, "ph-user-circle")
	assert.Contains(t, page, `class="btn btn-sec" href="/admin/users"`,
		"the admin link is unchanged, label included")

	// Sized as peers: one class, applied to both, so they cannot drift apart.
	assert.Equal(t, 2, strings.Count(page, "icon-btn"),
		"both controls wear the same size class, so they cannot drift apart")
	assert.Contains(t, galleryAsset(t, newTestRouter(t), "/assets/gallery/components.css"), ".icon-btn{")
}

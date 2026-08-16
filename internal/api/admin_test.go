package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-utap. Once Exhibit issues its own credentials it *is* the user directory,
// so somebody has to create accounts, reset forgotten passwords (by hand, which
// is what keeps SMTP out of the product entirely) and switch an account off.
//
// The whole risk of the feature is the authority boundary it sits on. av-g2dx
// is a person acting on their own account and needs nothing but a session;
// this is an admin acting on the instance and needs strictly more. The two will
// share page furniture, so the thing these tests defend is that they never
// share authority: get it wrong and any account can reset the admin's password,
// which is worse than not shipping the feature at all.
//
// Three properties, each with a test below:
//
//  1. A non-admin reaches none of it, and the refusal says nothing about
//     whether the account they aimed at exists.
//  2. Disabling ends the sessions that already exist — not merely the next
//     login. av-30rj made sessions server-side rows precisely so that is
//     possible, and a disable a logged-in browser survives is not a disable.
//  3. The instance cannot be locked out of itself: the last admin who can
//     still sign in is neither demotable nor disable-able.

const (
	adminName  = "boss"
	memberName = "member"
	goodPass   = "a-long-enough-passphrase"
)

// adminInstance is an instance with a login, an admin, and an ordinary member —
// each with a live session. Two accounts is the whole point: with one, every
// authority bug is invisible.
type adminInstance struct {
	ro     *Router
	st     store.Store
	admin  *store.User
	member *store.User
	// Real cookies from real logins, not hand-written rows: the sessions have
	// to be the same kind of thing a disable is later expected to destroy.
	adminCookie  *http.Cookie
	memberCookie *http.Cookie
}

func newAdminInstance(t *testing.T) adminInstance {
	t.Helper()
	// A local credential configured for a *third* name, so the environment
	// break-glass pair is present (it arms the gate the way a real instance
	// does) without being either account under test.
	ro, st := newLoginTestRouter(t, nil, newTestCredential(t, "breakglass", goodPass))
	ctx := context.Background()

	admin, err := st.CreateLocalUser(ctx, store.NewLocalUser{ExternalID: auth.LocalExternalID(adminName), Email: adminName, PasswordHash: testHash(t, goodPass)})
	require.NoError(t, err)
	require.True(t, admin.IsAdmin, "the first account on an instance is its admin (av-rzvf)")

	member, err := st.CreateLocalUser(ctx, store.NewLocalUser{ExternalID: auth.LocalExternalID(memberName), Email: memberName, PasswordHash: testHash(t, goodPass)})
	require.NoError(t, err)
	require.False(t, member.IsAdmin)

	return adminInstance{
		ro: ro, st: st, admin: admin, member: member,
		adminCookie:  loginAs(t, ro, adminName, goodPass),
		memberCookie: loginAs(t, ro, memberName, goodPass),
	}
}

// do issues one request, optionally with a session cookie and a JSON body.
func (in adminInstance) do(t *testing.T, method, path string, c *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		r = httptest.NewRequest(method, path, bytes.NewReader(encoded))
		r.Header.Set("Content-Type", "application/json")
	}
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, r)
	return w
}

func userPath(id int64) string { return "/api/admin/users/" + strconv.FormatInt(id, 10) }

// reload reads an account back from the store, so an assertion is about what
// was written rather than about what a handler said it wrote.
func (in adminInstance) reload(t *testing.T, id int64) *store.User {
	t.Helper()
	u, err := in.st.GetUser(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, u)
	return u
}

// --- 1. the authority boundary -----------------------------------------

// The ticket, stated as the attack it prevents: an ordinary account resetting
// the admin's password. Every route that reaches another account is checked,
// with a real session belonging to a real non-admin user — the credential that
// would be sufficient for av-g2dx's own-account surface and must not be
// sufficient here.
func TestNonAdminReachesNoAdminRoute(t *testing.T) {
	in := newAdminInstance(t)

	for _, c := range []struct {
		what   string
		method string
		path   string
		body   any
	}{
		{"the account list page", http.MethodGet, "/admin/users", nil},
		{"the account list API", http.MethodGet, "/api/admin/users", nil},
		{"creating an account", http.MethodPost, "/api/admin/users",
			map[string]any{"username": "smuggled", "password": goodPass}},
		{"resetting the admin's password", http.MethodPatch, userPath(in.admin.ID),
			map[string]any{"password": "attacker-chosen-passphrase"}},
		{"disabling the admin", http.MethodPatch, userPath(in.admin.ID),
			map[string]any{"disabled": true}},
		{"promoting themselves", http.MethodPatch, userPath(in.member.ID),
			map[string]any{"is_admin": true}},
	} {
		t.Run(c.what, func(t *testing.T) {
			w := in.do(t, c.method, c.path, in.memberCookie, c.body)
			assert.Equal(t, http.StatusNotFound, w.Code,
				"%s must be out of a non-admin's reach — a session is authorization for "+
					"your own account (av-g2dx), never for somebody else's (av-utap)", c.what)
		})
	}

	// Nothing was written by any of it.
	assert.True(t, in.reload(t, in.admin.ID).IsAdmin)
	assert.False(t, in.reload(t, in.admin.ID).Disabled)
	assert.False(t, in.reload(t, in.member.ID).IsAdmin)
	_, _, err := in.st.LookupLocalCredential(context.Background(), auth.LocalExternalID("smuggled"))
	assert.ErrorIs(t, err, store.ErrNotFound, "a refused create must not have created anything")

	// And the admin's password is untouched, proved the only way that matters:
	// they can still sign in with it.
	require.NotNil(t, loginAs(t, in.ro, adminName, goodPass))
}

// The refusal must not double as an account oracle. A non-admin probing ids
// learns who exists if "you may not" and "there is no such user" differ by so
// much as a status code — and the id space is small integers, so probing it is
// trivial.
func TestAdminRefusalDoesNotRevealWhetherTheAccountExists(t *testing.T) {
	in := newAdminInstance(t)
	const missingID = 9999
	require.NotEqual(t, int64(missingID), in.admin.ID)

	body := map[string]any{"password": "some-long-enough-passphrase"}
	existing := in.do(t, http.MethodPatch, userPath(in.admin.ID), in.memberCookie, body)
	absent := in.do(t, http.MethodPatch, userPath(missingID), in.memberCookie, body)

	assert.Equal(t, existing.Code, absent.Code)
	assert.Equal(t, existing.Body.String(), absent.Body.String(),
		"a non-admin must get the same answer for an account that exists and one that does not — "+
			"otherwise the refusal enumerates the instance's user directory (av-utap)")

	// An *admin* acting on a missing id gets that same 404 too, which is what
	// keeps the two indistinguishable from outside.
	assert.Equal(t, http.StatusNotFound,
		in.do(t, http.MethodPatch, userPath(missingID), in.adminCookie, body).Code)
}

// The credentials that are not a person, at the one function that decides it.
// An agent session is steered by text Exhibit did not author (av-e0yj) and a
// public visitor presented nothing at all; neither is an admin, and neither
// becomes one by being let through some earlier gate.
//
// adminRequest reads the Principal its request's own gate already resolved
// rather than re-parsing the request itself (av-o5cf), so what each case here
// sets on context is exactly the Principal authMiddleware/sessionGate would
// have produced — including the control case, which constructs the ordinary
// session Principal rather than relying on adminRequest to notice the cookie
// still attached to r. The cookie is left on r throughout regardless, and
// deliberately unused by the agent/public cases, to show it carries no
// authority once the context says otherwise.
func TestNeitherAgentSessionsNorPublicVisitorsAreAdmins(t *testing.T) {
	in := newAdminInstance(t)
	r := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	r.AddCookie(in.adminCookie) // an admin's own cookie, deliberately

	grant := &agentscope.Grant{}
	withAgent := r.WithContext(withPrincipal(r.Context(),
		Principal{OwnerID: in.admin.ID, Kind: PrincipalAgentGrant, Grant: grant}))
	assert.False(t, in.ro.adminRequest(withAgent),
		"an agent session credential must never carry admin authority, whatever else the request holds")

	withPublic := r.WithContext(withPrincipal(r.Context(), Principal{Kind: PrincipalPublic, ReadOnly: true}))
	assert.False(t, in.ro.adminRequest(withPublic),
		"publishing a library says nothing about who administers the instance (av-wmp6)")

	// The control: the same cookie, resolved as an ordinary session Principal
	// (as sessionGate/authMiddleware would have done), is an admin.
	withSession := r.WithContext(withPrincipal(r.Context(), Principal{OwnerID: in.admin.ID, Kind: PrincipalSession}))
	assert.True(t, in.ro.adminRequest(withSession))

	// A PrincipalKind value nothing in this package issues must not fall
	// through to PrincipalNone's !loginEnabled() answer — that would grant
	// admin on a fully open instance to a value nobody chose.
	withUnknown := r.WithContext(withPrincipal(r.Context(), Principal{OwnerID: in.admin.ID, Kind: PrincipalKind(99)}))
	assert.False(t, in.ro.adminRequest(withUnknown))
}

// The operator's static token is admin, because it is already full authority
// over every route in the API — and because refusing it would leave an operator
// on a fresh instance unable to create the first account from anything but the
// CLI, while changing nothing about what they can actually reach.
func TestServiceTokenActsAsAdmin(t *testing.T) {
	in := newAdminInstance(t)

	r := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	r.Header.Set("Authorization", "Bearer "+in.ro.cfg.AuthToken)
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// A wrong token is nobody: it is not a session either, so it falls all the
	// way through to the API group's own 401.
	bad := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	bad.Header.Set("Authorization", "Bearer not-the-token")
	w = httptest.NewRecorder()
	in.ro.ServeHTTP(w, bad)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// An admin demoted or disabled mid-session stops being an admin on their very
// next request. The check reads the account per request rather than trusting
// something stamped into the session at login, which is the same property that
// makes logout immediate.
func TestDemotionTakesEffectOnTheNextRequest(t *testing.T) {
	in := newAdminInstance(t)
	require.Equal(t, http.StatusOK, in.do(t, http.MethodGet, "/api/admin/users", in.adminCookie, nil).Code)

	// Promote the member first, so demoting the admin is not the last-admin
	// refusal instead of the thing under test.
	require.Equal(t, http.StatusOK,
		in.do(t, http.MethodPatch, userPath(in.member.ID), in.adminCookie, map[string]any{"is_admin": true}).Code)
	require.NoError(t, in.st.SetUserAdmin(context.Background(), in.admin.ID, false))

	assert.Equal(t, http.StatusNotFound,
		in.do(t, http.MethodGet, "/api/admin/users", in.adminCookie, nil).Code,
		"the session is still valid, but the account behind it is no longer an admin")
}

// --- 2. disabling ------------------------------------------------------

// The acceptance criterion, using a live cookie after the disable.
//
// Refusing the *next* login would have been the easy half and is not what was
// asked for: the credential a disabled person is actually holding is the
// session already in their browser, and a `sessions` row outlives any decision
// about the password that minted it.
func TestDisablingTerminatesLiveSessions(t *testing.T) {
	in := newAdminInstance(t)

	// Non-vacuity: the cookie works before the disable, on a page and on the API.
	require.Equal(t, http.StatusOK, in.do(t, http.MethodGet, "/", in.memberCookie, nil).Code)
	require.Equal(t, http.StatusOK, in.do(t, http.MethodGet, "/api/artifacts", in.memberCookie, nil).Code)

	w := in.do(t, http.MethodPatch, userPath(in.member.ID), in.adminCookie, map[string]any{"disabled": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, in.reload(t, in.member.ID).Disabled)

	// The same cookie, immediately afterwards.
	page := in.do(t, http.MethodGet, "/", in.memberCookie, nil)
	assert.Equal(t, http.StatusFound, page.Code,
		"a disabled account's live session must stop working at once — av-30rj made sessions "+
			"server-side rows precisely so a disable can delete them (av-utap)")
	assert.Contains(t, page.Header().Get("Location"), "/auth/login")
	assert.Equal(t, http.StatusUnauthorized,
		in.do(t, http.MethodGet, "/api/artifacts", in.memberCookie, nil).Code)

	// And the next login is refused too — with the message a wrong password
	// gets, so being switched off is not something an outsider can detect.
	refused := submitLogin(t, in.ro, memberName, goodPass, "")
	assert.Equal(t, http.StatusUnauthorized, refused.Code)
	assert.Contains(t, refused.Body.String(), "don&#39;t match",
		"a disabled account is refused in the same words as a wrong password")

	// Re-enabling restores access with the password they already had; nothing
	// was destroyed, which is the difference between disabling and deleting.
	require.Equal(t, http.StatusOK,
		in.do(t, http.MethodPatch, userPath(in.member.ID), in.adminCookie, map[string]any{"disabled": false}).Code)
	require.NotNil(t, loginAs(t, in.ro, memberName, goodPass))
}

// The environment break-glass credential is the one path that deliberately
// ignores the users table — "no state in the database can shadow it" is its
// whole value. A disable is the single exception, because a disable a
// documented environment variable defeats is not a disable. The break-glass
// role survives intact: the last enabled admin can never be disabled, so there
// is always an account for LOGIN_USERNAME to name.
func TestDisablingBeatsTheEnvironmentCredential(t *testing.T) {
	ro, st := newLoginTestRouter(t, nil, newTestCredential(t, memberName, goodPass))
	ctx := context.Background()

	_, err := st.CreateLocalUser(ctx, store.NewLocalUser{ExternalID: auth.LocalExternalID(adminName), Email: adminName, PasswordHash: testHash(t, goodPass)})
	require.NoError(t, err)
	member, err := st.CreateLocalUser(ctx, store.NewLocalUser{ExternalID: auth.LocalExternalID(memberName), Email: memberName, PasswordHash: testHash(t, goodPass)})
	require.NoError(t, err)

	// The env pair names this account and always accepts its password.
	require.Equal(t, http.StatusFound, submitLogin(t, ro, memberName, goodPass, "").Code)

	require.NoError(t, st.SetUserDisabled(ctx, member.ID, true))
	assert.Equal(t, http.StatusUnauthorized, submitLogin(t, ro, memberName, goodPass, "").Code,
		"LOGIN_USERNAME/LOGIN_PASSWORD_HASH must not re-admit an account an admin has switched off")
}

// The OIDC half of the same rule. An identity a provider issued has no password
// to remove, which is exactly why disabling had to be a column rather than the
// clearing of `password_hash` (migration 017) — and why the refusal has to sit
// at the callback, the one point a provider identity becomes a session.
func TestDisablingRefusesAProviderIdentityToo(t *testing.T) {
	idp := &stubProvider{identities: []auth.Identity{{ExternalID: "sub-1", Email: "a@example.test"}}}
	ro, st := newIdentityTestRouter(t, idp)

	ctx := context.Background()
	// Somebody else has to be able to administer the instance first, or the
	// disable below is refused for the *other* correct reason.
	other, err := st.UpsertUser(ctx, "sub-other", "other@example.test")
	require.NoError(t, err)
	require.NoError(t, st.SetUserAdmin(ctx, other.ID, true))

	require.NotNil(t, runLogin(t, ro)) // the provider's first login creates the row
	user, err := st.GetUserByExternalID(ctx, "sub-1")
	require.NoError(t, err)
	require.False(t, user.HasPassword, "an OIDC identity has no hash to remove — the point of the column")

	require.NoError(t, st.SetUserDisabled(ctx, user.ID, true))

	// Walk the callback again; the provider still vouches for them, and this
	// instance still refuses.
	state := "the-state"
	cb := httptest.NewRequest("GET", "/auth/callback?code=c&state="+url.QueryEscape(state), nil)
	cb.AddCookie(&http.Cookie{Name: stateCookieName, Value: state})
	cb.AddCookie(&http.Cookie{Name: verifierCookieName, Value: "the-verifier"})
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, cb)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Nil(t, cookiesFrom(w)[sessionCookieName],
		"no session may be landed for a disabled identity")
}

// --- 3. the instance cannot lock itself out ----------------------------

func TestTheLastAdminCannotBeDemotedOrDisabled(t *testing.T) {
	in := newAdminInstance(t)

	for _, change := range []map[string]any{{"is_admin": false}, {"disabled": true}} {
		w := in.do(t, http.MethodPatch, userPath(in.admin.ID), in.adminCookie, change)
		assert.Equal(t, http.StatusConflict, w.Code,
			"%v on the only admin must be refused — an instance with nobody who can create an "+
				"account or reset a password is not recoverable from the UI (av-utap)", change)
		assert.Contains(t, w.Body.String(), "last admin")
	}
	assert.True(t, in.reload(t, in.admin.ID).IsAdmin)
	assert.False(t, in.reload(t, in.admin.ID).Disabled)

	// With a second admin the same changes go through, so the guard is a
	// guard and not a permanent prohibition.
	require.Equal(t, http.StatusOK,
		in.do(t, http.MethodPatch, userPath(in.member.ID), in.adminCookie, map[string]any{"is_admin": true}).Code)
	require.Equal(t, http.StatusOK,
		in.do(t, http.MethodPatch, userPath(in.admin.ID), in.adminCookie, map[string]any{"disabled": true}).Code)
	assert.True(t, in.reload(t, in.admin.ID).Disabled)
}

// A *disabled* admin is not a way back in, so it must not satisfy the
// invariant. Otherwise disabling one admin and then demoting the other leaves
// an instance whose only admin cannot sign in — locked out by two changes each
// of which looked legal on its own.
func TestADisabledAdminDoesNotCountAsTheRemainingAdmin(t *testing.T) {
	in := newAdminInstance(t)
	ctx := context.Background()

	require.NoError(t, in.st.SetUserAdmin(ctx, in.member.ID, true))
	require.NoError(t, in.st.SetUserDisabled(ctx, in.member.ID, true))

	assert.ErrorIs(t, in.st.SetUserDisabled(ctx, in.admin.ID, true), store.ErrLastAdmin,
		"the other admin is switched off, so this one is the last who can still sign in")
	assert.ErrorIs(t, in.st.SetUserAdmin(ctx, in.admin.ID, false), store.ErrLastAdmin)
	assert.False(t, in.reload(t, in.admin.ID).Disabled, "a refusal must write nothing")
}

// --- the admin's own capabilities --------------------------------------

func TestAdminCreatesAnAccountAndResetsItsPassword(t *testing.T) {
	in := newAdminInstance(t)
	const newName, newPass, resetPass = "newcomer", "first-passphrase-here", "second-passphrase-here"

	w := in.do(t, http.MethodPost, "/api/admin/users", in.adminCookie,
		map[string]any{"username": strings.ToUpper(newName), "password": newPass})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var created adminUserView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, newName, created.Name, "login names are normalized in one place (auth.NormalizeLoginName)")
	assert.Equal(t, "local", created.Kind)
	assert.False(t, created.IsAdmin)
	assert.NotContains(t, w.Body.String(), newPass, "a password must not come back in a response")

	// The account is real: it signs in.
	require.NotNil(t, loginAs(t, in.ro, newName, newPass))

	// The reset — by an admin, not by mail, which is the choice that keeps SMTP
	// out of the product.
	require.Equal(t, http.StatusOK,
		in.do(t, http.MethodPatch, userPath(created.ID), in.adminCookie,
			map[string]any{"password": resetPass}).Code)
	assert.Equal(t, http.StatusUnauthorized, submitLogin(t, in.ro, newName, newPass, "").Code)
	require.NotNil(t, loginAs(t, in.ro, newName, resetPass))

	// Two accounts cannot share a login name — the schema invariant, surfaced.
	dup := in.do(t, http.MethodPost, "/api/admin/users", in.adminCookie,
		map[string]any{"username": newName, "password": goodPass})
	assert.Equal(t, http.StatusConflict, dup.Code)

	// And a password floor, so an admin cannot provision an account nobody has
	// to guess at.
	short := in.do(t, http.MethodPost, "/api/admin/users", in.adminCookie,
		map[string]any{"username": "shorty", "password": "abc"})
	assert.Equal(t, http.StatusBadRequest, short.Code)
}

// An admin setting somebody's password is not a guess at one, so it must not be
// charged to the throttle av-t21v put on /auth/local. Routing the reset through
// the login endpoint would have made an admin fixing several accounts in a row
// throttle themselves out of the instance.
func TestAdminPasswordResetDoesNotSpendTheLoginThrottle(t *testing.T) {
	in := newAdminInstance(t)

	before := in.ro.logins.ip.size() + in.ro.logins.user.size()
	for i := 0; i < 6; i++ {
		require.Equal(t, http.StatusOK,
			in.do(t, http.MethodPatch, userPath(in.member.ID), in.adminCookie,
				map[string]any{"password": "passphrase-number-" + strconv.Itoa(i)}).Code)
	}
	assert.Equal(t, before, in.ro.logins.ip.size()+in.ro.logins.user.size(),
		"an admin reset must not touch the login rate limiter (av-t21v) — it asserts a "+
			"credential rather than guessing one")

	// And the member can still sign in with the last one set, immediately.
	require.NotNil(t, loginAs(t, in.ro, memberName, "passphrase-number-5"))
}

// The page itself: an admin sees the directory, a member gets the same styled
// 404 an unrouted path gets, and the gallery offers the link to exactly one of
// them.
func TestAdminPageIsVisibleOnlyToAdmins(t *testing.T) {
	in := newAdminInstance(t)

	page := in.do(t, http.MethodGet, "/admin/users", in.adminCookie, nil)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), memberName)
	assert.Equal(t, "no-store", page.Header().Get("Cache-Control"),
		"the page lists every account on the instance; no shared cache may hold it")

	refused := in.do(t, http.MethodGet, "/admin/users", in.memberCookie, nil)
	assert.Equal(t, http.StatusNotFound, refused.Code)
	assert.NotContains(t, refused.Body.String(), memberName,
		"the refusal must not leak the directory it refused")

	assert.Contains(t, in.do(t, http.MethodGet, "/", in.adminCookie, nil).Body.String(), "/admin/users")
	assert.NotContains(t, in.do(t, http.MethodGet, "/", in.memberCookie, nil).Body.String(), "/admin/users")
}

// The self-hoster's instance, which this feature must not disturb. With no
// login configured there is one user, who is the operator holding the token,
// and no notion of anybody else to be — so the surface is theirs, exactly as
// every other page on such an instance already is.
func TestSingleUserInstanceStillReachesTheAdminSurface(t *testing.T) {
	ro := newTestRouter(t)

	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/users", nil))
	assert.Equal(t, http.StatusOK, w.Code, "a single-user instance has no gate to fail (av-utap)")

	r := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	r.Header.Set("Authorization", "Bearer "+ro.cfg.AuthToken)
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

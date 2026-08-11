// Administration of *other people's* accounts (av-utap).
//
// The boundary this file draws is the whole point of the ticket, and it is a
// boundary between two epics that will share page furniture:
//
//   - **av-g2dx** is a person acting on their own account — their password,
//     their settings, their sessions. A session is the whole authorization.
//   - **av-utap (here)** is an admin acting on the instance — creating someone
//     else's account, resetting someone else's password, switching someone
//     else off. A session is *not* sufficient authorization for any of it.
//
// So every route that reaches another account passes adminOnly, and none of
// them shares a handler with a route that does not. Getting this wrong in the
// obvious way — hanging an admin control off the settings page that av-qwld
// will build, guarded by nothing but being logged in — lets any account reset
// the admin's password, which is strictly worse than not shipping the feature.
//
// Passwords are reset by an admin rather than mailed, which is what keeps SMTP
// out of the product entirely (spec §3.2 of av-sz4e; Immich makes the same
// trade). There is no reset link, no verification mail, and nothing here sends
// one.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
)

// minPasswordLength is the one rule imposed on an admin-set password. It is a
// length floor and nothing else — no character classes, no rotation — because
// those are the rules that produce `Password1!` and a sticky note, and because
// the account is provisioned by an admin who is already trusted with the whole
// instance.
const minPasswordLength = 8

// --- authority ---------------------------------------------------------

// adminOnly is the guard. It answers **404**, not 403, and the same 404
// whatever was asked for.
//
// Two properties come out of that, and both are requirements rather than
// preferences. To a non-admin the administration surface simply does not exist,
// so nothing here advertises that an admin screen is worth attacking. And
// because the check runs *before* any handler looks at the target, a refusal
// cannot differ between "you may not touch user 7" and "there is no user 7" —
// the response is byte-identical, so it reveals nothing about who has an
// account on this instance. An admin acting on an id that does not exist gets
// the same 404 from the handler, which is what keeps the two indistinguishable
// from outside.
func (ro *Router) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ro.adminRequest(r) {
			slog.WarnContext(r.Context(), "admin surface refused",
				slog.String("method", r.Method), slog.String("path", r.URL.Path))
			// ro.notFound renders the styled page for a page route and keeps
			// /api/* on the plain error its JSON clients expect.
			ro.notFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// adminRequest reports whether this request may act on the instance's
// accounts, purely as a function of the Principal the request's own gate
// (authMiddleware or sessionGate) already resolved — it does not re-parse the
// request for a session cookie or a bearer token itself (av-o5cf; it used to,
// which is what let TestNeitherAgentSessionsNorPublicVisitorsAreAdmins matter:
// a request could carry a real admin cookie and still be refused, because
// context — not the cookie — is what this function has always answered from).
//
// The cases, one per PrincipalKind:
//
//  1. **An agent session credential is never an admin.** It is steered by text
//     Exhibit did not author (av-e0yj). agentScopeAllows already refuses every
//     path outside its one artifact, so this is belt and braces; belt and
//     braces is the correct amount for a check whose failure hands a prompt
//     injection the user table.
//  2. **An anonymous visitor on a public instance is never an admin** (av-wmp6).
//     Publishing a library says nothing about who administers it.
//  3. **The static service token is.** It is the operator's own credential and
//     already carries full authority over every route in the API; refusing it
//     the admin routes would mean an operator could not create the first
//     account from anything but the CLI, while changing nothing about what they
//     can reach.
//  4. **A session, if the account behind it is an enabled admin.** The lookup
//     is per-request rather than baked into the session at login, so an admin
//     demoted or disabled while logged in stops being one on their next
//     request — the same property that makes logout immediate.
//  5. **PrincipalNone: only an instance with no login at all.** This is a
//     property of the instance (ro.loginEnabled()), not of the Principal —
//     PrincipalNone means "no credential resolved," which happens both for a
//     fully open instance and, in principle, nowhere else, so the fallback
//     stays an explicit instance check rather than a fifth Kind. Such an
//     instance has one user, who is the operator holding the token, and no
//     notion of anyone else to be. It is the single-user default, where every
//     page is already served to whoever can reach the origin; this changes
//     nothing about that, and the API mutations behind the page still require
//     the token.
func (ro *Router) adminRequest(r *http.Request) bool {
	ctx := r.Context()
	p := principalFromCtx(ctx)
	switch p.Kind {
	case PrincipalAgentGrant, PrincipalPublic:
		return false
	case PrincipalServiceToken:
		return true
	case PrincipalSession:
		u, err := ro.cfg.Store.GetUser(ctx, p.OwnerID)
		if err != nil || u == nil {
			return false
		}
		return u.IsAdmin && !u.Disabled
	default: // PrincipalNone
		return !ro.loginEnabled()
	}
}

// --- the JSON API ------------------------------------------------------

// adminUserView is one account as the admin screen sees it. The stored hash is
// not a field on store.User at all, so there is nothing to remember to strip;
// what travels is the shape of the account, never the credential.
type adminUserView struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	// Kind is "local" or "sso" — which columns are populated, said in the
	// word the UI uses. It is what tells an admin that "reset password" is
	// meaningless for a row their identity provider owns.
	Kind      string    `json:"kind"`
	IsAdmin   bool      `json:"is_admin"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
}

func newAdminUserView(u *store.User) adminUserView {
	kind := "sso"
	if u.HasPassword {
		kind = "local"
	}
	return adminUserView{
		ID: u.ID, Name: u.Email, Email: u.Email, Kind: kind,
		IsAdmin: u.IsAdmin, Disabled: u.Disabled, CreatedAt: u.CreatedAt,
	}
}

func (ro *Router) listAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := ro.cfg.Store.ListUsers(r.Context())
	if err != nil {
		serverError(w, r, "admin list users", err)
		return
	}
	out := make([]adminUserView, 0, len(users))
	for _, u := range users {
		out = append(out, newAdminUserView(u))
	}
	writeJSON(w, http.StatusOK, out)
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
}

// createAdminUser provisions a local account — the UI's half of `user add`.
//
// It goes through the same store call the CLI does, so "one account per login
// name" stays the schema invariant external_id's UNIQUE constraint already
// makes it, rather than a rule two code paths each implement.
func (ro *Router) createAdminUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	name := auth.NormalizeLoginName(req.Username)
	if name == "" {
		writeError(w, http.StatusBadRequest, "a login name is required")
		return
	}
	if len(req.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest,
			"a password of at least "+strconv.Itoa(minPasswordLength)+" characters is required")
		return
	}
	// The plaintext is hashed here and never travels further: no store call,
	// log line or error string in this file takes a password.
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		serverError(w, r, "admin hash password", err)
		return
	}
	user, err := ro.cfg.Store.CreateLocalUser(r.Context(), auth.LocalExternalID(name), name, hash)
	if errors.Is(err, store.ErrDuplicateName) {
		writeError(w, http.StatusConflict, "that login name already has an account")
		return
	}
	if err != nil {
		serverError(w, r, "admin create user", err)
		return
	}
	// Promotion is a second statement rather than an argument to the insert,
	// because insertUser's is_admin is computed by the insert itself (the
	// first-user rule) and is not the caller's to set. A new account is
	// therefore created first and promoted after, which is also the order that
	// fails safe if the second write does not land.
	if req.IsAdmin && !user.IsAdmin {
		if err := ro.cfg.Store.SetUserAdmin(r.Context(), user.ID, true); err != nil {
			serverError(w, r, "admin promote new user", err)
			return
		}
		user.IsAdmin = true
	}
	slog.InfoContext(r.Context(), "admin created an account",
		slog.Int64("user_id", user.ID), slog.Bool("is_admin", user.IsAdmin))
	writeJSON(w, http.StatusCreated, newAdminUserView(user))
}

// updateUserRequest is the mutable half of an account, as pointers: absent
// means "leave it alone", which is what makes one route serve three unrelated
// admin actions without any of them implying the others.
type updateUserRequest struct {
	Password *string `json:"password"`
	Disabled *bool   `json:"disabled"`
	IsAdmin  *bool   `json:"is_admin"`
}

// updateAdminUser resets a password, disables/enables an account, or
// promotes/demotes it.
//
// Note what this route is *not*: it is not `/auth/local`, so an admin setting a
// password does not spend the login throttle av-t21v put on that endpoint. The
// two are different acts — one asserts a credential, the other guesses one —
// and only the guess is worth rate-limiting.
func (ro *Router) updateAdminUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		ro.notFound(w, r)
		return
	}
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	// Resolved once, up front: every branch below needs the account to exist,
	// and a missing one must answer exactly as adminOnly's refusal does.
	target, err := ro.cfg.Store.GetUser(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && target == nil) {
		ro.notFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, "admin lookup user", err)
		return
	}

	if req.Password != nil {
		if len(*req.Password) < minPasswordLength {
			writeError(w, http.StatusBadRequest,
				"a password of at least "+strconv.Itoa(minPasswordLength)+" characters is required")
			return
		}
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			serverError(w, r, "admin hash password", err)
			return
		}
		if err := ro.cfg.Store.SetLocalPassword(r.Context(), id, hash); err != nil {
			serverError(w, r, "admin set password", err)
			return
		}
		slog.InfoContext(r.Context(), "admin reset an account password", slog.Int64("user_id", id))
	}
	if req.IsAdmin != nil && *req.IsAdmin != target.IsAdmin {
		if err := ro.cfg.Store.SetUserAdmin(r.Context(), id, *req.IsAdmin); err != nil {
			ro.writeAdminChangeError(w, r, "admin set role", err)
			return
		}
	}
	if req.Disabled != nil && *req.Disabled != target.Disabled {
		if err := ro.cfg.Store.SetUserDisabled(r.Context(), id, *req.Disabled); err != nil {
			ro.writeAdminChangeError(w, r, "admin set disabled", err)
			return
		}
		slog.InfoContext(r.Context(), "admin changed an account's sign-in state",
			slog.Int64("user_id", id), slog.Bool("disabled", *req.Disabled))
	}

	updated, err := ro.cfg.Store.GetUser(r.Context(), id)
	if err != nil || updated == nil {
		serverError(w, r, "admin reread user", err)
		return
	}
	writeJSON(w, http.StatusOK, newAdminUserView(updated))
}

// --- the page ----------------------------------------------------------

// adminPageData feeds /admin/users.
//
// The account list is rendered server-side like every other page in this app;
// the controls on it post to the JSON API above, which is the single write path
// (architecture §4.1) and the reason this page has no form handler of its own.
//
// SelfID is the viewer's own account. The page needs it for one thing only —
// marking which row is "you", so an admin about to disable themselves can see
// that is what they are doing.
type adminPageData struct {
	Favicon template.URL
	LogoSVG template.HTML
	Users   []adminUserView
	SelfID  int64
	pageCredentials
}

// adminUsersPage renders the account list. It is registered inside the page
// group (so it carries an owner like every other page) *and* behind adminOnly
// (so carrying an owner is not mistaken for carrying authority) — the two are
// different questions and this route is the one place both are answered at
// once.
//
// It is deliberately a page under /admin/ rather than a tab on the settings
// page av-qwld will build. The two surfaces will share furniture — the shell,
// the header, the stylesheet — and must not share authority, and a URL prefix
// is the cheapest way to keep "acting on the instance" from drifting into
// "acting on yourself".
func (ro *Router) adminUsersPage(w http.ResponseWriter, r *http.Request) {
	users, err := ro.cfg.Store.ListUsers(r.Context())
	if err != nil {
		serverError(w, r, "admin users page", err)
		return
	}
	views := make([]adminUserView, 0, len(users))
	for _, u := range users {
		views = append(views, newAdminUserView(u))
	}
	page, err := renderPage("admin", adminPageData{
		Favicon:         template.URL(exhibitLogoDataURI),
		LogoSVG:         template.HTML(exhibitLogoSVG),
		Users:           views,
		SelfID:          ownerIDFromCtx(r.Context()),
		pageCredentials: ro.pageCredentials(r),
	})
	if err != nil {
		serverError(w, r, "admin users render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Names and email addresses of every account on the instance: never a
	// shared or proxy cache's to hold.
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, page)
}

// writeAdminChangeError reports the two refusals the store can return.
// ErrLastAdmin is a 409 with its reason spelled out — it is the one error here
// the admin can act on, and "conflict with the current state" is exactly what
// it is. ErrNotFound stays the same silent 404 everything else in this file
// answers.
func (ro *Router) writeAdminChangeError(w http.ResponseWriter, r *http.Request, label string, err error) {
	switch {
	case errors.Is(err, store.ErrLastAdmin):
		writeError(w, http.StatusConflict,
			"this is the last admin who can still sign in — promote or enable another account first")
	case errors.Is(err, store.ErrNotFound):
		ro.notFound(w, r)
	default:
		serverError(w, r, label, err)
	}
}

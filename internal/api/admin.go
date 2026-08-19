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
	"github.com/momja/Exhibit/internal/humanize"
	"github.com/momja/Exhibit/internal/store"
)

// minPasswordLength is the one rule imposed on an admin-set password. It is a
// length floor and nothing else — no character classes, no rotation — because
// those are the rules that produce `Password1!` and a sticky note, and because
// the account is provisioned by an admin who is already trusted with the whole
// instance.
const minPasswordLength = 8

// maxPasswordLength is bcrypt's own ceiling: auth.HashPassword rejects
// anything longer with an error. Checked here so that error surfaces as the
// same 400 the length floor does, rather than as a 500 from serverError.
const maxPasswordLength = 72

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
	case PrincipalNone:
		return !ro.loginEnabled()
	default:
		// An unrecognized PrincipalKind is not "no credential" — falling
		// through to the PrincipalNone case would grant admin on a fully
		// open instance to a value nothing here issued. Refuse it.
		return false
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
	// The entitlement, exactly as stored (av-2p8z) — a nil storage limit
	// means "none of its own", not "none at all". It is on the admin view and
	// on no other, because setting it is an admin's and only an admin's: an
	// entitlement a person can raise on themselves is not a limit.
	//
	// **Embedded, so it serializes flat** — `plan`, `storage_limit_bytes` and
	// `entitlement_ref` at the top level, exactly where updateUserRequest
	// expects them. That is deliberate: this route is the integration point an
	// external system maintaining these values uses (deployment.md §9), and a
	// read shape that nests what the write shape flattens turns the obvious
	// round trip — GET a user, edit the object, PATCH it back — into a silent
	// no-op, since an unrecognized key decodes to nothing and the handler
	// writes nothing. One shape both directions, like `is_admin` and
	// `disabled` beside it.
	store.Entitlement
	// Custom marks an account carrying an entitlement of its own, so the
	// drift list and the row badge agree by construction rather than by two
	// predicates that have to be kept in step.
	Custom bool `json:"entitlement_custom"`
	// StorageLimit is the ceiling this account is on, phrased for a page —
	// see storageLimitPhrase for which of the two numbers that is.
	StorageLimit string `json:"storage_limit"`
}

// adminUserView is a method rather than a free function because phrasing the
// resolved limit needs the instance's entitlement configuration, and reading
// it off the Router is what keeps the page and the resolver from disagreeing.
func (ro *Router) adminUserView(u *store.User) adminUserView {
	kind := "sso"
	if u.HasPassword {
		kind = "local"
	}
	return adminUserView{
		ID: u.ID, Name: u.Email, Email: u.Email, Kind: kind,
		IsAdmin: u.IsAdmin, Disabled: u.Disabled, CreatedAt: u.CreatedAt,
		Entitlement:  u.Entitlement,
		Custom:       !u.Entitlement.IsDefault(),
		StorageLimit: ro.storageLimitPhrase(u.Entitlement),
	}
}

// storageLimitPhrase says which ceiling this account is on: **its own when it
// has one**, and otherwise whatever an account with none resolves to here.
//
// The account's own number is shown even where limits are switched off, and
// that ordering is the point rather than a detail. Resolving first would
// short-circuit to "Unlimited" for every row on an instance with limits off —
// so 5 GiB, 10 GiB and 0 would all render identically, and the drift list,
// whose entire job is to surface what an external system last wrote, could not
// show the one thing it exists for. It would also pair "Unlimited" with a
// "Custom" badge on the same row, which is a sentence contradicting itself.
// Which mode the instance is in is stated once, above the table.
//
// The fallback still goes through the resolver, because that is the part with
// a rule in it that could drift. An account's own value has no rule: it is the
// value.
//
// An unresolvable one renders as "Unresolved" rather than as a number, and
// rather than failing the page — the same fail-closed answer the resolver
// gives a gate, said where somebody can act on it. An admin screen that 500s
// because one row is bad hides the one row an admin needs to find.
func (ro *Router) storageLimitPhrase(ent store.Entitlement) string {
	if own := ent.StorageLimitBytes; own != nil {
		if *own < 0 {
			return "Unresolved"
		}
		return humanize.Bytes(*own)
	}
	allowance, err := ro.cfg.Entitlements.resolve(store.Entitlement{})
	switch {
	case err != nil:
		return "Unresolved"
	case allowance.Storage.Unlimited:
		return "Unlimited"
	default:
		return humanize.Bytes(allowance.Storage.Bytes)
	}
}

// listAdminUsers is the directory, and — with `?entitlement=custom` — the
// drift list: only the accounts carrying an entitlement of their own.
//
// The filter is a query parameter on the existing route rather than a route of
// its own, because it answers the same question about the same rows in the
// same shape. An unrecognized value lists everybody: this is a view filter, so
// the failure mode of a typo should be "you were shown too much", never a
// refusal an external client has to special-case.
func (ro *Router) listAdminUsers(w http.ResponseWriter, r *http.Request) {
	var users []*store.User
	var err error
	if r.URL.Query().Get("entitlement") == "custom" {
		users, err = ro.cfg.Store.ListEntitlementOverrides(r.Context())
	} else {
		users, err = ro.cfg.Store.ListUsers(r.Context())
	}
	if err != nil {
		serverError(w, r, "admin list users", err)
		return
	}
	out := make([]adminUserView, 0, len(users))
	for _, u := range users {
		out = append(out, ro.adminUserView(u))
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
	if len(req.Password) > maxPasswordLength {
		writeError(w, http.StatusBadRequest,
			"a password of at most "+strconv.Itoa(maxPasswordLength)+" characters is required")
		return
	}
	// The plaintext is hashed here and never travels further: no store call,
	// log line or error string in this file takes a password.
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		serverError(w, r, "admin hash password", err)
		return
	}
	user, err := ro.cfg.Store.CreateLocalUser(r.Context(), store.NewLocalUser{
		ExternalID: auth.LocalExternalID(name), Email: name, PasswordHash: hash,
	})
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
	writeJSON(w, http.StatusCreated, ro.adminUserView(user))
}

// updateUserRequest is the mutable half of an account, as pointers: absent
// means "leave it alone", which is what makes one route serve three unrelated
// admin actions without any of them implying the others.
type updateUserRequest struct {
	Password *string `json:"password"`
	Disabled *bool   `json:"disabled"`
	IsAdmin  *bool   `json:"is_admin"`
	// The entitlement fields (av-2p8z), extending the same shape rather than
	// earning a route of their own: they are unrelated to the three above in
	// exactly the way those three are unrelated to each other, and they need
	// the same authority. An external system that maintains them is an
	// ordinary API client of this endpoint, authenticating with the service
	// token like any other.
	Plan *string `json:"plan"`
	// StorageLimitBytes needs three states where a pointer carries two:
	// absent leaves the ceiling alone, `null` puts the account back on the
	// instance default, and a number sets it. See nullableInt64 — a pointer
	// would silently collapse the first two, and "clear this override" is
	// precisely what a downgrade is.
	StorageLimitBytes nullableInt64 `json:"storage_limit_bytes"`
	// EntitlementRef is the opaque external reference. The empty string
	// clears it, which needs no third state: unlike a limit, "" and "unset"
	// are the same thing for a string nothing parses.
	EntitlementRef *string `json:"entitlement_ref"`
}

// nullableInt64 distinguishes the three states one JSON field can be in:
// absent (leave it alone), `null` (clear it), and a number (set it).
//
// Go's usual answer, a *int64, has only two: encoding/json decodes both an
// absent key and an explicit null to nil. That collapse is not tolerable here
// because clearing a per-owner limit — putting an account back on the instance
// default — is exactly what a downgrade is, and it would become unexpressible.
//
// Set is true only when UnmarshalJSON ran, which happens only when the key was
// present, so the zero value is "absent" without anything having to say so.
type nullableInt64 struct {
	Set   bool
	Value *int64
}

func (n *nullableInt64) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Value = nil
		return nil
	}
	var v int64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

// entitlementPatch turns the request's entitlement fields into the store's
// patch. Nothing else in this file reads them, so the mapping is in one place
// and the handler below stays about ordering and refusals.
func (req updateUserRequest) entitlementPatch() store.EntitlementPatch {
	return store.EntitlementPatch{
		Plan:              req.Plan,
		Ref:               req.EntitlementRef,
		SetStorageLimit:   req.StorageLimitBytes.Set,
		StorageLimitBytes: req.StorageLimitBytes.Value,
	}
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

	// Every refusable input is checked before anything is written, so a
	// request that fails, fails before it has changed anything. The store
	// refuses a negative ceiling too (and so does the column's CHECK); this is
	// here to make it a 400 with a sentence in it rather than a 500 arriving
	// after a role change has already landed.
	if limit := req.StorageLimitBytes; limit.Set && limit.Value != nil && *limit.Value < 0 {
		writeError(w, http.StatusBadRequest,
			"a storage limit is a number of bytes and cannot be negative; send null to put this account back on the instance default")
		return
	}

	// Role and disabled-state changes run first, ahead of the password reset.
	// Both can be refused by the store (ErrLastAdmin), and running them before
	// an otherwise-irreversible password write means that refusal leaves
	// nothing else half-applied — a request that fails, fails before it has
	// changed anything.
	if req.IsAdmin != nil && *req.IsAdmin != target.IsAdmin {
		if err := ro.cfg.Store.SetUserAdmin(r.Context(), id, *req.IsAdmin); err != nil {
			ro.writeAdminChangeError(w, r, "admin set role", err)
			return
		}
		slog.InfoContext(r.Context(), "admin changed an account's role",
			slog.Int64("user_id", id), slog.Bool("is_admin", *req.IsAdmin))
	}
	if req.Disabled != nil && *req.Disabled != target.Disabled {
		if err := ro.cfg.Store.SetUserDisabled(r.Context(), id, *req.Disabled); err != nil {
			ro.writeAdminChangeError(w, r, "admin set disabled", err)
			return
		}
		slog.InfoContext(r.Context(), "admin changed an account's sign-in state",
			slog.Int64("user_id", id), slog.Bool("disabled", *req.Disabled))
	}
	// The entitlement, before the password and after the two refusable
	// changes, on the same reasoning: it is reversible from this same page a
	// moment later, and an irreversible password write should be the last
	// thing a request does.
	if patch := req.entitlementPatch(); !patch.Empty() {
		if err := ro.cfg.Store.SetEntitlement(r.Context(), id, patch); err != nil {
			if errors.Is(err, store.ErrInvalidEntitlement) {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			ro.writeAdminChangeError(w, r, "admin set entitlement", err)
			return
		}
		// The plan and the reference are the operator's own strings and are
		// logged; the numbers are what an audit of "who raised whose ceiling"
		// actually needs.
		slog.InfoContext(r.Context(), "admin changed an account's entitlement",
			slog.Int64("user_id", id),
			slog.Bool("storage_limit_set", patch.SetStorageLimit),
			slog.Any("storage_limit_bytes", patch.StorageLimitBytes))
	}
	if req.Password != nil {
		if len(*req.Password) < minPasswordLength {
			writeError(w, http.StatusBadRequest,
				"a password of at least "+strconv.Itoa(minPasswordLength)+" characters is required")
			return
		}
		if len(*req.Password) > maxPasswordLength {
			writeError(w, http.StatusBadRequest,
				"a password of at most "+strconv.Itoa(maxPasswordLength)+" characters is required")
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

	updated, err := ro.cfg.Store.GetUser(r.Context(), id)
	if err == nil && updated == nil {
		err = fmt.Errorf("user %d vanished after update", id)
	}
	if err != nil {
		serverError(w, r, "admin reread user", err)
		return
	}
	writeJSON(w, http.StatusOK, ro.adminUserView(updated))
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
	// Entitlements is the instance's own configuration (av-2p8z), so the page
	// can say which of the two states it is in. The controls render either
	// way — an entitlement is data whether or not anything reads it yet — but
	// a limit shown on an instance that enforces nothing would be a lie of
	// omission, so the page says so once, at the top of the section, rather
	// than annotating every row.
	Entitlements Entitlements
	// DefaultStorageLimit phrases the instance default, which is what every
	// account with no entitlement of its own is on.
	DefaultStorageLimit string
	// Custom is the drift list: the accounts carrying an entitlement of their
	// own. An entitlement an external system maintains can fall out of step
	// with that system's view of reality — a downgrade it failed to deliver
	// leaves somebody on a raised ceiling indefinitely — and keeping them
	// current is that system's job, but *seeing* them is not.
	//
	// Read through the same store call `?entitlement=custom` answers with, so
	// the page and the API cannot come to different conclusions about who is
	// on a custom entitlement.
	Custom []adminUserView
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
		views = append(views, ro.adminUserView(u))
	}
	custom, err := ro.cfg.Store.ListEntitlementOverrides(r.Context())
	if err != nil {
		serverError(w, r, "admin entitlement overrides", err)
		return
	}
	customViews := make([]adminUserView, 0, len(custom))
	for _, u := range custom {
		customViews = append(customViews, ro.adminUserView(u))
	}
	page, err := renderPage("admin", adminPageData{
		Favicon:             template.URL(exhibitLogoDataURI),
		LogoSVG:             template.HTML(exhibitLogoSVG),
		Users:               views,
		SelfID:              ownerIDFromCtx(r.Context()),
		Entitlements:        ro.cfg.Entitlements,
		DefaultStorageLimit: ro.storageLimitPhrase(store.Entitlement{}),
		Custom:              customViews,
		pageCredentials:     ro.pageCredentials(r),
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

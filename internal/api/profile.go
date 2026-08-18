// The surface a person manages their *own* account from (av-qo05, epic
// av-g2dx).
//
// It is admin.go's opposite number, and the pairing is the point. Both pages
// read the same `users` rows and wear the same furniture — the settings shell,
// settings.css, the settingsHeader partial — and they differ in exactly one
// thing, which is the thing that matters: authority. `/admin/users` reaches
// *other* accounts, so it passes adminOnly. `/profile` reaches one account,
// the caller's own, so a session is the whole authorization and there is no
// second guard to get right. Nothing here takes an id from the request; the
// only account this file can name is `ownerIDFromCtx`'s.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/momja/Exhibit/internal/humanize"
	"github.com/momja/Exhibit/internal/store"
)

// profileAccount is the signed-in account as its owner sees it.
//
// It is not adminUserView. That view is a row in a directory an admin reads
// across; this is a person reading about themselves, so it carries no id, no
// role and no disabled flag — none of which this page can act on — and it
// carries instead the two things a table never needed: whether there is an
// account behind the request at all, and where the display name came from.
type profileAccount struct {
	// SignedIn is false on an instance with no login configured. That is not
	// an error: sessionGate is a pass-through there, ownerMiddleware supplies
	// the single-user default, and no `users` row exists behind it. The page
	// says so rather than rendering an account it invented.
	SignedIn bool
	// Name is what the page puts at the top of the Account section — see
	// profileName for how it is resolved and why the rule is not admin.go's.
	Name string
	// NameIsSubject marks a Name that came from `external_id` rather than
	// `email`: an opaque provider subject, true but unreadable. The template
	// labels it as such instead of passing it off as an address, because a
	// UUID presented as "your account" reads as a bug.
	NameIsSubject bool
	// HasPassword is "signs in with a password here" vs "signs in through an
	// identity provider" — the same distinction admin.tmpl's Sign-in column
	// draws, and the one that decides what this page could ever do for them.
	HasPassword bool
	// Artifacts and Shares are what deletion would destroy, counted and
	// phrased server-side (av-4wyq). Shares carry the weight of the pair: an
	// artifact is the person's own, but a share is a URL somebody *else* may
	// be holding, with no account on this instance and no way to be told it
	// stopped working.
	Artifacts string
	Shares    string
	// ArtifactCount and ShareCount are the same two numbers unphrased, so the
	// template can decide whether a clause applies at all rather than
	// rendering it with a zero in it. "No share links will break" is noise on
	// an account that never made one, and a confirmation built around "your no
	// artifacts" is how a confirmation stops being read.
	ArtifactCount int64
	ShareCount    int64
	// Storage is what the account is holding, phrased for a person
	// (av-fw1b) — bodies and widgets today, whatever else the schema learns
	// to reference later. It sits in the Account section rather than the
	// danger zone because "what is actually using my disk" is a question
	// self-hosters have every day and deletion is the once. Nothing here
	// refuses on it; a limit read from the same number is av-10bw's.
	Storage string
	// DeleteBlocked is why this account cannot be deleted, or empty when it
	// can. It is a reason rather than a boolean because a control that cannot
	// act is useless without the sentence saying why — the same rule the
	// widget-generate button follows — and because the two cases that block
	// it are blocked for entirely different reasons.
	//
	// Rendering the control disabled is also the honest ordering: the store
	// would refuse the last enabled admin anyway (ErrLastAdmin), and letting
	// that refusal arrive *after* someone typed a confirmation phrase for an
	// irreversible act is a worse way to learn it.
	DeleteBlocked string
}

// profileName resolves the one name this page can put on the account.
//
// admin.go's rule is `u.Email` and nothing else (newAdminUserView), and that
// is right for a *table*: an identity whose provider sent no email renders one
// blank cell among many, which is cosmetic. Here the name *is* the section, so
// the same rule renders the page's only content empty. And an empty email is a
// shape to expect rather than a corrupt row — `users.email` is NOT NULL and
// defaults to the empty string (migration 013), a portable second key beside
// `external_id` rather than something an identity provider guarantees.
//
// So: the email when there is one; otherwise the subject the provider knows
// this person by, which is opaque but true and is the string an operator would
// need to find the row; otherwise nothing, and the template states the sign-in
// route instead. A local account never reaches the second step —
// CreateLocalUser writes the login name into both columns — so the opaque case
// belongs to identity providers alone.
//
// Deliberately local to this page rather than pushed into admin.go: the
// fallback exists because a name rendered alone must not be blank, and that is
// a property of this layout, not of the row. If a third surface ever needs the
// same answer, that is when it earns a shared helper.
func profileName(u *store.User) (name string, isSubject bool) {
	if u.Email != "" {
		return u.Email, false
	}
	if u.ExternalID != "" {
		return u.ExternalID, true
	}
	return "", false
}

// profilePageData feeds /profile.
//
// Sections, from the first one, even though only Account has content today:
// this page is where av-g2dx's remaining stories land — the BYO agent key,
// active sessions, export — and a page laid out in cards takes each of those
// as an addition rather than a redesign.
type profilePageData struct {
	Favicon template.URL
	Account profileAccount
	pageCredentials
}

// profilePage renders the account surface.
//
// It is registered inside the page group and *not* behind adminOnly, which is
// the whole authority statement: the group supplies the owner, and the owner is
// the only account this handler reads. A visitor with no session never arrives
// — sessionGate redirects them to /auth/login first, on an instance that has
// one — so there is no unauthenticated branch here to get wrong.
func (ro *Router) profilePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var acct profileAccount
	u, err := ro.cfg.Store.GetUser(ctx, ownerIDFromCtx(ctx))
	switch {
	case errors.Is(err, store.ErrNotFound) || (err == nil && u == nil):
		// The self-hoster's default shape: no login, so no row. Left as the
		// zero profileAccount, which the template reads as "no account to
		// show" — not a 404 and not a 500, because nothing is missing.
	case err != nil:
		serverError(w, r, "profile page", err)
		return
	default:
		acct.SignedIn = true
		acct.Name, acct.NameIsSubject = profileName(u)
		acct.HasPassword = u.HasPassword
		if err := ro.fillDeleteSection(ctx, u, &acct); err != nil {
			serverError(w, r, "profile delete section", err)
			return
		}
	}
	if !acct.SignedIn {
		// No login configured, so there is no `users` row and nothing for
		// deletion to act on. The section still renders — it is where the
		// action lives whichever shape the instance has — with the control
		// off and the reason attached, exactly as the last-admin case does.
		acct.DeleteBlocked = "This instance has no login configured, so there is no account to delete. " +
			"Its library belongs to whoever can reach the origin; removing it is the operator's to do on the host."
	}

	page, err := renderPage("profile", profilePageData{
		Favicon:         template.URL(exhibitLogoDataURI),
		Account:         acct,
		pageCredentials: ro.pageCredentials(r),
	})
	if err != nil {
		serverError(w, r, "profile render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Who is signed in, named: never a shared or proxy cache's to hold, for
	// the same reason /admin/users is not.
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, page)
}

// --- deleting the account (av-4wyq) ------------------------------------
//
// The section's copy is the hardest part of this feature, and it is load-
// bearing rather than decorative. Deleting here erases what Exhibit holds; it
// does not remove anyone from the identity provider that issued their login,
// which Exhibit has no authority over (deployment.md §3.4). Because
// `users.external_id` is UNIQUE and `UpsertUser` creates the row just in time,
// the same person signing in again gets a **new** owner id and an empty
// library. The confirmation says that outright. Getting it wrong is worse than
// not shipping: someone deletes, finds their login still works, and reasonably
// concludes nothing happened.

// deleteAccountConfirmation is the phrase the confirmation requires to be
// typed, and the API requires to be sent.
//
// A typed phrase rather than a second button, because a click can be a
// mis-tap and this operation has no undo anywhere near it (there is no soft
// delete, no trash, and no snapshot — av-1rvm covers state durability
// separately). It is checked server-side as well as in the page for the usual
// reason: the page is a client, and a client-side interlock is a courtesy to
// whoever is clicking, never a control.
//
// The phrase is the *act*, not the account's name. GitHub's convention is to
// type the resource name, but here the name can be a provider subject — an
// opaque UUID nobody can retype — and the sentence "delete my library" carries
// the correction this whole section exists to make: what goes is the library,
// not the identity.
const deleteAccountConfirmation = "delete my library"

type deleteAccountRequest struct {
	Confirm string `json:"confirm"`
}

// deleteAccount erases the caller's own account and everything in it.
//
// It takes **no id**, from the path or the body, and that is the entire
// authorization argument — the same one profilePage makes. `/api/admin/users`
// is where acting on somebody else lives, behind adminOnly; this route cannot
// name another account, so a session is sufficient for it and there is no
// second guard to get right. It is registered outside that group deliberately.
//
// It also requires a *session* specifically. The service token reaches every
// other route on the instance, but "delete my account" has no meaning for a
// credential that is not a person: it would resolve to the single-user default
// owner and erase whatever library happened to be sitting there. An operator
// who means that has the CLI and the host.
func (ro *Router) deleteAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !sessionAuthed(ctx) {
		writeError(w, http.StatusNotFound,
			"account deletion acts on the signed-in account, and this request has none")
		return
	}
	// Redundant today — the only read-only principal is the anonymous public
	// visitor, which is not a session and was refused above — but every
	// mutating handler in this package asks, and the most destructive one is
	// the wrong place to be the exception. av-7k7b's read-only *session* is
	// the case that would make it load-bearing.
	if principalFromCtx(ctx).ReadOnly {
		writeError(w, http.StatusForbidden, "read-only visitor may not mutate")
		return
	}

	var req deleteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Confirm != deleteAccountConfirmation {
		writeError(w, http.StatusBadRequest,
			`this is irreversible: send {"confirm": "`+deleteAccountConfirmation+`"} to proceed`)
		return
	}

	ownerID := ownerIDFromCtx(ctx)
	blobIDs, err := ro.cfg.Store.DeleteAccount(ctx, ownerID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such account")
		return
	case errors.Is(err, store.ErrLastAdmin):
		writeError(w, http.StatusConflict,
			"this is the only account that can administer this instance. "+
				"Make another account an admin first, then delete this one.")
		return
	case err != nil:
		serverError(w, r, "delete account", err)
		return
	}

	// The rows are gone, which took the sessions with them (ON DELETE CASCADE
	// on sessions.user_id) — so the browser is already signed out server-side.
	// Clearing the cookie stops it presenting a credential no row backs.
	ro.clearCookie(w, sessionCookieName)

	slog.InfoContext(ctx, "account deleted",
		slog.Int64("user_id", ownerID), slog.Int("blobs", len(blobIDs)))

	// Then the bytes, in the same row-first order and for the same reason as
	// deleteArtifactBlobs (artifacts.go). A 500 here reports an erasure that
	// only half happened: the account is gone and cannot be retried, but some
	// artifact bodies are still on the volume, and that is exactly the thing a
	// person deleting their library must not be told succeeded.
	if err := deleteBlobs(ctx, ro.cfg.Store, ro.cfg.Blob, blobIDs); err != nil {
		serverError(w, r, "delete account blobs", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// fillDeleteSection populates what the danger zone states before anyone
// commits to it: how much is about to go, and whether it may go at all — plus
// the storage figure the Account section shows, which is the same summary read
// and so is not worth a second query.
func (ro *Router) fillDeleteSection(ctx context.Context, u *store.User, acct *profileAccount) error {
	sum, err := ro.cfg.Store.GetAccountSummary(ctx, u.ID)
	if err != nil {
		return err
	}
	acct.Artifacts = countPhrase(sum.Artifacts, "artifact", "artifacts")
	acct.Shares = countPhrase(sum.Shares, "share link", "share links")
	acct.ArtifactCount = sum.Artifacts
	acct.ShareCount = sum.Shares
	acct.Storage = humanize.Bytes(sum.StorageBytes)

	last, err := ro.isLastEnabledAdmin(ctx, u)
	if err != nil {
		return err
	}
	if last {
		acct.DeleteBlocked = "You are the only account that can administer this instance. " +
			"Deleting it would leave nobody able to create an account, re-enable one, or reset a password — " +
			"and no account left to promote. Make someone else an admin first."
	}
	return nil
}

// isLastEnabledAdmin reports whether u is the only account that can still
// administer this instance — the condition the store refuses a deletion on.
//
// Read here from ListUsers rather than asked of the store as its own count,
// because the store already answers it where it matters: the guard is inside
// DeleteAccount's WHERE clause, atomic under SQLite's single writer. This is
// the page telling someone in advance, and a page's answer is a snapshot
// whatever query produces it. A second store method would look authoritative
// and would not be.
func (ro *Router) isLastEnabledAdmin(ctx context.Context, u *store.User) (bool, error) {
	if !u.IsAdmin || u.Disabled {
		return false, nil
	}
	users, err := ro.cfg.Store.ListUsers(ctx)
	if err != nil {
		return false, err
	}
	for _, other := range users {
		if other.ID != u.ID && other.IsAdmin && !other.Disabled {
			return false, nil
		}
	}
	return true, nil
}

// countPhrase renders a count with its noun, so the template states a quantity
// rather than assembling one. "No artifacts" instead of "0 artifacts": the
// sentence is read by someone deciding whether to go through with this, and
// zero deserves a word rather than a digit.
func countPhrase(n int64, singular, plural string) string {
	switch n {
	case 0:
		return "no " + plural
	case 1:
		return "1 " + singular
	default:
		return strconv.FormatInt(n, 10) + " " + plural
	}
}

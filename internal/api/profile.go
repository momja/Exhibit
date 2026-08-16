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
	"errors"
	"fmt"
	"html/template"
	"net/http"

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

package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
)

type createShareRequest struct {
	ArtifactID string `json:"artifact_id"`

	// Recipients are typed handles — a login name, or an email address — and
	// their presence is what makes this request a *grant* rather than a mint
	// of the artifact's anonymous link. Several at once, because "gave friends
	// access, dropped the link in the group chat" is one gesture and not three
	// (av-7k7b): one submit, one request, and a result per name.
	//
	// Handles rather than user ids, deliberately. An id is not something a
	// person knows about their friend, and the only thing that would supply
	// one — a picker over the instance's accounts — publishes the whole user
	// directory to every user, which av-utap went out of its way to avoid from
	// a different door.
	Recipients []string `json:"recipients"`

	// Replace rotates the anonymous link: delete the existing one and mint a
	// new one, in a single request. A leaked link wants replacing, and the
	// two-step version — toggle off, toggle on — leaves the user unsure it
	// worked and leaves the artifact with no link at all if the second half
	// fails. Meaningless for a grant, and refused there rather than ignored.
	Replace bool `json:"replace"`

	// Public is a tombstone on exactly ExpiresAt's grounds, and it is what
	// closes av-20xv. The column was accepted, stored, and never read by
	// ServeShare, so `public: false` was precisely the value a caller would
	// set believing they had restricted something — an access control the API
	// advertised and did not have. av-lrae dropped it rather than wiring it
	// up, because once a recipient is on the row it means exactly "no
	// recipient": a share with nobody named on it *is* the public link.
	// Continuing to accept the key would leave the request shape offering a
	// choice the resource no longer has.
	Public json.RawMessage `json:"public"`

	// ExpiresAt is a tombstone, not a setting. Share expiry was removed in
	// av-8ipt; this field exists only so a request that still asks for one is
	// answered with a 400 instead of quietly getting a share that never
	// expires. Accepting a field the server discards is precisely the defect
	// av-20xv exists to fix, and re-creating it here to save four lines would
	// be a poor trade. Typed as RawMessage because the value is never read —
	// only its presence is.
	ExpiresAt json.RawMessage `json:"expires_at"`
}

// createShareResponse answers both shapes this route mints. Share/ShareURL
// describe the artifact's anonymous link; Results reports one outcome per
// typed handle on a bulk grant. Exactly one half is populated, and which is
// decided by what the request asked for — a grant has no single URL to return
// (the artifact's own address is the recipient's door), and a link names
// nobody to report on.
type createShareResponse struct {
	Share    *store.Share  `json:"share,omitempty"`
	ShareURL string        `json:"share_url,omitempty"`
	Results  []grantResult `json:"results,omitempty"`
}

// grantResult is one name's outcome. Bulk granting reports per name rather
// than failing the request, because the alternative for "alice, bob, tpyo" is
// all-or-nothing: either three people are refused over one typo, or the
// request succeeds and the typo is never mentioned. Neither tells the owner
// what they need to know, which is exactly which of their friends can now open
// the thing.
//
// Name is echoed back as typed so a caller can pair a result with the field it
// came from, and so the page can say which spelling failed.
type grantResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	ShareID string `json:"share_id,omitempty"`
}

// The outcomes a handle can have. `unknown` is the one that matters: this
// field CONFIRMS EXISTENCE on purpose. Answering "added" for a name nobody
// holds would leave the owner believing their friend has access when the
// friend has none — a silence that is discovered weeks later, by the friend —
// and that is a worse failure than leaking one bit per guessed name. A picker
// would have leaked the entire directory for nothing; rate limiting is the
// answer if guessing ever becomes a real problem (av-6xjd, 2026-09-05).
const (
	grantStatusGranted   = "granted"
	grantStatusExisting  = "already_shared"
	grantStatusUnknown   = "unknown"
	grantStatusAmbiguous = "ambiguous"
	grantStatusSelf      = "self"
)

// maxBulkRecipients bounds one submit. It is a guard rather than a product
// rule — av-wrbu owns the service's policy on oversized requests — and it is
// here because this is the one route where a single body multiplies into an
// unbounded number of user lookups and inserts. A hundred is far past any
// plausible group chat.
const maxBulkRecipients = 100

func (ro *Router) createShare(w http.ResponseWriter, r *http.Request) {
	if principalFromCtx(r.Context()).ReadOnly {
		writeError(w, http.StatusForbidden, "read-only visitor may not mutate")
		return
	}
	var req createShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ArtifactID == "" {
		http.Error(w, "artifact_id is required", http.StatusBadRequest)
		return
	}
	// Any mention of either key is an error, whatever its value — one rule with
	// no sub-cases. The first says out loud that a share now lives until it is
	// deleted (DELETE /api/shares/:id is how it ends); the second, that a share
	// with no recipient is already the public one.
	if req.ExpiresAt != nil {
		http.Error(w, "expires_at is no longer supported: shares live until deleted (DELETE /api/shares/:id)", http.StatusBadRequest)
		return
	}
	if req.Public != nil {
		http.Error(w, "public is no longer supported: a share with no recipient is the artifact's public link (av-20xv)", http.StatusBadRequest)
		return
	}

	ownerID := ownerIDFromCtx(r.Context())

	// Verify the artifact exists in this owner's library. Minting a share for
	// someone else's artifact would publish it through the deliberately
	// unauthenticated /s/:id path, so this is the sharpest of the ownership
	// checks — and CreateShare re-asserts it in SQL besides.
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, req.ArtifactID)
	if err != nil {
		serverError(w, r, "create share artifact lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}

	// Named recipients make this a bulk grant; nobody named makes it the
	// artifact's one anonymous link. The two are the same resource — a share
	// row, told apart by recipient_id (av-lrae) — which is why they are one
	// route, and they diverge here rather than at the door because everything
	// above this line (the tombstones, the ownership check) is identical.
	if len(req.Recipients) > 0 {
		if req.Replace {
			writeError(w, http.StatusBadRequest,
				"replace applies to the artifact's public link, not to a grant: revoke a grant with DELETE /api/shares/:id")
			return
		}
		ro.grantShares(w, r, ownerID, req.ArtifactID, req.Recipients)
		return
	}

	sh := &store.Share{
		ID:         uuid.New().String(),
		ArtifactID: req.ArtifactID,
	}

	// Replace is one store call rather than a delete followed by a create,
	// because the failure between those two leaves the artifact with no link —
	// worse than either the old one or the new one (see the Store interface).
	if req.Replace {
		replaced, err := ro.cfg.Store.ReplaceAnonymousLink(r.Context(), ownerID, req.ArtifactID, sh.ID)
		if err != nil {
			if errors.Is(err, store.ErrDuplicateShare) {
				writeError(w, http.StatusConflict, "this artifact already has a public link")
				return
			}
			writeArtifactError(w, r, "replace share link", err)
			return
		}
		sh = replaced
		slog.DebugContext(r.Context(), "share link replaced",
			slog.String("share_id", sh.ID), slog.String("artifact_id", req.ArtifactID))
	} else if err := ro.cfg.Store.CreateShare(r.Context(), ownerID, sh); err != nil {
		// The artifact already has its one link (av-lrae). That is a schema
		// invariant doing its job, so it is the caller's answer — 409 — rather
		// than a 500 reporting our own constraint as a fault.
		if errors.Is(err, store.ErrDuplicateShare) {
			writeError(w, http.StatusConflict, "this artifact already has a public link")
			return
		}
		writeArtifactError(w, r, "create share", err)
		return
	} else {
		slog.DebugContext(r.Context(), "share created",
			slog.String("share_id", sh.ID), slog.String("artifact_id", req.ArtifactID))
	}

	resp := createShareResponse{
		Share:    sh,
		ShareURL: ro.shareURL(sh.ID),
	}
	writeJSON(w, http.StatusCreated, resp)
}

// shareURL is where an anonymous link is served: the render origin, which is
// the whole point of the two-origin model — a share hands somebody untrusted
// code, and it must not run where the app's cookies live. One definition so
// the mint response, the panel and any future caller cannot disagree.
func (ro *Router) shareURL(shareID string) string {
	return ro.cfg.RenderOrigin + "/s/" + shareID
}

// grantShares resolves each typed handle and mints a grant for it, reporting
// one result per name.
//
// It answers 200 even when every name failed, and that is deliberate: the
// request *succeeded* in reporting what happened to each handle, and a caller
// reading `results` learns strictly more than it would from a status code
// covering three different names. The failure this avoids is all-or-nothing —
// three people refused over one typo, or a typo never mentioned.
func (ro *Router) grantShares(w http.ResponseWriter, r *http.Request, ownerID int64, artifactID string, names []string) {
	// Blank entries are a trailing comma, not a name somebody typed, so they
	// are dropped before the loop rather than reported as unknown.
	handles := []string{}
	for _, n := range names {
		if trimmed := strings.TrimSpace(n); trimmed != "" {
			handles = append(handles, trimmed)
		}
	}
	if len(handles) > maxBulkRecipients {
		writeError(w, http.StatusBadRequest, "too many recipients in one request")
		return
	}

	results := make([]grantResult, 0, len(handles))
	for _, name := range handles {
		res := grantResult{Name: name}
		u, err := ro.resolveRecipient(r.Context(), name)
		switch {
		case errors.Is(err, store.ErrNotFound):
			res.Status = grantStatusUnknown
		case errors.Is(err, store.ErrAmbiguousUser):
			res.Status = grantStatusAmbiguous
		case err != nil:
			serverError(w, r, "resolve share recipient", err)
			return
		case u.ID == ownerID:
			// The owner already has every access a grant could confer, and a
			// row saying so would show up in their own list of who they shared
			// with. Reported rather than silently skipped, so a mistyped
			// self-grant is visible instead of looking like it worked.
			res.Status = grantStatusSelf
		default:
			sh := &store.Share{ID: uuid.New().String(), ArtifactID: artifactID, RecipientID: &u.ID}
			err := ro.cfg.Store.CreateShare(r.Context(), ownerID, sh)
			switch {
			case err == nil:
				res.Status, res.ShareID = grantStatusGranted, sh.ID
				slog.DebugContext(r.Context(), "share granted",
					slog.String("share_id", sh.ID), slog.String("artifact_id", artifactID),
					slog.Int64("recipient_id", u.ID))
			case errors.Is(err, store.ErrDuplicateShare):
				// One grant per (artifact, person) is a unique index, so this
				// is the schema reporting that the person already has access —
				// which is what the owner wanted, and not an error to them.
				res.Status = grantStatusExisting
			default:
				serverError(w, r, "create share grant", err)
				return
			}
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, createShareResponse{Results: results})
}

// resolveRecipient turns a typed handle into the account it names: the login
// name of an account this instance issued, or an email address.
//
// The order is not arbitrary. `local:<normalized>` is checked first because
// external_id carries a UNIQUE constraint, so for a local account the lookup
// is exact and free — the login name *is* the identity. Only then does email
// come into play, and it exists for the accounts that have no login name at
// all: an OIDC identity, whose external_id is a provider subject nobody could
// type. One resolver, so the fuzziness lives in exactly one place.
//
// The clean alternative is a real users.username column, unique, which would
// also retire the display-name fallbacks /profile and /admin/users each carry.
// That is account-management work (av-g2dx, av-utap), not sharing work, and is
// deliberately not pulled in here.
func (ro *Router) resolveRecipient(ctx context.Context, name string) (*store.User, error) {
	u, err := ro.cfg.Store.GetUserByExternalID(ctx, auth.LocalExternalID(name))
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return ro.cfg.Store.GetUserByEmail(ctx, auth.NormalizeLoginName(name))
}

// listArtifactShares enumerates who can open an artifact — the audit half of
// sharing (av-6xjd), and the half most likely to be cut.
//
// It is owner-scoped through the store, so a recipient reading it gets the 404
// a nonexistent artifact gets: they can use the artifact, and the guest list is
// not part of what a grant carries.
func (ro *Router) listArtifactShares(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	id := urlParamID(r, "artifactID")
	shares, err := ro.cfg.Store.ListArtifactShares(r.Context(), ownerID, id)
	if err != nil {
		writeArtifactError(w, r, "list artifact shares", err)
		return
	}
	writeJSON(w, http.StatusOK, newArtifactSharesView(shares, ro.shareURL))
}

// artifactSharesView is one artifact's sharing state, and it is deliberately
// one type serving two consumers: the JSON route above, and the owner's
// server-rendered share panel (gallery.go). The panel is the surface people
// use and the route is what a script or a curl can audit with; splitting them
// would be two answers to "who can open this" with nothing keeping them equal.
//
// The two halves are separate fields rather than one list, because a link and
// a grant are not two of a kind: the link is singular, is a URL, and is
// toggled; a grant is one of many, is a person, and is revoked.
type artifactSharesView struct {
	Link   *shareLinkView   `json:"link"`
	Grants []shareGrantView `json:"grants"`
}

type shareLinkView struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type shareGrantView struct {
	ID          string `json:"id"`
	RecipientID int64  `json:"recipient_id"`
	// Name is the label a person recognizes, resolved by shareRecipientLabel.
	// It is attacker-influenced text — a login name, or an address a provider
	// reported — so every surface that renders it must escape it. On the page
	// that is html/template's contextual escaping; nothing builds this into
	// markup by hand.
	Name string `json:"name"`
	// Disabled says an admin has switched this account off, so the grant is
	// inert until it is switched back on. Shown rather than filtered, because
	// a row that silently vanishes from the guest list is a row the owner
	// cannot revoke and does not know exists.
	Disabled bool `json:"disabled"`
}

func newArtifactSharesView(shares []store.ArtifactShare, shareURL func(string) string) artifactSharesView {
	view := artifactSharesView{Grants: []shareGrantView{}}
	for _, sh := range shares {
		if sh.RecipientID == nil {
			// The schema permits exactly one (a partial unique index,
			// av-lrae), so the last-wins assignment here can only ever run
			// once; writing it as an assignment rather than a break keeps that
			// a fact about the database instead of a loop invariant.
			link := shareLinkView{ID: sh.ID, URL: shareURL(sh.ID)}
			view.Link = &link
			continue
		}
		g := shareGrantView{ID: sh.ID, RecipientID: *sh.RecipientID}
		if sh.Recipient != nil {
			g.Name = shareRecipientLabel(sh.Recipient)
			g.Disabled = sh.Recipient.Disabled
		} else {
			// A grant whose account is gone should be impossible (the FK
			// cascades), but a list that refuses to render because of one is
			// an owner who cannot revoke the rest.
			g.Name = "unknown account"
		}
		view.Grants = append(view.Grants, g)
	}
	return view
}

// shareRecipientLabel is how a granted account is named back to the owner who
// granted it.
//
// The login name first, because that is what they typed. `local:` is stripped
// rather than shown: it is a namespace prefix that exists so an account this
// instance issued can never collide with a provider subject (auth.go), and it
// is not part of anybody's name. An OIDC identity has no such name, so it
// falls back to the address — and, failing that, to the subject, which is
// opaque but is at least the thing the account actually is.
//
// This is the same fallback ladder /profile carries for the same reason, and
// like that one it stays local to its surface: it exists because a name
// rendered alone must not be blank. A real users.username column would retire
// all of them at once (av-g2dx).
func shareRecipientLabel(u *store.User) string {
	if name, ok := strings.CutPrefix(u.ExternalID, "local:"); ok && name != "" {
		return name
	}
	if u.Email != "" {
		return u.Email
	}
	if u.ExternalID != "" {
		return u.ExternalID
	}
	return "account #" + strconv.FormatInt(u.ID, 10)
}

func (ro *Router) deleteShare(w http.ResponseWriter, r *http.Request) {
	if principalFromCtx(r.Context()).ReadOnly {
		writeError(w, http.StatusForbidden, "read-only visitor may not mutate")
		return
	}
	id := chi.URLParam(r, "shareID")

	ownerID := ownerIDFromCtx(r.Context())
	sh, err := ro.cfg.Store.GetShare(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "delete share lookup", err)
		return
	}
	if sh == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := ro.cfg.Store.DeleteShare(r.Context(), ownerID, id); err != nil {
		writeArtifactError(w, r, "delete share", err)
		return
	}

	slog.DebugContext(r.Context(), "share deleted", slog.String("share_id", id))
	w.WriteHeader(http.StatusNoContent)
}

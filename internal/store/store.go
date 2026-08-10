package store

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors returned by Store implementations; handlers map them to
// HTTP status codes.
var (
	// ErrNotFound means the row (or an owner-scoped row it references) does not exist.
	ErrNotFound = errors.New("not found")
	// ErrDuplicateName means a name uniqueness constraint was violated —
	// per-owner for tags and collections, instance-wide for a local login
	// name (av-rzvf), which has only one namespace because there is only one
	// user directory.
	ErrDuplicateName = errors.New("name already exists")
	// ErrLastAdmin means a change was refused because it would have left the
	// instance with no enabled admin — nobody able to create an account,
	// re-enable one, or reset a password (av-utap). It is a refusal, not a
	// failure: nothing was written.
	ErrLastAdmin = errors.New("the last enabled admin cannot be demoted or disabled")
)

type Tier int

const (
	Tier1 Tier = 1
	Tier2 Tier = 2
)

type Artifact struct {
	ID           string    `json:"id"`
	OwnerID      int64     `json:"owner_id"`
	Title        string    `json:"title"`
	SourceBlobID string    `json:"source_blob_id"`
	SourceURL    string    `json:"source_url"`
	Tier         Tier      `json:"tier"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// NetworkAllowlist is the artifact's approved origins, hydrated on read
	// from the decision='allow' rows of artifact_network_origins (exhibit-x87).
	// It is a projection, not a column: writes go through the origin-decision
	// methods on Store (PutArtifact and UpdateArtifact's "network_allowlist"
	// key translate to allow-row writes so the API shape stays unchanged).
	NetworkAllowlist []string `json:"network_allowlist"`
	// DownloadsApproved records the user's first-use approval of the
	// host-mediated download bridge for this artifact. False means the host
	// frame prompts on the next download attempt.
	DownloadsApproved bool `json:"downloads_approved"`
	// ClipboardApproved is the same first-use approval for the clipboard
	// bridge (navigator.clipboard read/write proxied through the host).
	ClipboardApproved bool `json:"clipboard_approved"`
	// WidgetBlobID is the blob holding this artifact's widget — the small,
	// informative document its gallery card renders (av-fafu). Empty means the
	// artifact has no widget and its card falls back to the default tile. The
	// widget has no identity of its own: it reads this artifact's state and
	// renders under this artifact's CSP allowlist.
	WidgetBlobID string `json:"widget_blob_id"`
	Tags         []*Tag `json:"tags"` // populated on read by GetArtifact/ListArtifacts
	// SourceText is the artifact's body reduced to its visible text (see
	// ExtractSearchText), written into PutArtifact only to seed the
	// artifacts_fts search index (§8.2/§3.3: search over source, not just
	// title). It is never scanned back out by GetArtifact/ListArtifacts —
	// the blob store remains the body's source of truth — so this field is
	// write-only from the caller's perspective.
	SourceText string `json:"-"`
}

type Collection struct {
	ID      string `json:"id"`
	OwnerID int64  `json:"owner_id"`
	Name    string `json:"name"`
}

// DefaultTagColor is applied to a tag when no color is supplied.
const DefaultTagColor = "#6B7280"

type Tag struct {
	ID      string `json:"id"`
	OwnerID int64  `json:"owner_id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
}

// Origin decision values. Only DecisionAllow reaches the render CSP;
// DecisionBlock is a "don't ask again" marker for the runtime permission
// prompt (exhibit-fr7) and must never widen the policy.
const (
	DecisionAllow = "allow"
	DecisionBlock = "block"
)

// OriginDecision is one user decision about one origin an artifact may
// contact. Source records where the decision came from ("user", "legacy",
// "runtime", …) and is informational only.
type OriginDecision struct {
	Origin    string    `json:"origin"`
	Decision  string    `json:"decision"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Share is a link to an artifact. It has no lifetime of its own: a share lives
// until its row is deleted (av-8ipt dropped the never-set expires_at column).
type Share struct {
	ID         string `json:"id"`
	ArtifactID string `json:"artifact_id"`
	Public     bool   `json:"public"`
}

// AgentKey is an owner's BYO agent provider credential. KeyCiphertext is the
// AES-GCM-sealed API key; the store never sees plaintext (Exh-ky6e).
type AgentKey struct {
	OwnerID       int64     `json:"owner_id"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	KeyCiphertext string    `json:"-"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ListOptions struct {
	// OwnerID scopes the listing to one owner. It is not optional: the zero
	// value matches no owner, so a caller that forgets it gets an empty list
	// rather than everyone's library.
	OwnerID     int64
	Query       string
	Tags        []string
	Collections []string
	Limit       int
	Offset      int
}

// Store is the seam between handlers and persistence (architecture §3.3).
//
// # Owner scoping (av-ep8k)
//
// Every method that names an artifact takes the requesting owner and filters
// on it *in SQL* — for the artifact-child tables (state, origin decisions,
// transcripts, shares, collection membership) through the owner-scoped EXISTS
// subquery the tag joins use. Ownership is therefore a property of the query,
// not a pre-check a future caller can forget to perform.
//
// The invariant these methods hold: **another owner's id is indistinguishable
// from an id that does not exist.** A cross-tenant read returns the same
// (nil, nil) a missing row does; a cross-tenant write returns ErrNotFound.
// Neither ever returns a permission error, because "you may not touch this"
// confirms the row exists and turns the API into a membership oracle over
// artifact ids.
//
// The two Unscoped accessors are the deliberate, greppable exceptions — see
// their doc comments.
type Store interface {
	// Artifacts. PutArtifact takes the owner from a.OwnerID.
	PutArtifact(ctx context.Context, a *Artifact) error
	// GetArtifact returns (nil, nil) when the artifact does not exist *or*
	// belongs to another owner — the two cases are deliberately identical
	// (see the type comment). Handlers turn nil into 404.
	GetArtifact(ctx context.Context, ownerID int64, id string) (*Artifact, error)
	// GetArtifactUnscoped resolves an artifact with no owner check. It exists
	// for the render surface and the share path, which have no session and no
	// owner in context: on RENDER_ORIGIN authorization is the signed render
	// token (av-c5aq) or the share row itself (architecture §7), not an owner
	// comparison. The name is long and explicit so `grep Unscoped` enumerates
	// the entire un-owner-scoped read surface.
	GetArtifactUnscoped(ctx context.Context, id string) (*Artifact, error)
	ListArtifacts(ctx context.Context, opts ListOptions) ([]*Artifact, error)
	UpdateArtifact(ctx context.Context, ownerID int64, id string, updates map[string]any) error
	DeleteArtifact(ctx context.Context, ownerID int64, id string) error

	// Network origin decisions (exhibit-x87). ListOriginDecisions returns
	// every decision for an artifact, allow and block alike, ordered by
	// origin. AllowedOrigins is the CSP's read path: the origins of the
	// decision='allow' rows only. SetOriginDecision upserts one origin's
	// decision (the (artifact, origin) primary key means an origin can never
	// hold two decisions). ReplaceAllowedOrigins makes the allow set exactly
	// origins — it upserts those and deletes the artifact's other *allow*
	// rows, deliberately leaving block rows untouched so a caller that only
	// knows the allowlist (the edit page's single PATCH) can never silently
	// clear a "don't ask again" decision.
	//
	// All five are owner-scoped on the artifact: another owner's artifact has
	// no decisions to read and cannot be written to (ErrNotFound).
	ListOriginDecisions(ctx context.Context, ownerID int64, artifactID string) ([]OriginDecision, error)
	AllowedOrigins(ctx context.Context, ownerID int64, artifactID string) ([]string, error)
	SetOriginDecision(ctx context.Context, ownerID int64, artifactID, origin, decision, source string) error
	DeleteOriginDecision(ctx context.Context, ownerID int64, artifactID, origin string) error
	ReplaceAllowedOrigins(ctx context.Context, ownerID int64, artifactID string, origins []string, source string) error

	// Collections. The membership writes require both the artifact and the
	// collection to belong to ownerID, so a shared id can never link one
	// owner's artifact into another's shelf.
	CreateCollection(ctx context.Context, c *Collection) error
	ListCollections(ctx context.Context, ownerID int64) ([]*Collection, error)
	AddArtifactToCollection(ctx context.Context, ownerID int64, artifactID, collectionID string) error
	RemoveArtifactFromCollection(ctx context.Context, ownerID int64, artifactID, collectionID string) error

	// Tags. All mutations are owner-scoped: rows belonging to another owner
	// are treated as nonexistent (ErrNotFound). Tag names are unique per
	// owner (ErrDuplicateName on conflict).
	CreateTag(ctx context.Context, t *Tag) error
	ListTags(ctx context.Context, ownerID int64) ([]*Tag, error)
	// UpdateTag renames and/or recolors a tag; a nil name or color leaves
	// that field unchanged. Returns the updated tag.
	UpdateTag(ctx context.Context, ownerID int64, id string, name, color *string) (*Tag, error)
	// DeleteTag removes the tag globally; its artifact associations are
	// removed via ON DELETE CASCADE.
	DeleteTag(ctx context.Context, ownerID int64, id string) error
	AddArtifactTag(ctx context.Context, ownerID int64, artifactID, tagID string) error
	// RemoveArtifactTag detaches a tag from an artifact; ErrNotFound if the
	// pairing did not exist.
	RemoveArtifactTag(ctx context.Context, ownerID int64, artifactID, tagID string) error

	// State. These methods take TWO principals, because they answer two
	// different questions (av-q0ub):
	//
	//   ownerID — may this caller reach this artifact at all? Authorization,
	//             enforced as the same owner-scoped EXISTS predicate every
	//             other artifact-child method here uses, so a foreign artifact
	//             id stays indistinguishable from one that does not exist.
	//   userID  — whose rows? Selection: the viewer the state belongs to,
	//             stored as artifact_state.user_id. It comes from the signed
	//             render token on the render path (av-c5aq) and from the
	//             session on the API path (av-30rj).
	//
	// Today every caller passes the same value for both, because the only
	// principal that can reach an artifact is its owner. They diverge the
	// moment a non-owner may view a shared artifact (av-7k7b) — which is why
	// they are two parameters rather than one, and why artifactID sits between
	// them: transposing the two is then a compile error rather than a
	// cross-tenant read.
	//
	// DeleteState and ClearState are deliberately idempotent — the caller's
	// intent is "this key must not exist" / "no state of mine must remain",
	// which a missing row already satisfies. That matters because their
	// callers (the state inspector's delete, and the storage shim's
	// removeItem/clear write-through) routinely fire on keys the server never
	// saw. Neither scoping changes that: another owner's artifact, and another
	// viewer's rows, hold nothing *this* caller can remove, so the deletes
	// stay silent no-ops there too and those rows survive untouched.
	GetState(ctx context.Context, ownerID int64, artifactID string, userID int64) (map[string]string, error)
	SetState(ctx context.Context, ownerID int64, artifactID string, userID int64, key, value string) error
	DeleteState(ctx context.Context, ownerID int64, artifactID string, userID int64, key string) error
	ClearState(ctx context.Context, ownerID int64, artifactID string, userID int64) error

	// Agent (Exh-yvhp). SetAgentKey upserts the owner's single configured
	// provider key; GetAgentKey returns nil when none is set.
	SetAgentKey(ctx context.Context, k *AgentKey) error
	GetAgentKey(ctx context.Context, ownerID int64) (*AgentKey, error)
	DeleteAgentKey(ctx context.Context, ownerID int64) error
	// SaveTranscript upserts the agent conversation that produced an
	// artifact (messagesJSON is the Pi session's message list).
	SaveTranscript(ctx context.Context, ownerID int64, artifactID, sessionID, messagesJSON string) error
	// ListTranscripts returns messagesJSON per session for an artifact.
	ListTranscripts(ctx context.Context, ownerID int64, artifactID string) (map[string]string, error)

	// Shares. A share is minted and revoked by the artifact's owner, so those
	// two are owner-scoped; resolving one to serve it is not (below).
	CreateShare(ctx context.Context, ownerID int64, s *Share) error
	GetShare(ctx context.Context, ownerID int64, id string) (*Share, error)
	// GetShareUnscoped resolves a share row with no owner check — the second
	// deliberate exception. `GET /s/:id` is answered for anyone holding the
	// link, because the share row *is* the authorization (architecture §7);
	// there is no owner to compare against by design.
	GetShareUnscoped(ctx context.Context, id string) (*Share, error)
	DeleteShare(ctx context.Context, ownerID int64, id string) error

	// Lifecycle
	Close() error

	// --- Identity and sessions (av-30rj) -------------------------------
	// The identity provider is a login-time concern only: it is exchanged
	// once, at the callback, for a session row here. Everything below is
	// ours and identical whichever provider issued the identity, which is
	// what keeps a provider swap confined to one constructor. Types live in
	// users.go, the SQLite implementation in sqlite_users.go.

	// UpsertUser returns the user for a provider identity, creating the row
	// just-in-time on first login and refreshing the stored email on every
	// later one. The returned ID is the integer the rest of the schema
	// already calls owner_id.
	UpsertUser(ctx context.Context, externalID, email string) (*User, error)
	GetUser(ctx context.Context, id int64) (*User, error)
	// GetUserByExternalID is the same row reached by the key a *person* is
	// known by — a provider subject, or `local:<name>` — for callers holding
	// a name rather than an owner id. Unlike LookupLocalCredential below, it
	// does not treat "has no password" as "does not exist".
	GetUserByExternalID(ctx context.Context, externalID string) (*User, error)

	// CreateSession records a logged-in browser under a caller-supplied
	// opaque id. GetSession returns ErrNotFound when that id is unknown *or*
	// expired — a revoked session and a lapsed one are the same answer to
	// the only question a caller asks. DeleteSession is the revocation, and
	// is idempotent. DeleteExpiredSessions is a janitor; nothing depends on
	// it having run.
	CreateSession(ctx context.Context, s *Session) error
	GetSession(ctx context.Context, id string) (*Session, error)
	DeleteSession(ctx context.Context, id string) error
	DeleteExpiredSessions(ctx context.Context) (int64, error)

	// --- Local credentials (av-rzvf) -----------------------------------
	// A local account is a users row above with password_hash filled in, so
	// these methods read and write the same rows in the same owner_id space
	// as an OIDC identity — they are not a second directory. externalID is
	// auth.LocalExternalID(name); the hash is bcrypt, produced and compared
	// by internal/auth and only ever stored here.

	// LookupLocalCredential returns an account and its stored hash, or
	// ErrNotFound when the name is unknown *or* the account has no password.
	LookupLocalCredential(ctx context.Context, externalID string) (*User, string, error)
	// CreateLocalUser provisions an account, ErrDuplicateName if the name is
	// taken. SetLocalPassword changes one, or removes it when hash is empty.
	CreateLocalUser(ctx context.Context, externalID, email, passwordHash string) (*User, error)
	SetLocalPassword(ctx context.Context, userID int64, passwordHash string) error
	// CountLocalCredentials answers "does this instance have a login of its
	// own?"; ListUsers is the instance's user directory, oldest first.
	CountLocalCredentials(ctx context.Context) (int64, error)
	ListUsers(ctx context.Context) ([]*User, error)

	// --- Administration (av-utap) --------------------------------------
	// An admin acting on the *instance*, as distinct from a person acting on
	// their own account (av-g2dx). Both refuse with ErrLastAdmin rather than
	// leave the instance with no enabled admin, and both answer ErrNotFound
	// for an id that does not exist.

	// SetUserAdmin promotes (unguarded) or demotes (guarded) an account.
	SetUserAdmin(ctx context.Context, userID int64, admin bool) error
	// SetUserDisabled stops an account signing in — and, when disabling,
	// deletes that user's sessions in the same transaction. Revoking the
	// live sessions is part of the operation rather than a courtesy the
	// caller adds: a disable a logged-in browser survives is not a disable.
	SetUserDisabled(ctx context.Context, userID int64, disabled bool) error
}

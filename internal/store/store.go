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
	// ErrNotUpdatable means an update map named a column callers may not
	// write (updatableArtifactColumns). The key came out of a decoded request
	// body, so it is a bad request rather than a server fault, and handlers
	// map it to 400 — a 500 would report the caller's typo as our outage.
	ErrNotUpdatable = errors.New("not an updatable column")
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
	// LinksApproved is the same first-use approval for the link navigation
	// bridge (av-r0dk): external anchor activations proxied through the host
	// frame and opened in a new tab. False means the host prompts on the
	// artifact's first external-link click.
	LinksApproved bool `json:"links_approved"`
	// CameraApproved and MicrophoneApproved are the same first-use approval for
	// the media gate (av-mv3k): navigator.mediaDevices.getUserMedia in the
	// frame, which asks the host and then settles. Two flags rather than one
	// because the prompt names the devices the artifact actually asked for — a
	// dictation tool granted a microphone must not thereby hold a camera.
	//
	// These two differ from the other three approvals in where they are
	// enforced. A capture device is unreachable from the sandbox's opaque
	// origin and cannot be handed in either (a MediaStreamTrack is not
	// transferable), so the grant is spent on a *top-level* render, and these
	// build that document's Permissions-Policy header. That is what keeps it
	// per-artifact: a browser permission is per-origin, so without the header
	// one approved artifact would hand the camera to every other artifact on
	// the render origin. Neither affects the CSP.
	CameraApproved     bool `json:"camera_approved"`
	MicrophoneApproved bool `json:"microphone_approved"`
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

// OwnerID and ViewerID name the two principals the state methods below take.
// Both hold a users.id value and are numerically interchangeable as plain
// int64, which is exactly what let a caller transpose them and still
// compile despite the doc comment's claim otherwise. Distinct types close
// that gap: passing one where the other belongs is now a compile error, not
// a silent cross-tenant read.
type OwnerID int64
type ViewerID int64

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
	// SetWidgetBlobID attaches (or, with an empty blobID, detaches) the
	// artifact's gallery-card widget body. Separate from UpdateArtifact
	// because widget_blob_id is not caller-writable: the generic update map is
	// a decoded PATCH body, and this id is minted server-side.
	SetWidgetBlobID(ctx context.Context, ownerID int64, id, blobID string) error
	// DeleteArtifact removes the artifact and returns the blob ids it queued
	// for deletion; DeleteWidget does the same for the widget an artifact
	// detaches. Both enqueue inside the transaction that dropped the
	// reference, and only for a blob no remaining row names (blobqueue.go).
	// The caller hands what comes back to DrainBlobDeletions.
	DeleteArtifact(ctx context.Context, ownerID int64, id string) ([]string, error)
	DeleteWidget(ctx context.Context, ownerID int64, artifactID string) ([]string, error)

	// Out-of-line assets (av-20fk): the binary payloads a page fetches at run
	// time, stored as blobs of their own rather than base64 inside the body.
	//
	// ReplaceArtifactAssets installs one ingest's or refetch's worth as the
	// artifact's current generation and retires the previous set, returning
	// what it queued for deletion. Replacing rather than appending is what
	// stops a repeated refetch accumulating a full set every time.
	//
	// The two Unscoped reads are the render surface's, and carry the same
	// exception GetArtifactUnscoped does: a share serves an artifact to
	// someone with no account, so there is no owner to scope by. Neither
	// exposes more than the render already does — they are the bytes the
	// served document is about to fetch — and the asset lookup still takes
	// the artifact id, so one artifact cannot address another's.
	ReplaceArtifactAssets(ctx context.Context, ownerID int64, artifactID, generationID string, assets []ArtifactAsset) ([]string, error)
	ListArtifactAssets(ctx context.Context, ownerID int64, artifactID string) ([]ArtifactAsset, error)
	ArtifactAssetsUnscoped(ctx context.Context, artifactID string) ([]ArtifactAsset, error)
	GetArtifactAssetUnscoped(ctx context.Context, artifactID, assetID string) (*ArtifactAsset, error)
	DeleteArtifactAsset(ctx context.Context, ownerID int64, artifactID, assetID string) ([]string, error)

	// The blob deletion queue (av-8gyd). Rows and bytes live in two stores
	// that cannot commit together, so the *intent* to remove the bytes is
	// recorded in the transaction that removed the rows, and these three
	// finish the job: drain what one operation just enqueued (synchronously,
	// after it) or drain the whole queue at startup, which is where a crashed
	// process's leftovers are reclaimed. Draining is idempotent, so repeating
	// one costs nothing.
	//
	// None of the three takes an owner, and none can: a blob id reaches the
	// queue only once the last row naming it — and with it the last record of
	// whose it was — has been deleted.
	PendingBlobDeletions(ctx context.Context) ([]string, error)
	DrainBlobDeletions(ctx context.Context, blobs BlobDeleter, ids []string) (int, error)
	DrainAllBlobDeletions(ctx context.Context, blobs BlobDeleter) (int, error)

	// LockBlobs excludes a caller that is about to write bytes and commit a
	// row naming them from the drain that is about to unlink those same bytes
	// (bloblock.go). It belongs on this interface rather than inside the store
	// because the two halves of that race live in different packages: the
	// drain is here, and writing an asset's bytes before referencing them is
	// the API's ingest path. A caller holds it across [write bytes … commit
	// the referencing row] and — since the database runs on one connection —
	// never takes it while already inside a transaction.
	//
	// Only content-addressed ids can lose this race, since only they can be
	// referenced again after being condemned; a caller minting a fresh id has
	// nothing to exclude and need not call this.
	LockBlobs(ids ...string) func()

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
	GetState(ctx context.Context, ownerID OwnerID, artifactID string, userID ViewerID) (map[string]string, error)
	SetState(ctx context.Context, ownerID OwnerID, artifactID string, userID ViewerID, key, value string) error
	DeleteState(ctx context.Context, ownerID OwnerID, artifactID string, userID ViewerID, key string) error
	ClearState(ctx context.Context, ownerID OwnerID, artifactID string, userID ViewerID) error

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

	// --- Storage accounting (av-fw1b) ----------------------------------
	// How many bytes an owner is holding. A length is recorded where the
	// bytes are written; the total is derived on read by joining those
	// lengths to the rows that reference them, so there is no counter to
	// drift. Nothing here refuses anything — limits are av-10bw's.
	// sqlite_storage.go carries the reasoning, including why a shared blob
	// is charged in full to every referencing owner.

	// RecordBlobSize persists a blob's byte length; idempotent by upsert,
	// since bodies are rewritten in place under the same blob id.
	RecordBlobSize(ctx context.Context, blobID string, bytes int64) error
	// ForgetBlobSizes drops the lengths of blob ids nothing references any
	// more. Ids that are still referenced by *anyone* are left alone, so a
	// caller may pass every id it just deleted without knowing which were
	// shared. Idempotent.
	ForgetBlobSizes(ctx context.Context, blobIDs []string) error
	// StorageUsage is the owner's total stored bytes, in one query and
	// without touching the blob store.
	StorageUsage(ctx context.Context, ownerID int64) (int64, error)

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
	CreateLocalUser(ctx context.Context, u NewLocalUser) (*User, error)
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

	// --- Per-owner entitlements (av-2p8z) ------------------------------
	// What an owner is allowed, stored on their users row and read back with
	// it (User.Entitlement). Only an admin sets these — an entitlement a
	// person can raise on themselves is not a limit — which is why they are
	// here, in the block of methods that act on somebody else's account, and
	// not in the block below it.
	//
	// Nothing in this interface resolves an entitlement or refuses anything
	// because of one. "What is this owner allowed" needs the instance's
	// configured default as well as these rows, so it is one function in
	// package api and gates ask that rather than reading these columns.

	// SetEntitlement applies a partial change to one account's entitlement:
	// an unset field in the patch is left alone, and a patch that clears the
	// storage limit puts the account back on the instance default.
	// ErrInvalidEntitlement for a limit that is not one; ErrNotFound for an
	// account that does not exist.
	SetEntitlement(ctx context.Context, userID int64, p EntitlementPatch) error
	// ListEntitlementOverrides returns every account carrying an entitlement
	// of its own — the drift surface. An entitlement an external system
	// maintains can fall out of step with that system's view of reality, and
	// a discrepancy nobody can list is one nobody can find.
	ListEntitlementOverrides(ctx context.Context) ([]*User, error)

	// --- A person's own account (av-4wyq, epic av-g2dx) -----------------
	// The two above are an admin acting on somebody else. These two are an
	// account acting on itself, and neither takes an id a request could
	// supply — the caller passes the owner its own session already resolved.

	// GetAccountSummary counts what deleting the account would destroy, so
	// the confirmation can state it rather than gesture at it. Shares are
	// counted because they are the consequence that lands on somebody else
	// (see the type).
	GetAccountSummary(ctx context.Context, userID int64) (AccountSummary, error)
	// DeleteAccount erases the account and everything it owns, returning the
	// blob ids it queued for deletion — collected and enqueued inside the same
	// transaction, because after it commits nothing can name them again.
	// ErrLastAdmin when the account is the instance's only enabled admin;
	// ErrNotFound when there is no such account. sqlite_account.go says what
	// it deletes and what the schema's cascades delete for it.
	DeleteAccount(ctx context.Context, userID int64) ([]string, error)
}

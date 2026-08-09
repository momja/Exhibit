package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Local credential login (av-q30x): the second way into an instance, beside
// the redirect-based IdentityProvider above.
//
// It is deliberately *not* an IdentityProvider implementation. That interface
// is AuthURL/Exchange — an external authority to redirect to, and a code to
// redeem. A username and a password have neither, and forcing them through
// would mean inventing a self-redirect and a fake authorization code that
// exist only to satisfy a shape. The two are instead two *login paths* that
// converge on the same session layer: each ends by handing an Identity to the
// same session-creation call, after which nothing downstream knows or cares
// which path produced it.
//
// Accounts live in the database (av-rzvf): a local account is a users row with
// password_hash filled in, provisioned by an operator, so an instance can have
// as many as it likes. What that avoids is what made password auth a bad trade
// for a multi-user product in the first place (av-30rj): with no
// self-registration there is nothing to verify, and with an admin holding the
// reset there is no reset mail and therefore no SMTP requirement.
//
// Credential below is what remains in the environment, and it is now one
// account's password rather than the only one there is — see cmd/server for
// the bootstrap-and-break-glass role it plays.

// localIDPrefix namespaces a local account's `users.external_id` so it can
// never collide with a provider subject, the only other thing that column ever
// holds.
const localIDPrefix = "local:"

// LocalExternalID is the `users.external_id` a local login name resolves to.
//
// It derives from the name because with more than one local account the name
// *is* the identity, not a label on the single one that exists. (Before
// av-rzvf this was the constant "local", which was correct while an instance
// had exactly one local credential and renaming it had to relabel rather than
// orphan the library — migration 015 re-keys that row.)
//
// The name is normalized first, which is what makes "one account per name" a
// real constraint rather than one that Alice@example.com can walk around.
func LocalExternalID(name string) string { return localIDPrefix + NormalizeLoginName(name) }

// NormalizeLoginName canonicalizes a login name — trimmed and lowercased —
// because the name is now a database key and email-shaped keys are compared
// case-insensitively everywhere a person will expect them to be. Applying it
// in one place is what keeps the stored external_id, the stored email, and the
// name typed into the form the same string.
func NormalizeLoginName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Credential is the username/password pair an instance may be configured with
// in its environment. The plaintext password never exists in this process —
// only the bcrypt hash the operator supplied — so there is nothing here to leak
// into a log, a crash dump, or an error string.
type Credential struct {
	username string
	hash     []byte
}

// NewCredential builds the credential from what the operator set at deploy.
// passwordHash must be a bcrypt hash, not a password: an instance that would
// accept plaintext here has no reason to hash anything at all (see
// HashPassword). A malformed value is an error rather than a credential that
// can never match, so a pasted-in plaintext password fails at startup where
// the operator is watching, instead of at their first login attempt.
func NewCredential(username, passwordHash string) (*Credential, error) {
	if username == "" {
		return nil, errors.New("local login: username is empty")
	}
	if passwordHash == "" {
		return nil, errors.New("local login: password hash is empty")
	}
	if _, err := bcrypt.Cost([]byte(passwordHash)); err != nil {
		return nil, fmt.Errorf("local login: password hash is not a bcrypt hash (%w) — "+
			"it must be the output of `server hash-password`, not the password itself", err)
	}
	return &Credential{username: NormalizeLoginName(username), hash: []byte(passwordHash)}, nil
}

// HashPassword produces the value NewCredential wants. It lives here rather
// than in the command that calls it so the hashing parameters are stated once,
// beside the verification that has to agree with them.
//
// bcrypt at the library's default cost: deliberately slow, salted per call (so
// the same password hashes differently every time), and the conventional
// choice the ticket named. The slowness *used* to be the whole of the rate
// limiting — a guess costs the attacker the same tens of milliseconds it costs
// us — which was tolerable for one credential and stopped being so once an
// instance issues several (av-sz4e). The endpoint is throttled in its own right
// now (av-t21v, internal/api/loginratelimit.go); what this cost still buys, and
// no request-rate limit can, is that a *stolen hash* is expensive to attack
// offline.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// Username is the name the login form asks for. Exported so the login page can
// say whose instance this is; it is a label, never a secret.
func (c *Credential) Username() string { return c.username }

// Names reports whether a submitted login name is this credential's, compared
// in constant time on the normalized form. It is split from the password check
// because the login path has to decide *which* stored hash a submission is
// checked against — this credential's or the account row's — before it spends
// a bcrypt compare, and doing both here would mean spending two.
func (c *Credential) Names(name string) bool {
	return subtle.ConstantTimeCompare(
		[]byte(NormalizeLoginName(name)), []byte(c.username)) == 1
}

// VerifyPassword checks a submitted password against this credential's hash.
func (c *Credential) VerifyPassword(password string) bool {
	return bcrypt.CompareHashAndPassword(c.hash, []byte(password)) == nil
}

// Verify reports whether a submitted username and password are the configured
// ones.
//
// The bcrypt call runs even when the username is already known to be wrong.
// That is not about hiding *which* field was wrong — the login name is barely
// a secret — but about the endpoint having one cost regardless of input, so
// nothing about the stored credential can be recovered by timing a few
// thousand attempts.
func (c *Credential) Verify(username, password string) bool {
	nameOK := c.Names(username)
	passOK := c.VerifyPassword(password)
	return nameOK && passOK
}

// Identity is what this credential hands to the session layer, in exactly the
// shape a provider exchange produces. The login name travels as Email — the
// column that exists to be the human-readable, provider-independent handle on
// a user row — and as the external id it derives, which is the key the account
// is found by on every later login.
func (c *Credential) Identity() Identity {
	return Identity{ExternalID: LocalExternalID(c.username), Email: c.username}
}

// VerifyStoredPassword compares a submitted password against a hash read from
// the users table. An empty hash — an OIDC identity, or an account whose
// password was removed — never matches, and still costs a bcrypt compare, so
// the login endpoint takes the same time whether or not the name exists. That
// uniform cost is why this is a function here rather than an `if hash == ""`
// at the call site, where it would read as a shortcut worth taking.
func VerifyStoredPassword(hash, password string) bool {
	if hash == "" {
		// Spend the compare, then refuse unconditionally. The result is
		// discarded rather than returned, so this stays a false answer even
		// if somebody ever guesses the decoy's preimage.
		bcrypt.CompareHashAndPassword([]byte(decoyPasswordHash), []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// decoyPasswordHash is a well-formed bcrypt hash at the same cost the real ones
// use, existing only so an absent password costs what a present one costs. It
// is a hardcoded constant rather than something generated at startup because it
// is not a secret and never decides anything — VerifyStoredPassword throws its
// answer away.
const decoyPasswordHash = "$2a$10$3xJmTMqAJdjNPUEmZ6BJH.j8x6vNjD1Qk5jvVQZQeQOaKUxJ0eIYS"

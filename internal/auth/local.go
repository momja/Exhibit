package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"

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
// The whole surface is one credential, set once at deploy. That is what keeps
// it small enough to be worth having: with no registration there is no
// verification, with no self-service there is no reset mail and therefore no
// SMTP, and with one operator there is nobody to lock out. Those were the
// costs that made password auth a bad trade for a multi-user product
// (av-30rj); none of them are paid here.

// LocalExternalID is the `users.external_id` every local login resolves to.
//
// It is a constant rather than something derived from the username, because
// the username is a *label* on the one local credential, not an identity of
// its own: an instance has exactly one, and renaming it must not orphan the
// library the previous name owned. The value is namespaced so it can never
// collide with a provider subject, which is the only other thing that column
// ever holds.
const LocalExternalID = "local"

// Credential is the single username/password pair an instance may be
// configured with. The plaintext password never exists in this process — only
// the bcrypt hash the operator supplied — so there is nothing here to leak
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
	return &Credential{username: username, hash: []byte(passwordHash)}, nil
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

// Verify reports whether a submitted username and password are the configured
// ones.
//
// Both comparisons are constant-time, and the bcrypt call runs even when the
// username is already known to be wrong. Neither is about hiding *which* field
// was wrong — with a single credential the username is barely a secret — but
// about the endpoint having one cost regardless of input, so nothing about the
// stored credential can be recovered by timing a few thousand attempts.
func (c *Credential) Verify(username, password string) bool {
	nameOK := subtle.ConstantTimeCompare([]byte(username), []byte(c.username)) == 1
	passOK := bcrypt.CompareHashAndPassword(c.hash, []byte(password)) == nil
	return nameOK && passOK
}

// Identity is what this credential hands to the session layer, in exactly the
// shape a provider exchange produces. The username travels as Email — the
// column that exists to be the human-readable, provider-independent handle on
// a user row, and which UpsertUser refreshes on every login, so renaming the
// login keeps the same owner and relabels it.
func (c *Credential) Identity() Identity {
	return Identity{ExternalID: LocalExternalID, Email: c.username}
}

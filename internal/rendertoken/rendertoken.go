// Package rendertoken mints and verifies the short-lived, narrowly scoped
// credential that lets the render origin serve a document to a known principal
// without ever holding a session (av-c5aq).
//
// Why not a cookie. A top-level GET RENDER_ORIGIN/a/:id is not sandboxed: it is
// a real-origin document with the artifact's own script inlined into it. Any
// cookie readable there is readable by the artifact, which can post it to any
// origin on its allowlist. So the render origin must stay sessionless, and the
// principal has to arrive in the URL instead.
//
// Why that is safe. A token is scoped to exactly one (artifact, owner) pair and
// expires within minutes. The artifact can read it out of location.href, but it
// grants only what the artifact already has — access to itself — for a few more
// minutes. That property is the whole design: it is why the scope must never be
// widened to an owner, a collection, or a long lifetime.
//
// The token is an HMAC-SHA256 tag, not a JWT: there is one issuer, one
// verifier, one algorithm, and three fields, so a signature-suite negotiation
// would be pure attack surface.
package rendertoken

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TTL is how long a minted token stays valid. Long enough that a page whose
// frames were minted at render time can finish loading them (and htmx can swap
// a fragment or two), short enough that a token captured out of location.href
// by the artifact it already belongs to is worth little. Links that a user may
// click long after page load must not embed a token at all — they go through
// the app origin, which mints on demand.
const TTL = 10 * time.Minute

// Param is the query parameter the token travels in.
const Param = "t"

// KeyPurpose is the domain-separation label for deriving the signing key from
// the server secret, so render tokens and the AES-GCM key that seals agent
// provider keys never share key material.
const KeyPurpose = "exhibit/render-token/v1"

// version prefixes every signed message. It is inside the MAC, so a future
// format cannot be produced by replaying a v1 tag.
const version = "v1"

var (
	// ErrInvalid covers every "this is not a token I issued for this artifact"
	// case — malformed, wrong artifact, bad signature. They are deliberately
	// one error: distinguishing them for the caller would only help an attacker
	// distinguish them too.
	ErrInvalid = errors.New("rendertoken: invalid token")
	// ErrExpired is separate because it is the one failure a legitimate client
	// hits, and the fix (mint a new one) is different.
	ErrExpired = errors.New("rendertoken: token expired")
)

// Signer mints and verifies render tokens under one key.
type Signer struct {
	key [32]byte
}

// NewSigner returns a Signer over an already-derived 32-byte key. Callers
// derive it from the server secret (see KeyPurpose) rather than configuring a
// second secret.
func NewSigner(key [32]byte) *Signer {
	return &Signer{key: key}
}

// NewRandomSigner returns a Signer over a fresh random key, valid only for the
// life of the process. It is the fail-closed fallback for a service started
// with no server secret at all: tokens still work end to end within the
// process, they simply do not survive a restart. Failing closed here matters —
// the alternative, an unsigned render surface, is the hole this package exists
// to close.
func NewRandomSigner() *Signer {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		// crypto/rand failing is not a condition this process can serve
		// through: every token it minted would be predictable.
		panic("rendertoken: no entropy for signing key: " + err.Error())
	}
	return NewSigner(key)
}

// Mint returns a token authorizing ownerID to render artifactID for TTL.
func (s *Signer) Mint(artifactID string, ownerID int64) string {
	return s.mintAt(artifactID, ownerID, time.Now())
}

func (s *Signer) mintAt(artifactID string, ownerID int64, now time.Time) string {
	claims := fmt.Sprintf("%d.%d", ownerID, now.Add(TTL).Unix())
	return claims + "." + s.tag(artifactID, claims)
}

// Verify checks tok against artifactID and returns the owner it authorizes.
//
// artifactID comes from the URL the render surface is answering, and is mixed
// into the MAC rather than carried in the token. So a token minted for artifact
// A simply fails to verify on artifact B's route — the scoping is the signature
// itself, not a field a verifier could forget to compare.
func (s *Signer) Verify(tok, artifactID string) (ownerID int64, err error) {
	owner, exp, sig, ok := split(tok)
	if !ok {
		return 0, ErrInvalid
	}
	claims := owner + "." + exp
	if !hmac.Equal([]byte(sig), []byte(s.tag(artifactID, claims))) {
		return 0, ErrInvalid
	}
	// Parsed only after the MAC checks out, so nothing downstream ever acts on
	// unauthenticated numbers.
	id, err := strconv.ParseInt(owner, 10, 64)
	if err != nil {
		return 0, ErrInvalid
	}
	expUnix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return 0, ErrInvalid
	}
	if time.Now().After(time.Unix(expUnix, 0)) {
		return 0, ErrExpired
	}
	return id, nil
}

// split cuts "owner.exp.tag" without allocating a slice for the parts.
func split(tok string) (owner, exp, sig string, ok bool) {
	i := strings.IndexByte(tok, '.')
	if i < 0 {
		return "", "", "", false
	}
	j := strings.IndexByte(tok[i+1:], '.')
	if j < 0 {
		return "", "", "", false
	}
	j += i + 1
	return tok[:i], tok[i+1 : j], tok[j+1:], true
}

// tag is the authenticator over (version, artifact, claims). The fields are
// separated by a byte that cannot occur in any of them, so no two different
// tuples can serialize to the same message.
func (s *Signer) tag(artifactID, claims string) string {
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte(version))
	mac.Write([]byte{0})
	mac.Write([]byte(artifactID))
	mac.Write([]byte{0})
	mac.Write([]byte(claims))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

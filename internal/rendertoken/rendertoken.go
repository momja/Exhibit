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

// anonymousClaim is the trailing claim marking a document rendered for nobody
// in particular — a visitor reading a public instance's library with no
// credential of their own (av-wmp6).
//
// It is a claim rather than a query parameter because it *subtracts* authority:
// the render surface inlines no state for an anonymous viewer and its shim
// persists none. A parameter would be a privilege the viewer could drop by
// editing the URL; inside the MAC it is the issuer's statement, not the
// bearer's.
//
// It is optional and last, so a token minted without it is byte-for-byte the
// token this package minted before the claim existed. Nothing in flight breaks
// on the deploy that adds it.
const anonymousClaim = "a"

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

// Claims is what a verified token says: who the document is being rendered
// for, and whether that principal is a person at all.
//
// The two travel together because the render surface needs both and must never
// act on one without the other. OwnerID authorizes the read — it is checked
// against the artifact's owner. Anonymous says the reader has no identity of
// their own, which is a different question with a different answer: their
// document gets the artifact, and none of the owner's state.
type Claims struct {
	// OwnerID is the owner whose artifact this token renders.
	OwnerID int64
	// Anonymous marks a viewer with no identity — a public instance's
	// unauthenticated visitor. The artifact renders; nobody's state does.
	Anonymous bool
}

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

// Mint returns a token authorizing ownerID to render artifactID for TTL, as
// that owner: their state is the state the document inlines.
func (s *Signer) Mint(artifactID string, ownerID int64) string {
	return s.MintFor(artifactID, ownerID, TTL)
}

// MintAnonymous returns a token that renders ownerID's artifact for a viewer
// with no identity — the public-instance case (av-wmp6). The document it
// authorizes carries the artifact and no state at all.
func (s *Signer) MintAnonymous(artifactID string, ownerID int64) string {
	return s.MintAnonymousFor(artifactID, ownerID, TTL)
}

// MintFor is Mint with an explicit lifetime: for a caller whose horizon is
// known to be shorter than TTL, and for tests, which need an already-expired
// token (a non-positive d) without waiting minutes to get one.
func (s *Signer) MintFor(artifactID string, ownerID int64, d time.Duration) string {
	return s.mint(artifactID, Claims{OwnerID: ownerID}, d)
}

// MintAnonymousFor is MintAnonymous with an explicit lifetime.
func (s *Signer) MintAnonymousFor(artifactID string, ownerID int64, d time.Duration) string {
	return s.mint(artifactID, Claims{OwnerID: ownerID, Anonymous: true}, d)
}

func (s *Signer) mint(artifactID string, c Claims, d time.Duration) string {
	// A positive request can never outlive TTL — that ceiling is the whole
	// security property this package documents (see the package comment). A
	// non-positive d is left alone: tests rely on it to mint an
	// already-expired token.
	if d > TTL {
		d = TTL
	}
	claims := fmt.Sprintf("%d.%d", c.OwnerID, time.Now().Add(d).Unix())
	if c.Anonymous {
		claims += "." + anonymousClaim
	}
	return claims + "." + s.tag(artifactID, claims)
}

// Verify checks tok against artifactID and returns what it claims.
//
// artifactID comes from the URL the render surface is answering, and is mixed
// into the MAC rather than carried in the token. So a token minted for artifact
// A simply fails to verify on artifact B's route — the scoping is the signature
// itself, not a field a verifier could forget to compare.
func (s *Signer) Verify(tok, artifactID string) (Claims, error) {
	// The tag is the last field, so everything before it is the signed message
	// whatever it contains. Adding a claim therefore changes what is parsed,
	// never what is authenticated — and a base64url tag cannot contain the
	// separator, so the cut is unambiguous.
	i := strings.LastIndexByte(tok, '.')
	if i < 0 {
		return Claims{}, ErrInvalid
	}
	claims, sig := tok[:i], tok[i+1:]
	if !hmac.Equal([]byte(sig), []byte(s.tag(artifactID, claims))) {
		return Claims{}, ErrInvalid
	}
	// Parsed only after the MAC checks out, so nothing downstream ever acts on
	// unauthenticated numbers.
	owner, exp, anonymous, ok := splitClaims(claims)
	if !ok {
		return Claims{}, ErrInvalid
	}
	id, err := strconv.ParseInt(owner, 10, 64)
	if err != nil {
		return Claims{}, ErrInvalid
	}
	expUnix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return Claims{}, ErrInvalid
	}
	if time.Now().After(time.Unix(expUnix, 0)) {
		return Claims{}, ErrExpired
	}
	return Claims{OwnerID: id, Anonymous: anonymous}, nil
}

// splitClaims cuts "owner.exp" or "owner.exp.a" into its fields. An unknown
// trailing field is rejected rather than ignored: a signed message this version
// cannot fully read is one it must not act on half of.
func splitClaims(claims string) (owner, exp string, anonymous, ok bool) {
	owner, rest, found := strings.Cut(claims, ".")
	if !found {
		return "", "", false, false
	}
	exp, extra, found := strings.Cut(rest, ".")
	if !found {
		return owner, exp, false, true
	}
	if extra != anonymousClaim {
		return "", "", false, false
	}
	return owner, exp, true, true
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

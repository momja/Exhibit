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
// verifier, one algorithm, and a claim set this package defines in full, so a
// signature-suite negotiation would be pure attack surface.
package rendertoken

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
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
//
// It stays inside the MAC and out of the wire (av-6axy). A wire version buys
// exactly one thing — two verifiers running side by side through a format
// rollout — and the TTL above already buys it: cut the format over and every
// token minted under the old one ages out within ten minutes. What a wire
// version costs is a field every reader has to branch on forever, for a
// rollout that lasts an afternoon.
const version = "v1"

// The claim keys. Claims travel as `name=value` pairs joined by '.', which is
// the whole of what this encoding buys over the positional `owner.exp[.a]` it
// replaced (av-6axy): a claim can be added without the fields beside it
// changing meaning. Positionally, a third optional field is ambiguous the
// moment a second one exists — a reader seeing one trailing field has to guess
// which claim it is — and that guess is made about a message that grants
// authority.
const (
	// claimOwner is the owner whose artifact this is. Required. It authorizes
	// the read: the render surface checks it against the artifact's own owner
	// and makes every owner-scoped Store call under it.
	claimOwner = "o"
	// claimExpiry is the deadline, unix seconds. Required.
	claimExpiry = "e"
	// claimViewer is the principal whose state rows the document inlines.
	// Absent means "the owner", which is what keeps the common case — the two
	// being the same person — at exactly two claims.
	claimViewer = "p"
	// claimAnonymous marks a document rendered for nobody in particular — a
	// visitor reading a public instance's library with no credential of their
	// own (av-wmp6).
	//
	// It is a claim rather than a query parameter because it *subtracts*
	// authority: the render surface inlines no state for an anonymous viewer
	// and its shim persists none. A parameter would be a privilege the viewer
	// could drop by editing the URL; inside the MAC it is the issuer's
	// statement, not the bearer's.
	//
	// It is its own key rather than claimViewer=0 for the same reason. A zero
	// principal is one careless `if viewerID == 0 { viewerID = ownerID }` away
	// from silently promoting a nobody to the owner, and that failure is
	// invisible from outside: the document renders, just with somebody's
	// private state in it. A key that has to be *absent* to mean "not
	// anonymous" has no falsy value to be confused about.
	claimAnonymous = "a"
	// claimShare is the share row this render happens under. Absent means the
	// render is not a share.
	claimShare = "s"
)

// anonymousValue is the only value claimAnonymous may carry. A boolean claim
// with one accepted spelling has exactly one byte string, and nothing has to
// decide what `a=0` or `a=false` mean — they are not tokens this package
// issues, so they do not verify.
const anonymousValue = "1"

var (
	// ErrInvalid covers every "this is not a token I issued for this artifact"
	// case — malformed, wrong artifact, bad signature. They are deliberately
	// one error: distinguishing them for the caller would only help an attacker
	// distinguish them too.
	ErrInvalid = errors.New("rendertoken: invalid token")
	// ErrExpired is separate because it is the one failure a legitimate client
	// hits, and the fix (mint a new one) is different.
	ErrExpired = errors.New("rendertoken: token expired")
	// ErrUnmintable is the mint-side counterpart of ErrInvalid: a claim set
	// this package refuses to serialize. It is returned rather than repaired,
	// because every way of repairing one issues a credential the caller did not
	// ask for.
	ErrUnmintable = errors.New("rendertoken: unmintable claims")
)

// Claims is what a verified token says: whose artifact is being rendered, for
// whom, and under what.
//
// OwnerID and ViewerID are two principals, not one value read twice, and the
// split is the one store.OwnerID / store.ViewerID already draw over the state
// rows (av-q0ub). OwnerID *authorizes* the read — it is checked against the
// artifact's owner, and it is the owner every owner-scoped Store call in the
// render path is made under. ViewerID *selects* whose rows the document
// inlines. They name the same person on every route that exists today, and
// they are different people the moment an artifact is shared with somebody
// else (av-7k7b): the recipient reaches the owner's artifact and reads their
// own state inside it. Collapsing them back into one field is not a
// simplification, it is a cross-tenant read waiting for the next route.
//
// Anonymous is the answer neither principal can express: a viewer with no
// identity at all. It is not ViewerID == 0 — see claimAnonymous — and a token
// carrying both it and a principal is rejected rather than resolved, because a
// claim that subtracts authority must not be negotiable with one that adds it.
//
// The zero value is deliberately useless: it authorizes nothing (owner 0 owns
// no artifact) and selects nobody's rows. The direction that matters is which
// way each end fails. A caller that hands Claims straight to the render surface
// — the share route is the one that does — and leaves ViewerID unset inlines
// *nobody's* state, never somebody else's. On the mint side an unset ViewerID
// is the wire's own default, "the owner", which is what keeps the ordinary
// token two claims wide; and Verify never hands back an unresolved one, so no
// reader downstream has a zero to interpret.
type Claims struct {
	// OwnerID is the owner whose artifact this token renders. It authorizes
	// the read.
	OwnerID int64
	// ViewerID is whose state rows the document inlines. Verify fills it from
	// OwnerID when the token carries no principal of its own, so no reader has
	// to know which spelling was on the wire.
	ViewerID int64
	// Anonymous marks a viewer with no identity — a public instance's
	// unauthenticated visitor. The artifact renders; nobody's state does. It
	// leaves ViewerID zero, because there is no principal to name.
	Anonymous bool
	// ShareID is the share row this render happens under, empty when it is not
	// a share. Nothing reads it yet: it is carried so the share routes av-7k7b
	// adds can tell "opened as a recipient" from "opened as the owner" out of
	// the credential itself, rather than from a parameter the recipient could
	// edit.
	ShareID string
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
	return mustMint(s.mint(artifactID, Claims{OwnerID: ownerID}, d))
}

// MintAnonymousFor is MintAnonymous with an explicit lifetime.
func (s *Signer) MintAnonymousFor(artifactID string, ownerID int64, d time.Duration) string {
	return mustMint(s.mint(artifactID, Claims{OwnerID: ownerID, Anonymous: true}, d))
}

// MintClaims returns a token carrying an arbitrary claim set, for what the
// four fixed constructors above do not cover — a share opened by its
// recipient, where the principal is not the owner.
//
// It is the fallible entry point because it is the only one that can be handed
// a value this package cannot serialize: the share id is free-form text, where
// every other claim is a number or a fixed byte. The constructors above build
// claim sets that cannot fail, which is why they still return a bare string
// rather than pushing an impossible error onto every call site.
func (s *Signer) MintClaims(artifactID string, c Claims) (string, error) {
	return s.MintClaimsFor(artifactID, c, TTL)
}

// MintClaimsFor is MintClaims with an explicit lifetime.
func (s *Signer) MintClaimsFor(artifactID string, c Claims, d time.Duration) (string, error) {
	return s.mint(artifactID, c, d)
}

func (s *Signer) mint(artifactID string, c Claims, d time.Duration) (string, error) {
	// A positive request can never outlive TTL — that ceiling is the whole
	// security property this package documents (see the package comment). A
	// non-positive d is left alone: tests rely on it to mint an
	// already-expired token.
	if d > TTL {
		d = TTL
	}
	claims, err := encodeClaims(c, time.Now().Add(d).Unix())
	if err != nil {
		return "", err
	}
	return claims + "." + s.tag(artifactID, claims), nil
}

// mustMint unwraps a mint whose claim set carries no free-form value and
// therefore cannot fail to encode. It exists so the four fixed constructors
// keep the signature their call sites already use. It panics rather than
// returning "" because an empty token is a credential-shaped hole that would
// surface far from here, as a 404 nobody could explain.
func mustMint(tok string, err error) string {
	if err != nil {
		panic("rendertoken: " + err.Error())
	}
	return tok
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
	c, expUnix, ok := parseClaims(claims)
	if !ok {
		return Claims{}, ErrInvalid
	}
	if time.Now().After(time.Unix(expUnix, 0)) {
		return Claims{}, ErrExpired
	}
	return c, nil
}

// encodeClaims serializes a claim set at a fixed expiry.
//
// The field order is fixed rather than incidental, and that is the point: one
// claim set has exactly one byte string, so two tokens are the same token if
// and only if they are the same bytes. It is written out as a sequence in the
// order the claim set is documented (o, e, p, a, s) rather than sorted at
// runtime — the property wanted is a canonical order, and a sort is a second
// place the order could quietly change.
//
// The two rules a caller can actually violate are checked here, at the single
// point every mint path passes through:
//
//   - A principal beside the anonymous claim is refused, not resolved. It is
//     the contradiction parseClaims rejects, refused at the end that could
//     still ask what was meant.
//   - No value may contain '.' or '=', the two bytes the encoding is made of.
//     Enforced rather than assumed: the share id is the one free-form value,
//     and a share id with a dot in it would mint a token that parses as two
//     claims, one of them unknown — so the credential would simply stop
//     working, at the far end, for reasons nothing logs.
func encodeClaims(c Claims, exp int64) (string, error) {
	if c.Anonymous && c.ViewerID != 0 {
		return "", ErrUnmintable
	}
	if c.ShareID != "" && !validValue(c.ShareID) {
		return "", ErrUnmintable
	}

	fields := make([]string, 0, 5)
	fields = append(fields, claimOwner+"="+strconv.FormatInt(c.OwnerID, 10))
	fields = append(fields, claimExpiry+"="+strconv.FormatInt(exp, 10))
	// Omitted when the viewer is the owner, and when the caller named no
	// viewer at all — the two spellings of the wire's default. That is what
	// "absent means the owner" is worth: the ordinary token, which is every
	// token any route mints today, carries no principal, so adding the claim
	// to the package cost the common case not one byte.
	if c.ViewerID != 0 && c.ViewerID != c.OwnerID {
		fields = append(fields, claimViewer+"="+strconv.FormatInt(c.ViewerID, 10))
	}
	if c.Anonymous {
		fields = append(fields, claimAnonymous+"="+anonymousValue)
	}
	if c.ShareID != "" {
		fields = append(fields, claimShare+"="+c.ShareID)
	}
	return strings.Join(fields, "."), nil
}

// parseClaims reads a signed claims string back, returning the claims and the
// expiry. It answers ok=false for anything it cannot read *in full* — a signed
// message this version half understands is one it must not act on half of.
//
// Three rejections carry the weight:
//
//   - A duplicate key. `o=1.o=2` must not mean whatever a last-wins reader
//     would make of it; the two obvious readers disagree, so neither is
//     allowed to be right.
//   - An unknown key. Fail closed, unlike JWT's ignore-what-you-don't-know: a
//     future claim that subtracts authority would otherwise be dropped here
//     and the subtraction would silently disappear.
//   - The anonymous claim beside a principal, in either order. Rejected rather
//     than resolved, because there is no reading of "for nobody, as user 7"
//     safe enough to guess at.
func parseClaims(s string) (Claims, int64, bool) {
	var (
		c          Claims
		exp        int64
		viewer     int64
		haveOwner  bool
		haveExpiry bool
		haveViewer bool
		seen       = make(map[string]bool, 5)
		err        error
	)
	// Split rather than a cut-until-empty loop, so an empty field is a field
	// and not a loop that quietly ended: "o=1.e=2." would otherwise parse as
	// the same claim set as "o=1.e=2", and one claim set is allowed exactly
	// one byte string.
	for _, field := range strings.Split(s, ".") {
		key, value, found := strings.Cut(field, "=")
		// A field with no '=' is not a claim; an empty key names nothing; and
		// a value carrying a separator cannot have survived the split intact,
		// so it is not a string this package produced, whatever the tag says.
		if !found || key == "" || !validValue(value) {
			return Claims{}, 0, false
		}
		if seen[key] {
			return Claims{}, 0, false
		}
		seen[key] = true

		switch key {
		case claimOwner:
			c.OwnerID, err = strconv.ParseInt(value, 10, 64)
			haveOwner = true
		case claimExpiry:
			exp, err = strconv.ParseInt(value, 10, 64)
			haveExpiry = true
		case claimViewer:
			viewer, err = strconv.ParseInt(value, 10, 64)
			haveViewer = true
		case claimAnonymous:
			if value != anonymousValue {
				return Claims{}, 0, false
			}
			c.Anonymous = true
		case claimShare:
			c.ShareID = value
		default:
			return Claims{}, 0, false
		}
		if err != nil {
			return Claims{}, 0, false
		}
	}
	if !haveOwner || !haveExpiry {
		return Claims{}, 0, false
	}
	if c.Anonymous && haveViewer {
		return Claims{}, 0, false
	}
	// "Absent means the owner", applied once, here, so no reader downstream
	// has to know the claim was optional — and so nobody writes the
	// zero-is-really-the-owner branch claimAnonymous exists to avoid. An
	// anonymous token keeps ViewerID at zero: it names no principal, and
	// filling one in would invent exactly what the claim removed.
	switch {
	case c.Anonymous:
		c.ViewerID = 0
	case haveViewer:
		c.ViewerID = viewer
	default:
		c.ViewerID = c.OwnerID
	}
	return c, exp, true
}

// validValue is the one rule every claim value obeys: non-empty, and made of
// no byte the encoding itself uses. Checked on both sides — a mint that can
// produce a value a parse must reject is a credential that fails at the far
// end, minutes later, for reasons the mint site never sees.
func validValue(v string) bool {
	return v != "" && !strings.ContainsAny(v, ".=")
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

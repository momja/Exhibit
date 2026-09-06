package rendertoken

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/secrets"
)

func TestMintedTokenVerifiesAndCarriesTheOwner(t *testing.T) {
	s := NewRandomSigner()

	c, err := s.Verify(s.Mint("artifact-1", 7), "artifact-1")
	if err != nil {
		t.Fatalf("a freshly minted token must verify: %v", err)
	}
	if c.OwnerID != 7 {
		t.Fatalf("owner = %d, want 7", c.OwnerID)
	}
	if c.Anonymous {
		t.Fatal("a token minted for an owner must not read as anonymous")
	}
	// The ordinary token names no principal, so the viewer is the owner. This
	// is the default every reader downstream relies on: it means "whose state
	// do I inline" has an answer without anyone checking whether the claim was
	// on the wire.
	if c.ViewerID != 7 {
		t.Fatalf("viewer = %d, want the owner (7)", c.ViewerID)
	}
}

// av-wmp6. The anonymous claim is what a public instance mints for a visitor
// with no credential, and the render surface subtracts state on the strength of
// it — so it has to survive the round trip intact, and the owner beside it must
// still be the owner (it is what authorizes the read).
func TestAnonymousTokenVerifiesAndSaysSo(t *testing.T) {
	s := NewRandomSigner()

	c, err := s.Verify(s.MintAnonymous("artifact-1", 7), "artifact-1")
	if err != nil {
		t.Fatalf("a freshly minted anonymous token must verify: %v", err)
	}
	if c.OwnerID != 7 {
		t.Fatalf("owner = %d, want 7", c.OwnerID)
	}
	if !c.Anonymous {
		t.Fatal("an anonymous token must verify as anonymous")
	}
	// And it names no principal at all. The owner beside it authorizes the
	// read; it must never become the viewer, or "render for nobody" would
	// inline the owner's private state into a stranger's document.
	if c.ViewerID != 0 {
		t.Fatalf("viewer = %d, want nobody (0) on an anonymous token", c.ViewerID)
	}
}

// The claim subtracts authority, so the interesting forgery is removing it: a
// visitor who could turn their own anonymous token into an identified one would
// have the owner's state inlined into their document. It is inside the MAC, so
// neither adding nor removing it survives — and the plain token likewise cannot
// be aged into an anonymous one, which keeps the two flavours from being
// interchangeable in either direction.
func TestTheAnonymousClaimCannotBeAddedOrRemoved(t *testing.T) {
	s := NewRandomSigner()

	anon := s.MintAnonymous("artifact-1", 1)
	// Strip the claim, keeping the tag: "o=1.e=X.a=1.tag" -> "o=1.e=X.tag".
	parts := strings.Split(anon, ".")
	if len(parts) != 4 || parts[2] != claimAnonymous+"="+anonymousValue {
		t.Fatalf("unexpected anonymous token shape %q", anon)
	}
	promoted := parts[0] + "." + parts[1] + "." + parts[3]
	if _, err := s.Verify(promoted, "artifact-1"); err == nil {
		t.Fatal("stripping the anonymous claim verified")
	}

	// And the other direction: appending it to an identified token.
	plain := s.Mint("artifact-1", 1)
	p := strings.Split(plain, ".")
	demoted := p[0] + "." + p[1] + "." + claimAnonymous + "=" + anonymousValue + "." + p[2]
	if _, err := s.Verify(demoted, "artifact-1"); err == nil {
		t.Fatal("appending the anonymous claim verified")
	}
}

// signed tags an arbitrary claims string, producing a token that genuinely is
// this signer's. It is how the parser gets tested at all: a hand-edited token
// dies at the MAC, so the only way to ask "what would Verify do with these
// claims" is to sign them first. Everything it builds is a message the signer
// would have to be broken to emit — which is the point, because the parser is
// the second line and has to hold on its own.
func signed(s *Signer, artifactID, claims string) string {
	return claims + "." + s.tag(artifactID, claims)
}

// futureExpiry is the exp claim of a token that has not expired, so a parse
// rejection under test is never the expiry rejection wearing its clothes.
func futureExpiry() string {
	return claimExpiry + "=" + strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)
}

// The parser's refusals (av-6axy), each closing a specific failure. They are
// one table because they share the property that matters: the MAC passes and
// the token is still rejected, so none of them is being caught by the
// signature and left quietly untested.
//
// A claim this version cannot read is one it must not act on half of — that is
// the through-line. Without it a future version's token would verify here as
// an ordinary identified one, which for any claim that subtracts authority (as
// the anonymous one does) means the subtraction silently disappears.
func TestMalformedClaimSetsAreRejected(t *testing.T) {
	s := NewRandomSigner()
	exp := futureExpiry()

	for _, tc := range []struct {
		name   string
		claims string
	}{
		// The duplicate key is the load-bearing rejection: last-wins and
		// first-wins readers disagree about who this token names, so neither
		// is allowed to be right.
		{"duplicate owner", "o=1." + exp + ".o=2"},
		{"duplicate expiry", "o=1." + exp + "." + exp},
		{"duplicate principal", "o=1." + exp + ".p=7.p=8"},
		// Fail closed on the unknown, unlike JWT.
		{"unknown key", "o=1." + exp + ".z=1"},
		{"unknown key beside known ones", "o=1." + exp + ".p=7.w=1"},
		// "For nobody, as user 7" has no safe reading, in either order.
		{"anonymous and a principal", "o=1." + exp + ".p=7.a=1"},
		{"a principal and anonymous", "o=1." + exp + ".a=1.p=7"},
		// A value carrying a separator cannot have survived the split, so it
		// is not a string this package produced, whatever the tag says.
		{"'=' in a value", "o=1." + exp + ".s=shr=abc"},
		{"'.' in a value", "o=1." + exp + ".s=shr.abc"},
		// The required claims are required.
		{"no owner", exp},
		{"no expiry", "o=1"},
		{"nothing at all", ""},
		// And the encoding's own edges.
		{"field with no value", "o=1." + exp + ".s"},
		{"empty value", "o=1." + exp + ".s="},
		{"empty key", "o=1." + exp + ".=1"},
		{"trailing separator", "o=1." + exp + "."},
		{"non-numeric owner", "o=one." + exp},
		// The anonymous claim has exactly one spelling, so nothing has to
		// decide what a falsy one means.
		{"anonymous with another value", "o=1." + exp + ".a=0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tok := signed(s, "artifact-1", tc.claims)
			if _, err := s.Verify(tok, "artifact-1"); !errors.Is(err, ErrInvalid) {
				t.Fatalf("claims %q must be invalid, got %v", tc.claims, err)
			}
		})
	}
}

// The artifact id is mixed into the MAC rather than carried as a field, so a
// token for one artifact cannot be replayed on another. This is the property
// the whole design rests on: it is why an artifact reading its own token out of
// location.href gains nothing.
func TestTokenDoesNotVerifyForADifferentArtifact(t *testing.T) {
	s := NewRandomSigner()
	tok := s.Mint("artifact-1", 1)

	if _, err := s.Verify(tok, "artifact-2"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a token for another artifact must be invalid, got %v", err)
	}
}

func TestTokenDoesNotVerifyUnderADifferentKey(t *testing.T) {
	tok := NewRandomSigner().Mint("artifact-1", 1)

	if _, err := NewRandomSigner().Verify(tok, "artifact-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a token from another key must be invalid, got %v", err)
	}
}

func TestExpiredTokenIsRejectedAsExpired(t *testing.T) {
	s := NewRandomSigner()

	if _, err := s.Verify(s.MintFor("artifact-1", 1, -time.Second), "artifact-1"); !errors.Is(err, ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

// The claims are authenticated, not merely encoded: an owner id edited in the
// URL must not survive verification. Without this, the token would be a
// self-service owner-selection form.
func TestTamperedClaimsAreRejected(t *testing.T) {
	s := NewRandomSigner()
	tok := s.Mint("artifact-1", 1)

	parts := strings.SplitN(tok, ".", 3)
	if len(parts) != 3 {
		t.Fatalf("unexpected token shape %q", tok)
	}
	for _, bad := range []string{
		"o=99." + parts[1] + "." + parts[2],              // promoted owner
		parts[0] + ".e=99999999999." + parts[2],          // extended deadline
		parts[0] + "." + parts[1] + ".p=99." + parts[2],  // borrowed principal
		parts[0] + "." + parts[1] + ".AAAA",              // forged tag
		parts[0] + "." + parts[1],                        // truncated
		"",                                               // absent
		"garbage",                                        // unparseable
		parts[0] + "." + parts[1] + "." + parts[2] + "x", // padded tag
	} {
		if _, err := s.Verify(bad, "artifact-1"); err == nil {
			t.Fatalf("tampered token %q verified", bad)
		}
	}
}

// The signing key comes from the same server secret that seals agent provider
// keys, so an operator configures one secret rather than two — and the same
// secret must reproduce the same key across restarts, or every open page's
// frames would break on deploy.
func TestKeyDerivedFromServerSecretIsStableAndDomainSeparated(t *testing.T) {
	a, err := secrets.Load("server-secret", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := secrets.Load("server-secret", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := secrets.Load("a-different-secret", "")
	if err != nil {
		t.Fatal(err)
	}

	tok := NewSigner(a.DeriveKey(KeyPurpose)).Mint("artifact-1", 1)
	if _, err := NewSigner(b.DeriveKey(KeyPurpose)).Verify(tok, "artifact-1"); err != nil {
		t.Fatalf("the same server secret must derive the same signing key: %v", err)
	}
	if _, err := NewSigner(other.DeriveKey(KeyPurpose)).Verify(tok, "artifact-1"); err == nil {
		t.Fatal("a different server secret must not verify")
	}
	// Domain separation: another purpose is another key, so a token can never
	// be confused with anything else derived from the same secret.
	if _, err := NewSigner(a.DeriveKey("some/other/purpose")).Verify(tok, "artifact-1"); err == nil {
		t.Fatal("a different purpose must derive a different key")
	}
}

// The TTL is the other half of what makes a URL-borne credential acceptable.
// Minutes, not hours: long enough for a page's frames to load, short enough
// that a captured token is close to worthless.
func TestTTLStaysShort(t *testing.T) {
	if TTL > 15*time.Minute {
		t.Fatalf("render token TTL grew to %v; the scope is meant to stay narrow", TTL)
	}
}

// A caller asking for longer than TTL still gets a token that expires no
// later than TTL — the ceiling is enforced at mint, not left to caller
// discipline. Non-positive durations (needed to mint an already-expired
// token for TestExpiredTokenIsRejected-style tests) are left untouched.
func TestMintForCapsRequestedDurationAtTTL(t *testing.T) {
	s := NewRandomSigner()

	before := time.Now()
	tok := s.MintFor("artifact-1", 1, 100*time.Hour)
	claims, err := s.Verify(tok, "artifact-1")
	if err != nil {
		t.Fatalf("unexpected verify error: %v", err)
	}
	_ = claims

	i := strings.LastIndexByte(tok, '.')
	exp := parseExpiry(t, tok[:i])
	if exp.After(before.Add(TTL + time.Second)) {
		t.Fatalf("token requested for 100h expires at %v, more than TTL past mint", exp)
	}

	// Non-positive still yields an already-expired token.
	expired := s.MintFor("artifact-1", 1, -time.Minute)
	if _, err := s.Verify(expired, "artifact-1"); !errors.Is(err, ErrExpired) {
		t.Fatalf("expected ErrExpired for a non-positive duration, got %v", err)
	}
}

// parseExpiry pulls the exp claim out of an unverified claims string, purely
// to check the mint-time ceiling in TestMintForCapsRequestedDurationAtTTL.
func parseExpiry(t *testing.T, claims string) time.Time {
	t.Helper()
	_, exp, ok := parseClaims(claims)
	if !ok {
		t.Fatalf("malformed claims: %q", claims)
	}
	return time.Unix(exp, 0)
}

// The ordinary token — every token any route mints today — is exactly two
// claims and a tag, and adding the principal to the package cost it not one
// byte (av-6axy AC#1). "Absent means the owner" is what buys that: the claim
// the whole ticket exists for is simply not on the wire in the common case.
//
// The wire is checked here literally, rather than only through a round trip,
// because the format is a contract with the routes av-7k7b adds next and with
// anyone reading a token out of a log. A round trip would pass just as happily
// on an encoding nobody meant.
func TestOrdinaryTokenIsTwoClaimsAndATag(t *testing.T) {
	s := NewRandomSigner()

	tok := s.Mint("artifact-1", 7)
	fields := strings.Split(tok, ".")
	if len(fields) != 3 {
		t.Fatalf("token %q: want owner, expiry and tag, got %d fields", tok, len(fields))
	}
	if fields[0] != "o=7" {
		t.Fatalf("first claim = %q, want o=7", fields[0])
	}
	if !strings.HasPrefix(fields[1], "e=") {
		t.Fatalf("second claim = %q, want an e= expiry", fields[1])
	}
	if _, err := strconv.ParseInt(strings.TrimPrefix(fields[1], "e="), 10, 64); err != nil {
		t.Fatalf("expiry %q is not unix seconds: %v", fields[1], err)
	}
}

// Naming the owner as the principal is the same claim set as naming nobody, so
// it is the same bytes: one claim set has exactly one encoding, which is what
// makes "the same token" and "the same bytes" the same question. It is also
// the concrete form of AC#1 — a caller that starts passing ViewerID
// explicitly changes nothing on the wire.
func TestAPrincipalEqualToTheOwnerIsNotOnTheWire(t *testing.T) {
	s := NewRandomSigner()

	explicit, err := s.MintClaims("artifact-1", Claims{OwnerID: 7, ViewerID: 7})
	if err != nil {
		t.Fatalf("minting an owner-as-principal token: %v", err)
	}
	// Compare the claims rather than the whole token: two mints a second apart
	// carry different expiries, which is not what this is about.
	if got, want := claimsOf(explicit), claimsOf(s.Mint("artifact-1", 7)); got != want {
		t.Fatalf("claims = %q, want the ordinary token's %q", got, want)
	}
	if strings.Contains(explicit, "."+claimViewer+"=") {
		t.Fatalf("token %q carries a principal claim it did not need", explicit)
	}
}

// The claim the ticket exists for: on a directed share the owner authorizes
// the read and somebody else's rows are the ones inlined, so both principals
// have to survive the round trip and stay distinguishable at the far end.
func TestPrincipalTravelsSeparatelyFromTheOwner(t *testing.T) {
	s := NewRandomSigner()

	tok, err := s.MintClaims("artifact-1", Claims{OwnerID: 1, ViewerID: 7})
	if err != nil {
		t.Fatalf("minting a principal token: %v", err)
	}
	if !strings.Contains(tok, "."+claimViewer+"=7.") {
		t.Fatalf("token %q does not carry the principal", tok)
	}

	c, err := s.Verify(tok, "artifact-1")
	if err != nil {
		t.Fatalf("a freshly minted principal token must verify: %v", err)
	}
	if c.OwnerID != 1 {
		t.Fatalf("owner = %d, want 1 — the owner authorizes the read", c.OwnerID)
	}
	if c.ViewerID != 7 {
		t.Fatalf("viewer = %d, want 7 — the principal selects the state rows", c.ViewerID)
	}
	if c.Anonymous {
		t.Fatal("a token naming a principal must not read as anonymous")
	}
}

// The share claim records what a render is happening under. Nothing reads it
// yet; it round-trips now so that av-7k7b's routes can rely on it being in the
// credential rather than in a parameter the recipient could edit.
func TestShareClaimTravels(t *testing.T) {
	s := NewRandomSigner()

	tok, err := s.MintClaims("artifact-1", Claims{OwnerID: 1, ViewerID: 7, ShareID: "shr_abc"})
	if err != nil {
		t.Fatalf("minting a share token: %v", err)
	}
	c, err := s.Verify(tok, "artifact-1")
	if err != nil {
		t.Fatalf("a freshly minted share token must verify: %v", err)
	}
	if c.ShareID != "shr_abc" {
		t.Fatalf("share = %q, want shr_abc", c.ShareID)
	}
	if c.OwnerID != 1 || c.ViewerID != 7 {
		t.Fatalf("claims = (owner %d, viewer %d), want (1, 7)", c.OwnerID, c.ViewerID)
	}
}

// What mint refuses to produce. Both refusals mirror a parse rejection, at the
// only end that can still ask what was meant: a token minted here and rejected
// there fails minutes later, in another process, for reasons the mint site
// never sees.
func TestMintRefusesClaimSetsVerifyWouldReject(t *testing.T) {
	s := NewRandomSigner()

	for _, tc := range []struct {
		name   string
		claims Claims
	}{
		// The a/p contradiction, refused where it is still a caller's mistake.
		{"anonymous with a principal", Claims{OwnerID: 1, ViewerID: 7, Anonymous: true}},
		// The share id is the one free-form value, so it is the one that can
		// carry a separator into the encoding.
		{"share id with a '.'", Claims{OwnerID: 1, ShareID: "shr.abc"}},
		{"share id with an '='", Claims{OwnerID: 1, ShareID: "shr=abc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tok, err := s.MintClaims("artifact-1", tc.claims)
			if !errors.Is(err, ErrUnmintable) {
				t.Fatalf("want ErrUnmintable, got token %q and err %v", tok, err)
			}
			if tok != "" {
				t.Fatalf("a refused mint must return no token, got %q", tok)
			}
		})
	}
}

// claimsOf cuts the tag off a token, leaving the signed message. It is
// LastIndexByte for the same reason Verify uses it: the tag is the last field,
// so the cut stays unambiguous however many claims precede it.
func claimsOf(tok string) string {
	i := strings.LastIndexByte(tok, '.')
	if i < 0 {
		return tok
	}
	return tok[:i]
}

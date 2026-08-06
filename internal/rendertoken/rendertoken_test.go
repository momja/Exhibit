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
	// Strip the claim, keeping the tag: "owner.exp.a.tag" -> "owner.exp.tag".
	parts := strings.Split(anon, ".")
	if len(parts) != 4 || parts[2] != anonymousClaim {
		t.Fatalf("unexpected anonymous token shape %q", anon)
	}
	promoted := parts[0] + "." + parts[1] + "." + parts[3]
	if _, err := s.Verify(promoted, "artifact-1"); err == nil {
		t.Fatal("stripping the anonymous claim verified")
	}

	// And the other direction: appending it to an identified token.
	plain := s.Mint("artifact-1", 1)
	p := strings.Split(plain, ".")
	demoted := p[0] + "." + p[1] + "." + anonymousClaim + "." + p[2]
	if _, err := s.Verify(demoted, "artifact-1"); err == nil {
		t.Fatal("appending the anonymous claim verified")
	}
}

// A claim this version cannot read is one it must not act on half of: an
// unknown trailing field is invalid, not ignored. Without this, a future
// version's token would verify here as an ordinary identified one — which for
// any claim that subtracts authority (as the anonymous one does) means the
// subtraction silently disappears.
func TestAnUnknownClaimIsRejected(t *testing.T) {
	s := NewRandomSigner()

	claims := "1." + strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10) + ".z"
	tok := claims + "." + s.tag("artifact-1", claims)
	if _, err := s.Verify(tok, "artifact-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("an unknown claim must be invalid, got %v", err)
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
		"99." + parts[1] + "." + parts[2],     // promoted owner
		parts[0] + ".99999999999." + parts[2], // extended deadline
		parts[0] + "." + parts[1] + ".AAAA",   // forged tag
		parts[0] + "." + parts[1],             // truncated
		"",                                    // absent
		"garbage",                             // unparseable
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

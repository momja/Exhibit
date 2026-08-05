package rendertoken

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/secrets"
)

func TestMintedTokenVerifiesAndCarriesTheOwner(t *testing.T) {
	s := NewRandomSigner()

	owner, err := s.Verify(s.Mint("artifact-1", 7), "artifact-1")
	if err != nil {
		t.Fatalf("a freshly minted token must verify: %v", err)
	}
	if owner != 7 {
		t.Fatalf("owner = %d, want 7", owner)
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

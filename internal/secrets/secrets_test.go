package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripWithEnvSecret(t *testing.T) {
	b, err := Load("hunter2", "")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Encrypt("sk-ant-verysecret")
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "sk-ant-verysecret" {
		t.Fatal("ciphertext equals plaintext")
	}
	plain, err := b.Decrypt(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "sk-ant-verysecret" {
		t.Fatalf("round trip mismatch: %q", plain)
	}
}

func TestGeneratedKeyFilePersists(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "secret.key")
	b1, err := Load("", keyFile)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b1.Encrypt("payload")
	if err != nil {
		t.Fatal(err)
	}
	// A second Load must read the same key back and decrypt successfully.
	b2, err := Load("", keyFile)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := b2.Decrypt(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "payload" {
		t.Fatalf("round trip mismatch: %q", plain)
	}
	info, err := os.Stat(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	b1, _ := Load("secret-a", "")
	b2, _ := Load("secret-b", "")
	sealed, err := b1.Encrypt("payload")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b2.Decrypt(sealed); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}

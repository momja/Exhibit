// Package secrets provides at-rest encryption for small secrets (the BYO
// agent API keys, Exh-ky6e). AES-256-GCM under a single server secret: either
// EXHIBIT_SECRET from the environment, or a random key generated once and
// persisted next to the database. The threat model is the key at rest — the
// artifact sandbox already cannot reach the app origin, so a leaked database
// or blob volume should not leak provider keys in the clear.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Box encrypts and decrypts small secrets with a fixed 256-bit key.
type Box struct {
	key [32]byte
}

// Load builds a Box from envSecret when non-empty (any string, hashed to 256
// bits), otherwise from a random key persisted at keyFile (created 0600 on
// first use).
func Load(envSecret, keyFile string) (*Box, error) {
	b := &Box{}
	if envSecret != "" {
		b.key = sha256.Sum256([]byte(envSecret))
		return b, nil
	}
	raw, err := os.ReadFile(keyFile)
	if errors.Is(err, os.ErrNotExist) {
		raw = make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("generate server secret: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(keyFile), 0o755); err != nil {
			return nil, fmt.Errorf("create secret dir: %w", err)
		}
		if err := os.WriteFile(keyFile, raw, 0o600); err != nil {
			return nil, fmt.Errorf("persist server secret: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read server secret: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("server secret %s: want 32 bytes, got %d", keyFile, len(raw))
	}
	copy(b.key[:], raw)
	return b, nil
}

// Encrypt seals plaintext and returns base64(nonce || ciphertext).
func (b *Box) Encrypt(plaintext string) (string, error) {
	gcm, err := b.gcm()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt.
func (b *Box) Decrypt(encoded string) (string, error) {
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}
	gcm, err := b.gcm()
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	plain, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(plain), nil
}

func (b *Box) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(b.key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

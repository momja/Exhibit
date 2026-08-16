package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/crypto/bcrypt"
)

// The decoy hash exists so an absent password costs what a present one
// costs (VerifyStoredPassword). That only holds if its cost factor tracks
// whatever HashPassword actually uses — pin it here so the two cannot drift
// apart silently.
func TestDecoyPasswordHashCostMatchesRealHashes(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(decoyPasswordHash))
	require.NoError(t, err)
	assert.Equal(t, bcrypt.DefaultCost, cost)
}

func TestVerifyStoredPasswordRejectsEmptyHash(t *testing.T) {
	assert.False(t, VerifyStoredPassword("", "anything"))
}

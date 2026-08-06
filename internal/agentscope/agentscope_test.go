package agentscope

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssuedTokensAreDistinctAndResolveToTheirScope(t *testing.T) {
	reg := NewRegistry()

	a, err := reg.Issue(1, "artifact-a")
	require.NoError(t, err)
	b, err := reg.Issue(1, "artifact-b")
	require.NoError(t, err)

	assert.NotEqual(t, a.Token(), b.Token())
	assert.True(t, strings.HasPrefix(a.Token(), "exagent_"))
	assert.Equal(t, "artifact-a", reg.Resolve(a.Token()).Scope().ArtifactID)
	assert.Equal(t, "artifact-b", reg.Resolve(b.Token()).Scope().ArtifactID)
}

func TestUnknownAndEmptyTokensResolveToNothing(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Issue(1, "artifact-a")
	require.NoError(t, err)

	assert.Nil(t, reg.Resolve(""))
	assert.Nil(t, reg.Resolve("exagent_not-a-real-token"))
}

// A create-mode grant binds once. The scope only ever narrows, so a session
// cannot walk from artifact to artifact by creating more of them.
func TestBindArtifactOnlyNarrows(t *testing.T) {
	reg := NewRegistry()
	g, err := reg.Issue(1, "")
	require.NoError(t, err)
	require.Empty(t, g.Scope().ArtifactID)

	g.BindArtifact("")
	assert.Empty(t, g.Scope().ArtifactID, "an empty id is not a binding")

	g.BindArtifact("first")
	g.BindArtifact("second")
	assert.Equal(t, "first", g.Scope().ArtifactID)
}

// A grant issued for an artifact is already bound and cannot be rebound.
func TestBoundGrantIgnoresRebinding(t *testing.T) {
	reg := NewRegistry()
	g, err := reg.Issue(1, "artifact-a")
	require.NoError(t, err)

	g.BindArtifact("artifact-b")
	assert.Equal(t, "artifact-a", g.Scope().ArtifactID)
}

// A credential must not outlive the session it belongs to.
func TestRevokeRetiresTheToken(t *testing.T) {
	reg := NewRegistry()
	g, err := reg.Issue(1, "artifact-a")
	require.NoError(t, err)
	require.NotNil(t, reg.Resolve(g.Token()))

	reg.Revoke(g)
	assert.Nil(t, reg.Resolve(g.Token()))
	reg.Revoke(g)   // idempotent: kill() and finish() both call it
	reg.Revoke(nil) // and a nil grant is not a panic
}

// The API middleware resolves a scope on every request while a create handler
// may be binding one; both run concurrently under -race.
func TestGrantIsSafeForConcurrentUse(t *testing.T) {
	reg := NewRegistry()
	g, err := reg.Issue(1, "")
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); g.BindArtifact("artifact-a") }()
		go func() { defer wg.Done(); reg.Resolve(g.Token()).Scope() }()
	}
	wg.Wait()
	assert.Equal(t, "artifact-a", g.Scope().ArtifactID)
}

package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentStateAccessDefaultIsOffUntilSet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	allowed, err := s.AgentStateAccessDefault(ctx, 1)
	require.NoError(t, err)
	assert.False(t, allowed, "an owner who never answered keeps saved data from agents")

	require.NoError(t, s.SetAgentStateAccessDefault(ctx, 1, true))
	allowed, err = s.AgentStateAccessDefault(ctx, 1)
	require.NoError(t, err)
	assert.True(t, allowed)

	other, err := s.AgentStateAccessDefault(ctx, 2)
	require.NoError(t, err)
	assert.False(t, other, "one owner's answer is not another's")

	require.NoError(t, s.SetAgentStateAccessDefault(ctx, 1, false))
	allowed, err = s.AgentStateAccessDefault(ctx, 1)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestArtifactAgentStateAccessDefaultsToInheritAndIsValidated(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "a", OwnerID: 1, Title: "t", SourceBlobID: "b", Tier: Tier1}))

	a, err := s.GetArtifact(ctx, 1, "a")
	require.NoError(t, err)
	assert.Equal(t, AgentStateInherit, a.AgentStateAccess)

	require.NoError(t, s.UpdateArtifact(ctx, 1, "a", map[string]any{"agent_state_access": AgentStateAllow}))
	a, err = s.GetArtifact(ctx, 1, "a")
	require.NoError(t, err)
	assert.Equal(t, AgentStateAllow, a.AgentStateAccess)

	for _, bad := range []any{"yes", "", true, 1} {
		assert.Error(t, s.UpdateArtifact(ctx, 1, "a", map[string]any{"agent_state_access": bad}), "%v", bad)
	}
}

func TestAgentStateAllowedResolvesOverrideBeforeDefault(t *testing.T) {
	for _, tc := range []struct {
		access         string
		accountDefault bool
		want           bool
	}{
		{AgentStateInherit, false, false},
		{AgentStateInherit, true, true},
		{AgentStateAllow, false, true},
		{AgentStateAllow, true, true},
		{AgentStateDeny, false, false},
		{AgentStateDeny, true, false},
		{"", true, false},
		{"unknown", true, false},
	} {
		a := &Artifact{AgentStateAccess: tc.access}
		assert.Equal(t, tc.want, a.AgentStateAllowed(tc.accountDefault), "%q with default %t", tc.access, tc.accountDefault)
	}
}

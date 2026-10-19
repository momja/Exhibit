package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnclaimedPreparationExpiresWithoutBrowserCleanup(t *testing.T) {
	m := newLimitedManager(t)
	s := createSession(t, m, 7)
	token := s.grant.Token()
	require.True(t, s.prepared)
	require.NotNil(t, s.preparationTimer)
	s.expirePreparation()
	assert.Nil(t, m.Get(7, s.ID))
	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("unclaimed sidecar was not killed")
	}
	assert.Nil(t, m.cfg.Credentials.Resolve(token))
}

func TestAcceptedPromptClaimsServerPreparationButGuardRefusalDoesNot(t *testing.T) {
	m := newLimitedManager(t)
	accepted := createSession(t, m, 7)
	accepted.promptLanded("the accepted prompt")
	assert.False(t, accepted.prepared)
	assert.Nil(t, accepted.preparationTimer)
	accepted.expirePreparation()
	assert.NotNil(t, m.Get(7, accepted.ID), "normal idle lifetime applies after acceptance")

	refused := createSession(t, m, 7)
	timer := refused.preparationTimer
	for range 3 {
		refused.guardBlocked = true
		refused.promptLanded("blocked")
	}
	assert.Same(t, timer, refused.preparationTimer, "refused requests never renew the deadline")
	assert.True(t, refused.prepared)
	refused.expirePreparation()
	assert.Nil(t, m.Get(7, refused.ID))
}

func TestPreparationDeadlineWaitsForPromptAdmission(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(map[bool]string{true: "accepted", false: "refused"}[allowed], func(t *testing.T) {
			m := newLimitedManager(t)
			s := createSession(t, m, 7)
			s.promptInFlight = true
			s.expirePreparation()
			assert.True(t, s.preparationExpired)
			require.NotNil(t, m.Get(7, s.ID), "a Send being screened survives its preparation deadline")
			s.guardBlocked = !allowed
			s.promptLanded("submitted near expiry")
			s.finishPromptAdmission()
			if allowed {
				assert.NotNil(t, m.Get(7, s.ID))
				assert.False(t, s.prepared)
			} else {
				assert.Nil(t, m.Get(7, s.ID))
			}
		})
	}
}

func TestAgentStartClaimsPreparationWithoutWaitingForPromptResponse(t *testing.T) {
	m := newLimitedManager(t)
	s := createSession(t, m, 7)
	s.handleLine([]byte(`{"type":"agent_start"}`))
	assert.False(t, s.prepared)
	assert.Nil(t, s.preparationTimer)
	s.expirePreparation()
	assert.NotNil(t, m.Get(7, s.ID))
}

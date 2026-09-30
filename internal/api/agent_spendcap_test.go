package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The startup rule (av-99f4): absence of the feature is unlimited, absence of
// a limit the feature requires is a refusal to boot. A platform credential
// with no cap at all must not start — superseding av-siqf's warning, which was
// the right answer only while no cap existed to require.
func TestPlatformModeWithoutACapDoesNotBoot(t *testing.T) {
	pk := &PlatformKey{Provider: "exhibit-mock", APIKey: "k"}

	err := RequireSpendCaps(pk, agent.SpendCaps{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AGENT_SPEND_CAP", "the failure names the variables that would fix it")

	assert.NoError(t, RequireSpendCaps(nil, agent.SpendCaps{}),
		"a BYOK instance with no caps is today's behaviour, unchanged")

	assert.NoError(t, RequireSpendCaps(pk, agent.SpendCaps{TurnSeconds: 60}),
		"any ceiling is a cap — the layers fail differently by design")

	var ownerBudget int64 = 1
	assert.NoError(t, RequireSpendCaps(pk, agent.SpendCaps{OwnerMicrosPerMonth: &ownerBudget}))
}

// The pre-spawn refusal end to end through the route both session creators
// share: an owner over their budget never gets a subprocess, and the message
// names the limit and when it resets.
func TestASessionIsRefusedBeforeSpawnWhenTheOwnerIsOverBudget(t *testing.T) {
	r := newTestRouter(t)
	enablePlatformMode(r)

	ownerBudget := int64(1_000_000)
	caps := agent.SpendCaps{OwnerMicrosPerMonth: &ownerBudget}
	mgr, err := agent.New(agent.Config{
		PiBin:       "pi", // never spawned: the refusal below is before Create
		WorkRoot:    t.TempDir(),
		APIBaseURL:  "http://app.test",
		Credentials: agentscope.NewRegistry(),
		Caps:        caps,
	}, r.cfg.Store)
	require.NoError(t, err)
	r.cfg.Agent = mgr

	// defaultOwnerID (1) is who a service-token request resolves to, and it
	// is over the budget below.
	require.NoError(t, r.cfg.Store.RecordAgentUsage(context.Background(), store.AgentUsage{
		OwnerID: 1, SessionID: "earlier", Provider: "anthropic", Model: "m",
		PaidBy: store.PaidByPlatform, Source: store.UsageSourceModel,
		InputTokens: 1, OutputTokens: 1, CostMicros: 1_200_000,
	}))

	w := doJSON(t, r, "POST", "/api/agent/sessions", map[string]any{})
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "refused before spawn, not after")
	assert.Contains(t, w.Body.String(), "monthly agent budget")
	assert.Contains(t, w.Body.String(), "resets on", "the message names when the limit resets")
}

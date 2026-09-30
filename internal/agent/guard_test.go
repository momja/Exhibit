package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardrailFromEnv(t *testing.T) {
	set := func(provider, model, key string) {
		t.Setenv("GUARDRAIL_PROVIDER", provider)
		t.Setenv("GUARDRAIL_MODEL", model)
		t.Setenv("GUARDRAIL_API_KEY", key)
	}

	t.Run("unset is no guardrail", func(t *testing.T) {
		set("", "", "  ")
		g, err := GuardrailFromEnv()
		require.NoError(t, err)
		assert.Nil(t, g)
	})

	t.Run("all three set", func(t *testing.T) {
		set(" typesafe ", "jev-latest", "k")
		g, err := GuardrailFromEnv()
		require.NoError(t, err)
		assert.Equal(t, &Guardrail{Provider: "typesafe", Model: "jev-latest", APIKey: "k"}, g)
	})

	t.Run("partly set names what is missing", func(t *testing.T) {
		set("typesafe", "", "")
		_, err := GuardrailFromEnv()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "GUARDRAIL_MODEL")
		assert.Contains(t, err.Error(), "GUARDRAIL_API_KEY")
		assert.NotContains(t, err.Error(), "GUARDRAIL_PROVIDER")
	})
}

func TestGuardSignalOf(t *testing.T) {
	verb, detail, ok := guardSignalOf([]byte(`{"type":"extension_ui_request","id":"1","method":"notify","message":"exhibit_guard:blocked p(violates)=0.910","notifyType":"warning"}`))
	assert.True(t, ok)
	assert.Equal(t, "blocked", verb)
	assert.Equal(t, "p(violates)=0.910", detail)

	verb, detail, ok = guardSignalOf([]byte(`{"type":"extension_ui_request","method":"notify","message":"exhibit_guard:usage {\"model\":\"m\"}"}`))
	assert.True(t, ok)
	assert.Equal(t, "usage", verb)
	assert.Equal(t, `{"model":"m"}`, detail)

	for _, line := range []string{
		`{"type":"extension_ui_request","method":"notify","message":"something else"}`,
		`{"type":"extension_ui_request","method":"setStatus","message":"exhibit_guard:blocked"}`,
		`{"type":"message_update","message":"exhibit_guard:blocked"}`,
		`not json`,
	} {
		_, _, ok := guardSignalOf([]byte(line))
		assert.False(t, ok, line)
	}
}

// Only a block or an error is a refusal. A usage report arrives after every
// screen, allowed ones included, and must neither mark the prompt blocked nor
// reach the browser — but it is the meter's input (av-2yws): one guardrail
// row per screen, tagged as operator spend.
func TestGuardUsageSignalIsMeteredNotABlock(t *testing.T) {
	s, db := newMeteringSession(t, false)
	s.handleGuardSignal("usage", `{"provider":"p","model":"m","usage":{"input":10,"output":2,"cost":{"total":0.001}}}`)
	assert.False(t, s.guardBlocked)
	assert.Empty(t, s.backlog)

	assert.Equal(t, int64(0), sessionTotals(t, db, s).CostMicros,
		"screening is not one conversation's spend — the session ceiling never sees it")
	instance, err := db.InstanceAgentSpend(context.Background(), time.Time{})
	require.NoError(t, err)
	assert.Equal(t, int64(1_000), instance.CostMicros,
		"operator money whatever key the session runs on — here a BYO one")
	assert.Equal(t, int64(0), ownerTotals(t, db).CostMicros, "and never the owner's budget")

	s.handleGuardSignal("blocked", "p(violates)=0.99")
	assert.True(t, s.guardBlocked)
	require.Len(t, s.backlog, 1)
	assert.Contains(t, string(s.backlog[0]), "exhibit_guard_blocked")
}

// The guard extension's env names the fence the system prompt promises, so
// its cut and composePrompt's blocks agree.
func TestGuardrailEnvCarriesTheDataFence(t *testing.T) {
	g := &Guardrail{Provider: "p", Model: "m", APIKey: "k"}
	assert.Contains(t, g.env("n0nce"), "EXHIBIT_GUARD_DATA_FENCE="+beginFence("n0nce"))
}

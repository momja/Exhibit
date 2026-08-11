package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The registry's owner predicate, on its own.
//
// The routes above it are pinned end to end in internal/api against a real pi
// subprocess, but the property being relied on is this one: the *lookup* is
// owner-scoped, so a handler cannot reach another owner's session even by
// forgetting to check. This is the in-memory counterpart of what av-ep8k did in
// SQL, and the reason it is a parameter rather than a caller's `if`.
func TestManagerGetIsOwnerScoped(t *testing.T) {
	m := &Manager{sessions: map[string]*Session{
		"s1": {ID: "s1", OwnerID: 1},
		"s2": {ID: "s2", OwnerID: 2},
	}}

	require.NotNil(t, m.Get(1, "s1"))
	require.NotNil(t, m.Get(2, "s2"))

	// Another owner's live session reads back exactly as an id that was never
	// issued — that identical nil is what lets the API answer both with the same
	// 404 rather than becoming an oracle over which session ids exist.
	assert.Nil(t, m.Get(2, "s1"))
	assert.Nil(t, m.Get(1, "s2"))
	assert.Nil(t, m.Get(1, "never-issued"))

	// Owner 0 is a request nobody attributed (api.noOwner). It matches nothing
	// here for the same reason it matches no row: owner ids start at 1.
	assert.Nil(t, m.Get(0, "s1"))
}

// Close is scoped too, and more sharply than Get: it kills a subprocess, so an
// unscoped id would be a one-request denial of service against anyone whose
// session id leaked.
func TestManagerCloseIgnoresAnotherOwnersSession(t *testing.T) {
	m := &Manager{sessions: map[string]*Session{"s1": {ID: "s1", OwnerID: 1}}}

	m.Close(2, "s1")
	assert.NotNil(t, m.Get(1, "s1"), "owner 2 closed owner 1's session")

	m.Close(1, "never-issued")
	assert.NotNil(t, m.Get(1, "s1"))
}

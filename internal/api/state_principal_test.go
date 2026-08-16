package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/momja/Exhibit/internal/store"
)

// av-q0ub, through the routes. The store tests fix the invariant; these fix
// that every state route actually carries the principal down to it, since a
// route that forgot to would leak or clobber without any store change.

// otherViewer is a principal that is not the authenticated session. Rows are
// planted for it through the Store directly, because no route mints a non-owner
// viewer yet — a shared artifact opened by someone else (av-7k7b) is what will.
// Planting them is how the routes get asked the question early.
const otherViewer int64 = 42

// AC#2: two viewers' state on one artifact neither collides nor is readable
// across, through any API route. Every state route is exercised against a
// planted foreign row, and the row must be there, unchanged, at the end of
// each one.
func TestStateRoutesNeverTouchAnotherViewersRows(t *testing.T) {
	ctx := context.Background()

	// Each subtest gets its own router so one route's damage can't be mistaken
	// for another's.
	setup := func(t *testing.T) (*Router, string) {
		t.Helper()
		r := newTestRouter(t)
		id := createTestArtifact(t, r, "Shared")
		// The session's own row and the other viewer's row share a key, which
		// is the collision the primary key has to admit.
		putState(t, r, id, "draft", "the session's")
		require.NoError(t, r.cfg.Store.SetState(ctx, store.OwnerID(defaultOwnerID), id, store.ViewerID(otherViewer), "draft", "not the session's"))
		require.NoError(t, r.cfg.Store.SetState(ctx, store.OwnerID(defaultOwnerID), id, store.ViewerID(otherViewer), "private", "nor this"))
		return r, id
	}

	assertForeignRowsIntact := func(t *testing.T, r *Router, id string) {
		t.Helper()
		foreign, err := r.cfg.Store.GetState(ctx, store.OwnerID(defaultOwnerID), id, store.ViewerID(otherViewer))
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"draft":   "not the session's",
			"private": "nor this",
		}, foreign, "no state route may reach another viewer's rows")
	}

	t.Run("GET returns only the session's rows", func(t *testing.T) {
		r, id := setup(t)

		assert.Equal(t, map[string]string{"draft": "the session's"}, getState(t, r, id),
			"a read must not return the union of every viewer's state")
		assertForeignRowsIntact(t, r, id)
	})

	t.Run("PUT writes only the session's row", func(t *testing.T) {
		r, id := setup(t)

		putState(t, r, id, "draft", "rewritten")

		assert.Equal(t, "rewritten", getState(t, r, id)["draft"])
		assertForeignRowsIntact(t, r, id)
	})

	t.Run("DELETE of one key removes only the session's row", func(t *testing.T) {
		r, id := setup(t)

		w := doJSON(t, r, "DELETE", "/api/artifacts/"+id+"/state?key=draft", nil)
		require.Equal(t, http.StatusNoContent, w.Code)

		_, present := getState(t, r, id)["draft"]
		assert.False(t, present, "the session's own key must be gone")
		assertForeignRowsIntact(t, r, id)
	})

	t.Run("erase-all erases only the session's rows", func(t *testing.T) {
		r, id := setup(t)

		w := doJSON(t, r, "DELETE", "/api/artifacts/"+id+"/state", nil)
		require.Equal(t, http.StatusNoContent, w.Code)

		assert.Empty(t, getState(t, r, id), "the session's state must be gone")
		assertForeignRowsIntact(t, r, id)
	})

	// The foreign viewer's key is not merely filtered out of the response —
	// it is unreachable. A key only they hold reads as absent, exactly like a
	// key nobody ever wrote.
	t.Run("a foreign key is indistinguishable from an absent one", func(t *testing.T) {
		r, id := setup(t)

		state := getState(t, r, id)
		_, foreignKey := state["private"]
		_, neverWritten := state["no-such-key"]
		assert.Equal(t, neverWritten, foreignKey,
			"another viewer's key must read exactly like one that was never stored")
	})
}

// AC#5, through the routes: the property the whole state design exists for.
// "Two devices" is two independent HTTP requests carrying the same session —
// there is nothing device-shaped in the request, and adding a principal must
// not have quietly invented one. If this fails, cross-device sync is broken and
// the feature is pointless.
func TestCrossDeviceSyncForOneUserIsUnchanged(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run tracker")

	// iPhone: the host frame writes through on the shim's behalf.
	putState(t, r, id, "runs", `[{"km":5}]`)

	// Mac: a separate request, same session, no shared client state.
	assert.Equal(t, map[string]string{"runs": `[{"km":5}]`}, getState(t, r, id),
		"a second device must read what the first wrote")

	// Mac writes back; the iPhone sees the update rather than a second,
	// device-local copy.
	putState(t, r, id, "runs", `[{"km":5},{"km":8}]`)
	assert.Equal(t, map[string]string{"runs": `[{"km":5},{"km":8}]`}, getState(t, r, id),
		"last write wins over one shared row")

	// Deletes cross devices too — the failure this guards against is a
	// removeItem that lands in one device's rows and leaves the other's, so
	// the key resurrects on the next load.
	w := doJSON(t, r, "DELETE", "/api/artifacts/"+id+"/state?key=runs", nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, getState(t, r, id), "a delete on one device must be gone on the other")
}

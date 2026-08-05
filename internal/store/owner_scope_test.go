package store

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-ep8k: owner_id is a real predicate. These tests fix the invariant the
// whole ticket exists for — **another owner's artifact id is
// indistinguishable from an id that does not exist** — across every Store
// method that names an artifact.
//
// The table is the point. AC#4 asks that a method added without owner
// coverage be visible in review, so a new artifact-scoped method that isn't
// listed here also trips TestEveryArtifactScopedMethodTakesAnOwner below.

const (
	alice int64 = 1
	bob   int64 = 2
)

// putOwnedArtifact seeds one artifact plus its state, origin decision,
// transcript, share, tag and collection, so every method under test has
// something to find when it is allowed to — and something to leave alone
// when it isn't.
func putOwnedArtifact(t *testing.T, s *SQLiteStore, owner int64, id string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{
		ID: id, OwnerID: owner, Title: id, SourceBlobID: "blob-" + id, Tier: Tier1,
		NetworkAllowlist: []string{"https://seed.example.com"},
		SourceText:       id + " seedsearchterm",
	}))
	require.NoError(t, s.SetState(ctx, owner, id, "seed", "value"))
	require.NoError(t, s.SaveTranscript(ctx, owner, id, "session-"+id, `[{"role":"user"}]`))
	require.NoError(t, s.CreateShare(ctx, owner, &Share{ID: "share-" + id, ArtifactID: id, Public: true}))
	require.NoError(t, s.CreateTag(ctx, &Tag{ID: "tag-" + id, OwnerID: owner, Name: "tag-" + id}))
	require.NoError(t, s.CreateCollection(ctx, &Collection{ID: "col-" + id, OwnerID: owner, Name: "col-" + id}))
}

// denial is how a method reports "not yours". All three shapes are also what
// the method reports for an id that never existed — which is the invariant.
type denial int

const (
	denyEmptyRead   denial = iota // reads: (nil, nil) or an empty collection
	denyErrNotFound               // writes: store.ErrNotFound, never a 403
	denySilentNoop                // idempotent deletes: no error, and no effect
)

// ownerCase is one Store method exercised twice: once by the artifact's owner
// (must work) and once by the other owner (must be denied in deny's shape).
type ownerCase struct {
	name string
	deny denial
	// run performs the operation as owner against artifactID, reporting
	// whether a read came back empty.
	run func(ctx context.Context, s *SQLiteStore, owner int64, artifactID string) (empty bool, err error)
}

func ownerCases() []ownerCase {
	return []ownerCase{
		{"GetArtifact", denyEmptyRead, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			a, err := s.GetArtifact(ctx, o, id)
			return a == nil, err
		}},
		{"UpdateArtifact", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.UpdateArtifact(ctx, o, id, map[string]any{"title": "rewritten"})
		}},
		{"UpdateArtifact/allowlist-only", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.UpdateArtifact(ctx, o, id, map[string]any{"network_allowlist": []string{"https://evil.example.com"}})
		}},
		{"DeleteArtifact", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.DeleteArtifact(ctx, o, id)
		}},
		{"ListOriginDecisions", denyEmptyRead, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			d, err := s.ListOriginDecisions(ctx, o, id)
			return len(d) == 0, err
		}},
		{"AllowedOrigins", denyEmptyRead, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			origins, err := s.AllowedOrigins(ctx, o, id)
			return len(origins) == 0, err
		}},
		{"SetOriginDecision", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.SetOriginDecision(ctx, o, id, "https://evil.example.com", DecisionAllow, "user")
		}},
		{"DeleteOriginDecision", denySilentNoop, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.DeleteOriginDecision(ctx, o, id, "https://seed.example.com")
		}},
		{"ReplaceAllowedOrigins", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.ReplaceAllowedOrigins(ctx, o, id, []string{"https://evil.example.com"}, "user")
		}},
		{"AddArtifactToCollection", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.AddArtifactToCollection(ctx, o, id, "col-"+id)
		}},
		{"RemoveArtifactFromCollection", denySilentNoop, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			_ = s.AddArtifactToCollection(ctx, ownerOf(id), id, "col-"+id)
			return false, s.RemoveArtifactFromCollection(ctx, o, id, "col-"+id)
		}},
		{"AddArtifactTag", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.AddArtifactTag(ctx, o, id, "tag-"+id)
		}},
		{"RemoveArtifactTag", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			// Attach first as the rightful owner so the detach has something
			// to find; the denial must then be about the owner, not absence.
			_ = s.AddArtifactTag(ctx, ownerOf(id), id, "tag-"+id)
			return false, s.RemoveArtifactTag(ctx, o, id, "tag-"+id)
		}},
		{"GetState", denyEmptyRead, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			st, err := s.GetState(ctx, o, id)
			return len(st) == 0, err
		}},
		{"SetState", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.SetState(ctx, o, id, "planted", "by the wrong owner")
		}},
		{"DeleteState", denySilentNoop, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.DeleteState(ctx, o, id, "seed")
		}},
		{"ClearState", denySilentNoop, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.ClearState(ctx, o, id)
		}},
		{"SaveTranscript", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.SaveTranscript(ctx, o, id, "planted-session", `[{"role":"user"}]`)
		}},
		{"ListTranscripts", denyEmptyRead, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			ts, err := s.ListTranscripts(ctx, o, id)
			return len(ts) == 0, err
		}},
		{"CreateShare", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.CreateShare(ctx, o, &Share{ID: "planted-share-" + id, ArtifactID: id, Public: true})
		}},
		{"GetShare", denyEmptyRead, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			sh, err := s.GetShare(ctx, o, "share-"+id)
			return sh == nil, err
		}},
		{"DeleteShare", denyErrNotFound, func(ctx context.Context, s *SQLiteStore, o int64, id string) (bool, error) {
			return false, s.DeleteShare(ctx, o, "share-"+id)
		}},
	}
}

// ownerOf maps a fixture artifact id back to its owner, so a case can set
// itself up as the rightful owner before testing the denial.
func ownerOf(artifactID string) int64 {
	if artifactID == "bobs" {
		return bob
	}
	return alice
}

// TestArtifactScopedMethodsAllowTheOwner is the control: every case succeeds
// for the owner. Without it, a store that denied *everyone* would pass the
// cross-tenant test below.
func TestArtifactScopedMethodsAllowTheOwner(t *testing.T) {
	for _, tc := range ownerCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			putOwnedArtifact(t, s, alice, "alices")

			empty, err := tc.run(ctx, s, alice, "alices")
			require.NoError(t, err, "the owner's own call must succeed")
			assert.False(t, empty, "the owner must see their own rows")
		})
	}
}

// TestArtifactScopedMethodsDenyAnotherOwner is the ticket's core assertion:
// Bob's id, used by Alice, behaves exactly like an id that isn't there.
func TestArtifactScopedMethodsDenyAnotherOwner(t *testing.T) {
	for _, tc := range ownerCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			putOwnedArtifact(t, s, alice, "alices")
			putOwnedArtifact(t, s, bob, "bobs")

			empty, err := tc.run(ctx, s, alice, "bobs")

			switch tc.deny {
			case denyEmptyRead:
				require.NoError(t, err, "a foreign id is absent, not an error")
				assert.True(t, empty, "another owner's rows must read as absent")
			case denyErrNotFound:
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrNotFound),
					"a cross-tenant write must report ErrNotFound (404), never a permission "+
						"error: a 403 confirms the row exists and makes the API a membership "+
						"oracle over artifact ids; got %v", err)
			case denySilentNoop:
				require.NoError(t, err, "these deletes are idempotent by contract")
			}

			// Whatever the shape of the denial, Bob's library is untouched.
			assertBobIntact(t, s)

			// And the *same* call against an id that never existed behaves
			// identically — which is what "indistinguishable" means.
			emptyGhost, errGhost := tc.run(ctx, s, alice, "no-such-artifact")
			assert.Equal(t, empty, emptyGhost, "foreign id and unknown id must read alike")
			assert.Equal(t, errors.Is(err, ErrNotFound), errors.Is(errGhost, ErrNotFound),
				"foreign id and unknown id must fail alike")
		})
	}
}

// assertBobIntact checks the victim's rows survived whatever Alice attempted.
func assertBobIntact(t *testing.T, s *SQLiteStore) {
	t.Helper()
	ctx := context.Background()

	a, err := s.GetArtifact(ctx, bob, "bobs")
	require.NoError(t, err)
	require.NotNil(t, a, "the artifact itself must survive")
	assert.Equal(t, "bobs", a.Title, "the title must not have been rewritten")
	assert.Equal(t, []string{"https://seed.example.com"}, a.NetworkAllowlist,
		"the allowlist is the CSP's input — no other owner may widen it")
	// Some cases legitimately attach Bob's own tag as setup, so the assertion
	// is not "no tags" but "none of Alice's".
	for _, tag := range a.Tags {
		assert.Equal(t, bob, tag.OwnerID, "no tag from another owner may be attached")
	}

	state, err := s.GetState(ctx, bob, "bobs")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"seed": "value"}, state,
		"state must be neither erased nor planted into")

	ts, err := s.ListTranscripts(ctx, bob, "bobs")
	require.NoError(t, err)
	assert.Len(t, ts, 1, "no transcript may be planted on someone else's artifact")

	sh, err := s.GetShare(ctx, bob, "share-bobs")
	require.NoError(t, err)
	assert.NotNil(t, sh, "a share may only be revoked by the artifact's owner")
}

// AC#1: one owner's library never leaks into another's listing — including
// through the FTS5 search path, which matches on text both artifacts share.
func TestListArtifactsIsOneOwnersLibrary(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putOwnedArtifact(t, s, alice, "alices")
	putOwnedArtifact(t, s, bob, "bobs")

	for _, opts := range []ListOptions{
		{OwnerID: alice},
		{OwnerID: alice, Query: "seedsearchterm"}, // both artifacts match the index
	} {
		got, err := s.ListArtifacts(ctx, opts)
		require.NoError(t, err)
		require.Len(t, got, 1, "query %q returned someone else's artifact", opts.Query)
		assert.Equal(t, "alices", got[0].ID)
	}

	// A caller that forgets OwnerID gets nothing, not everything: the zero
	// value must fail closed.
	got, err := s.ListArtifacts(ctx, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, got, "an unset OwnerID must match no owner")
}

// AC#6: the two unscoped accessors resolve across owners on purpose. They are
// asserted here so their behaviour is a decision on record, not a leftover.
func TestUnscopedAccessorsAreDeliberatelyOwnerBlind(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putOwnedArtifact(t, s, bob, "bobs")

	a, err := s.GetArtifactUnscoped(ctx, "bobs")
	require.NoError(t, err)
	require.NotNil(t, a, "the render surface has no owner in context (av-c5aq)")
	assert.Equal(t, bob, a.OwnerID, "it carries its owner, so the state read can be scoped")

	sh, err := s.GetShareUnscoped(ctx, "share-bobs")
	require.NoError(t, err)
	require.NotNil(t, sh, "the share row is the authorization (architecture §7)")

	missing, err := s.GetArtifactUnscoped(ctx, "no-such-artifact")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

// TestUnscopedAccessorsAreCalledOnlyFromTheRenderSurface is AC#6's tripwire,
// and the reason the accessors are named the way they are: the un-owner-scoped
// read surface must stay small enough to enumerate, and enumerating it must be
// a grep.
//
// The render surface is the whole of it. Closing that gap — carrying a
// principal onto RENDER_ORIGIN in a signed token — is av-c5aq's job; until it
// lands, this test is what keeps the gap from quietly growing a second call
// site somewhere nobody is looking.
func TestUnscopedAccessorsAreCalledOnlyFromTheRenderSurface(t *testing.T) {
	// Package paths permitted to call an Unscoped accessor, each with the
	// reason it has no owner to scope by.
	allowed := map[string]string{
		"internal/store":  "the definitions themselves, and the test asserting them",
		"internal/render": "RENDER_ORIGIN has no session; share reads are authorized by the share row",
	}

	var offenders []string
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// node_modules and the like carry no Go we control.
			if name := d.Name(); name == "node_modules" || name == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !strings.Contains(string(body), "Unscoped(") {
			return nil
		}
		rel := filepath.ToSlash(filepath.Dir(strings.TrimPrefix(filepath.ToSlash(path), "../../")))
		if _, ok := allowed[rel]; !ok {
			offenders = append(offenders, filepath.ToSlash(path))
		}
		return nil
	})
	require.NoError(t, err)

	assert.Empty(t, offenders,
		"an Unscoped accessor is an un-owner-scoped read. New call sites belong in "+
			"internal/render (or in this test's allowlist, with a reason) — see av-ep8k AC#6, av-c5aq")
}

// TestEveryArtifactScopedMethodTakesAnOwner is AC#4's tripwire. A Store method
// added without an ownerID parameter fails here, so the omission surfaces in
// review rather than as a cross-tenant read in production.
func TestEveryArtifactScopedMethodTakesAnOwner(t *testing.T) {
	// Each exemption carries its reason. Adding to this list is the
	// deliberate act; forgetting the parameter is not.
	exempt := map[string]string{
		"Close":               "no data in its signature at all",
		"PutArtifact":         "carries the owner in Artifact.OwnerID",
		"ListArtifacts":       "carries the owner in ListOptions.OwnerID",
		"CreateCollection":    "carries the owner in Collection.OwnerID",
		"CreateTag":           "carries the owner in Tag.OwnerID",
		"SetAgentKey":         "carries the owner in AgentKey.OwnerID",
		"GetArtifactUnscoped": "deliberate render/share exception (av-c5aq)",
		"GetShareUnscoped":    "deliberate share exception (architecture §7)",

		// Identity and sessions (av-30rj). These methods *establish* who the
		// owner is; they cannot take one as input without assuming the answer
		// to the question they exist to ask. A session id is a bearer
		// credential looked up before any owner is known, and a user row is
		// keyed by the provider's external id, not by owner_id. They are
		// exempt because they sit upstream of owner scoping, not outside it.
		"UpsertUser":            "resolves a provider identity to an owner; there is no owner yet",
		"GetUser":               "keyed by user id, which *is* the owner id",
		"CreateSession":         "session rows carry their own UserID; the caller has just authenticated",
		"GetSession":            "looks up a bearer credential before any owner is known",
		"DeleteSession":         "revocation by session id; the holder need not be resolved first",
		"DeleteExpiredSessions": "a janitor over all rows, owned by no one",
	}

	iface := reflect.TypeOf((*Store)(nil)).Elem()
	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)
		if _, ok := exempt[m.Name]; ok {
			continue
		}
		ft := m.Type
		require.Greater(t, ft.NumIn(), 1, "Store.%s takes no arguments beyond ctx", m.Name)
		assert.Equal(t, reflect.TypeOf(int64(0)), ft.In(1),
			"Store.%s must take the requesting ownerID as its first argument after ctx, "+
				"or be listed in this test's exemptions with a reason (av-ep8k AC#4)", m.Name)
	}
}

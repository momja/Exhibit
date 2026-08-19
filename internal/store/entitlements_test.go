package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-2p8z at the storage layer. What is stored is a *partial* answer — a limit
// this account has of its own, or none — and the tests below are about the
// three states of that, because the middle one (explicitly cleared, back on
// the instance default) is what a downgrade is and is the one a two-state
// encoding would have lost.

func ptr(n int64) *int64 { return &n }

func newEntitlementUser(t *testing.T, s *SQLiteStore, sub string) *User {
	t.Helper()
	u, err := s.UpsertUser(context.Background(), sub, sub+"@example.test")
	require.NoError(t, err)
	require.True(t, u.Entitlement.IsDefault(), "a new account carries no entitlement of its own")
	return u
}

func TestSetEntitlementPatchesOnlyWhatItNames(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u := newEntitlementUser(t, s, "sub-1")

	require.NoError(t, s.SetEntitlement(ctx, u.ID, EntitlementPatch{
		Plan: strPtr("household"), Ref: strPtr("acct-7"),
		SetStorageLimit: true, StorageLimitBytes: ptr(5 << 30),
	}))
	got, err := s.GetUser(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "household", got.Entitlement.Plan)
	assert.Equal(t, "acct-7", got.Entitlement.Ref)
	require.NotNil(t, got.Entitlement.StorageLimitBytes)
	assert.Equal(t, int64(5<<30), *got.Entitlement.StorageLimitBytes)

	// A patch that names one field leaves the others exactly as they were.
	// This is the property the whole three-state encoding exists to protect:
	// an external client that sends only a plan must not silently reset a
	// ceiling an admin raised by hand.
	require.NoError(t, s.SetEntitlement(ctx, u.ID, EntitlementPatch{Plan: strPtr("larger")}))
	got, err = s.GetUser(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "larger", got.Entitlement.Plan)
	assert.Equal(t, "acct-7", got.Entitlement.Ref)
	require.NotNil(t, got.Entitlement.StorageLimitBytes)

	// And a patch that names the limit with no value clears it — the account
	// goes back to the instance default rather than to zero or to unlimited.
	require.NoError(t, s.SetEntitlement(ctx, u.ID, EntitlementPatch{SetStorageLimit: true}))
	got, err = s.GetUser(ctx, u.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Entitlement.StorageLimitBytes)
	assert.Equal(t, "larger", got.Entitlement.Plan, "clearing the limit is not clearing the plan")
}

// A limit of zero is a real limit — "this account may store nothing more" —
// and is not the same as having none. Worth a test of its own because it is
// exactly the value a nil-versus-zero confusion would swallow.
func TestZeroStorageLimitIsALimitAndNotAnAbsentOne(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u := newEntitlementUser(t, s, "sub-1")

	require.NoError(t, s.SetEntitlement(ctx, u.ID, EntitlementPatch{SetStorageLimit: true, StorageLimitBytes: ptr(0)}))
	got, err := s.GetUser(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Entitlement.StorageLimitBytes)
	assert.Equal(t, int64(0), *got.Entitlement.StorageLimitBytes)
	assert.False(t, got.Entitlement.IsDefault(), "an explicit zero is an entitlement of its own")
}

func TestSetEntitlementRefusesANonsenseLimitAndAMissingAccount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u := newEntitlementUser(t, s, "sub-1")

	err := s.SetEntitlement(ctx, u.ID, EntitlementPatch{SetStorageLimit: true, StorageLimitBytes: ptr(-1)})
	assert.ErrorIs(t, err, ErrInvalidEntitlement)
	got, err := s.GetUser(ctx, u.ID)
	require.NoError(t, err)
	assert.True(t, got.Entitlement.IsDefault(), "a refused patch writes nothing")

	// An account that does not exist is ErrNotFound whatever the patch says —
	// including an empty one, so a route's refusal never depends on the shape
	// of the body it was sent.
	assert.ErrorIs(t, s.SetEntitlement(ctx, 999, EntitlementPatch{Plan: strPtr("x")}), ErrNotFound)
	assert.ErrorIs(t, s.SetEntitlement(ctx, 999, EntitlementPatch{}), ErrNotFound)
}

// The drift list, and the one thing that could quietly go wrong about it: its
// predicate is written twice — once in SQL, once as Entitlement.IsDefault —
// and the two must not come apart. Every combination of the three fields is
// checked against both.
func TestOverrideListingAgreesWithIsDefault(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		patch EntitlementPatch
	}{
		{"nothing", EntitlementPatch{}},
		{"plan only", EntitlementPatch{Plan: strPtr("household")}},
		{"limit only", EntitlementPatch{SetStorageLimit: true, StorageLimitBytes: ptr(1 << 20)}},
		{"zero limit", EntitlementPatch{SetStorageLimit: true, StorageLimitBytes: ptr(0)}},
		{"reference only", EntitlementPatch{Ref: strPtr("acct-9")}},
		{"all three", EntitlementPatch{Plan: strPtr("big"), Ref: strPtr("acct-10"), SetStorageLimit: true, StorageLimitBytes: ptr(2)}},
		{"cleared again", EntitlementPatch{Plan: strPtr(""), Ref: strPtr(""), SetStorageLimit: true}},
	}
	for _, c := range cases {
		u := newEntitlementUser(t, s, "sub-"+c.name)
		require.NoError(t, s.SetEntitlement(ctx, u.ID, c.patch), c.name)
	}

	users, err := s.ListUsers(ctx)
	require.NoError(t, err)
	overrides, err := s.ListEntitlementOverrides(ctx)
	require.NoError(t, err)

	listed := map[int64]bool{}
	for _, u := range overrides {
		listed[u.ID] = true
		assert.False(t, u.Entitlement.IsDefault(), "listed as an override but IsDefault says otherwise")
	}
	for _, u := range users {
		assert.Equal(t, !u.Entitlement.IsDefault(), listed[u.ID],
			"the SQL predicate and Entitlement.IsDefault disagree about %q", u.Email)
	}
	assert.Len(t, overrides, 5, "the empty patch and the re-cleared one are on the instance default")
}

// Entitlements are columns on `users`, which is what makes "deleting the
// account removes them" true by construction rather than by a statement
// somebody has to remember to write. Asserted anyway: the acceptance criterion
// says it, and a future migration that moved them to a table of their own
// would need to bring this with it.
func TestDeletingAnAccountTakesItsEntitlementWithIt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Two accounts, so the one being deleted is never the instance's last
	// enabled admin — DeleteAccount refuses that, and the refusal would make
	// this test pass for the wrong reason.
	keeper := newEntitlementUser(t, s, "sub-admin")
	going := newEntitlementUser(t, s, "sub-going")
	require.NoError(t, s.SetEntitlement(ctx, going.ID, EntitlementPatch{
		Plan: strPtr("household"), Ref: strPtr("acct-7"),
		SetStorageLimit: true, StorageLimitBytes: ptr(1 << 30),
	}))

	_, err := s.DeleteAccount(ctx, going.ID)
	require.NoError(t, err)

	_, err = s.GetUser(ctx, going.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	overrides, err := s.ListEntitlementOverrides(ctx)
	require.NoError(t, err)
	assert.Empty(t, overrides, "the entitlement went with the row it was a column of")

	// And a re-created identity with the same subject starts on the default —
	// users.id is AUTOINCREMENT, so this is a new account, not the old one
	// with its allowance still attached.
	again, err := s.UpsertUser(ctx, "sub-going", "sub-going@example.test")
	require.NoError(t, err)
	assert.NotEqual(t, going.ID, again.ID)
	assert.True(t, again.Entitlement.IsDefault())

	kept, err := s.GetUser(ctx, keeper.ID)
	require.NoError(t, err)
	assert.True(t, kept.Entitlement.IsDefault(), "and nobody else's entitlement moved")
}

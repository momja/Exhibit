package api

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-2p8z. Two properties are the ticket, and everything here is one of them.
//
//  1. **Fail closed on ambiguity, never on absence.** Limits switched off is
//     absence and must stay unlimited forever; an entitlement that cannot be
//     resolved is ambiguity and must refuse. The two look alike and are not.
//  2. **Never on /profile.** An entitlement a person can raise on themselves
//     is not a limit, so setting one is admin-only — and the test that says so
//     has to be structural, or the next field added to /profile erodes it.

func ptr(n int64) *int64      { return &n }
func strPtr(s string) *string { return &s }

// enforcing is an instance with limits switched on and a default configured —
// the state a hosted instance is in, and the only one in which anything here
// can refuse.
func enforcing(defaultBytes int64) Entitlements {
	return Entitlements{Enforced: true, DefaultPlan: "included", DefaultStorageBytes: defaultBytes}
}

// --- 1. configuration ---------------------------------------------------

// The startup failure the ticket asks for by name. Booting with limits on and
// no default would leave every unprovisioned account unlimited on an instance
// whose operator believes limits are in force — and a *warning* about that is
// the version that scrolls past, which is why this is an error the caller
// makes fatal.
func TestLimitsWithoutADefaultFailAtStartup(t *testing.T) {
	t.Setenv(envEntitlementsEnabled, "true")
	_, err := EntitlementsFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), envEntitlementsDefaultBytes)

	t.Setenv(envEntitlementsDefaultBytes, "1073741824")
	e, err := EntitlementsFromEnv()
	require.NoError(t, err)
	assert.True(t, e.Enforced)
	assert.Equal(t, int64(1073741824), e.DefaultStorageBytes)
}

// The default: nothing configured is limits switched off, which is every
// self-hosted instance.
func TestNoConfigurationIsLimitsSwitchedOff(t *testing.T) {
	t.Setenv(envEntitlementsEnabled, "")
	t.Setenv(envEntitlementsDefaultBytes, "")
	e, err := EntitlementsFromEnv()
	require.NoError(t, err)
	assert.Equal(t, Entitlements{}, e)

	// A default configured while the switch is off is accepted and inert:
	// it is the state an operator passes through while setting this up, and
	// refusing it would mean the two variables have to be introduced at once.
	t.Setenv(envEntitlementsDefaultBytes, "42")
	e, err = EntitlementsFromEnv()
	require.NoError(t, err)
	assert.False(t, e.Enforced)
	assert.Equal(t, int64(42), e.DefaultStorageBytes)
}

// The switch itself is strict, unlike the other boolean knob in this package.
// envBool reads an unrecognized value as off and warns, which is right when off
// is the safe direction; here off means *no limits at all*, so guessing it is
// the failure the rest of this file exists to prevent.
func TestAnUnrecognizedSwitchValueIsAStartupError(t *testing.T) {
	t.Setenv(envEntitlementsDefaultBytes, "1000")
	for _, bad := range []string{"treu", "enabled", "1.0"} {
		t.Setenv(envEntitlementsEnabled, bad)
		_, err := EntitlementsFromEnv()
		assert.Error(t, err, "%q must not be read as 'no limits at all'", bad)
	}
	// The spellings that are not typos stay silent, in both directions.
	for value, want := range map[string]bool{"": false, "false": false, "off": false, "no": false,
		"true": true, "1": true, "yes": true, "on": true} {
		t.Setenv(envEntitlementsEnabled, value)
		e, err := EntitlementsFromEnv()
		require.NoError(t, err, "%q", value)
		assert.Equal(t, want, e.Enforced, "%q", value)
	}
}

func TestAMalformedDefaultIsAStartupError(t *testing.T) {
	t.Setenv(envEntitlementsEnabled, "")
	for _, bad := range []string{"lots", "-1", "5GiB"} {
		t.Setenv(envEntitlementsDefaultBytes, bad)
		_, err := EntitlementsFromEnv()
		assert.Error(t, err, "%q should not boot as though it were configured", bad)
	}
}

// --- 2. resolution ------------------------------------------------------

// countingStore records whether the resolver went to the database at all. With
// limits switched off it must not: a self-hosted instance pays nothing for a
// feature it has not turned on, and has no failure mode from one either.
type countingStore struct {
	store.Store
	reads int
}

func (c *countingStore) GetUser(ctx context.Context, id int64) (*store.User, error) {
	c.reads++
	return c.Store.GetUser(ctx, id)
}

func TestLimitsOffResolveUnlimitedWithoutReadingARow(t *testing.T) {
	r := newTestRouter(t)
	counting := &countingStore{Store: r.cfg.Store}
	r.cfg.Store = counting

	allowance, err := r.resolveAllowance(context.Background(), 1)
	require.NoError(t, err)
	assert.True(t, allowance.Storage.Unlimited)
	assert.True(t, allowance.Storage.Allows(1<<62), "an instance with limits off refuses nothing")
	assert.Zero(t, counting.reads, "limits off reads no entitlement row")
}

// Absence is the default, and the default is a limit. The case that matters is
// owner 1 on a single-user instance, which has no `users` row and never will:
// resolving it must not error, and must not resolve to unlimited either.
func TestAnAccountWithNoRowResolvesToTheInstanceDefault(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.Entitlements = enforcing(1000)

	allowance, err := r.resolveAllowance(context.Background(), 1)
	require.NoError(t, err)
	assert.False(t, allowance.Storage.Unlimited)
	assert.Equal(t, int64(1000), allowance.Storage.Bytes)
	assert.Equal(t, "included", allowance.Plan)
	assert.True(t, allowance.Storage.Allows(1000))
	assert.False(t, allowance.Storage.Allows(1001))
}

// The per-owner half: a limit stored on the account wins over the default, and
// the plan label travels with it — but each falls back independently, because
// a label carries no limit and an account can have either without the other.
func TestAStoredEntitlementOverridesTheDefaultFieldByField(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.Entitlements = enforcing(1000)
	ctx := context.Background()

	u, err := r.cfg.Store.UpsertUser(ctx, "sub-1", "one@example.test")
	require.NoError(t, err)

	require.NoError(t, r.cfg.Store.SetEntitlement(ctx, u.ID, store.EntitlementPatch{
		SetStorageLimit: true, StorageLimitBytes: ptr(9000),
	}))
	allowance, err := r.resolveAllowance(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(9000), allowance.Storage.Bytes)
	assert.Equal(t, "included", allowance.Plan, "a limit of their own does not give them a plan label")

	require.NoError(t, r.cfg.Store.SetEntitlement(ctx, u.ID, store.EntitlementPatch{Plan: strPtr("household")}))
	allowance, err = r.resolveAllowance(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "household", allowance.Plan)
	assert.Equal(t, int64(9000), allowance.Storage.Bytes)

	// Clearing the limit is a downgrade, not a deletion of the account's
	// entitlement: back to the default, never to unlimited.
	require.NoError(t, r.cfg.Store.SetEntitlement(ctx, u.ID, store.EntitlementPatch{SetStorageLimit: true}))
	allowance, err = r.resolveAllowance(ctx, u.ID)
	require.NoError(t, err)
	assert.False(t, allowance.Storage.Unlimited)
	assert.Equal(t, int64(1000), allowance.Storage.Bytes)
}

// brokenUserStore is a store whose account read fails — the "database error"
// half of what the ticket means by an entitlement that cannot be resolved.
type brokenUserStore struct {
	store.Store
}

func (brokenUserStore) GetUser(context.Context, int64) (*store.User, error) {
	return nil, errors.New("database is on fire")
}

// nonsenseUserStore returns a row that makes no sense — the other half.
// Unreachable through the API and through the schema (migration 022 carries a
// CHECK), which is exactly why the resolver is asked about it directly: this
// is the branch that decides what happens when a row arrives some way nobody
// anticipated.
type nonsenseUserStore struct {
	store.Store
}

func (nonsenseUserStore) GetUser(_ context.Context, id int64) (*store.User, error) {
	return &store.User{ID: id, Entitlement: store.Entitlement{StorageLimitBytes: ptr(-1)}}, nil
}

func TestAnUnresolvableEntitlementRefusesRatherThanAllows(t *testing.T) {
	for name, broken := range map[string]store.Store{
		"a read that fails":         brokenUserStore{},
		"a row that makes no sense": nonsenseUserStore{},
	} {
		t.Run(name, func(t *testing.T) {
			r := newTestRouter(t)
			r.cfg.Entitlements = enforcing(1000)
			r.cfg.Store = broken

			allowance, err := r.resolveAllowance(context.Background(), 7)
			require.Error(t, err, "an entitlement that cannot be resolved is an error, not a default")

			// The Allowance beside the error refuses everything, so a caller
			// that ignores the error still fails closed. This is the whole
			// reason Limit's zero value is a ceiling of zero rather than
			// "unlimited" — the mistake has to fall in the safe direction.
			assert.False(t, allowance.Storage.Unlimited)
			assert.False(t, allowance.Storage.Allows(1), "the zero Allowance must refuse")
			assert.True(t, allowance.Storage.Allows(0))
		})
	}
}

// The same failure with limits switched *off* is not a failure at all. This is
// the distinction the ticket calls the whole design: a self-hoster who upgrades
// must never meet a ceiling they did not ask for, whatever the database is
// doing.
func TestLimitsOffAreUnaffectedByAFailingStore(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.Store = brokenUserStore{}

	allowance, err := r.resolveAllowance(context.Background(), 7)
	require.NoError(t, err)
	assert.True(t, allowance.Storage.Unlimited)
}

// --- 3. the /profile boundary ------------------------------------------

// The structural half of "never on /profile", and the reason it is written
// with a parser rather than as a request.
//
// A request-level test proves that today's /profile has no entitlement
// control. It cannot prove that tomorrow's does not, and the failure mode is
// silent and total: a person who can raise their own ceiling is not limited by
// it. So the rule is enforced on the *only* statement that writes those
// columns — Store.SetEntitlement — which may be called from admin.go and
// nowhere else in this package. admin.go is the file every route in it passes
// adminOnly.
func TestOnlyTheAdminSurfaceWritesAnEntitlement(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go") && fi.Name() != "admin.go"
	}, 0)
	require.NoError(t, err)

	var offenders []string
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetEntitlement" {
					offenders = append(offenders, fset.Position(call.Pos()).String())
				}
				return true
			})
		}
	}
	assert.Empty(t, offenders,
		"an entitlement is set from admin.go and nowhere else (av-2p8z): every route in that file passes "+
			"adminOnly, and an entitlement a person can raise on themselves is not a limit")
}

// And the request-level half: the page a person reaches with nothing but a
// session offers no way to name any of it.
func TestProfileOffersNoEntitlementControl(t *testing.T) {
	in := newAdminInstance(t)
	in.ro.cfg.Entitlements = enforcing(1000)
	require.NoError(t, in.st.SetEntitlement(context.Background(), in.member.ID, store.EntitlementPatch{
		Plan: strPtr("household"), SetStorageLimit: true, StorageLimitBytes: ptr(9000),
	}))

	w := in.do(t, "GET", "/profile", in.memberCookie, nil)
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	for _, field := range []string{"storage_limit_bytes", "entitlement_ref", "entitlement-dialog", `data-action="entitlement"`} {
		assert.NotContains(t, body, field,
			"/profile must offer no way to set an entitlement — that is /admin/users' and only /admin/users'")
	}
}

// --- 4. the admin route -------------------------------------------------

// The whole control surface, through the route an external system would use:
// one PATCH on the endpoint that already resets passwords and switches
// accounts off. No new route group and no new credential — adminOnly already
// grants the static service token full authority, which is what an out-of-tree
// client authenticates with.
func TestAnAdminSetsAnotherAccountsEntitlement(t *testing.T) {
	in := newAdminInstance(t)
	in.ro.cfg.Entitlements = enforcing(1000)

	w := in.do(t, "PATCH", userPath(in.member.ID), in.adminCookie, map[string]any{
		"plan": "household", "storage_limit_bytes": 9000, "entitlement_ref": "acct-7",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	ent := in.reload(t, in.member.ID).Entitlement
	assert.Equal(t, "household", ent.Plan)
	assert.Equal(t, "acct-7", ent.Ref)
	require.NotNil(t, ent.StorageLimitBytes)
	assert.Equal(t, int64(9000), *ent.StorageLimitBytes)

	// And it is what the owner then resolves to, through the one resolution
	// function — the round trip the ticket is actually for.
	allowance, err := in.ro.resolveAllowance(context.Background(), in.member.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(9000), allowance.Storage.Bytes)
	assert.Equal(t, "household", allowance.Plan)

	// An explicit null clears the ceiling and leaves everything else. This is
	// what a downgrade is, and it is the state a *int64 request field would
	// have made unexpressible — absent and null would both have decoded to
	// nil, so "put them back on the default" would read as "change nothing".
	w = in.do(t, "PATCH", userPath(in.member.ID), in.adminCookie, map[string]any{"storage_limit_bytes": nil})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	ent = in.reload(t, in.member.ID).Entitlement
	assert.Nil(t, ent.StorageLimitBytes)
	assert.Equal(t, "household", ent.Plan, "clearing the ceiling is not clearing the plan")
	assert.Equal(t, "acct-7", ent.Ref)
}

// A PATCH that names none of the entitlement fields leaves them exactly as
// they were, so the three admin actions that already shared this route stay
// independent of the three this ticket adds.
func TestAPatchThatNamesNoEntitlementLeavesItAlone(t *testing.T) {
	in := newAdminInstance(t)
	ctx := context.Background()
	require.NoError(t, in.st.SetEntitlement(ctx, in.member.ID, store.EntitlementPatch{
		Plan: strPtr("household"), SetStorageLimit: true, StorageLimitBytes: ptr(9000),
	}))

	w := in.do(t, "PATCH", userPath(in.member.ID), in.adminCookie, map[string]any{"is_admin": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	after := in.reload(t, in.member.ID)
	assert.True(t, after.IsAdmin)
	assert.Equal(t, "household", after.Entitlement.Plan)
	require.NotNil(t, after.Entitlement.StorageLimitBytes)
	assert.Equal(t, int64(9000), *after.Entitlement.StorageLimitBytes)
}

// A ceiling that is not a ceiling is a 400 with a sentence in it, refused
// before anything else in the request has been written — so a request that
// fails, fails before it has changed anything.
func TestANegativeCeilingIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	in := newAdminInstance(t)

	w := in.do(t, "PATCH", userPath(in.member.ID), in.adminCookie, map[string]any{
		"is_admin": true, "storage_limit_bytes": -1,
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "negative")

	after := in.reload(t, in.member.ID)
	assert.False(t, after.IsAdmin, "the role change in the same body did not land either")
	assert.True(t, after.Entitlement.IsDefault())
}

// The authority boundary, stated as the thing it prevents: an ordinary account
// raising its own ceiling. The refusal is adminOnly's 404 — the same one every
// other administrative address gives, so it says nothing about whether the
// account aimed at exists.
func TestANonAdminCannotSetAnyEntitlement(t *testing.T) {
	in := newAdminInstance(t)

	for _, target := range []int64{in.member.ID, in.admin.ID, 999} {
		w := in.do(t, "PATCH", userPath(target), in.memberCookie, map[string]any{
			"plan": "unlimited-please", "storage_limit_bytes": 1 << 40,
		})
		assert.Equal(t, http.StatusNotFound, w.Code, "target %d", target)
	}
	assert.True(t, in.reload(t, in.member.ID).Entitlement.IsDefault(),
		"a member cannot raise their own ceiling")
	assert.True(t, in.reload(t, in.admin.ID).Entitlement.IsDefault())

	// Nor can they read the directory the entitlements are listed in.
	assert.Equal(t, http.StatusNotFound,
		in.do(t, "GET", "/api/admin/users?entitlement=custom", in.memberCookie, nil).Code)
}

// The drift list. An entitlement an external system maintains can fall out of
// step with that system's view of reality, and a discrepancy nobody can list
// is one nobody can find.
func TestAccountsOnTheirOwnEntitlementCanBeListed(t *testing.T) {
	in := newAdminInstance(t)

	w := in.do(t, "GET", "/api/admin/users?entitlement=custom", in.adminCookie, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var listed []adminUserView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	assert.Empty(t, listed, "a fresh instance has everybody on the default")

	require.Equal(t, http.StatusOK, in.do(t, "PATCH", userPath(in.member.ID), in.adminCookie,
		map[string]any{"entitlement_ref": "acct-7"}).Code)

	w = in.do(t, "GET", "/api/admin/users?entitlement=custom", in.adminCookie, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	require.Len(t, listed, 1, "an entitlement of their own is what puts an account on this list")
	assert.Equal(t, in.member.ID, listed[0].ID)
	assert.True(t, listed[0].Custom)

	// Without the filter it is still the whole directory: this is a view
	// filter on the existing route, not a second listing that could disagree.
	w = in.do(t, "GET", "/api/admin/users", in.adminCookie, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	assert.Len(t, listed, 2)
}

// The page an admin actually uses says both halves: what each account is
// allowed, and which accounts are on something other than the default.
func TestTheAdminPageShowsEntitlementsAndTheOnesThatDiffer(t *testing.T) {
	in := newAdminInstance(t)
	in.ro.cfg.Entitlements = enforcing(5 << 30)
	require.NoError(t, in.st.SetEntitlement(context.Background(), in.member.ID, store.EntitlementPatch{
		Plan: strPtr("household"), SetStorageLimit: true, StorageLimitBytes: ptr(10 << 30),
	}))

	w := in.do(t, "GET", "/admin/users", in.adminCookie, nil)
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "10.0 GiB", "the account's own ceiling")
	assert.Contains(t, body, "5.0 GiB", "and the default everybody else is on")
	assert.Contains(t, body, "household")
	assert.Contains(t, body, "Accounts on their own entitlement")
	assert.Contains(t, body, `data-action="entitlement"`)
}

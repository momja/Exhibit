// Per-owner entitlements (av-2p8z): what an owner is allowed, stored as
// columns on `users` and set by an admin.
//
// This file stores an answer and stops there. It knows nothing about why an
// owner is on the plan they are on — there is no payment state anywhere in
// this repo, in any form, and whatever maintains these values on a commercial
// instance is an ordinary authenticated API client calling
// PATCH /api/admin/users/:id like every other client of the single write path.
// On a self-hosted instance the feature is the whole feature: a household can
// give one person a larger allowance than another, which is unaskable
// otherwise.
//
// Nothing here refuses anything, and nothing here resolves anything either.
// "What is this owner allowed, right now, on this instance" is one function
// (api.Router.resolveAllowance, internal/api/entitlements.go), because that
// question needs the instance's configuration as well as these rows — and
// because gates must ask one thing rather than read three columns and each
// invent their own fallback.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidEntitlement means a limit was not a limit — today, a negative byte
// ceiling. It is the caller's bad request rather than a server fault, so
// handlers map it to 400.
//
// The same rule is a CHECK constraint on the column (migration 022), which is
// belt and braces on purpose: the constraint stops a row arriving around this
// package, and this stops one arriving through it with a legible error rather
// than a raw SQLite message.
var ErrInvalidEntitlement = errors.New("not a valid entitlement")

// Entitlement is what one owner is allowed, exactly as stored.
//
// It is deliberately not the *resolved* answer — that is api.Allowance, which
// folds these values together with the instance's configured default and is
// what a gate reads. The difference is visible in StorageLimitBytes's nil,
// which means "this owner has none of their own" and is meaningless without
// knowing what the instance's default is.
type Entitlement struct {
	// Plan is a label for display and for grouping. Nothing reads a limit out
	// of it: limits are stored per owner below, so an instance can grant one
	// person more without inventing a plan for them, and renaming a plan can
	// never move anybody's ceiling.
	Plan string `json:"plan"`
	// StorageLimitBytes is this owner's ceiling on stored bytes, or nil for
	// "no limit of their own — the instance default applies".
	//
	// nil is absence, not ambiguity, and the two must not be confused: absence
	// resolves to the default, which is itself a limit, so it never resolves
	// to unlimited on an instance that has limits switched on. Unlimited is an
	// instance-wide state (limits switched off), which is why there is no
	// sentinel value here for a reader to mistake for a byte count.
	StorageLimitBytes *int64 `json:"storage_limit_bytes"`
	// Ref is an opaque string an operator's own system uses to recognize this
	// account, in the spirit of a ticket's --external-ref. Nothing parses it,
	// nothing joins on it, and no code path behaves differently because of its
	// value. It lives here rather than in that system because it is durable
	// with the account: the account outlives that system being rebuilt,
	// replaced, or dropped.
	Ref string `json:"entitlement_ref"`
}

// IsDefault reports whether this account carries no entitlement of its own,
// and is therefore on whatever the instance's default is.
//
// The comparison is against the *unset* record rather than against the
// resolved values, and that is the useful question: an external system writes
// these columns, so what it last wrote is where its drift from reality shows
// up. An account it set to the same numbers as the default still shows here,
// because "this was provisioned deliberately" is the fact worth seeing.
func (e Entitlement) IsDefault() bool {
	return e.Plan == "" && e.StorageLimitBytes == nil && e.Ref == ""
}

// EntitlementPatch is a partial update, in updateUserRequest's shape: a nil
// field is left alone.
//
// StorageLimitBytes needs two fields rather than one because it has three
// states and a pointer only carries two. SetStorageLimit false leaves the
// column as it is; true writes StorageLimitBytes, and a nil value there writes
// NULL — which is how an admin puts an account back on the instance default
// rather than merely lowering it to something.
type EntitlementPatch struct {
	Plan              *string
	Ref               *string
	SetStorageLimit   bool
	StorageLimitBytes *int64
}

// Empty reports whether the patch would change nothing, so a caller can skip
// the write rather than issue an UPDATE with no assignments.
func (p EntitlementPatch) Empty() bool {
	return p.Plan == nil && p.Ref == nil && !p.SetStorageLimit
}

// SetEntitlement applies a patch to one account.
//
// It is the only statement in the system that writes these columns, which is
// what the admin-only boundary is enforced against: a test walks package api's
// AST and fails on a call to this from anywhere but admin.go, so a later
// profile field cannot quietly acquire the ability to raise its own owner's
// ceiling (av-2p8z, /profile is emphatically not this surface).
//
// ErrNotFound for an account that does not exist — the same answer every other
// admin mutator gives, so a refusal is not an oracle over the directory.
func (s *SQLiteStore) SetEntitlement(ctx context.Context, userID int64, p EntitlementPatch) error {
	if p.Empty() {
		// Nothing to write. Still confirm the account exists, so "patch a user
		// that isn't there" is 404 whether or not the patch happened to be
		// empty — a route whose refusal depends on the body's shape is a
		// route nobody can reason about.
		_, err := s.GetUser(ctx, userID)
		return err
	}
	if p.SetStorageLimit && p.StorageLimitBytes != nil && *p.StorageLimitBytes < 0 {
		return fmt.Errorf("%w: storage limit %d is negative", ErrInvalidEntitlement, *p.StorageLimitBytes)
	}

	sets, args := []string{}, []any{}
	if p.Plan != nil {
		sets, args = append(sets, "plan = ?"), append(args, *p.Plan)
	}
	if p.SetStorageLimit {
		var value any
		if p.StorageLimitBytes != nil {
			value = *p.StorageLimitBytes
		}
		sets, args = append(sets, "storage_limit_bytes = ?"), append(args, value)
	}
	if p.Ref != nil {
		sets, args = append(sets, "entitlement_ref = ?"), append(args, *p.Ref)
	}
	args = append(args, userID)

	query := "UPDATE users SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	return s.exactlyOneRow(ctx, query, args...)
}

// ListEntitlementOverrides returns every account carrying an entitlement of
// its own, oldest first.
//
// This is the drift surface, and it is the reason it exists at all. An
// entitlement maintained by an external system can fall out of step with that
// system's view of reality — a downgrade it failed to deliver leaves somebody
// on a raised ceiling indefinitely, and nothing on this instance would ever
// mention it. Keeping them current is that system's job; *seeing* them is not,
// and a list nobody can produce is a discrepancy nobody can find.
//
// The predicate is IsDefault's, spelled in SQL. Deliberately not "resolves
// differently from the default": see IsDefault for why what was written is the
// more useful question than what it happens to work out to.
func (s *SQLiteStore) ListEntitlementOverrides(ctx context.Context) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+userColumns+` FROM users
          WHERE plan != '' OR storage_limit_bytes IS NOT NULL OR entitlement_ref != ''
          ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		var sc userScan
		if err := rows.Scan(sc.dest()...); err != nil {
			return nil, err
		}
		out = append(out, sc.user())
	}
	return out, rows.Err()
}

// userScan holds the destinations for one userColumns row and the two values
// that need converting after the scan.
//
// It exists because three queries share that projection and a fourth column
// added to it must not end up scanned by only two of them — the failure would
// be a runtime "expected N destination arguments" from whichever query nobody
// exercised. One definition, one place to extend.
type userScan struct {
	u       User
	created any
	limit   sql.NullInt64
}

// dest returns the scan destinations for userColumns, in order, followed by
// any the caller appended to the projection itself.
func (s *userScan) dest(extra ...any) []any {
	return append([]any{
		&s.u.ID, &s.u.ExternalID, &s.u.Email, &s.created, &s.u.IsAdmin,
		&s.u.HasPassword, &s.u.Disabled,
		&s.u.Entitlement.Plan, &s.limit, &s.u.Entitlement.Ref,
	}, extra...)
}

// user finishes the scanned row: the timestamp SQLite handed back in whatever
// shape the driver chose, and the nullable limit as the pointer the API shape
// uses.
func (s *userScan) user() *User {
	s.u.CreatedAt = anyToTime(s.created)
	if s.limit.Valid {
		v := s.limit.Int64
		s.u.Entitlement.StorageLimitBytes = &v
	}
	out := s.u
	return &out
}

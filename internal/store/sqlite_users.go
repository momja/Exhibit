package store

import (
	"context"
	"database/sql"
	"errors"
)

// sqlTimeLayout is the format SQLite's own datetime('now') produces. Session
// expiry is written in it so `expires_at > datetime('now')` is a lexical
// comparison SQLite can make correctly — mixing RFC3339 into the same column
// would silently break that ordering.
const sqlTimeLayout = "2006-01-02 15:04:05"

// UpsertUser returns the user for a provider identity, creating the row on
// first login (av-30rj). external_id is the match key; email is refreshed on
// every login so the portable re-link key stays current when a person changes
// address at the provider.
//
// It reads before it writes rather than leading with INSERT … ON CONFLICT,
// because users.id is AUTOINCREMENT and SQLite consumes a sequence value on an
// insert attempt even when the conflict clause turns it into an update. A
// blind upsert would punch a hole in the owner ids on every repeat login.
func (s *SQLiteStore) UpsertUser(ctx context.Context, externalID, email string) (*User, error) {
	if externalID == "" {
		return nil, errors.New("upsert user: empty external id")
	}
	u, err := s.getUserBy(ctx, "external_id=?", externalID)
	switch {
	case err == nil:
		if u.Email == email {
			return u, nil
		}
		if _, err := s.db.ExecContext(ctx,
			"UPDATE users SET email=? WHERE id=?", email, u.ID); err != nil {
			return nil, err
		}
		u.Email = email
		return u, nil
	case !errors.Is(err, ErrNotFound):
		return nil, err
	}
	// First login. DO NOTHING rather than a conflict error, so two logins
	// racing on the same brand-new identity both resolve to the row below.
	if err := s.insertUser(ctx, externalID, email, nil); err != nil {
		return nil, err
	}
	return s.getUserBy(ctx, "external_id=?", externalID)
}

// insertUser is the only statement in the system that creates a users row, and
// therefore the only place the first-admin rule has to live (av-rzvf).
//
// `is_admin` is computed by the insert itself — NOT EXISTS over the table it is
// inserting into — rather than by the caller reading a count and then writing.
// A read-then-write would be two statements with a gap in the middle, and the
// row that gap decides is the one that administers the instance. As one
// statement it is atomic under SQLite's single writer, so two simultaneous
// first logins produce exactly one admin whichever wins.
//
// The `WHERE true` is not decoration: SQLite's parser cannot tell an
// ON CONFLICT clause from a join's ON clause after INSERT … SELECT, and a
// trailing WHERE is the documented way to disambiguate.
func (s *SQLiteStore) insertUser(ctx context.Context, externalID, email string, passwordHash *string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (external_id, email, password_hash, is_admin)
         SELECT ?, ?, ?, NOT EXISTS (SELECT 1 FROM users)
         WHERE true
         ON CONFLICT(external_id) DO NOTHING`,
		externalID, email, passwordHash)
	return err
}

// GetUser looks a user up by the integer every other table calls owner_id.
func (s *SQLiteStore) GetUser(ctx context.Context, id int64) (*User, error) {
	return s.getUserBy(ctx, "id=?", id)
}

// GetUserByExternalID looks a user up by the key a *person* is known by: the
// provider's subject claim, or `local:<name>` for an account this instance
// issued.
//
// LookupLocalCredential answers a similar-looking question and is deliberately
// not this one: it returns ErrNotFound for an account with no password,
// because the login path must not be able to tell an OIDC row from an absent
// one. An operator asking "disable this account" needs the opposite — an
// identity a provider issued has no password to remove and is precisely the
// kind of account that has to be disable-able (av-utap, migration 017).
func (s *SQLiteStore) GetUserByExternalID(ctx context.Context, externalID string) (*User, error) {
	return s.getUserBy(ctx, "external_id=?", externalID)
}

// userColumns is the projection every user read shares. password_hash is
// absent on purpose: it is selected by exactly one query, in
// LookupLocalCredential, so there is no path by which the hash reaches a
// caller that did not ask for it by name.
//
// The entitlement columns (av-2p8z) are here rather than behind an accessor of
// their own because the admin directory renders one row per account and would
// otherwise issue a query per row. userScan (entitlements.go) owns the
// destinations, so a column added to this list is scanned by every query that
// uses it or by none.
const userColumns = `id, external_id, email, created_at, is_admin,
                     password_hash IS NOT NULL, disabled_at IS NOT NULL,
                     plan, storage_limit_bytes, entitlement_ref`

func (s *SQLiteStore) getUserBy(ctx context.Context, where string, arg any) (*User, error) {
	var sc userScan
	err := s.db.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE "+where, arg,
	).Scan(sc.dest()...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return sc.user(), nil
}

// --- Local credentials (av-rzvf) ---------------------------------------
//
// A local account is a users row with password_hash filled in. It is the same
// row an OIDC identity gets and lives in the same owner_id space; the only
// difference is which columns are populated. That is what keeps "this instance
// issues its own credentials" from being a second user directory bolted beside
// the first.
//
// The lookup key is the login name, normalized and folded into external_id as
// `local:<name>` by auth.LocalExternalID. Reusing external_id's existing UNIQUE
// constraint makes "one local account per name" a schema invariant, without
// imposing uniqueness on email — which for an OIDC row is whatever the provider
// last reported and is not ours to constrain.

// LookupLocalCredential returns the account for a local login name together
// with its stored bcrypt hash. ErrNotFound covers both "no such account" and
// "that account has no password" (an OIDC identity), because the login path
// acts on them identically and separating them here would only invite a caller
// to leak the difference.
//
// The hash is returned rather than compared here: bcrypt is the auth package's
// business, and a store that verified passwords would be a store that decides
// authentication policy.
func (s *SQLiteStore) LookupLocalCredential(ctx context.Context, externalID string) (*User, string, error) {
	var sc userScan
	var hash sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT "+userColumns+", password_hash FROM users WHERE external_id=?", externalID,
	).Scan(sc.dest(&hash)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if !hash.Valid || hash.String == "" {
		return nil, "", ErrNotFound
	}
	return sc.user(), hash.String, nil
}

// CreateLocalUser provisions an account with a password. It is the CLI's
// entry point today and the admin UI's later (av-utap).
//
// An existing external_id is a conflict rather than an overwrite: "add a user"
// that silently resets an existing user's password is the kind of convenience
// that loses somebody their library. SetLocalPassword is the deliberate way to
// change one.
func (s *SQLiteStore) CreateLocalUser(ctx context.Context, u NewLocalUser) (*User, error) {
	switch {
	case u.ExternalID == "":
		return nil, errors.New("create local user: empty login name")
	case u.PasswordHash == "":
		return nil, errors.New("create local user: empty password hash")
	}
	if _, err := s.getUserBy(ctx, "external_id=?", u.ExternalID); err == nil {
		return nil, ErrDuplicateName
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if err := s.insertUser(ctx, u.ExternalID, u.Email, &u.PasswordHash); err != nil {
		return nil, err
	}
	return s.getUserBy(ctx, "external_id=?", u.ExternalID)
}

// SetLocalPassword replaces an account's password, or removes it when hash is
// empty — which is how an account becomes SSO-only again without losing the
// row, and therefore without losing the library that row owns.
//
// It revokes the account's sessions in the same transaction, on the same
// reasoning SetUserDisabled documents: a credential change that left an
// already-issued session alone would not actually lock out whoever was using
// it — the old password's holder either way, or an attacker holding a stolen
// cookie the reset was meant to answer. Setting and clearing the hash both
// take this path, since either is "this account's credential just changed".
func (s *SQLiteStore) SetLocalPassword(ctx context.Context, userID int64, passwordHash string) error {
	var value any
	if passwordHash != "" {
		value = passwordHash
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	res, err := tx.ExecContext(ctx, "UPDATE users SET password_hash=? WHERE id=?", value, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Administration (av-utap) ------------------------------------------
//
// Two mutators an admin reaches through, and one invariant they share: an
// instance must always keep at least one *enabled* admin. The guard is written
// into the UPDATE's WHERE clause rather than checked by the caller first,
// because a read-then-write is two statements with a gap in the middle and the
// row that gap decides is the one that can still administer the instance. As
// one statement it is atomic under SQLite's single writer, so two admins
// simultaneously demoting each other leave one standing whichever wins.
//
// A disabled admin does not satisfy the invariant — it cannot sign in, so it
// is not a way back in. Both guards therefore look for another admin that is
// `is_admin = 1 AND disabled_at IS NULL`.

// lastEnabledAdminGuard is that predicate: "somebody other than ?1 can still
// administer this instance". It is spelled once and shared, so the two
// mutators below cannot drift into disagreeing about what an admin is.
//
// `is_admin = 0 OR …` makes a no-op allowed unconditionally: demoting an
// account that is not an admin, or disabling one that is not, changes nothing
// about who administers the instance and must not be refused as though it did.
const lastEnabledAdminGuard = `(is_admin = 0 OR EXISTS (
      SELECT 1 FROM users WHERE id <> ?1 AND is_admin = 1 AND disabled_at IS NULL))`

// SetUserAdmin promotes or demotes an account.
//
// Promotion is unguarded — more admins is never the failure mode. Demotion
// carries the guard: ErrLastAdmin rather than an instance nobody can
// administer. ErrNotFound distinguishes "no such account" from "refused".
func (s *SQLiteStore) SetUserAdmin(ctx context.Context, userID int64, admin bool) error {
	if admin {
		return s.exactlyOneRow(ctx, "UPDATE users SET is_admin = 1 WHERE id = ?1", userID)
	}
	return s.guardedUserUpdate(ctx,
		"UPDATE users SET is_admin = 0 WHERE id = ?1 AND "+lastEnabledAdminGuard, userID)
}

// SetUserDisabled is the whole of "disable this account", and it is deliberately
// two writes in one transaction.
//
// Refusing future logins is only half a disable: the sessions already issued
// are what the person is actually using, and a `sessions` row outlives any
// decision about the credential that minted it. av-30rj made sessions
// server-side rows precisely so they can be deleted, so they are — here, beside
// the column, in one transaction, rather than left to whichever caller
// remembers. A caller that could disable without revoking would eventually be
// written.
//
// Re-disabling is idempotent and preserves the original timestamp (COALESCE),
// so "disable" is a state to assert rather than an event to fire — and a
// duplicate click does not rewrite when it happened.
func (s *SQLiteStore) SetUserDisabled(ctx context.Context, userID int64, disabled bool) error {
	if !disabled {
		return s.exactlyOneRow(ctx, "UPDATE users SET disabled_at = NULL WHERE id = ?1", userID)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	res, err := tx.ExecContext(ctx,
		`UPDATE users SET disabled_at = COALESCE(disabled_at, datetime('now'))
           WHERE id = ?1 AND `+lastEnabledAdminGuard, userID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return refusedOrMissing(ctx, tx, userID)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID); err != nil {
		return err
	}
	return tx.Commit()
}

// guardedUserUpdate runs a statement whose WHERE carries lastEnabledAdminGuard
// and turns "no rows changed" back into the reason for it.
func (s *SQLiteStore) guardedUserUpdate(ctx context.Context, query string, userID int64) error {
	res, err := s.db.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return refusedOrMissing(ctx, s.db, userID)
	}
	return nil
}

// rowQueryer is the one method refusedOrMissing needs, so it can be handed
// either the pool or an open transaction.
//
// That is not a generalization for its own sake — it is the difference between
// working and deadlocking. The pool is capped at one connection (SQLite is
// single-writer, sqlite.go), so a *DB read issued while a transaction is still
// open waits for a connection the transaction itself is holding, forever. The
// caller inside a transaction must therefore ask that transaction.
type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// refusedOrMissing separates the two things a guarded update matching no row
// can mean. Both are the caller's error to report; only one of them is worth
// explaining to the person who asked.
func refusedOrMissing(ctx context.Context, q rowQueryer, userID int64) error {
	var exists bool
	err := q.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = ?)", userID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return ErrLastAdmin
}

func (s *SQLiteStore) exactlyOneRow(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountLocalCredentials reports how many accounts can log in with a password.
// The server reads it once at startup to decide whether this instance has a
// login at all — see api.Config.LocalUsers.
//
// Disabled accounts are counted, deliberately. The question this answers is
// "does this instance have a login gate?", not "can anyone get in right now" —
// and an instance whose only local account is disabled must keep the gate up,
// not drop it and serve the library to whoever asks.
func (s *SQLiteStore) CountLocalCredentials(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM users WHERE password_hash IS NOT NULL").Scan(&n)
	return n, err
}

// ListUsers returns every account on the instance, oldest first — which is
// also admin-first, since the first row is the one the first-admin rule
// promoted.
func (s *SQLiteStore) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+userColumns+" FROM users ORDER BY id")
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

// CreateSession records a logged-in browser. The caller supplies the id — it
// is opaque random bytes minted at the callback, never derived from anything
// about the user.
func (s *SQLiteStore) CreateSession(ctx context.Context, sess *Session) error {
	if sess.ExpiresAt.IsZero() {
		return errors.New("create session: no expiry")
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		sess.ID, sess.UserID, sess.ExpiresAt.UTC().Format(sqlTimeLayout))
	return err
}

// GetSession returns the session for a cookie value, or ErrNotFound if it is
// missing *or* expired. Collapsing those two cases is deliberate: to every
// caller a revoked session and a lapsed one mean the same thing — this request
// is not authenticated — and keeping the expiry check in the query means no
// caller can forget it.
func (s *SQLiteStore) GetSession(ctx context.Context, id string) (*Session, error) {
	var sess Session
	var created, expires any
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, created_at, expires_at FROM sessions
         WHERE id=? AND expires_at > datetime('now')`, id,
	).Scan(&sess.ID, &sess.UserID, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sess.CreatedAt = anyToTime(created)
	sess.ExpiresAt = anyToTime(expires)
	return &sess, nil
}

// DeleteSession revokes one session. Idempotent — logging out twice, or
// logging out with a cookie the server has already forgotten, is not an error.
func (s *SQLiteStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id=?", id)
	return err
}

// DeleteExpiredSessions clears rows GetSession already refuses to return, so
// the table does not grow forever. Called at startup; nothing depends on it
// having run.
func (s *SQLiteStore) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM sessions WHERE expires_at <= datetime('now')")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

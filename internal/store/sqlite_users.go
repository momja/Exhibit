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

// userColumns is the projection every user read shares. password_hash is
// absent on purpose: it is selected by exactly one query, in
// LookupLocalCredential, so there is no path by which the hash reaches a
// caller that did not ask for it by name.
const userColumns = `id, external_id, email, created_at, is_admin,
                     password_hash IS NOT NULL`

func (s *SQLiteStore) getUserBy(ctx context.Context, where string, arg any) (*User, error) {
	var u User
	var created any
	err := s.db.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE "+where, arg,
	).Scan(&u.ID, &u.ExternalID, &u.Email, &created, &u.IsAdmin, &u.HasPassword)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = anyToTime(created)
	return &u, nil
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
	var u User
	var created any
	var hash sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT "+userColumns+", password_hash FROM users WHERE external_id=?", externalID,
	).Scan(&u.ID, &u.ExternalID, &u.Email, &created, &u.IsAdmin, &u.HasPassword, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if !hash.Valid || hash.String == "" {
		return nil, "", ErrNotFound
	}
	u.CreatedAt = anyToTime(created)
	return &u, hash.String, nil
}

// CreateLocalUser provisions an account with a password. It is the CLI's
// entry point today and the admin UI's later (av-utap).
//
// An existing external_id is a conflict rather than an overwrite: "add a user"
// that silently resets an existing user's password is the kind of convenience
// that loses somebody their library. SetLocalPassword is the deliberate way to
// change one.
func (s *SQLiteStore) CreateLocalUser(ctx context.Context, externalID, email, passwordHash string) (*User, error) {
	switch {
	case externalID == "":
		return nil, errors.New("create local user: empty login name")
	case passwordHash == "":
		return nil, errors.New("create local user: empty password hash")
	}
	if _, err := s.getUserBy(ctx, "external_id=?", externalID); err == nil {
		return nil, ErrDuplicateName
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if err := s.insertUser(ctx, externalID, email, &passwordHash); err != nil {
		return nil, err
	}
	return s.getUserBy(ctx, "external_id=?", externalID)
}

// SetLocalPassword replaces an account's password, or removes it when hash is
// empty — which is how an account becomes SSO-only again without losing the
// row, and therefore without losing the library that row owns.
func (s *SQLiteStore) SetLocalPassword(ctx context.Context, userID int64, passwordHash string) error {
	var value any
	if passwordHash != "" {
		value = passwordHash
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE users SET password_hash=? WHERE id=?", value, userID)
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
		var u User
		var created any
		if err := rows.Scan(&u.ID, &u.ExternalID, &u.Email, &created, &u.IsAdmin, &u.HasPassword); err != nil {
			return nil, err
		}
		u.CreatedAt = anyToTime(created)
		out = append(out, &u)
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

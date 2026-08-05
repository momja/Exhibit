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
func (s *SQLiteStore) UpsertUser(ctx context.Context, externalID, email string) (*User, error) {
	if externalID == "" {
		return nil, errors.New("upsert user: empty external id")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (external_id, email) VALUES (?, ?)
         ON CONFLICT(external_id) DO UPDATE SET email=excluded.email`,
		externalID, email)
	if err != nil {
		return nil, err
	}
	return s.getUserBy(ctx, "external_id=?", externalID)
}

// GetUser looks a user up by the integer every other table calls owner_id.
func (s *SQLiteStore) GetUser(ctx context.Context, id int64) (*User, error) {
	return s.getUserBy(ctx, "id=?", id)
}

func (s *SQLiteStore) getUserBy(ctx context.Context, where string, arg any) (*User, error) {
	var u User
	var created any
	err := s.db.QueryRowContext(ctx,
		"SELECT id, external_id, email, created_at FROM users WHERE "+where, arg,
	).Scan(&u.ID, &u.ExternalID, &u.Email, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = anyToTime(created)
	return &u, nil
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

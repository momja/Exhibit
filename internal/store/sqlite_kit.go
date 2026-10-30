package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/momja/Exhibit/internal/kit"
)

// ValidKitChoice reports whether v may be stored as a kit choice: a released
// kit's ID, or "" for no choice at this level (Exh-61ix).
func ValidKitChoice(v string) bool {
	if v == "" {
		return true
	}
	_, ok := kit.Get(v)
	return ok
}

func (s *SQLiteStore) LibraryKit(ctx context.Context, ownerID int64) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, "SELECT kit FROM library_settings WHERE owner_id = ?", ownerID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *SQLiteStore) SetLibraryKit(ctx context.Context, ownerID int64, kitID string) error {
	if !ValidKitChoice(kitID) {
		return fmt.Errorf("kit must be a released kit ID or empty")
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO library_settings (owner_id, kit) VALUES (?, ?) ON CONFLICT(owner_id) DO UPDATE SET kit = excluded.kit",
		ownerID, kitID)
	return err
}

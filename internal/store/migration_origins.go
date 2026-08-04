package store

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"

	"github.com/pressly/goose/v3"

	"github.com/momja/Exhibit/internal/origin"
)

// Rows written before av-i7hd were stored verbatim, so artifact_network_origins
// holds values that are not origins: full URLs with paths, a host spelled with a
// sentence-terminating dot, mixed case. Every one of them is pasted into a CSP
// header at render, where it means something other than what the allowlist
// editor showed the user — and near-duplicates make "one decision per (artifact,
// origin)" (docs/architecture.md §3.3) false in practice.
//
// Those rows are repaired here rather than left to be fixed on the artifact's
// next save, because "next save" may never come: an artifact nobody edits again
// keeps its dirty rows forever, and the invariant the rest of the code now
// assumes would hold only for recently-touched artifacts. The repair is a Go
// migration (version 23 — 12 is already spoken for by the widget_blob_id
// repair, av-9pm8) for the same reason that repair is Go too: it needs URL
// parsing, which SQL has no way to express.
//
// The rules, chosen so the repair can never widen a policy:
//
//   - A row whose value normalizes cleanly, or that only carries path/query/
//     fragment/userinfo noise, keeps the origin it always effectively named —
//     CSP already matched it host-wide for every directive that has no path.
//   - A row with no origin in it at all (a keyword, a relative path, garbage)
//     is deleted. It could not have granted anything meaningful, and leaving it
//     in a CSP header is the injection surface this ticket closes.
//   - When several rows collapse onto one origin, block wins over allow: a
//     "don't ask again" answer must never be upgraded into network access by a
//     data migration.
const repairOriginNormalizationVersion int64 = 23

var registerOriginRepairOnce sync.Once

func registerOriginNormalizationMigration() {
	registerOriginRepairOnce.Do(func() {
		m := goose.NewGoMigration(repairOriginNormalizationVersion,
			&goose.GoFunc{RunTx: normalizeStoredOrigins},
			nil,
		)
		m.Source = "023_normalize_network_origins.go"
		if err := goose.SetGlobalMigrations(m); err != nil {
			// Already registered in this process; nothing to do.
			return
		}
	})
}

// storedOrigin is one artifact_network_origins row, carried through the repair
// so created_at survives the rewrite (a decision's age is user-visible history,
// not a detail of its spelling).
type storedOrigin struct {
	artifactID string
	origin     string
	decision   string
	source     string
	createdAt  any
}

func normalizeStoredOrigins(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT artifact_id, origin, decision, source, created_at FROM artifact_network_origins`)
	if err != nil {
		return err
	}
	var stored []storedOrigin
	for rows.Next() {
		var r storedOrigin
		if err := rows.Scan(&r.artifactID, &r.origin, &r.decision, &r.source, &r.createdAt); err != nil {
			rows.Close()
			return err
		}
		stored = append(stored, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	type key struct{ artifactID, origin string }
	merged := make(map[key]storedOrigin, len(stored))
	order := make([]key, 0, len(stored))
	dirty, dropped := false, 0
	for _, r := range stored {
		normalized, _ := origin.NormalizeOrigin(r.origin)
		if normalized == "" {
			dirty, dropped = true, dropped+1
			slog.Warn("dropping unusable stored allowlist origin",
				slog.String("artifact_id", r.artifactID), slog.String("origin", r.origin))
			continue
		}
		if normalized != r.origin {
			dirty = true
		}
		k := key{r.artifactID, normalized}
		prev, collision := merged[k]
		if !collision {
			order = append(order, k)
		} else {
			dirty = true
			// Block wins: a repair must never turn "don't ask again" into
			// network access.
			if prev.decision == DecisionBlock {
				continue
			}
			if r.decision != DecisionBlock {
				continue // keep the first spelling's row
			}
		}
		r.origin = normalized
		merged[k] = r
	}
	if !dirty {
		return nil
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM artifact_network_origins`); err != nil {
		return err
	}
	for _, k := range order {
		r := merged[k]
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO artifact_network_origins (artifact_id, origin, decision, source, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			r.artifactID, r.origin, r.decision, r.source, r.createdAt); err != nil {
			return err
		}
	}
	slog.Info("normalized stored network origins",
		slog.Int("before", len(stored)), slog.Int("after", len(order)), slog.Int("dropped", dropped))
	return nil
}

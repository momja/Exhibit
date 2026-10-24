package store

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var vendorProviderID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,79}$`)

func validVendorIdentity(engine, provider string) bool {
	return validVendorEngine(engine) && vendorProviderID.MatchString(provider)
}

func validVendorEngine(engine string) bool { return engine == "nango" || engine == "composio" }

// VendorUsage combines the current locally stored connection gauge with the
// current UTC month's management and provider-contact counters. The daily
// ledger is anonymous: no user, connection, request or credential is recorded.
type VendorUsage struct {
	Engine            string `json:"engine"`
	Provider          string `json:"provider"`
	StoredConnections int    `json:"stored_connections"`
	ManagementCalls   int64  `json:"management_calls"`
	ForwardCalls      int64  `json:"forward_calls"`
	InjectCalls       int64  `json:"inject_calls"`
}

// RecordVendorCall reserves accounting immediately before a billable attempt.
// Failed attempts count too; cache hits do not call this method.
func (s *SQLiteStore) RecordVendorCall(ctx context.Context, engine, provider, operation string, now time.Time) error {
	if !validVendorIdentity(engine, provider) || now.IsZero() {
		return errors.New("invalid vendor usage attribution")
	}
	var management, forward, inject int
	switch operation {
	case "management":
		management = 1
	case "forward":
		forward = 1
	case "inject":
		inject = 1
	default:
		return errors.New("invalid vendor usage operation")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO vendor_usage(engine,provider,day,management_calls,forward_calls,inject_calls)
 VALUES(?,?,?,?,?,?) ON CONFLICT(engine,provider,day) DO UPDATE SET
 management_calls=management_calls+excluded.management_calls,
 forward_calls=forward_calls+excluded.forward_calls,inject_calls=inject_calls+excluded.inject_calls`,
		engine, provider, now.UTC().Format("2006-01-02"), management, forward, inject)
	return err
}

// VendorUsage returns counters from the first UTC day of now's month through
// now's UTC day, plus a current stored-connection gauge. Removed providers and
// connections retain their anonymous monthly spend; native connections do not
// contribute. A connection needing reconnection is still a stored account.
func (s *SQLiteStore) VendorUsage(ctx context.Context, now time.Time) ([]VendorUsage, error) {
	if now.IsZero() {
		return nil, errors.New("invalid vendor usage period")
	}
	utc := now.UTC()
	start := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	rows, err := s.db.QueryContext(ctx, `WITH stored AS (
 SELECT engine,provider,COUNT(*) AS connections FROM connections WHERE kind='vendor' GROUP BY engine,provider),
 calls AS (SELECT engine,provider,SUM(management_calls) AS management,SUM(forward_calls) AS forward,SUM(inject_calls) AS inject
 FROM vendor_usage WHERE day>=? AND day<=? GROUP BY engine,provider),
 identities AS (SELECT engine,provider FROM stored UNION SELECT engine,provider FROM calls)
 SELECT i.engine,i.provider,COALESCE(s.connections,0),COALESCE(c.management,0),COALESCE(c.forward,0),COALESCE(c.inject,0)
 FROM identities i LEFT JOIN stored s ON s.engine=i.engine AND s.provider=i.provider
 LEFT JOIN calls c ON c.engine=i.engine AND c.provider=i.provider ORDER BY i.engine,i.provider`, start, utc.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VendorUsage{}
	for rows.Next() {
		var usage VendorUsage
		if err := rows.Scan(&usage.Engine, &usage.Provider, &usage.StoredConnections, &usage.ManagementCalls, &usage.ForwardCalls, &usage.InjectCalls); err != nil {
			return nil, err
		}
		out = append(out, usage)
	}
	return out, rows.Err()
}

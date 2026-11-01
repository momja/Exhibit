package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/momja/Exhibit/internal/store"
)

func pendingVendorConnection(flow vendorFlow) store.PendingVendorConnection {
	raw, _ := json.Marshal(flow.request)
	expires := flow.expires
	if flow.cleanupExpires.After(expires) {
		expires = flow.cleanupExpires
	}
	if flow.request.Spec.Engine == "nango" {
		// The local browser flow deadline cannot invalidate a remote auth
		// link. An absent vendor expiry remains unknown, never assumed safe.
		expires = flow.cleanupExpires
	}
	return store.PendingVendorConnection{ID: flow.request.ID, OwnerID: flow.ownerID, RequestJSON: string(raw), ExpiresAt: expires, CleanupAt: flow.expires}
}

// A rejected local completion still owns the remote account. Cleanup is not
// cancelled with the browser request; failures remain durable for maintenance.
// The caller holds the flow lock, so a second completion cannot adopt it.
func (s *Service) abortVendorFlow(ctx context.Context, flow vendorFlow) error {
	s.mu.Lock()
	delete(s.vendorFlows, flow.request.ID)
	s.mu.Unlock()
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	pending := pendingVendorConnection(flow)
	pending.CleanupAt = s.cfg.Now()
	updateErr := s.cfg.Store.UpdatePendingVendorConnection(cleanup, pending)
	return errors.Join(updateErr, s.cleanupVendorConnection(cleanup, pending))
}

// CleanupVendorConnections retires unfinished authorizations before account
// erasure. Pending rows survive erasure as a last-resort durable cleanup queue.
func (s *Service) CleanupVendorConnections(ctx context.Context, ownerID int64) error {
	_, err := s.cleanupVendorConnections(ctx, ownerID, time.Time{})
	return err
}

func (s *Service) cleanupVendorConnections(ctx context.Context, ownerID int64, before time.Time) (int, error) {
	pending, err := s.cfg.Store.ListPendingVendorConnections(ctx, ownerID, before)
	if err != nil {
		return 0, err
	}
	count := 0
	var joined error
	for _, entry := range pending {
		release := s.vendorFlowLocks.acquire(entry.ID)
		// Start/completion may have changed the record while we waited.
		current, lookupErr := s.cfg.Store.ListPendingVendorConnections(ctx, entry.OwnerID, before)
		if lookupErr != nil {
			joined = errors.Join(joined, lookupErr)
			release()
			continue
		}
		for _, row := range current {
			if row.ID != entry.ID {
				continue
			}
			s.mu.Lock()
			delete(s.vendorFlows, row.ID)
			s.mu.Unlock()
			if err := s.cleanupVendorConnection(ctx, row); err != nil {
				joined = errors.Join(joined, fmt.Errorf("clean up managed authorization %s: %w", row.ID, err))
			} else {
				count++
			}
			break
		}
		release()
	}
	return count, joined
}

func (s *Service) cleanupVendorConnection(ctx context.Context, pending store.PendingVendorConnection) error {
	var request VendorFlowRequest
	if err := json.Unmarshal([]byte(pending.RequestJSON), &request); err != nil {
		return errors.New("invalid pending managed authorization")
	}
	engine, err := s.vendorEngine(request.Spec.Engine)
	if err != nil {
		return err
	}
	refs := []string{}
	if request.ExternalRef != "" {
		refs = append(refs, request.ExternalRef)
	}
	if request.Spec.Engine == "nango" {
		var discovered []string
		discovered, err = (nangoEngine{s: s}).pendingReferences(ctx, request)
		if err != nil {
			return err
		}
		for _, ref := range discovered {
			if ref != request.ExternalRef {
				refs = append(refs, ref)
			}
		}
	} else if len(refs) == 0 {
		return errors.New("managed authorization returned no remote reference; operator reconciliation is required")
	}
	adopted := false
	for _, ref := range refs {
		saved, err := s.cfg.Store.HasVendorConnectionReference(ctx, pending.OwnerID, request.Spec.Engine, request.Provider, ref)
		if err != nil {
			return err
		}
		if saved {
			adopted = true
			continue // Local save committed before cleanup bookkeeping failed.
		}
		spec := VendorConnectionSpec{OwnerTag: request.OwnerTag, Provider: request.Provider, ExternalRef: ref, Spec: request.Spec}
		if err := engine.Revoke(ctx, spec); err != nil {
			return err
		}
	}
	// Nango links remain capable of creating an account until they expire,
	// including after an earlier account was found and revoked. Successful
	// local adoption is the only earlier terminal state.
	if request.Spec.Engine == "nango" && !adopted {
		if pending.ExpiresAt.IsZero() {
			return errors.New("managed authorization returned no session expiry; operator reconciliation is required")
		}
		if pending.ExpiresAt.After(s.cfg.Now()) {
			return errors.New("managed authorization is still pending; retry after its session expires")
		}
	}
	return s.cfg.Store.DeletePendingVendorConnection(ctx, pending.OwnerID, pending.ID)
}

// Retiring the previous account can be retried within the same repair flow.
// The newly authorized replacement stays in the durable cleanup queue meanwhile.
type vendorRetirementPending struct{ error }

func (e *vendorRetirementPending) Unwrap() error { return e.error }

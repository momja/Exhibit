package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/momja/Exhibit/internal/store"
)

// SetupContext is a server-validated destination and optional existing account.
// It travels inside the browser-bound authentication flow, never in a callback
// redirect URL supplied by the provider.
type SetupContext struct {
	ArtifactID   string `json:"artifact_id,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
}

func (s *Service) validateSetup(ctx context.Context, ownerID int64, setup SetupContext) error {
	if setup.ArtifactID == "" {
		return nil
	}
	a, err := s.cfg.Store.GetArtifactReadableBy(ctx, store.ViewerID(ownerID), setup.ArtifactID)
	if err != nil {
		return err
	}
	if a == nil {
		return store.ErrNotFound
	}
	return nil
}

// ReconnectRequest carries replacement secrets only. A repair cannot change
// the destinations, credential position, quota, or account type already granted.
type ReconnectRequest struct {
	Secret    string `json:"secret,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	AccessURL string `json:"access_url,omitempty"`
}

func (s *Service) Reconnect(ctx context.Context, ownerID int64, id string, in ReconnectRequest) (*Connection, error) {
	if !s.cfg.CustomEnabled || s.cfg.Store == nil || s.cfg.Box == nil {
		return nil, ErrDisabled
	}
	lock, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	rec, err := s.cfg.Store.GetConnection(ctx, ownerID, id)
	if err != nil {
		return nil, err
	}
	if rec.Kind != "native" {
		return nil, fmt.Errorf("%w: reconnect this account with its sign-in provider", ErrInvalid)
	}
	r, old, err := s.readCredential(rec)
	if err != nil {
		return nil, err
	}
	var next credential
	switch rec.Provider {
	case "api-key", "bearer":
		if in.Secret == "" || in.Username != "" || in.Password != "" || in.AccessURL != "" {
			return nil, fmt.Errorf("%w: enter a replacement credential", ErrInvalid)
		}
		next.Value = in.Secret
	case "basic":
		if in.Secret != "" {
			return nil, ErrInvalid
		}
		if in.AccessURL != "" {
			if in.Username != "" || in.Password != "" {
				return nil, fmt.Errorf("%w: use a credential URL or separate username and password", ErrInvalid)
			}
			origin, parsed, err := parseBasicAccessURL(in.AccessURL)
			if err != nil {
				return nil, err
			}
			if len(r.Origins) != 1 || origin != r.Origins[0] || parsed.BasePath != old.BasePath {
				return nil, fmt.Errorf("%w: the replacement URL must keep the existing service and path", ErrInvalid)
			}
			next = parsed
		} else {
			next = credential{Username: in.Username, Password: in.Password, BasePath: old.BasePath, Endpoint: old.Endpoint}
		}
		if next.Username == "" || next.Password == "" || strings.Contains(next.Username, ":") {
			return nil, fmt.Errorf("%w: enter a username and password", ErrInvalid)
		}
	default:
		return nil, ErrInvalid
	}
	if strings.ContainsAny(next.Value+next.Username+next.Password, "\r\n") || len(next.Value)+len(next.Username)+len(next.Password) > 16384 {
		return nil, fmt.Errorf("%w: credential has an invalid format", ErrInvalid)
	}
	return s.repairCredential(ctx, rec, next, rec.ExternalRef)
}

// Caller holds the connection lock where concurrent request refresh is possible.
func (s *Service) repairCredential(ctx context.Context, rec *store.ConnectionRecord, next credential, externalRef string) (*Connection, error) {
	raw, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	sealed, err := s.cfg.Box.Encrypt(string(raw))
	if err != nil {
		return nil, err
	}
	if err := s.cfg.Store.RepairConnection(ctx, rec, sealed, externalRef); err != nil {
		return nil, err
	}
	s.mu.Lock()
	for token, binding := range s.standins {
		if binding.connectionID == rec.ID {
			delete(s.standins, token)
		}
	}
	s.mu.Unlock()
	updated, err := s.cfg.Store.GetConnection(ctx, rec.OwnerID, rec.ID)
	if err != nil {
		return nil, err
	}
	return s.metadata(ctx, updated)
}

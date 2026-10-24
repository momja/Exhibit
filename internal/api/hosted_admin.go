package api

import (
	"context"
	"strings"

	"github.com/momja/Exhibit/internal/store"
)

func isHostedUser(user *store.User) bool {
	return strings.HasPrefix(user.ExternalID, "clerk:") && len(user.ExternalID) > len("clerk:")
}

// Used only for explanatory UI. Mutations enforce the same rule atomically in
// the store; a read-count before writing would permit concurrent last-admin loss.
func (ro *Router) isLastEnabledHostedAdmin(ctx context.Context, user *store.User) (bool, error) {
	if !isHostedUser(user) || !user.IsAdmin || user.Disabled {
		return false, nil
	}
	users, err := ro.cfg.Store.ListUsers(ctx)
	if err != nil {
		return false, err
	}
	for _, other := range users {
		if other.ID != user.ID && isHostedUser(other) && other.IsAdmin && !other.Disabled {
			return false, nil
		}
	}
	return true, nil
}

package api

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Public-instance configuration (av-4ac9).
//
// An instance can opt into serving a read-only gallery to anonymous visitors.
// This file is that opt-in and nothing more: it reads the knobs, carries them,
// and answers one endpoint with the two display strings. It deliberately
// changes no authentication behaviour — the conditional auth middleware is
// av-wmp6, and with these variables unset (the default) every route
// authenticates exactly as it did before.
//
// The configuration is environment-based rather than a settings table because
// the server-rendered gallery consults it on every page render; a column would
// buy nothing and cost a database round trip per request.
const (
	envPublicModeEnabled         = "PUBLIC_MODE_ENABLED"
	envPublicInstanceName        = "PUBLIC_INSTANCE_NAME"
	envPublicInstanceDescription = "PUBLIC_INSTANCE_DESCRIPTION"
	envPublicOwnerID             = "PUBLIC_OWNER_ID"
)

// PublicMode is an instance's public-gallery configuration. The zero value is
// a private instance, which is what an operator who has set none of these
// variables gets.
type PublicMode struct {
	Enabled     bool
	Name        string
	Description string
	// OwnerID names whose library is the public one.
	//
	// Owner scoping became a real query predicate in av-ep8k — every artifact
	// read filters on an owner — so "the library" is no longer a well-defined
	// phrase on an instance that may hold several. A public instance must
	// therefore say which owner it is publishing. av-wmp6 is what resolves an
	// unauthenticated request to this owner; this file only names it.
	OwnerID int64
}

// PublicModeFromEnv reads the public-instance configuration from the process
// environment. Parsing lives here, next to the fields and their defaults,
// rather than in main — it is the whole of this configuration layer's
// behaviour and the part worth testing.
func PublicModeFromEnv() PublicMode {
	return PublicMode{
		Enabled:     envBool(envPublicModeEnabled),
		Name:        os.Getenv(envPublicInstanceName),
		Description: os.Getenv(envPublicInstanceDescription),
		OwnerID:     envOwnerID(envPublicOwnerID),
	}
}

// envBool reads a boolean knob, failing closed.
//
// It deliberately does not follow DEBUG's "any non-empty value is true" rule.
// That rule reads `PUBLIC_MODE_ENABLED=false` as an instruction to publish the
// library, which is the one misreading a knob like this must not make. A value
// that means nothing recognizable is likewise treated as off and logged, so a
// typo leaves the instance private and says so at startup.
func envBool(key string) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "yes", "on":
		return true
	case "no", "off":
		return false
	}
	v, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		slog.Warn("unrecognized boolean env var; treating as off",
			slog.String("var", key), slog.String("value", raw))
		return false
	}
	return v
}

// envOwnerID reads an owner id, defaulting to the owner every single-user
// library is already filed under. An unparseable value falls back to the same
// default rather than failing the boot: the wrong library is a visible,
// recoverable mistake, whereas an instance that will not start is not.
func envOwnerID(key string) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultOwnerID
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		slog.Warn("invalid owner id env var; using the default owner",
			slog.String("var", key), slog.String("value", raw),
			slog.Int64("owner_id", defaultOwnerID))
		return defaultOwnerID
	}
	return id
}

// publicSettingsResponse is what GET /api/settings/public answers with: the
// two strings a public gallery renders above the grid. It carries neither the
// enabled flag nor the owner id — the status code already says whether this is
// a public instance, and which owner it publishes is nobody's business but the
// server's.
type publicSettingsResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// publicSettings serves GET /api/settings/public.
//
// It sits outside the authenticated API group, beside the share route and the
// manifest, because its entire purpose is to be readable by a visitor who has
// no credential — an anonymous browser needs the instance's name to render the
// gallery it is being shown.
//
// A private instance answers 404, not an empty 200. Two reasons, and the
// second is the one that decides it:
//
//  1. An instance that has not opted into being public should not name itself
//     to an unauthenticated stranger. 404 makes it indistinguishable from an
//     instance that never had the feature, so probing learns nothing.
//  2. It is the less ambiguous answer for the frontend. 200 with empty strings
//     is already the meaningful response for "public, but the operator set no
//     name" — reusing it for "not public" would collapse two different states
//     into one body and force a second signal to tell them apart. 404 is one
//     branch: not a public instance.
func (ro *Router) publicSettings(w http.ResponseWriter, r *http.Request) {
	if !ro.cfg.Public.Enabled {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, publicSettingsResponse{
		Name:        ro.cfg.Public.Name,
		Description: ro.cfg.Public.Description,
	})
}

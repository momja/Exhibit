// Resolving what an owner is allowed (av-2p8z).
//
// The store holds three columns per account (internal/store/entitlements.go).
// This file turns them into the single answer everything else asks for, and
// owns the one decision the columns cannot make on their own: whether limits
// are in use on this instance at all.
//
// # The distinction the whole design rests on
//
// Two states look alike and are not:
//
//   - **Limits are not in use here.** Every self-hosted instance, and the
//     default. Everything resolves to unlimited and nothing is ever refused.
//     A self-hoster who upgrades must never meet a ceiling they did not ask
//     for, so absence can only ever mean "no limit".
//   - **Limits are in use but this owner's could not be resolved.** A database
//     error, a row that makes no sense. With limits switched on, "I don't know
//     what you're allowed" is not a reason to allow anything, so this refuses.
//
// Enforcement is therefore one explicit switch rather than something inferred
// from whether a default happens to be configured. Switched on with no default
// entitlement, the server fails at **startup** — the posture LOGIN_USERNAME
// without LOGIN_PASSWORD_HASH already takes — rather than booting into a state
// where every unprovisioned account is unlimited. A startup warning was the
// weaker version of that and is deliberately rejected: warnings scroll past,
// and the failure they guard is the one nobody notices until it is expensive.
//
// # What is deliberately out of tree
//
// Nothing here knows why an owner is on the plan they are on, and nothing in
// this repo does — not in any form, including as an interface with a stub
// implementation. Two reasons beyond the obvious one: Go's internal/ rule
// blocks an external module from importing internal/api, so the conventional
// seam-plus-private-implementation shape would force promoting api.Config to a
// public package — a permanent API-surface commitment made for a packaging
// reason; and an empty seam in a public tree discloses about as much as naming
// what would go behind it. Whatever maintains these values is an ordinary
// authenticated API client, calling PATCH /api/admin/users/:id like any other.
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/momja/Exhibit/internal/store"
)

const (
	envEntitlementsEnabled      = "ENTITLEMENTS_ENABLED"
	envEntitlementsDefaultPlan  = "ENTITLEMENTS_DEFAULT_PLAN"
	envEntitlementsDefaultBytes = "ENTITLEMENTS_DEFAULT_STORAGE_BYTES"
)

// Entitlements is the instance's entitlement configuration. The zero value is
// an instance with limits switched off, which is what an operator who has set
// none of these variables gets and what every self-hosted instance is.
type Entitlements struct {
	// Enforced is the single explicit switch. False is the default and means
	// resolveAllowance never reads a row, never fails, and answers unlimited
	// for everyone.
	Enforced bool
	// DefaultPlan labels an account that carries no plan of its own.
	DefaultPlan string
	// DefaultStorageBytes is the ceiling for an account with no limit of its
	// own — the baseline every unprovisioned account lands on, which is why
	// booting with Enforced and no value for it is refused rather than
	// defaulted.
	DefaultStorageBytes int64
}

// EntitlementsFromEnv reads the configuration, returning an error the caller
// makes fatal.
//
// An unparseable default is an error rather than a value fixed up at startup,
// on the same reasoning as a malformed AGENT_PROVIDER: an instance that boots
// looking configured and behaves differently from what the operator wrote is
// worse than one that does not boot.
//
// A default configured while the switch is off is accepted and inert. It is
// the state an operator passes through while setting the feature up, and
// refusing it would mean the two variables have to be introduced in one step.
func EntitlementsFromEnv() (Entitlements, error) {
	enforced, err := entitlementsSwitch()
	if err != nil {
		return Entitlements{}, err
	}
	e := Entitlements{
		Enforced:    enforced,
		DefaultPlan: strings.TrimSpace(os.Getenv(envEntitlementsDefaultPlan)),
	}
	raw := strings.TrimSpace(os.Getenv(envEntitlementsDefaultBytes))
	if raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return Entitlements{}, fmt.Errorf("%s must be a non-negative number of bytes, got %q", envEntitlementsDefaultBytes, raw)
		}
		e.DefaultStorageBytes = n
	} else if e.Enforced {
		// The startup failure the ticket asks for by name. Booting here would
		// mean every account with no entitlement of its own — which is every
		// account, on the instance where this is first switched on — is
		// unlimited, on an instance whose operator believes limits are in
		// force.
		return Entitlements{}, fmt.Errorf(
			"%s is on but %s is not set: an instance with limits switched on needs a default entitlement, "+
				"or every unprovisioned account is unlimited", envEntitlementsEnabled, envEntitlementsDefaultBytes)
	}
	return e, nil
}

// entitlementsSwitch reads the one knob that decides which of the two states
// this instance is in, and it is deliberately *not* envBool.
//
// envBool treats an unrecognized value as off and logs a warning, which is the
// safe direction for the knob it was written for: a typo leaves a library
// private. Here the directions are reversed — off means unlimited — so the same
// rule would let `ENTITLEMENTS_ENABLED=treu` quietly hand an instance whose
// operator believes limits are in force an instance with none, and say so in a
// warning that scrolls past. Same reasoning as the missing default below: a
// half-configured gate fails where the operator is watching.
//
// Unset and the two explicit falses are not typos and are not errors.
func entitlementsSwitch() (bool, error) {
	raw := strings.TrimSpace(os.Getenv(envEntitlementsEnabled))
	if raw == "" {
		return false, nil
	}
	switch strings.ToLower(raw) {
	case "yes", "on":
		return true, nil
	case "no", "off":
		return false, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true/false (or yes/no, on/off), got %q — "+
			"refusing to guess, because guessing \"off\" here means no limits at all", envEntitlementsEnabled, raw)
	}
	return v, nil
}

// LogStartup announces the mode, so which of the two an instance is in is
// visible where an operator is already looking.
func (e Entitlements) LogStartup() {
	if !e.Enforced {
		return
	}
	slog.Info("per-owner limits enforced",
		slog.String("default_plan", e.DefaultPlan),
		slog.Int64("default_storage_bytes", e.DefaultStorageBytes))
}

// Limit is a ceiling on one measured quantity.
//
// **The zero value refuses everything**, and that is deliberate rather than an
// accident of Go's defaults: a caller that ignores resolveAllowance's error
// and uses the Allowance it returned alongside it gets a limit of zero, not an
// unlimited one. Failing closed has to be the direction a mistake falls in,
// because the mistake is silent in the other direction.
//
// Unlimited is therefore always something a resolver says explicitly.
type Limit struct {
	Unlimited bool
	Bytes     int64
}

// Allows reports whether a total is within the limit. Gates ask this; nothing
// reads Bytes to compare it itself, because half the callers would forget the
// Unlimited case.
func (l Limit) Allows(total int64) bool { return l.Unlimited || total <= l.Bytes }

// Describe phrases the limit for a page or a log line.
func (l Limit) Describe() string {
	if l.Unlimited {
		return "unlimited"
	}
	return strconv.FormatInt(l.Bytes, 10) + " bytes"
}

// Allowance is what an owner is allowed right now — the resolved answer, and
// the only thing a gate ever reads.
//
// It is not store.Entitlement: that is what is *recorded*, with a nil limit
// meaning "none of their own", and it is meaningless without knowing the
// instance's default. This is the answer with the default already folded in.
type Allowance struct {
	// Plan is the label, for display and grouping. Nothing gates on it.
	Plan string
	// Storage is the ceiling on stored bytes — read by av-10bw's quota gate,
	// which never learns *why* the owner has the limit they have.
	Storage Limit
}

// unlimitedAllowance is what an instance with limits switched off resolves to
// for every owner. Spelled once, and constructed rather than zero-valued,
// because Limit's zero value refuses everything.
func unlimitedAllowance(plan string) Allowance {
	return Allowance{Plan: plan, Storage: Limit{Unlimited: true}}
}

// resolve folds one stored entitlement together with this instance's defaults
// into the answer everything else reads. It is the whole of the resolution
// rule, in one place, so that a gate and a page can never disagree about what
// an account is allowed.
//
// The rules, in the order they apply:
//
//  1. Limits switched off — unlimited, whatever the row says. An instance that
//     configures nothing behaves identically to one built before this existed.
//  2. The account's own values, each falling back to the default
//     independently: a plan label does not carry a limit, and an account can
//     have either without the other. An empty entitlement — including the one
//     an owner with no `users` row has — is therefore exactly the default,
//     which is itself a limit and never unlimited.
//  3. A value that makes no sense — an error. Not the default, and not a
//     nonsense number treated as a very small ceiling: with limits switched
//     on, "I don't know what you're allowed" is not a reason to allow
//     anything.
func (e Entitlements) resolve(ent store.Entitlement) (Allowance, error) {
	if !e.Enforced {
		return unlimitedAllowance(e.DefaultPlan), nil
	}
	out := Allowance{Plan: e.DefaultPlan, Storage: Limit{Bytes: e.DefaultStorageBytes}}
	if limit := ent.StorageLimitBytes; limit != nil {
		if *limit < 0 {
			// Unreachable through the API, which refuses it, and through the
			// schema, which carries a CHECK (migration 022) — which is
			// exactly why it is checked here as well. This is the branch that
			// decides what happens when a row arrives some way nobody
			// anticipated, and the answer has to be "refuse".
			return Allowance{}, fmt.Errorf("%w: storage limit %d is negative", store.ErrInvalidEntitlement, *limit)
		}
		out.Storage.Bytes = *limit
	}
	if ent.Plan != "" {
		out.Plan = ent.Plan
	}
	return out, nil
}

// resolveAllowance answers "what is this owner allowed", and is the function
// gates call. Nothing reads the entitlement columns to decide anything for
// itself, so there is exactly one place the fallback rules live and exactly
// one place they can be got wrong.
//
// It has no caller in this repo yet, and that is the ticket's boundary rather
// than an oversight: av-2p8z stores what an owner is allowed and resolves it,
// and av-10bw's storage quota — blocked on this — is the first gate to ask.
// What that gate inherits is the whole of the contract below, including the
// fail-closed direction, so it does not re-decide any of it.
//
// Three things about the shape are load-bearing.
//
// With limits switched off it reads **no row at all** — a self-hosted instance
// pays nothing for a feature it has not turned on, and has no failure mode
// from one either.
//
// An account that does not exist resolves to the default. That is *absence*,
// not ambiguity: owner 1 on a single-user instance has no `users` row and
// never will, and the default is a limit rather than unlimited, so this
// direction cannot open a hole.
//
// A read that fails is an **error**, logged here so that it is logged wherever
// this is called from. The caller refuses; it must not fall back to the
// default, because "the database is unreachable" is not evidence about what
// somebody is allowed. The Allowance returned beside the error is the zero
// value, whose Limit refuses everything — so a caller that ignores the error
// still fails closed.
func (ro *Router) resolveAllowance(ctx context.Context, ownerID int64) (Allowance, error) {
	cfg := ro.cfg.Entitlements
	if !cfg.Enforced {
		return unlimitedAllowance(cfg.DefaultPlan), nil
	}

	u, err := ro.cfg.Store.GetUser(ctx, ownerID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return ro.unresolved(ctx, ownerID, err)
	}
	var ent store.Entitlement
	if u != nil {
		ent = u.Entitlement
	}
	allowance, err := cfg.resolve(ent)
	if err != nil {
		return ro.unresolved(ctx, ownerID, err)
	}
	return allowance, nil
}

// unresolved logs the failure and returns the refusing zero Allowance beside
// it. One place, so the log line cannot be present on one failure path and
// missing from the other.
func (ro *Router) unresolved(ctx context.Context, ownerID int64, err error) (Allowance, error) {
	slog.ErrorContext(ctx, "entitlement unresolved: refusing rather than allowing",
		slog.Int64("owner_id", ownerID), slog.String("err", err.Error()))
	return Allowance{}, fmt.Errorf("resolve entitlement for owner %d: %w", ownerID, err)
}

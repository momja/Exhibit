---
id: av-u0vc
status: open
deps: []
links: [av-70t9, av-8xxs]
created: 2026-07-16T05:27:23Z
type: task
priority: 2
assignee: Max Omdal
tags: [capability, security, renderer, refactor]
---
# Capability registry: single source of truth for host-mediated capability bridges

The sandbox-security capabilities fall into three mechanically distinct shapes. This
ticket concerns only the **host-mediated capability bridge** family — capabilities the
opaque-origin sandbox *denies* and the shim *re-grants* by proxying to the trusted host
frame under a per-artifact first-use approval:

| Capability | Grant shape |
|---|---|
| Downloads | boolean `downloads_approved` |
| Clipboard | boolean `clipboard_approved` |
| Filesystem | bridge with **no** approval flag (user picks each file) |

(Network origins are the *other* family — a browser-enforced CSP built from
`network_allowlist`, list-valued, no host bridge. It is **explicitly out of scope** here
and must not be folded into this registry; booleans and origin-lists are not the same
thing.)

The bridge pattern is *consistent* (clipboard was built by copying downloads and follows
it faithfully) but not *ergonomic*: adding one new bridged capability today means editing
**~8 sites across 4 files**, most by copy-paste. This ticket introduces a single registry
so adding a capability is **one registry row + one shim hook + its modal copy**, and
closes a latent mass-assignment hole as a side effect.

## Current cost of adding one bridged capability (the ~8 sites)

1. **Migration** — `NNN_<cap>_approved.sql` (cf. `internal/store/migrations/005_downloads_approved.sql`, `006_clipboard_approved.sql`).
2. **Struct field** — `XxxApproved bool` on `store.Artifact` (`internal/store/store.go:34-41`).
3. **Store SELECT/Scan** — add the column to the SELECT list *and* the **positional** `rows.Scan(...)` at `internal/store/sqlite.go:89`. Positional and silent if misaligned — this is the sharpest footgun (already bit us once: **av-m8r2**, "renumber collision skips downloads_approved").
4. **Store INSERT** — column + arg at `internal/store/sqlite.go:124`.
5. **Store validation** — extend the `downloads_approved || clipboard_approved` bool branch in `UpdateArtifact` (`internal/store/sqlite.go:239`).
6. **API validation** — extend the *parallel* strict-bool loop in `updateArtifact` (`internal/api/artifacts.go:354`). Sites 5 and 6 are the **same flag-name list maintained in two files** — a drift hazard.
7. **Shim** — new interception + `postMessage({__avXxx:true,...})` in the `shimTemplate` const (`internal/render/render.go`).
8. **Host frame** — in `renderDetailPage` (`internal/api/gallery.go:940-1138`): a `message` listener, modal markup, `xxxApproved` var, `setXxxApproved`/`revokeXxx`, toolbar chip, and `renderXxxState()`.

The one piece of deliberate factoring already present is `setCapabilityApproved(field, approved, label)` (`internal/api/gallery.go:1011`), shared by downloads + clipboard — the seam this ticket generalizes.

## Secondary risk this closes: mass-assignment in UpdateArtifact

`UpdateArtifact` builds its SQL as `k+"=?"` directly from the PATCH body's JSON keys
(`internal/store/sqlite.go:246`) with **no column allowlist**. The only guards are the two
per-field bool checks and the `body` special-case. So a PATCH can write an arbitrary
column (e.g. `{"owner_id": 2}` — mass-assignment) and a crafted key such as
`"title=1, owner_id"` concatenates extra assignments into the SET clause. Blast radius is
small today (single-user, `owner_id` fixed to 1, trusted operator) but this becomes a real
hole the moment multi-user lands — and it is the *same* looseness that makes adding a
capability "free". A registry gives us the write-key allowlist for free.

## Proposed design: one registry

```go
// internal/capability (or internal/render) — the single source of truth.
type Capability struct {
    Name   string // "downloads" — also the frame message discriminator
    Column string // "downloads_approved"; "" = no approval gate (filesystem)
    Gated  bool   // whether a first-use approval flag exists
    Label  string // "download" — UI status/modal copy
}

var Capabilities = []Capability{
    {Name: "downloads",  Column: "downloads_approved",  Gated: true,  Label: "download"},
    {Name: "clipboard",  Column: "clipboard_approved",  Gated: true,  Label: "clipboard"},
    {Name: "filesystem", Column: "",                    Gated: false, Label: "file access"},
}
```

The registry becomes the single owner of:

- **The set of valid `*_approved` columns** → derive both bool-validation lists from it (delete the duplicate; sites 5 + 6 collapse to zero per-capability edits).
- **The `UpdateArtifact` write-key allowlist** → reject any key that is neither a known scalar column nor a registry flag (**closes the mass-assignment hole**).
- **Generic host UI** → the toolbar chip + approve/revoke JS, generalized from the existing `setCapabilityApproved`, plus a message-router that dispatches on a discriminator derived from `Name` instead of N hand-written `__avXxx` listeners.
- **Store read/write of the flag columns** → iterate the registry (or a named-column mapping) instead of hand-numbered positional `Scan`/`INSERT` args, so adding a column can't silently misalign (kills the av-m8r2 class).

**What the registry deliberately does NOT DRY** (irreducibly per-capability — do not force an abstraction):

- **The shim interception hook.** Each capability wraps a *different* Web API — `navigator.clipboard`, anchor `click` + `blob:`/`data:` recovery, the FSA pickers, a future `navigator.geolocation`. There is no generic "intercept a capability"; the hook stays per-capability, keyed by `Name`.
- **Modal copy.** The wording naming the capability and direction is capability-specific.

Net: "add a capability" drops from ~8 sites to **one registry row + one shim hook + its modal copy** (≈3 focused edits), with validation, the write-key allowlist, the toolbar chip, approve/revoke, and the message router all inherited.

## Reference example (ILLUSTRATIVE ONLY — not implemented by this ticket)

To show the target ergonomics, here is how adding a *gated* geolocation bridge
(`navigator.geolocation.getCurrentPosition` / `watchPosition`) would look **once the
registry exists**. This is a worked example to validate the design, **not** work to do
here:

1. **Migration** `007_geolocation_approved.sql`.
2. **Registry row** `{Name:"geolocation", Column:"geolocation_approved", Gated:true, Label:"location"}`.
3. **Shim hook** (render.go): replace `navigator.geolocation`, post `{__avCapability:"geolocation", id, op}` to the host, settle the artifact's callback on the reply.
4. **Host** (gallery.go): a `performGeolocation` op + modal copy ("wants to access your location").

Everything else — the `geolocation_approved` column validation, the `UpdateArtifact`
allowlist entry, the toolbar chip, approve/revoke, and the message router dispatch —
comes from the registry with **no new code**. Contrast with the ~8 sites required today.

## Acceptance Criteria

- A single Go registry enumerates the host-mediated capabilities and their approval columns; the API-layer and store-layer flag validation both derive from it (no duplicated flag-name list across `artifacts.go` and `sqlite.go`).
- `UpdateArtifact` rejects (400 at the API, error at the store) any PATCH key that is neither a known scalar column nor a registry-approved flag. Tests prove `{"owner_id": ...}` and a crafted compound key (e.g. `"title=1, owner_id"`) are rejected.
- Downloads, clipboard, and filesystem behavior are unchanged end-to-end — existing `internal/render/render_test.go`, `internal/api/downloads_test.go`, `internal/api/clipboard_test.go`, and gallery invariants still pass.
- The `*_approved` columns are read/written without hand-numbered positional `Scan`/`INSERT` args (named-column mapping or registry iteration), so adding a column cannot silently misalign (cf. av-m8r2).
- The generic host UI (toolbar chip + approve/revoke) and the frame message router are driven by the registry; only the shim interception hook and modal copy remain per-capability.
- Network origins (the CSP `network_allowlist`) are untouched and explicitly out of scope.

## Notes

Related: **av-70t9** (FSA polyfill) added filesystem as the fourth injected bridge and is
the most recent example of the copy-paste this ticket targets. The broader "the injected
preamble is overloaded — storage adapters vs capability bridges vs polyfills are different
families sharing only a delivery mechanism" terminology question is a separate concern
(see the discussion attached to this work); this ticket is scoped to the capability-bridge
family's registry only.


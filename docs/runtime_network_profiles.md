# Runtime Network Profiles

Design for making in-browser language runtimes (Pyodide first) approvable in one
step at ingest. Ticket: `av-5aje`. Backstop: `exhibit-fr7` (runtime escape prompt).
Data model note: coordinate with `exhibit-x87` (`artifact_network_origins`).

## 1. Problem

Pyodide generates its network footprint at runtime, so the static ingest scan
(`internal/scanner`) misses nearly all of it:

| Runtime behavior | Origin touched |
|---|---|
| `<script src=".../pyodide.js">` | CDN origin (visible to the scan) |
| `loadPyodide()` boot — wasm, `python_stdlib.zip`, lockfile | `indexURL` origin (default: derived from where the script loaded) |
| `pyodide.loadPackage("numpy")` | same `indexURL` origin |
| `micropip.install(...)` — metadata | `pypi.org` |
| `micropip.install(...)` — wheel files | `files.pythonhosted.org` |

Without help, the user approves origins one blocked request at a time:
render → blocked → approve → render → blocked again → repeat.

## 2. Key insight

**The allowlist is origin-granular, and micropip's origin set is closed.**

`micropip.install(anything)` only ever talks to `pypi.org` (JSON metadata API)
and `files.pythonhosted.org` (wheel files). Dynamic package names, transitive
dependencies, lazy installs behind button clicks — none of it expands the origin
set. We do not need to know *which packages* an artifact installs, only which
*origins* the runtime can ever reach, and for Pyodide that set is known in
advance.

## 3. Mechanism: runtime profiles

The ingest scanner gains a small **runtime-profile table**: signature → known
origin bundle. Pyodide is the first entry.

**Signatures** (any match triggers the profile):

- `loadPyodide(` in script source
- a `<script src>` whose URL matches a pyodide distribution
- `import micropip`
- a literal `indexURL` (used as the bundle's first origin when present)

**Pyodide bundle**, pre-checked on the ingest approval screen and labeled
"Pyodide runtime + Python package installs":

1. the script/`indexURL` origin (from the scan — covers boot + `loadPackage`)
2. `pypi.org`
3. `files.pythonhosted.org`

The scan remains **transparency, not enforcement** (spec §6.2): the profile only
pre-fills the approval screen. Origins are written to the artifact's origin
decisions exclusively through explicit user approval. The CSP stays the wall.

## 4. User experience

Ingesting a Pyodide artifact is identical to ingesting any other artifact — one
approval screen — except the runtime bundle is pre-checked and labeled. One
approval covers boot, `loadPackage`, and all micropip installs, including ones
behind user interaction. No probe sandbox, no extra approval moments, no
runtime prompts in the common case.

## 5. Edge cases and backstop

Caught by the existing literal-URL heuristic where the value is a string
literal; otherwise deferred to the runtime escape prompt (`exhibit-fr7`):

- custom `indexURL` / `micropip.install(..., index_urls="https://...")`
- direct wheel URLs: `micropip.install("https://other-host/x.whl")`

Genuinely unusual artifacts paying a runtime prompt is acceptable; the common
case must never see one.

## 6. Explicit non-goals

- **No runtime probe sandbox.** An earlier draft proposed running the artifact
  in a hidden iframe at ingest to observe real requests (with a fixpoint-deepening
  CSP loop). Unnecessary for Pyodide because its origin set is closed, and it
  adds a second rendering path with its own CSP and approval UX.
- **No vendoring of runtime assets or wheels.** Hosting wheels makes the service
  a package mirror — per-user storage or content-addressed dedup both end in
  bandwidth, availability, and supply-chain responsibility on our server. The
  lightness of the server is what makes hosted instances cheap and defensible.
  A Pyodide artifact leaning on CDN/PyPI uptime is an accepted durability
  property of that artifact class, same as any CDN-importing tier-2 artifact.

## 7. Extension path

Other in-browser runtimes share the staged-footprint shape (`ruby.wasm` stdlib
pack, `php-wasm`/WordPress Playground blueprints, JupyterLite). Each becomes a
new row in the profile table — signature → origin bundle — added **only when
users actually show up with that runtime**. Runtimes compiled to a single
`.wasm` with literal references (Go/Rust via `wasm_exec.js`, SQLite-wasm) need
no profile; the existing scan already covers them.

# Exhibit — Build-Time Frontend Assets

Companion to `technical_stack.md` (§13, *build-time vs runtime dependencies*). That
section states the rule; this page shows the mechanism — how the frontend assets that
`go:embed` serves are produced at build time instead of committed to the repository.

## 1. The rule

The running server ships as a single Go binary with its frontend assets embedded via
`go:embed`, and needs **no Node runtime and no network egress**. But those assets — the
CodeMirror editor bundle, the Phosphor Icons CSS + webfont, htmx — are *generated* by
Node tooling (esbuild, vendoring scripts). Rather than commit that generated output,
we produce it fresh whenever the binary is built.

> **Node is a build-time-only dependency.** The build generates the assets into
> `internal/api/assets/`; nothing under that directory is committed to git. Production
> carries only the built bytes.
>
> One workspace needs more than Node: `web/icons` subsets the Phosphor font with
> python3 + fonttools/brotli (§7) — a build-time-only dependency like Node itself.

This keeps generated output out of version control (no stale bundles, no noisy diffs on
dependency bumps) while preserving the deployment promise: one small image, one process,
no Node at runtime.

## 2. Asset workspaces

An **asset workspace** is any immediate subdirectory of `web/` that carries a
`package.json`. Each workspace owns its own `build` script and is responsible for writing
its output somewhere under `internal/api/assets/` (the `go:embed` root). That directory
is generated, not committed (§6).

The convention is the stable contract — a workspace's `package.json` `description`
documents what it builds and where, so the source stays self-describing without this
page enumerating each one.

### Committed derivatives: `web/hero-art`

The sign-in hero paintings are the one asset set whose generated output *is*
committed. `web/hero-art/images/` holds 56 encoded WebP/AVIF files and a
`manifest.json` (filename, pixel size, byte count), and the workspace's `build` only
copies them to `internal/api/assets/hero-art/`. The rule in §1 yields here because the
inputs cannot reach the build: the lossless masters live in `design_files/`, which
`.dockerignore` excludes, and the Node stage has neither the masters nor the Python
matting stack that makes them. Encoding is also not reproducible across platforms
(libvips output differs by CPU and version), so re-encoding in the image would change
the bytes every deploy.

- The workspace has no dependencies, so the asset build installs nothing for it.
  Its `build.mjs` removes the target directory first, copies the files named by the
  manifest (the manifest itself is not copied; nothing reads it) and exits non-zero if
  any copied file's size differs from the manifest's `bytes`.
- The encoder, which does need `sharp`, is a manual design-time tool outside `web/`:
  `design_files/brand-refresh/tools/hero-export/`. `npm run export` rewrites the
  derivatives; `npm run check` re-encodes in memory and fails on any byte drift, an
  unlisted file, a missing file, or a size over the per-kind budget. Run `check` after
  touching the masters, the encoder or its lockfile. Use the sharp version in that
  lockfile and the Linux architecture recorded in the production art README.
- Masters are lossless WebP, not PNG, to keep the repository small; only the sky,
  clouds and hill layers are masters. See
  `design_files/brand-refresh/production/README.md`.

## 3. The build entrypoint

`scripts/build-assets.sh` is the single source of truth for building assets. It
**discovers** workspaces rather than enumerating them: it globs `web/*/package.json`,
and for each match runs `npm ci && npm run build --if-present` inside that workspace. If
no workspace is found it exits non-zero, so a broken checkout fails loudly.

Because discovery lives in the script, **adding a new asset never means editing the
Dockerfile or Makefile** — see §5.

## 4. How it's invoked

Same script in every context:

| Context | Command | What happens |
|---------|---------|--------------|
| Local dev | `make assets` | Runs `scripts/build-assets.sh` directly. |
| Local build | `make build` | `build` depends on `assets`, so assets regenerate, then `go build`. |
| Container image | `docker build` | A `node:22` stage runs `scripts/build-assets.sh`; the Go stage overlays its output before `go build`. |

### Docker specifics

The `Dockerfile` is a three-stage build:

1. **`assets` stage** (`node:22-bookworm-slim`) — apt-installs `python3`,
   `python3-fonttools`, and `python3-brotli` for the icon subset step (§7), then
   copies `web/` and `scripts/` and runs `scripts/build-assets.sh`, producing
   `internal/api/assets/`.
2. **`builder` stage** (`golang:1.25`) — copies the source, then overlays the freshly
   built assets with `COPY --from=assets /app/internal/api/assets/ ./internal/api/assets/`
   so `go:embed` finds them at compile time, then `go build`.
3. **runtime stage** (`distroless/static-debian12`) — carries only the compiled binary.
   No Node, no assets on disk, no network egress.

`.dockerignore` excludes both `node_modules/` and `internal/api/assets/` from the build
context. This guarantees the Go stage embeds **only** what the Node stage produced inside
the image — never a stale copy from the developer's working tree.

## 5. Adding a new asset workspace

No build-plumbing edits required:

1. Create `web/<name>/` with a `package.json`.
2. Give it a `build` script that writes its output somewhere under
   `internal/api/assets/` (e.g. `internal/api/assets/<name>/…`).
3. Reference the output from a server-rendered page or handler, served under `/assets/`.

`scripts/build-assets.sh` picks it up automatically on the next `make assets` /
`docker build`. Add the built output path to `.gitignore` if it lands outside the
already-ignored `internal/api/assets/` tree (it shouldn't).

## 6. The Phosphor icon registry

The icons workspace (`web/icons`) does two load-time trims while vendoring: it
rewrites upstream's `font-display: block` to `swap`, and it subsets the font and
stylesheet to exactly the icon classes listed in `web/icons/icons.txt` (~50 of
1530 — woff2 147KB → 6KB, CSS 78KB → 5KB). The set is **explicit, not scraped**:
adding an icon means adding one `ph-*` line to that file and rebuilding assets
(`make assets`).

The failure modes are loud by design:

- A registry name matching no upstream rule fails the build
  (`icons.txt: .ph-… matches no upstream rule`) — a typo breaks the build,
  never ships tofu.
- A template, script, or stylesheet naming an unregistered icon fails
  `TestIconRegistryCoversUsage`, which names the missing icon and the fix.
- A registry or template edit without a rebuild fails `TestServedCSSCoversUsage`
  against the embedded CSS.

Subsetting needs python3 + fonttools/brotli. The Dockerfile's asset stage installs
them (§4); a local checkout without them warns and ships the full font with the
swap rewrite still applied, so `make assets` keeps working everywhere.

The same workspace bakes the agent's boiling working indicator (Exh-s8yu):
`web/icons/boil.mjs` redraws `@phosphor-icons/core`'s pencil-simple three times
with seeded noise and writes `phosphor/pencil-simple-boil.svg`. Its inputs are
pinned (`@phosphor-icons/core` and `svg-path-properties` at exact versions), so
rerunning the build writes identical bytes. `web/icons/boil.test.mjs` holds it
to that, compares the embedded file with a fresh bake, and keeps it within 2 KB
gzipped; `TestIconBakeSuite` runs it under `go test`.

## 7. Why "generate, don't commit" is safe

The design fails **loud**, never silent:

- `internal/api/assets/` is gitignored, so a fresh checkout has it empty.
- `internal/api/assets.go` embeds it with `//go:embed assets`. If the directory is empty,
  the compile fails with `pattern assets: no matching files found` — a bare `go build`
  that skipped the asset step **cannot** produce a binary with missing or stale assets.
- `make build` depends on `assets`, so the normal local path regenerates them first.

So the only way to get a server binary is to have run the asset build immediately
before it, against the current source — exactly the guarantee committing the output was
meant to give, without the committed output.

## 8. The brand fonts

The fonts workspace (`web/fonts`) vendors Schibsted Grotesk and Fraunces (both SIL OFL)
from the exact-pinned `@fontsource-variable/*` packages (`package.json` plus the
lockfile's integrity hashes) into `internal/api/assets/fonts/`, with the licence files
beside them. Schibsted Grotesk is one variable woff2 narrowed to wght 400-800 by
fontTools' instancer. Fraunces is two **static** instances, upright and italic (from Fraunces' italic source; the sign-in headline's "small"), each pinned at opsz 72,
wght 600, SOFT 100, WONK 0: each has no variation axes and is a fraction of the variable
file. `fonts.css` declares the `@font-face`s with `font-display: swap` and relative
`url()`s, which resolve inside the versioned `/assets/v/<build>/` tree; the shared
`fontsHead` partial links it. Changing the headline weight or any axis means editing
`build.mjs`, the single source for those settings.

The instancing needs python3 + fonttools/brotli. Unlike the icon subset there is no
fallback (the full variable Fraunces is about ten times larger), so a missing toolchain
fails the build with an install hint. The Dockerfile's asset stage already installs them.

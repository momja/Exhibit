# Artifact Viewer — Build-Time Frontend Assets

Companion to `technical_stack.md` (§13, *build-time vs runtime dependencies*). That
section states the rule; this page shows the mechanism — how the frontend assets that
`go:embed` serves are produced at build time instead of committed to the repository.

## 1. The rule

The running server ships as a single Go binary with its frontend assets embedded via
`go:embed`, and needs **no Node runtime and no network egress**. But those assets — the
CodeMirror editor bundle and the Phosphor Icons CSS + webfont — are *generated* by Node
tooling (esbuild, an icon-vendoring script). Rather than commit that generated output,
we produce it fresh whenever the binary is built.

> **Node is a build-time-only dependency.** The build generates the assets into
> `internal/api/assets/`; nothing under that directory is committed to git. Production
> carries only the built bytes.

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

1. **`assets` stage** (`node:22-bookworm-slim`) — copies `web/` and `scripts/`, runs
   `scripts/build-assets.sh`, producing `internal/api/assets/`.
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
3. Reference the output from a `templ` template or handler, served under `/assets/`.

`scripts/build-assets.sh` picks it up automatically on the next `make assets` /
`docker build`. Add the built output path to `.gitignore` if it lands outside the
already-ignored `internal/api/assets/` tree (it shouldn't).

## 6. Why "generate, don't commit" is safe

The design fails **loud**, never silent:

- `internal/api/assets/` is gitignored, so a fresh checkout has it empty.
- `internal/api/assets.go` embeds it with `//go:embed assets`. If the directory is empty,
  the compile fails with `pattern assets: no matching files found` — a bare `go build`
  that skipped the asset step **cannot** produce a binary with missing or stale assets.
- `make build` depends on `assets`, so the normal local path regenerates them first.

So the only way to get a server binary is to have run the asset build immediately
before it, against the current source — exactly the guarantee committing the output was
meant to give, without the committed output.

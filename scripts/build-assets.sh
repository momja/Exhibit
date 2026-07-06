#!/usr/bin/env sh
# Build every frontend asset workspace into the go:embed-served asset directory
# (internal/api/assets). An "asset workspace" is any immediate subdirectory of
# web/ that carries a package.json; each owns its own `build` script (an esbuild
# bundle, an icon vendoring step, ...) that writes its output into
# internal/api/assets.
#
# Workspaces are discovered here rather than enumerated in the Dockerfile and
# Makefile, so adding a new asset needs no build-plumbing edit: drop a workspace
# under web/ with a `build` script and it is picked up automatically.
#
# Node is a build-time-only dependency; the built output is not committed to git.
# A checkout that skips this step leaves internal/api/assets empty, so the
# //go:embed in internal/api/assets.go fails the compile loudly instead of
# embedding stale bytes.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

found=0
for pkg in "$root"/web/*/package.json; do
	[ -e "$pkg" ] || continue # unmatched glob stays literal — skip it
	workspace=$(dirname -- "$pkg")
	found=1
	echo ">> building assets in ${workspace#"$root"/}"
	( cd -- "$workspace" && npm ci && npm run build --if-present )
done

[ "$found" -eq 1 ] || {
	echo "build-assets: no asset workspaces (web/*/package.json) found" >&2
	exit 1
}

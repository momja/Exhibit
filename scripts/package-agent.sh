#!/bin/sh
# Produce a relocatable Linux release for local hosts or Sprite Services.
# Dependencies: Go, curl, tar, xz, sha256sum. No Docker or core asset build.
set -eu

repository=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
release_dir=${1:?usage: scripts/package-agent.sh OUTPUT_DIRECTORY [amd64|arm64]}
case "$(uname -m)" in x86_64) host_arch=x64; default_arch=amd64 ;; aarch64|arm64) host_arch=arm64; default_arch=arm64 ;; *) echo "unsupported build host" >&2; exit 1 ;; esac
architecture=${2:-$default_arch}
case "$architecture" in amd64) node_arch=x64 ;; arm64) node_arch=arm64 ;; *) echo "unsupported architecture" >&2; exit 1 ;; esac
node_version=22.22.0
pi_version=1.3.0

# Refuse an existing destination so packaging cannot delete somebody's files.
mkdir "$release_dir"
release_dir=$(CDPATH= cd -- "$release_dir" && pwd)
download_dir=$(mktemp -d)
trap 'rm -rf "$download_dir"' EXIT HUP INT TERM
archive="node-v${node_version}-linux-${node_arch}.tar.xz"
curl --fail --silent --show-error --location "https://nodejs.org/dist/v${node_version}/${archive}" -o "$download_dir/$archive"
curl --fail --silent --show-error --location "https://nodejs.org/dist/v${node_version}/SHASUMS256.txt" -o "$download_dir/SHASUMS256.txt"
(cd "$download_dir" && awk -v file="$archive" '$2 == file {print}' SHASUMS256.txt > selected.sha256 && test -s selected.sha256 && sha256sum -c selected.sha256)
mkdir "$release_dir/node"
tar -xJf "$download_dir/$archive" --strip-components=1 -C "$release_dir/node"
cp "$repository/cmd/exhibit-agent/runtime/package.json" "$repository/cmd/exhibit-agent/runtime/package-lock.json" "$release_dir/"
# npm itself must run with the bundled Node, not whichever version built core.
build_node="$release_dir/node"
if [ "$host_arch" != "$node_arch" ]; then
  host_archive="node-v${node_version}-linux-${host_arch}.tar.xz"
  curl --fail --silent --show-error --location "https://nodejs.org/dist/v${node_version}/${host_archive}" -o "$download_dir/$host_archive"
  (cd "$download_dir" && awk -v file="$host_archive" '$2 == file {print}' SHASUMS256.txt > host.sha256 && test -s host.sha256 && sha256sum -c host.sha256)
  mkdir "$download_dir/host-node"
  tar -xJf "$download_dir/$host_archive" --strip-components=1 -C "$download_dir/host-node"
  build_node="$download_dir/host-node"
fi
PATH="$build_node/bin:$PATH" "$build_node/bin/node" "$release_dir/node/lib/node_modules/npm/bin/npm-cli.js" ci --prefix "$release_dir" --cpu="$node_arch" --os=linux --ignore-scripts --engine-strict --omit=dev --no-audit --no-fund
(cd "$repository" && CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' -o "$release_dir/exhibit-agent" ./cmd/exhibit-agent)
cp "$repository/cmd/exhibit-agent/runtime/launch" "$release_dir/launch"
chmod 755 "$release_dir/launch"
printf 'protocol=1\npi=%s\nnode=%s\narchitecture=%s\n' "$pi_version" "$node_version" "$architecture" > "$release_dir/versions"
(cd "$release_dir" && sha256sum exhibit-agent launch package-lock.json > checksums.sha256)
if [ "$host_arch" = "$node_arch" ]; then "$release_dir/launch" --version; fi

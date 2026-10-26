#!/usr/bin/env bash
# Package a prebuilt, pinned runtime tree; no network or package installation.
set -euo pipefail
if [[ $# != 3 ]]; then
  echo 'usage: package-release.sh VERSION RUNTIME_DIRECTORY OUTPUT.tar.gz' >&2
  exit 2
fi
release_version=$1
runtime_directory=$(realpath -- "$2")
release_output=$(realpath -m -- "$3")
if [[ ! $release_version =~ ^[a-zA-Z0-9][a-zA-Z0-9._-]{0,95}$ ]]; then
  echo 'release version must be an immutable revision/version' >&2
  exit 2
fi
for executable in exhibit-agent node pi; do
  if [[ ! -x "$runtime_directory/bin/$executable" ]]; then
    echo "missing executable bin/$executable in prepared runtime" >&2
    exit 2
  fi
  executable_target=$(realpath -- "$runtime_directory/bin/$executable")
  case "$executable_target" in
    "$runtime_directory"/*) ;;
    *) echo 'runtime executables must resolve inside the prepared tree' >&2; exit 2 ;;
  esac
done
case "$release_output" in
  "$runtime_directory"/*) echo 'output must be outside the runtime directory' >&2; exit 2 ;;
esac
if [[ -e $release_output || -e $release_output.sha256 ]]; then
  echo 'refusing to overwrite an existing release' >&2
  exit 2
fi
# Include the full tree: bin/pi may be a relative symlink into lib/node_modules,
# and the worker embeds its extensions. Never stage credentials or scratch here.
mkdir -p "$(dirname "$release_output")"
release_staging=$(mktemp -d "$(dirname "$release_output")/.release-XXXXXXXX")
trap 'rm -rf "$release_staging"' EXIT
printf '%s\n' "$release_version" > "$release_staging/EXHIBIT_RELEASE"
# SOURCE_DATE_EPOCH is a build timestamp, not a credential. Normalizing ownership
# and timestamps makes repeated packaging of the same tree reproducible.
build_epoch=${SOURCE_DATE_EPOCH:-0}
tar --sort=name --mtime="@$build_epoch" --owner=0 --group=0 --numeric-owner \
  -czf "$release_staging/release.tar.gz" -C "$runtime_directory" . \
  -C "$release_staging" EXHIBIT_RELEASE
ln "$release_staging/release.tar.gz" "$release_output"
(cd "$(dirname "$release_output")" && sha256sum "$(basename "$release_output")" > "$(basename "$release_output").sha256")
echo "Packaged $release_version; configure the HTTPS archive URL and SHA256 from $release_output.sha256"

# Agent setup: from source

Start with the prompt and preflight in [agent-setup.md](agent-setup.md). These
steps build a local server with embedded frontend assets and a pinned Pi runtime.
Commands assume the checkout root and a POSIX shell. Keep existing environment,
data and runtime paths when resuming an installation.

## 1. Download and install dependencies

Inspect [go.mod](../go.mod) and [Dockerfile](../Dockerfile) before choosing tool
versions. Use Go at least as new as the `go` directive, the Dockerfile's Node
major with a patch compatible with Pi, and exactly its `PI_VERSION` for Pi.
Do not install an unpinned latest Pi or rely on an older distribution Go/Node.

- **Ubuntu 24.04:** install host tools with:

  ```sh
  sudo apt-get update
  sudo apt-get install -y --no-install-recommends git make curl ca-certificates xz-utils python3 python3-fonttools python3-brotli
  ```

  For missing Go, read the version from `awk '$1 == "go" {print $2; exit}' go.mod`.
  Map `uname -m` to `amd64` for `x86_64` or `arm64` for `aarch64`. Download
  `https://go.dev/dl/go<VERSION>.linux-<ARCH>.tar.gz` using `curl --fail --location`
  into a temporary directory. Verify its SHA-256 against the matching file in
  [Go's download metadata](https://go.dev/dl/?mode=json&include=all), then unpack
  into a **new user-owned versioned directory**, such as
  `$HOME/.local/share/exhibit/toolchains/go-<VERSION>`. Add its `go/bin` directory
  to this setup shell's PATH. Follow [Go's installation reference](https://go.dev/doc/install)
  for layout; do not unpack over an existing Go tree.

  For missing/incompatible Node, read the Node major from the Dockerfile's
  runtime `FROM node:...` line. Download the matching Linux `x64` or `arm64`
  archive and `SHASUMS256.txt` from
  `https://nodejs.org/dist/latest-v<MAJOR>.x/`, using the current filenames there.
  Verify the archive checksum, unpack into a new user-owned versioned directory,
  and add its `bin` to PATH. Use the [official Node downloads](https://nodejs.org/en/download)
  for platform and checksum guidance. Keep the resolved version/path in the
  handoff. A compatible existing install can be reused.
- **macOS:** install command line tools using `xcode-select --install` if Git or
  make is absent. With an existing [Homebrew](https://brew.sh) installation,
  `brew install go python` supplies Go and Python; verify Go against `go.mod`.
  Install the versioned `node@<MAJOR>` formula matching the Dockerfile, then use
  its `bin` directory on PATH. If Homebrew is absent, the agent can download and
  run its official installer after inspecting it; let the person complete any
  administrator prompt. Official Go/Node architecture-specific installers are
  another option. For icon subsetting, create a Python virtual environment and
  install `fonttools[woff]` there, adding that environment's `bin` to PATH so
  `python3 -m fontTools.subset` finds it. Do not modify system Python packages.
- **WSL2:** run the Ubuntu steps inside the distribution, keeping the checkout,
  data, and runtime inside its Linux filesystem. Verify access from the Windows
  browser separately. Native PowerShell is not covered by these shell examples.

Python/fontTools/Brotli are used only to subset the icon font. If unavailable,
the build warns and embeds the full font; this is a working fallback, and should
be mentioned in the handoff. They are included above for a complete asset build.

Install the pinned Pi in an isolated, ignored prefix rather than changing a
global Pi installation:

```sh
exhibit_pi_version=$(sed -n 's/^ARG PI_VERSION=//p' Dockerfile)
test -n "$exhibit_pi_version"
mkdir -p .exhibit-setup
npm install --prefix "$PWD/.exhibit-setup/pi" --global --ignore-scripts --engine-strict \
  "@earendil-works/pi-coding-agent@$exhibit_pi_version"
go version
node --version
npm --version
make --version
.exhibit-setup/pi/bin/pi --version
```

`--engine-strict` refuses incompatible Node versions. Keep Node installed at
runtime: the server launches Pi through its Node shebang. Preserve the resolved
Go/Node/Python PATH for the launcher below; interactive version-manager setup
alone is insufficient for a background service.

## 2. Build and persist configuration

```sh
go mod download
make build
```

`make build` runs `npm ci` in each asset workspace before compiling `bin/server`.
Do not skip it in favor of a bare `go build`; generated assets are not committed.

For a new installation create `.env.setup` exclusively (an existing file should
be reused, not overwritten). Choose free ports first and adapt these values if
necessary. The file is **shell syntax**, read only by the launcher created here:

```sh
python3 - <<'PY'
import os
from pathlib import Path
import secrets
import shlex

root = Path.cwd().resolve()
settings = {
    'AUTH_TOKEN': secrets.token_hex(32),
    'APP_ORIGIN': 'http://localhost:8080',
    'RENDER_ORIGIN': 'http://localhost:8081',
    'ADDR': '127.0.0.1:8080',
    'RENDER_ADDR': '127.0.0.1:8081',
    'DATA_DIR': str(root / 'data'),
    'PI_BIN': str(root / '.exhibit-setup/pi/bin/pi'),
    'PATH': os.environ['PATH'],
}
fd = os.open('.env.setup', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w') as config:
    for name, value in settings.items():
        config.write(name + '=' + shlex.quote(value) + '\n')
PY
```

Review inherited Exhibit variables by name before launch, especially `CLERK_*`,
`OIDC_*`, `LOGIN_*`, `AGENT_*` and storage settings. Unset unintended values in
the launch shell, preserving deliberate existing configuration. Self-hosting
requires no Clerk configuration. The server does **not** automatically load
`.env`, `.env.local`, or `.env.setup`. Create this launcher once:

```sh
(set -C; cat > .exhibit-setup/run <<'SH'
#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
set -a
. ./.env.setup
set +a
exec ./bin/server "$@"
SH
)
chmod 700 .exhibit-setup/run
```

Only source the file this setup created and you control; shell configuration is
executable. Optional settings go into this file with shell quoting; consult
[deployment §2](deployment.md#2-configuration) rather than copying every option.
Keep `DATA_DIR` absolute and stable, including for account subcommands. For a
new installation, verify it is empty, then create the first admin account
**before** starting the server, entering a password locally with terminal echo
disabled (or via a private stdin prompt supplied by your agent's terminal tool):

```sh
.exhibit-setup/run user add admin
```

On an existing installation, use `.exhibit-setup/run user list` to inspect
accounts and retain them. If already booted with the default account, change its
password via `.exhibit-setup/run user passwd admin` or the Accounts UI. Do not
put a plaintext password into a shell argument, chat message, or saved command.

## 3. Launch and verify

Start in the foreground once to inspect startup errors:

```sh
.exhibit-setup/run
```

After checking it, stop with Ctrl-C. For a simple background session:

```sh
nohup .exhibit-setup/run > .exhibit-setup/server.log 2>&1 < /dev/null &
exhibit_pid=$!
printf '%s\n' "$exhibit_pid" > .exhibit-setup/server.pid
```

Only start this when no instance is running; otherwise a failed second launch
can replace the PID file. Check the log and actual process before recording
success. Finish the shared [verification](agent-setup.md#verify-the-installation),
including credential entry through the Exhibit UI and an artifact/state test.

## 4. Stop, restart, and recover

```sh
tail -n 100 -f .exhibit-setup/server.log
```

For a background process, read its PID and inspect the process before stopping:

```sh
exhibit_pid=$(cat .exhibit-setup/server.pid)
ps -p "$exhibit_pid" -o pid=,args=
# Only after confirming this is this installation's bin/server:
kill -TERM "$exhibit_pid"
```

Wait for that process to exit, then start again with the background command
above. A PID file can be stale; never kill an unrelated process just because its
PID matches. This loads saved environment changes on every restart. This simple
background process does not start after reboot; use the platform service manager
if requested, as described in the [handoff](agent-setup.md#network-access-and-handoff).

- **Go too old:** compare `go version` with `go.mod`, install the required
  toolchain, and update the launch/build PATH. Do not lower `go.mod` to match it.
- **Missing assets / `pattern assets: no matching files found`:** use `make build`
  and fix the first failed npm workspace; do not create placeholder assets.
- **`EACCES` from npm:** use the user-owned Pi prefix above; avoid `sudo npm`.
- **Pi missing or Node engine error:** verify the pinned executable and Node
  through the saved PATH, set absolute `PI_BIN`, then restart. A working gallery
  with a 503 agent response is a broken runtime installation.
- **Settings not applied / wrong data:** start through the launcher, inspect the
  selected config path without dumping its secrets, and verify the absolute
  data path. Running `bin/server` directly does not load `.env.setup`.
- **Port conflict or failed render:** inspect listeners, change both local
  addresses and their corresponding origins, then restart and repeat checks.
- **Provider key missing:** complete the UI credential step; setting a key in
  your shell for an unrelated coding agent does not configure Exhibit.

On a re-run, keep `.env.setup`, the data directory and its `secret.key`, and the
launcher. Reinstall only a missing/broken pinned runtime. Do not rebuild an
existing installation onto a new revision without treating it as an upgrade.

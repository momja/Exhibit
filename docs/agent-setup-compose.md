# Agent setup: Docker Compose

Start with the prompt and preflight in [agent-setup.md](agent-setup.md). Run these
steps from the checkout root. The examples create a **new local installation**;
on a re-run, keep existing configuration, credentials and the same Compose
project/data volume. The base `docker-compose.yml` publishes ports on every
interface; the local overlay below replaces those bindings with loopback ones.

## 1. Install prerequisites

Install Git, Python 3 (for generating configuration) and Docker with the Compose
plugin. Go, Node, npm, and Pi are installed **inside the image**, so they are not
host prerequisites for this path.

- **Ubuntu 24.04:** run `sudo apt-get update` and
  `sudo apt-get install -y git python3 ca-certificates curl`. If Docker is absent,
  follow Docker's [Ubuntu apt repository installation](https://docs.docker.com/engine/install/ubuntu/#install-using-the-apt-repository)
  to install `docker-ce`, `docker-ce-cli`, `containerd.io`, `docker-buildx-plugin`
  and `docker-compose-plugin`. Execute the official repository/keyring steps,
  then `sudo systemctl enable --now docker`. Inspect existing Docker packages
  before replacing them. Use `sudo docker` if the current user lacks access;
  do not silently add a user to the privileged Docker group.
- **macOS:** install Git with `xcode-select --install` if necessary; let the person
  complete its GUI. Install [Docker Desktop for the detected architecture](https://docs.docker.com/desktop/setup/install/mac-install/),
  then open it and wait for the engine. If Homebrew is already installed,
  `brew install --cask docker` and `brew install python` are alternatives. Have
  the person complete any first-run licensing/privilege prompts.
- **Windows/WSL2:** use an Ubuntu terminal and the same Git/Python prerequisites.
  Follow [Docker Desktop WSL integration](https://docs.docker.com/desktop/features/wsl/).
  Do not install a competing engine inside a distribution already using Desktop.

Check the installed tools:

```sh
git --version
python3 --version
docker version
docker compose version
docker info
```

Both Docker client and server must be available. The overlay uses
[`!override`](https://docs.docker.com/reference/compose-file/merge/#replace-value),
which requires Compose **2.24.4 or later**. Upgrade the Compose plugin if older;
do not remove the tag, since ordinary merging would retain the public bindings.

## 2. Save configuration

Choose free host ports before creating files; these examples use 8080/8081.
Create `.env.setup` once, without printing its generated token:

```sh
python3 - <<'PY'
import os
import secrets

fd = os.open('.env.setup', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w') as config:
    config.write('AUTH_TOKEN=' + secrets.token_hex(32) + '\n')
    config.write('APP_ORIGIN=http://localhost:8080\n')
    config.write('RENDER_ORIGIN=http://localhost:8081\n')
PY
```

An existing file makes this command fail intentionally. Inspect and reuse it;
do not delete it to retry. `.env.setup` is for this installation; existing `.env`
and `.env.local` files should not be imported unintentionally.

Create `.exhibit-setup/compose.yaml` once (exclusive creation prevents clobbering):

```sh
mkdir -p .exhibit-setup
(set -C; cat > .exhibit-setup/compose.yaml <<'YAML'
services:
  app:
    ports: !override
      - "127.0.0.1:8080:8080"
      - "127.0.0.1:8081:8081"
    environment:
      ADDR: ":8080"
      RENDER_ADDR: ":8081"
    env_file:
      - .env.setup
    restart: unless-stopped
YAML
)
```

All overlay paths resolve against the **first** Compose file, the repository's
`docker-compose.yml`, so `.env.setup` is at the checkout root. `--env-file` below
supplies interpolation for variables the base file lists; `env_file` also passes
additional settings such as `EXHIBIT_SECRET`, `LOGIN_*`, or `OIDC_*` to the app.
The service's explicit `DATA_DIR=/data` and overlay listen addresses still win
over values in the env file. Keep `/data` inside the container; its named volume
stores the database, blobs, encryption secret, and agent sessions.

If choosing different local ports, change **both sides** of each overlay port
mapping, the overlay's corresponding listen address, and the origins in
`.env.setup`. For example, app port 8180 means `127.0.0.1:8180:8180`, `ADDR: ":8180"`,
and `APP_ORIGIN=http://localhost:8180`. The Pi subprocess calls `APP_ORIGIN` from
inside the container, so a loopback origin's port must match its internal
listener too. Local-only setup needs no other
settings. Leave provider credentials for the UI step in the shared guide.
Use [deployment §2](deployment.md#2-configuration) for optional settings.

## 3. Build and start

In a fresh installation, choose and keep the project name `exhibit`. If an
instance already exists, use its project name and volume instead; a new name
creates a different empty library. Define this shell helper in the checkout:

```sh
exhibit_compose() {
  docker compose --project-name exhibit --env-file .env.setup \
    -f docker-compose.yml -f .exhibit-setup/compose.yaml "$@"
}
exhibit_compose config --quiet
exhibit_compose up --build -d app
exhibit_compose ps app
exhibit_compose logs --tail=60 app
exhibit_compose exec -T app pi --version
exhibit_compose exec -T app node -e 'fetch(process.env.APP_ORIGIN + "/auth/login").then(r => { if (!r.ok) process.exit(1); console.log("App origin reachable from container"); }).catch(() => process.exit(1))'
```

The helper must be redefined in a new shell. Save its exact invocation in the
handoff. Use `config --quiet`, not a full config/environment dump that exposes
secrets. Exported shell variables take precedence over the interpolation file:
if a value is unexpectedly different, inspect and unset only the conflicting
Exhibit variable in this shell and recreate the service.

Change the seeded admin password in a local terminal, with terminal echo
disabled while it is typed (restore it even if interrupted), or sign in and use
the Accounts UI:

```sh
exhibit_compose exec app /server user passwd admin
```

Do not reset an existing installation's password automatically. Follow
[Verify the installation](agent-setup.md#verify-the-installation), including an
artifact/state round trip and provider setup. Only the app is needed here;
Chromium, MinIO, replication, OIDC, and hosted Clerk are separate optional choices.

## 4. Operate and recover

Run from the checkout with the helper defined:

```sh
exhibit_compose stop app                 # stop, preserve data
exhibit_compose up -d app                # start; also applies env/overlay changes
exhibit_compose restart app              # restart with the current container config
exhibit_compose logs --tail=100 -f app   # Ctrl-C stops following logs
exhibit_compose ps app
```

Record the actual volume name and mount returned by
`docker inspect --format '{{json .Mounts}}' "$(exhibit_compose ps -q app)"`.
Never use `down -v` or prune volumes as a setup/recovery step. `restart` alone
does **not** apply changed environment values; use `up -d app` for those.
`unless-stopped` restarts the container with the Docker engine; Docker Desktop
must itself be running after login on macOS/Windows.

- **Cannot connect to Docker:** start the daemon/Desktop, check the selected
  Docker context, and handle the actual permission error. Do not start another
  engine against an existing installation.
- **Unknown `!override` tag:** install a supported Compose version.
- **Port already allocated:** identify the owner, choose free local ports, and
  update both sides of the mappings, listen addresses, and origins together
  before `up -d app`.
- **Build/download failure:** read the failed build step; check disk space,
  registry/npm/Go access and the machine's proxy/CA setup. Retry the build after
  fixing that cause; do not erase application data.
- **Login/settings ignored:** confirm this exact helper and `.env.setup` are
  used. Additional settings require the overlay's `env_file`; the base file does
  not forward every setting by itself. Change the initial password via stdin/UI.
- **Broken iframe or agent:** check that both origins are reachable from the
  browser and the server can reach `APP_ORIGIN`. See logs and the
  [shared checks](agent-setup.md#verify-the-installation); an absent Pi executable
  requires rebuilding the image, while a missing provider key is a UI setup step.

For remote access, follow the [network handoff](agent-setup.md#network-access-and-handoff)
and keep the loopback backend bindings when the proxy runs on this same host.
If the proxy is in a container, route it over a private Docker network instead;
its `localhost` is not the host's listener.

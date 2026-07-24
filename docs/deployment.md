# Exhibit — Deployment

How to run Exhibit as a Docker container: the image, `docker run`/Compose usage,
every configuration setting, data persistence, backups, and the AI agent surface's
opt-out. See `technical_stack.md` §12 for the reasoning behind these choices;
this doc is the practical how-to.

## 1. The image

`Dockerfile` is a three-stage build:

1. **`assets`** (Node) — bundles CodeMirror, Phosphor Icons, and the gallery's JS/CSS
   into `internal/api/assets/` via `scripts/build-assets.sh`. Build-time only.
2. **`builder`** (Go) — compiles `cmd/server` into a static, `CGO_ENABLED=0` binary
   with the assets from stage 1 embedded (`go:embed`).
3. **Runtime** (`node:22-bookworm-slim`) — copies in the static binary. The base image
   is Node, not `distroless`/`scratch`, **only** because it also installs the `pi`
   agent harness as a global npm package for the AI agent sidecar (§3.7 of
   `architecture.md`) — `pi` ships as an npm bin with a `#!/usr/bin/env node`
   shebang, which needs a real userland. The server binary itself has no runtime
   dependency on Node. See §5 if you want to drop this.

```bash
docker build -t exhibit .
```

The image exposes two ports — `8080` (app surface: gallery + API) and `8081`
(render surface: sandboxed artifact rendering) — and declares `/data` as a volume
for the SQLite database and blob store.

## 2. Quick start: `docker run`

```bash
docker run -p 8080:8080 -p 8081:8081 \
  -e AUTH_TOKEN=changeme \
  -e APP_ORIGIN=https://app.example.com \
  -e RENDER_ORIGIN=https://artifacts.example.com \
  -v exhibit-data:/data \
  exhibit
```

Set a real `AUTH_TOKEN` and correct `APP_ORIGIN`/`RENDER_ORIGIN` before exposing this
beyond localhost — see §4 and §6.

## 3. `docker-compose.yml`

The repo ships a Compose file with three services, only one of which runs by default:

```bash
docker compose up                          # app only — single SQLite file, nothing else
docker compose --profile replication up    # + Litestream (async backup / restore)
docker compose --profile replication-local up   # + Litestream + a local MinIO bucket
```

- **`app`** — always runs. Reads `APP_ORIGIN`, `RENDER_ORIGIN`, and `AUTH_TOKEN` from
  a `.env` file or your shell (defaults to `localhost` origins and `dev-token` if
  unset — fine for local use, not for anything reachable over a network).
- **`litestream`** (profile `replication`) — streams the SQLite WAL to an
  S3-compatible bucket and supervises the app process (`replicate -exec "/server"`),
  so a fresh/empty volume auto-restores from the last backup before the app starts.
  Needs `LITESTREAM_ACCESS_KEY_ID`, `LITESTREAM_SECRET_ACCESS_KEY`, and
  `LITESTREAM_BUCKET_URL` in your environment, plus a `litestream.yml` config file
  next to the Compose file (mounted at `/etc/litestream.yml`) — **this file is not
  included in the repo**; write your own pointing at your bucket before enabling
  this profile. This is async backup / point-in-time restore, not live HA.
- **`minio`** (profile `replication-local`) — an optional local S3 target for
  homelab use when you don't already have a bucket. `MINIO_ROOT_USER` /
  `MINIO_ROOT_PASSWORD` default to `minioadmin`/`minioadmin` — change these before
  exposing port 9000/9001 beyond localhost.

Treat `docker-compose.yml` as a documented starting point, not a fixed deployment
target: TLS termination, hostnames, and how far you take the backup profile are
yours to compose around it.

## 4. Configuration reference

All configuration is environment variables read once at startup
(`cmd/server/main.go`). None require a restart-free reload.

| Variable | Default | Meaning |
|----------|---------|---------|
| `ADDR` | `:8080` | Listen address for the app surface (gallery + API) |
| `RENDER_ADDR` | `:8081` | Listen address for the render surface (artifact iframes) |
| `APP_ORIGIN` | `http://localhost:8080` | Public URL of the app surface; used to build links and as the postMessage target the render-surface bridges trust |
| `RENDER_ORIGIN` | `http://localhost:8081` | Public URL of the render surface. **Must be a different origin from `APP_ORIGIN`** — this separation is the core sandboxing boundary (`architecture.md` §4) |
| `AUTH_TOKEN` | `dev-token` | Static bearer token required on every API call. Change this before deploying anywhere reachable over a network |
| `DATA_DIR` | `./data` | Directory for the SQLite database (`app.db`) and blob storage (`blobs/`) |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`; unrecognized values fall back to `info` |
| `DEBUG` | unset | Any non-empty value forces debug-level logging, overriding `LOG_LEVEL` |
| `PI_BIN` | `pi` | Executable name/path for the `pi` agent harness. If not found on `PATH`, the AI agent surface disables itself and nothing else changes — see §5 |
| `EXHIBIT_SECRET` | unset | Server secret used to encrypt stored BYO agent provider API keys (AES-256-GCM). If unset, a random key is generated once and persisted at `DATA_DIR/secret.key` (mode `0600`). Only relevant if the agent surface is enabled |
| `MOCK_LLM_URL` | unset | Dev/test only — base URL of `cmd/mockllm`, a deterministic OpenAI-compatible server. Registers the `exhibit-mock` provider for testing the agent pipeline without real provider credentials. Leave unset in production |

`APP_ORIGIN` and `RENDER_ORIGIN` are the two hostnames you point a reverse proxy at
(§6); the values here are what the server puts in URLs, cookies-equivalent auth
checks, and the CSP it generates — they must match what visitors actually see in
their browser, including scheme.

## 5. Deploying without the AI agent components

The agent sidecar (`pi`, `EXHIBIT_SECRET`, the `/agent` chat UI) is entirely
optional at runtime: `main.go` does `exec.LookPath(piBin)` at startup, and if `pi`
isn't found it logs a warning and disables the surface — nothing else in the app
changes (no code paths assume it's present). Two ways to get an AI-free deployment:

**Minimal effort — starve the shipped image.** Point `PI_BIN` at a name that
doesn't exist:

```bash
docker run ... -e PI_BIN=disabled exhibit
```

`pi` is still installed in the container (unused), so the image is the same size,
but the agent surface never activates.

**Smaller image — build a custom runtime stage.** Since the server binary itself
is a static `CGO_ENABLED=0` Go binary with no Node dependency, replace the final
stage of `Dockerfile` with a minimal base and skip the `pi` install entirely:

```dockerfile
# --- Runtime stage (no AI agent surface) ---
FROM gcr.io/distroless/static-debian12

COPY --from=builder /bin/server /server

VOLUME ["/data"]
ENV DATA_DIR=/data
ENV ADDR=:8080
ENV RENDER_ADDR=:8081

EXPOSE 8080 8081
ENTRYPOINT ["/server"]
```

Keep the `assets` and `builder` stages unchanged. This drops the Node base image
and the `@earendil-works/pi-coding-agent` npm package, landing back at the
`distroless`/`scratch`-sized image the core product targets (`technical_stack.md`
§2, §12).

## 6. Data persistence

Everything durable lives under `DATA_DIR` (`/data` in the container):

- `app.db` — the SQLite database (metadata, search index, state, agent transcripts
  if the agent surface is enabled). WAL mode.
- `blobs/` — artifact bodies, one file per blob.
- `secret.key` — generated only if `EXHIBIT_SECRET` is unset and the agent surface
  is enabled.

Mount `/data` as a named volume or bind mount; without it, all data is lost when
the container is removed. There is no separate database service to provision —
SQLite is embedded in the binary.

## 7. Reverse proxy / TLS

Exhibit does not terminate TLS or ship a proxy — it serves plain HTTP on the two
ports above and expects a proxy in front (Caddy, Traefik, nginx, a cloud load
balancer) to route `APP_ORIGIN` and `RENDER_ORIGIN` to them and handle certificates.
The only hard requirement is that the two origins are genuinely different
hostnames — this is what keeps artifact code from reaching the app's cookies and
storage (`architecture.md` §4). A single wildcard cert covering both is enough;
per-artifact subdomains are an optional hardening step, not a requirement
(`technical_stack.md` §12).

## 8. See also

- `architecture.md` — component boundaries and why the two-origin split exists.
- `security.md` — the full CSP / sandbox / capability-bridge policy.
- `agent.md` — the AI agent sidecar in detail, if you're keeping it enabled.
- Root `README.md` — build-from-source instructions for non-Docker setups.

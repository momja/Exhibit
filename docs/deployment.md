# Exhibit — Deployment

> [!WARNING]
> Exhibit is in very early development. Breaking changes are likely, and there
> is no guarantee of upgrade compatibility. Use at your own risk.

## 1. Run it

Clone the repo:

```bash
git clone https://github.com/momja/Exhibit.git
cd Exhibit
```

Then run it with Compose:

```bash
AUTH_TOKEN=changeme \
APP_ORIGIN=https://app.example.com \
RENDER_ORIGIN=https://artifacts.example.com \
docker compose up
```

Open `APP_ORIGIN`. Two ports: `8080` is the gallery/API, `8081` is where artifacts
render — they must be different origins (put them on different hostnames behind
your proxy; see [Reverse proxy / TLS](#4-reverse-proxy--tls)). Any of the env vars
below can be set the same way; omit them all and it defaults to `localhost`
origins and a `dev-token` auth token, which is fine for trying it out locally.

## 2. Configuration

Env vars, all optional except `AUTH_TOKEN`.

> [!NOTE]
> Exhibit is currently single-tenant, single-user: there's no login system, and
> `AUTH_TOKEN` is a single shared static token, not a real auth boundary. If
> you're exposing an instance beyond your own machine, put your own
> authentication in front of it (e.g. at your reverse proxy) or be comfortable
> with the consequences of running without one.

| Variable | Default | Meaning |
|----------|---------|---------|
| `AUTH_TOKEN` | `dev-token` | API bearer token — **change this** |
| `APP_ORIGIN` | `http://localhost:8080` | Public URL of the gallery/API |
| `RENDER_ORIGIN` | `http://localhost:8081` | Public URL of the artifact renderer (must differ from `APP_ORIGIN` — see [why](./architecture.md#4-trust-boundaries)) |
| `DATA_DIR` | `./data` | Where the SQLite DB + blobs live |
| `ADDR` | `:8080` | App listen address |
| `RENDER_ADDR` | `:8081` | Render listen address |
| `LOG_LEVEL` / `DEBUG` | `info` | `debug`/`info`/`warn`/`error`; `DEBUG=1` forces debug |
| `PI_BIN` | `pi` | AI agent executable — unset/missing just disables that feature |
| `EXHIBIT_SECRET` | auto | Encrypts stored agent API keys; auto-generated if unset |

## 3. No AI agent features

Nothing to configure — if `pi` isn't on `PATH`, the agent surface disables itself
automatically. To shrink the image too, drop the AI stuff at build time by
swapping `Dockerfile`'s runtime stage:

```dockerfile
FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/server /server
VOLUME ["/data"]
ENV DATA_DIR=/data ADDR=:8080 RENDER_ADDR=:8081
EXPOSE 8080 8081
ENTRYPOINT ["/server"]
```

(Keep the `assets` and `builder` stages as-is.)

## 4. Reverse proxy / TLS

Bring your own (Caddy, nginx, Traefik, a cloud LB). Exhibit speaks plain HTTP;
point your proxy's two hostnames at `APP_ORIGIN`/`RENDER_ORIGIN` and terminate
TLS there. They must be different hostnames — that's the artifact sandbox
boundary, not just cosmetics.

## 5. Backups (optional)

`docker-compose.yml` includes a `replication` profile that runs Litestream
(streams the SQLite WAL to S3). You'll need your own `litestream.yml` (not
included) and `LITESTREAM_ACCESS_KEY_ID`/`LITESTREAM_SECRET_ACCESS_KEY`/
`LITESTREAM_BUCKET_URL`. Skip this to start — plain SQLite on a mounted volume
is fine until you need it.

---

More detail: [architecture.md](./architecture.md) (why two origins),
[security.md](./security.md) (CSP/sandbox policy), [agent.md](./agent.md) (the
AI agent sidecar).

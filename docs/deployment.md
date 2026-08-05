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
docker compose up
```

or configure your origins (auth token is currently a placeholder. It's required but does not actually authenticate):

```bash
AUTH_TOKEN=changeme \
APP_ORIGIN=https://app.example.com \
RENDER_ORIGIN=https://artifacts.example.com \
docker compose up
```

Open `APP_ORIGIN`. Two ports: `8080` is the gallery/API, `8081` is where artifacts
render — they must be different origins (put them on different hostnames behind
your proxy; see [Reverse proxy / TLS](#5-reverse-proxy--tls)). Any of the env vars
below can be set the same way; omit them all and it defaults to `localhost`
origins and a `dev-token` auth token, which is fine for trying it out locally.

## 2. Configuration

Env vars, all optional except `AUTH_TOKEN`.

> [!NOTE]
> Out of the box Exhibit is single-user: there is no login, and `AUTH_TOKEN` is
> a single shared static token, not a real auth boundary. If you're exposing an
> instance beyond your own machine, give it a login — see
> [Logging in](#3-logging-in-optional) for the two supported ways — or be
> comfortable with the consequences of running without one.

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
| `OIDC_ISSUER` | *(unset)* | Identity provider to delegate login to. Unset = single-user, no login |
| `OIDC_CLIENT_ID` | *(unset)* | Client id registered at that provider — required when `OIDC_ISSUER` is set |
| `OIDC_CLIENT_SECRET` | *(unset)* | Client secret, if your provider issues one |

## 3. Logging in (optional)

Two supported ways to put a login in front of an instance. Neither is more
"official" than the other; pick whichever fits what you already run.

### 3.1 Authenticate at your proxy

Exhibit already expects you to bring your own reverse proxy and TLS (§5), and
the same proxy is a perfectly good place to put authentication: Authelia,
Tailscale (or `tailscale serve`), oauth2-proxy, or plain HTTP basic auth all
work, because they gate the request before it ever reaches the app. Nothing
needs configuring in Exhibit for this — leave `OIDC_ISSUER` unset and the
instance stays a single-user library behind whatever door you put on it.

Gate the **app origin**. The render origin serves artifacts and share links,
which are meant to be openable by people who have no account.

### 3.2 Delegate login to an OIDC provider

Set the three `OIDC_*` variables and Exhibit gains its own login: `/auth/login`
sends the browser to your provider (Authorization Code + PKCE), `/auth/callback`
exchanges the code once for a session of Exhibit's own, and `/auth/logout`
revokes that session server-side.

```bash
OIDC_ISSUER=https://auth.example.com/application/o/exhibit/ \
OIDC_CLIENT_ID=exhibit \
OIDC_CLIENT_SECRET=... \
APP_ORIGIN=https://app.example.com \
docker compose up
```

- Register `APP_ORIGIN + /auth/callback` as the redirect URI at your provider.
- Everything else is read from the issuer's
  `/.well-known/openid-configuration`, so any spec-compliant provider —
  Authentik, Keycloak, Zitadel, Dex, Ory, or a hosted one — is configuration
  rather than code. The issuer is contacted once at startup; an unreachable or
  misconfigured one fails the boot rather than the first login.
- The session lives in an `HttpOnly`, `SameSite=Lax` cookie on the app origin
  only, never on `RENDER_ORIGIN` — a cookie readable there would be readable by
  artifact code. It is marked `Secure` when `APP_ORIGIN` is `https://`.
- `AUTH_TOKEN` keeps working for API and CLI clients, which have no browser to
  log in with.

> [!IMPORTANT]
> A user row is created the first time an identity logs in, and user ids start
> at 1 — the same id everything in an existing single-user library is already
> filed under. So on an instance you are upgrading, **complete the first login
> yourself** before letting anyone else in: that first identity adopts the
> existing library.

Exhibit deliberately has no username-and-password login of its own. Owning
passwords means owning hashing, reset mail, verification, rate limiting and
eventually MFA — a product commitment that buys nothing the two options above
don't already cover.

## 4. No AI agent features

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

## 5. Reverse proxy / TLS

Bring your own (Caddy, nginx, Traefik, a cloud LB). Exhibit speaks plain HTTP;
point your proxy's two hostnames at `APP_ORIGIN`/`RENDER_ORIGIN` and terminate
TLS there. They must be different hostnames — that's the artifact sandbox
boundary, not just cosmetics.

## 6. Backups (optional)

`docker-compose.yml` includes a `replication` profile that runs Litestream
(streams the SQLite WAL to S3). You'll need your own `litestream.yml` (not
included) and `LITESTREAM_ACCESS_KEY_ID`/`LITESTREAM_SECRET_ACCESS_KEY`/
`LITESTREAM_BUCKET_URL`. Skip this to start — plain SQLite on a mounted volume
is fine until you need it.

---

More detail: [architecture.md](./architecture.md) (why two origins),
[security.md](./security.md) (CSP/sandbox policy), [agent.md](./agent.md) (the
AI agent sidecar).

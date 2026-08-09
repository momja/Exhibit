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
> [Logging in](#3-logging-in-optional) for the three supported ways — or be
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
| `LOGIN_USERNAME` | *(unset)* | Username for Exhibit's own login. Set with `LOGIN_PASSWORD_HASH` to get a login with no identity server — see [§3.2](#32-log-in-with-a-username-and-password) |
| `LOGIN_PASSWORD_HASH` | *(unset)* | The **bcrypt hash** of that password, not the password. Produce it with the `hash-password` subcommand (§3.2) |
| `OIDC_ISSUER` | *(unset)* | Identity provider to delegate login to. Unset = no OIDC |
| `OIDC_CLIENT_ID` | *(unset)* | Client id registered at that provider — required when `OIDC_ISSUER` is set |
| `OIDC_CLIENT_SECRET` | *(unset)* | Client secret, if your provider issues one |
| `PUBLIC_MODE_ENABLED` | `false` | Publishes this instance's library for reading without a credential. Accepts `true`/`1`/`yes`/`on`; anything unrecognized is read as off |
| `PUBLIC_INSTANCE_NAME` | *(unset)* | What this instance calls itself |
| `PUBLIC_INSTANCE_DESCRIPTION` | *(unset)* | One line about it |
| `PUBLIC_OWNER_ID` | `1` | Whose library the instance publishes — every artifact query filters on an owner, so a public instance has to name one. `1` is the owner a single-user library is already filed under |

The four `PUBLIC_*` variables are read at startup and surfaced at
`GET /api/settings/public`, which answers `{"name", "description"}` with no
authentication when `PUBLIC_MODE_ENABLED` is on, and `404`s when it is off — an
instance that has not opted in does not name itself to a stranger.

**What turning it on actually opens.** Exactly two reads, to a request carrying
no credential at all: `GET /api/artifacts` and `GET /api/artifacts/:id`, scoped
to `PUBLIC_OWNER_ID`'s library. Nothing else changes. Every mutating method
still requires the token or a session — a visitor cannot create, edit, or delete
anything — and so does every other read, including artifact state, agent
transcripts, tags, collections, shares, and the agent's provider key.

**What a visitor does not get: your data inside the tools.** An artifact opened
by an unauthenticated visitor renders with **no state inlined and no state
saved** — it boots empty, and anything it stores lives only until they close the
tab. A tool of yours that tracks runs shows a stranger an empty tracker, not
your runs, and its gallery tile shows its empty state. This is the conservative
default, not a per-artifact choice: publishing a library is one environment
variable, and it should not also publish everything the library's tools hold.
If you *want* a link that carries your data with it, that is what a share
(`/s/:id`) is — a decision you make one artifact at a time, and it still renders
as you see it.

## 3. Logging in (optional)

Three supported ways to put a login in front of an instance. None is more
"official" than the others; pick whichever fits what you already run. Setting
none of them leaves the instance as it has always been: no login, no gate,
`AUTH_TOKEN` and a single owner.

### 3.1 Authenticate at your proxy

Exhibit already expects you to bring your own reverse proxy and TLS (§5), and
the same proxy is a perfectly good place to put authentication: Authelia,
Tailscale (or `tailscale serve`), oauth2-proxy, or plain HTTP basic auth all
work, because they gate the request before it ever reaches the app. Nothing
needs configuring in Exhibit for this — leave `OIDC_ISSUER` unset and the
instance stays a single-user library behind whatever door you put on it.

Gate the **app origin**. The render origin serves artifacts and share links,
which are meant to be openable by people who have no account.

### 3.2 Log in with a username and password

The path that needs nothing else running. Two variables and the instance has a
login page.

![The login page](screenshots/av-q30x/01-login.png)

First, hash the password. Exhibit takes a **bcrypt hash**, never the password
itself, so run the binary's one subcommand and type the password when it asks:

```bash
docker compose run --rm app hash-password
# Enter the password, then press Enter and ctrl-D:
#
# Set this as LOGIN_PASSWORD_HASH (with LOGIN_USERNAME):
# $2a$10$N9qo8uLOickgx2ZMRZoMy...
```

It reads the password from stdin rather than an argument, so it never lands in
your shell history or a process list. Then:

```bash
LOGIN_USERNAME=curator \
LOGIN_PASSWORD_HASH='$2a$10$N9qo8uLOickgx2ZMRZoMy...' \
APP_ORIGIN=https://app.example.com \
docker compose up
```

Single-quote the hash: it contains `$`, which your shell will otherwise eat.

- `/auth/login` now serves a login page, and every gallery page redirects there
  until you sign in. `/auth/logout` revokes the session server-side.
- The session is the same one §3.3 describes — the same cookie with the same
  attributes, the same `sessions` row, the same expiry. There is one session
  layer; a username and password is just a second way to reach it.
- `AUTH_TOKEN` keeps working for API and CLI clients.
- **Why a hash and not the password.** Hashing a plaintext that is sitting in
  the process environment right beside it would protect nothing. Supplying the
  hash means the password exists only in your head — the value in your
  environment, your compose file, and `docker inspect` is useless anywhere
  else. That matters more here than for `AUTH_TOKEN`, because a password is the
  kind of secret people reuse across services.
- Setting one variable of the pair without the other **fails at startup**,
  rather than quietly leaving the instance with no login. So does a
  `LOGIN_PASSWORD_HASH` that is not a bcrypt hash — which is what a pasted-in
  plaintext password looks like.
- There is no registration, no password reset and no email. One credential, set
  at deploy; to change it, generate a new hash and restart. This is what keeps
  the feature small enough to be worth having, and it is the reason it fits a
  single-user library rather than a multi-user product.
- Changing `LOGIN_USERNAME` later relabels the same owner rather than creating a
  second one — your library is not orphaned by a rename.

To combine it with §3.3, set both: the login page then offers the password form
and a button through to your provider.

### 3.3 Delegate login to an OIDC provider

Set the three `OIDC_*` variables and Exhibit gains its own login: `/auth/login`
sends the browser to your provider (Authorization Code + PKCE — or offers the
button that does, if §3.2 is configured too), `/auth/callback` exchanges the
code once for a session of Exhibit's own, and `/auth/logout` revokes that
session server-side.

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
> existing library. The same is true of §3.2's credential, which is one identity
> like any other.

When both this and §3.2 are configured, `/auth/login` presents both and either
lands the same kind of session. They are two identities, so they are two owners:
sign in the way you intend to keep using before you fill the library.

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

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
> [Logging in](#3-logging-in-optional) for the three supported ways, two of
> which also give you more than one user — or be comfortable with the
> consequences of running without one.

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
| `LOGIN_USERNAME` | *(unset)* | Names an account for the bootstrap / break-glass login — how you get in on an empty instance, or after losing a password. Accounts themselves are created with the `user add` subcommand, not here; see [§3.2](#32-log-in-with-a-username-and-password) |
| `LOGIN_PASSWORD_HASH` | *(unset)* | The **bcrypt hash** of that password, not the password. Produce it with the `hash-password` subcommand. Set with `LOGIN_USERNAME`; it stays accepted for that account for as long as both are set (§3.2) |
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

Proxy auth ([§3.1](#31-authenticate-at-your-proxy)) secures a **single-user**
library — it is one door in front of one owner. The other two each give you as
many people with separate libraries as you like: accounts Exhibit issues itself
([§3.2](#32-log-in-with-a-username-and-password)) or identities from your
provider ([§3.3](#33-delegate-login-to-an-oidc-provider)). Read
[§3.4](#34-running-it-for-more-than-one-person) before you offer anyone an
account either way — the pieces Exhibit does not have yet are the ones that
matter operationally.

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

The path that needs nothing else running. Accounts live in Exhibit's own
database, so you can have one or several, and you create them yourself — there
is no registration form and no email anywhere in this.

![The login page](screenshots/av-q30x/01-login.png)

**Create an account.** The `user` subcommands read the password from stdin
rather than an argument, so it never lands in your shell history or a process
list:

```bash
docker compose run --rm app user add curator@example.com
# Enter a password for curator@example.com, then press Enter and ctrl-D:
# created curator@example.com (owner id 1) — the first account on an instance is its admin
# restart the server so it starts requiring a login

docker compose run --rm app user add partner@example.com   # a second person
docker compose run --rm app user list                      # who exists
docker compose run --rm app user passwd curator@example.com  # forgot it
```

The login name is the account's key. It is folded to lowercase and trimmed, so
`Curator@Example.com` and `curator@example.com` are one account, not two.
Anything works as a name; an email address is the convention because it is what
the surrounding UI labels it as.

Restart the server after the **first** account — that is what tells it the
instance is no longer single-user and to start requiring a login. Later
accounts need no restart.

That is all. `/auth/login` now serves a login page, every gallery page redirects
there until you sign in, and `/auth/logout` revokes the session server-side.

**The first account on an instance is its admin.** That is the same rule as
§3.4's callout below, seen from the other side: user ids start at 1, which is
the id everything in an existing single-user library is already filed under, so
the first account adopts that library. Create yours first. (The admin flag is
recorded now and is what the coming admin screen will check; today it grants
nothing at runtime.)

#### The bootstrap and break-glass credential

`LOGIN_USERNAME` / `LOGIN_PASSWORD_HASH` still work, and are worth
understanding rather than skipping: they are how you get in when the database
cannot help you — an empty instance with no account yet, or an account whose
password you have lost.

```bash
docker compose run --rm app hash-password
# Enter the password, then press Enter and ctrl-D:
#
# Set this as LOGIN_PASSWORD_HASH (with LOGIN_USERNAME):
# $2a$10$N9qo8uLOickgx2ZMRZoMy...
```

```bash
LOGIN_USERNAME=curator@example.com \
LOGIN_PASSWORD_HASH='$2a$10$N9qo8uLOickgx2ZMRZoMy...' \
APP_ORIGIN=https://app.example.com \
docker compose up
```

Single-quote the hash: it contains `$`, which your shell will otherwise eat.

- **It names an account.** `LOGIN_USERNAME` is a login name like any other, and
  `LOGIN_PASSWORD_HASH` is an *additional* password accepted for it. On an empty
  instance that creates the account (and by the rule above, makes it the admin).
  On a populated one it lets you into the existing account of that name — your
  own library, not a separate rescue account. The stored password keeps working
  alongside it, so you can log in, run `user passwd`, and be back to normal
  without another restart.
- **It stays live for as long as it is set** — it does not stop working once
  accounts exist. That is deliberate, and it is a trade: it means anyone who can
  read your process environment can log in as that account, and disabling the
  account in the database will not stop them. The alternative — expiring it at
  the first account — would remove exactly the case it exists for, leaving a
  locked-out operator with no way in short of editing SQLite by hand. It is also
  a bypass you already grant: `AUTH_TOKEN` sits in the same environment and is
  full access to every API route.
- **Turn it off by unsetting both variables and restarting.** Accounts you
  created with `user add` keep working, so this is a safe thing to do once
  you are in. Startup logs a warning while it is enabled, so it cannot be left
  on unnoticed.
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

#### Either way

- The session is the same one §3.3 describes — the same cookie with the same
  attributes, the same `sessions` row, the same expiry. There is one session
  layer; a username and password is just a second way to reach it.
- `AUTH_TOKEN` keeps working for API and CLI clients.
- There is no self-registration, no password reset flow and no email. You create
  accounts and you reset passwords, from the CLI. That is what keeps this small
  enough to be worth having: no SMTP to configure and nothing to verify, because
  you vouched for the account by creating it.

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
  log in with. It stops being handed to browsers: once a provider is configured,
  the gallery pages embed no bearer token at all and authenticate on the session
  cookie, so logging out really does end that browser's API access
  (`security.md` §1.5).

> [!IMPORTANT]
> A user row is created the first time an identity logs in, and user ids start
> at 1 — the same id everything in an existing single-user library is already
> filed under. So on an instance you are upgrading, **complete the first login
> yourself** before letting anyone else in: that first identity adopts the
> existing library, and is also the instance's admin. The same is true of §3.2's
> accounts — the two kinds of user share one table and one id space, and
> whichever arrives first is first.

When both this and §3.2 are configured, `/auth/login` presents both and either
lands the same kind of session. They are two identities, so they are two owners:
sign in the way you intend to keep using before you fill the library.

### 3.4 Running it for more than one person

**Two of the three give you multiple users**, and they are not exclusive — an
instance can run both at once, and a person who signs in each way is two
accounts, not one. Proxy auth (§3.1) is the exception: it is one door in front
of one library.

- **§3.2, accounts Exhibit issues.** You are the user directory. You create
  accounts with `user add` and reset passwords with `user passwd`; there is no
  self-registration, so there is nothing to verify and no mail to send. This is
  the household and small-team path — nothing else to run.
- **§3.3, identities from your provider.** Your provider is the user directory.
  Granting someone access to the Exhibit application there is the whole of
  provisioning; Exhibit only records that it has met them. This is the path for
  people already running Authentik, Keycloak or similar.

Both write to the same `users` table and the same `owner_id` space. An account
with a password and an identity without one are the same kind of row — that is
what keeps them one directory rather than two, and it is why the first account
of *either* kind is the instance's admin.

**How people get accounts.** Either you run `user add`, or they complete their
first login at your provider. In both cases a `users` row is written and that
person's library starts empty.

**What each person gets.** Their own library, isolated in the database rather
than in the interface: listing, reading, editing, deleting, artifact state,
widgets, tags, collections and shares are all scoped to the owner behind the
session. Another user's artifact id answers exactly as an id that never existed
does — `404`, never `403` — so the API cannot be used to find out what other
people have.

**What multi-user does not include.** None of these are hidden behind a
setting, so read them before offering accounts to anyone:

- **No sharing between accounts.** A share link (§7 of `architecture.md`) is the
  only path from one library to another, and it is anonymous by design: anyone
  holding the link can open it, account or not. There is no "share with this
  user".
- **No administration screen.** `user list`, `user add` and `user passwd` at the
  CLI are the whole of it. There is no way to sign someone out, disable an
  account, set per-user quotas, or see what another user holds. The `is_admin`
  flag is recorded on the first account but grants nothing at runtime yet.
- **No account deletion, and this one has a sharp edge.** Removing someone at
  your provider — or changing their password here — stops them signing in, but
  their artifacts remain in the database under their owner id and there is no
  interface to reassign or delete them. Recovering a departed user's library
  currently means SQL. Plan for that before you depend on it.

**Upgrading an existing single-user instance.** User ids start at 1, which is
the id a single-user library is already filed under, so the first account in
adopts everything already there: **create your own account first, or complete
the first login yourself**, before anyone else. Everyone after starts empty.
(Same caution as §3.3's callout, same reason.)

**Trying it before you commit to a provider.** Any spec-compliant one works, so
you don't need a hosted service to see the flow end to end — Dex or Authentik in
a container is enough. The entire contract is a discovery document, an authorize
endpoint, a token endpoint and a JWKS URL; if a provider satisfies
`/.well-known/openid-configuration`, Exhibit will talk to it.

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

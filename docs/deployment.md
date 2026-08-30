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
| `DATA_DIR` | `./data` | Where the SQLite DB lives, and the artifact bodies too unless `BLOB_S3_BUCKET` is set |
| `ADDR` | `:8080` | App listen address |
| `RENDER_ADDR` | `:8081` | Render listen address |
| `SINGLE_LISTENER` | `false` | Serve both origins from `ADDR` alone, choosing between them by the request's `Host` header. For platforms whose proxy routes by port and cannot map two hostnames to two ports (Fly.io); see [§5.1](#51-one-port-two-hostnames). Accepts `true`/`1`/`yes`/`on`. `RENDER_ADDR` is unused when it is on |
| `LOG_LEVEL` / `DEBUG` | `info` | `debug`/`info`/`warn`/`error`; `DEBUG=1` forces debug |
| `PI_BIN` | `pi` | AI agent executable — unset/missing just disables that feature |
| `EXHIBIT_SECRET` | auto | Encrypts stored agent API keys; auto-generated if unset |
| `AGENT_API_KEY` | *(unset)* | The instance's **own** provider key for the AI agent. Unset = bring-your-own-key, the default and what every existing instance does. Set = platform mode; read [§4.1](#41-letting-the-instance-supply-the-agent-key-platform-mode) before you set it |
| `AGENT_PROVIDER` | *(unset)* | Which provider `AGENT_API_KEY` belongs to — `anthropic`, `openai`, `google`, `openrouter`, `opencode-go`. **Required** when the key is set; missing or unrecognized is a startup failure, not a surprise at the first session |
| `AGENT_MODEL` | *(unset)* | Model for platform-mode sessions. Optional — empty leaves it to the provider's default. Your choice, and never shown to users |
| `LOGIN_USERNAME` | *(unset)* | Names an account for the bootstrap / break-glass login — how you get in on an empty instance, or after losing a password. Accounts themselves are created with the `user add` subcommand, not here; see [§3.2](#32-log-in-with-a-username-and-password) |
| `LOGIN_PASSWORD_HASH` | *(unset)* | The **bcrypt hash** of that password, not the password. Produce it with the `hash-password` subcommand. Set with `LOGIN_USERNAME`; it stays accepted for that account for as long as both are set (§3.2) |
| `OIDC_ISSUER` | *(unset)* | Identity provider to delegate login to. Unset = no OIDC |
| `OIDC_CLIENT_ID` | *(unset)* | Client id registered at that provider — required when `OIDC_ISSUER` is set |
| `OIDC_CLIENT_SECRET` | *(unset)* | Client secret, if your provider issues one |
| `PUBLIC_MODE_ENABLED` | `false` | Publishes this instance's library for reading without a credential. Accepts `true`/`1`/`yes`/`on`; anything unrecognized is read as off |
| `PUBLIC_INSTANCE_NAME` | *(unset)* | What this instance calls itself |
| `PUBLIC_INSTANCE_DESCRIPTION` | *(unset)* | One line about it |
| `PUBLIC_OWNER_ID` | `1` | Whose library the instance publishes — every artifact query filters on an owner, so a public instance has to name one. `1` is the owner a single-user library is already filed under |
| `BLOB_S3_BUCKET` | *(unset)* | Store artifact bodies in this S3-compatible bucket instead of on disk. Unset = filesystem under `DATA_DIR`, exactly as before (§7) |
| `BLOB_S3_ENDPOINT` | AWS S3 | The bucket's API host, with an optional scheme: `http://localhost:9000`, `https://minio.example.com`, or a bare host. **Without a scheme, TLS is assumed** |
| `BLOB_S3_REGION` | *(unset)* | Region, when your provider needs one. MinIO does not |
| `BLOB_S3_ACCESS_KEY_ID` | *(unset)* | Access key. Leave both keys unset to use the ambient AWS credential chain (env vars, `~/.aws/credentials`, instance role) |
| `BLOB_S3_SECRET_ACCESS_KEY` | *(unset)* | Secret key |
| `BLOB_S3_PREFIX` | *(unset)* | Key prefix, if the bucket holds something else too |
| `ENTITLEMENTS_ENABLED` | `false` | Whether per-owner limits are in use on this instance. Unset = off, and every account is unlimited exactly as before (§9). Accepts `true`/`1`/`yes`/`on` and their opposites; anything else is a **startup failure** rather than a guess, because guessing "off" here means no limits at all |
| `ENTITLEMENTS_DEFAULT_STORAGE_BYTES` | *(unset)* | The storage ceiling for an account with no entitlement of its own, in bytes. **Required** when limits are on — the server refuses to start without it rather than boot with every account unlimited |
| `ENTITLEMENTS_DEFAULT_PLAN` | *(unset)* | A label for accounts on that default. Display only; no limit is ever read out of a plan name |

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

**You already have an account.** An instance with no other way in creates one on
startup, so there is nothing to run before you can sign in:

```text
username: admin
password: changeme
```

> [!IMPORTANT]
> Change it immediately. Until you do, anyone who can reach this instance can
> sign in as its admin — and an instance on a public hostname is reachable from
> the moment it boots. The server repeats this warning in its log on **every**
> startup for as long as the default is still in place.

> [!WARNING]
> **This also happens when you upgrade an instance that never had a login.**
> "No other way in" means no `OIDC_ISSUER`, no `LOGIN_USERNAME` pair, and no
> account in the database — which is exactly the state of every single-user
> library from before this feature existed. So the first boot on the new image
> seeds `admin` / `changeme` and starts sending your gallery pages to
> `/auth/login`, where they were previously open. Nothing is lost: the seeded
> account is user 1, the id your library is already filed under, so it *is*
> your library and your artifacts, state and shares are all still there.
> Two things to know before you upgrade:
>
> - **Change the password in the same sitting.** Your instance is reachable now,
>   and between that boot and your `user passwd` the default is the admin
>   password.
> - **If you authenticate at your proxy (§3.1), you now have a second login
>   behind it** — one you did not ask for and whose password is public.
>   Set `LOGIN_USERNAME`/`LOGIN_PASSWORD_HASH` to a credential of your own
>   before upgrading and no account is seeded at all, or change the seeded
>   password afterwards like anyone else. Either is fine; leaving it is not.

```bash
docker compose exec app /server user passwd admin
# Enter a password for admin, then press Enter and ctrl-D:
```

**Add more people.** From the app, at **Accounts** in the gallery header — the
screen an admin creates accounts, resets passwords and switches accounts off
from (§3.5). Or at the CLI, one subcommand per person:

```bash
docker compose exec app /server user add partner@example.com
docker compose exec app /server user list
```

The `user` subcommands read the password from **stdin rather than an argument**,
so it never lands in your shell history or a process list. Run them
interactively and type at the prompt — piping a password in (`echo … |`) works,
but puts it straight back into your history, which is the thing this avoids.

`exec` runs inside the container that is already up, which is why `/server` is
spelled out: it replaces the command rather than appending to the image's
entrypoint. If the instance is **not** running, use `run` instead — it starts a
throwaway container and does append, so the binary is implied:

```bash
docker compose run --rm app user list
```

The login name is the account's key. It is folded to lowercase and trimmed, so
`Curator@Example.com` and `curator@example.com` are one account, not two.
Anything works as a name; an email address is the convention because it is what
the surrounding UI labels it as.

The seeded admin appears only when the instance has **no other way in**. If
`OIDC_ISSUER` or the `LOGIN_USERNAME` pair is set you have already chosen how you
sign in, and no default account is created — adding a guessable second door to a
configured instance would be a backdoor, not a convenience.

That is all. `/auth/login` now serves a login page, every gallery page redirects
there until you sign in, and `/auth/logout` revokes the session server-side.

**The first account on an instance is its admin.** That is the same rule as
§3.4's callout below, seen from the other side: user ids start at 1, which is
the id everything in an existing single-user library is already filed under, so
the first account adopts that library. Create yours first. The admin flag is
what the **Accounts** screen checks (§3.5), so the first account is the one that
can create the rest.

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
  accounts and reset passwords from the **Accounts** screen (§3.5) or the `user`
  subcommands; there is no self-registration, so there is nothing to verify and
  no mail to send. This is the household and small-team path — nothing else to
  run.
- **§3.3, identities from your provider.** Your provider is the user directory.
  Granting someone access to the Exhibit application there is the whole of
  provisioning; Exhibit only records that it has met them. This is the path for
  people already running Authentik, Keycloak or similar.

Both write to the same `users` table and the same `owner_id` space. An account
with a password and an identity without one are the same kind of row — that is
what keeps them one directory rather than two, and it is why the first account
of *either* kind is the instance's admin.

**How people get accounts.** Either an admin creates one (§3.5), or they
complete their first login at your provider. In both cases a `users` row is
written and that person's library starts empty.

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
- **No per-user quotas, and no window into anyone else's library.** An admin
  administers *accounts* (§3.5) — who exists, who may sign in, who is an admin.
  There is nothing that shows what another person holds, and nothing that limits
  how much of it they hold.
- **Account deletion is the person's own, and nobody else's.** Anyone signed in
  can erase their account and everything in it from `/profile` — rows and
  artifact files both. What there is *no* interface for is doing it to somebody
  else: an admin can disable an account (§3.5), which stops the person signing
  in and signs them out at once, but their artifacts remain in the database
  under their owner id with no way to reassign or delete them from the product.
  Erasing a departed user's library still means SQL. Plan for that before you
  depend on it.
- **Deleting here does not delete the identity.** Exhibit erases what it holds;
  it has no authority over the identity provider that issued the login. The same
  person signing in again is a *new* account with an empty library, because
  `users.external_id` is unique and the row is created at first login. The
  confirmation on `/profile` says so, and anyone offering accounts to other
  people should know it before they are asked.
- **No self-service password change.** Nobody can change their own password; an
  admin resets it for them. That is the trade that keeps mail out of the product
  entirely — no SMTP to configure, no reset links, nothing to verify.

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

### 3.5 Managing accounts

Once Exhibit issues credentials, Exhibit *is* the user directory — so somebody
has to create accounts and reset the passwords people forget. That somebody is
an admin, at **Accounts** in the gallery header (`/admin/users`). The link
appears only for an admin, and the page refuses everyone else with the same
`404` an address that does not exist gets.

| Action | What it does |
|---|---|
| **Create account** | A login name and a password of at least 8 characters. Optionally an admin. Tell them the password out of band — nothing is emailed. |
| **Reset password** | Sets a new one immediately. Only for accounts Exhibit issued; an identity from your provider has no password here to reset. |
| **Make admin / Demote** | An admin may manage accounts. Everything else about the two is identical. |
| **Disable / Enable** | Disable signs the account out **everywhere, at once**, and refuses any further sign-in. Nothing is deleted; enabling restores access with the same password. |

**Password reset is by an admin, not by email.** That is the specific choice
that keeps SMTP out of the product — no mail server to run, no reset links to
expire, no addresses to verify. On a household or small-team instance the admin
is reachable by other means anyway. It is what Immich does, for the same reason.

**Disabling really is a sign-out.** Sessions are rows in the database rather
than signed tokens, so disabling an account deletes them: the person's next
request — on any device already signed in — lands back at the login page. That
holds for an identity from your provider too, and it holds against the
`LOGIN_USERNAME` break-glass pair, which is the one place a database value is
otherwise allowed to be ignored.

**You cannot lock the instance out of itself.** Demoting or disabling the last
admin who can still sign in is refused. A disabled admin does not count as the
one remaining, so you cannot get there in two steps either.

The same operations exist at the CLI, which is what you have when nobody can
sign in at all:

```bash
docker compose exec app /server user list
docker compose exec app /server user add partner@example.com
docker compose exec app /server user passwd partner@example.com
docker compose exec app /server user disable partner@example.com
docker compose exec app /server user enable partner@example.com
```

`user disable` revokes that account's sessions exactly as the screen does, and
refuses the last admin for the same reason.

**What is not here.** Managing your own account is `/profile`, not this CLI and
not the admin screen: deleting your account and the library it owns lives there
(and only there — nothing lets an admin delete somebody else's). Self-service
password change and "sign out my other devices" do not exist yet on either
surface.

`/profile`'s deletion refuses the instance's last enabled admin, with the reason
on the disabled control rather than as a surprise after the confirmation. If
that account is the one you want gone, promote somebody else first.

## 4. AI agent (optional)

Nothing to configure for the default: if `pi` is on `PATH` the agent surface
works, and each user brings their own provider key, entered in the UI and
encrypted at rest under `EXHIBIT_SECRET`. That is the right shape for a
self-hosted library, where the operator is the user and the key is theirs.

### 4.1 Letting the instance supply the agent key (platform mode)

Set `AGENT_API_KEY` and `AGENT_PROVIDER` and the instance runs *every* agent
session on that one credential:

```bash
AGENT_API_KEY=sk-ant-...        # your provider key
AGENT_PROVIDER=anthropic        # required; unknown values fail at startup
AGENT_MODEL=claude-sonnet-4-5   # optional
```

This is for a deployment where the people using it are not the people paying
for it. Nobody is asked for a key, because there is nowhere to put one: the
key screen is gone, the per-owner key API answers `404`, and neither the
provider nor the model is reported anywhere — not in a response, not in the
page, not in the agent's event stream. If you want to *choose* your model, do
not use this mode; bring your own key, which gives you that choice in full.

Existing per-user keys are left alone: they are not read while platform mode is
on, not deleted, and unsetting the variable restores each user's own key exactly
as it was.

> [!WARNING]
> **There is no spend cap.** Every agent session bills your provider account,
> and Exhibit currently measures nothing: it cannot attribute a session's cost
> to a user, and it cannot stop one that runs away. Usage billing meters after
> the fact, so a bad day is money already spent.
>
> Only enable this on an instance whose users you control. Do not put it in
> front of open signups, or behind a public-mode gallery, until per-owner
> metering and budgets exist.

The startup log repeats this warning so the instance says it out loud every
time it boots.

### 4.2 No AI agent features

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

### 5.1 One port, two hostnames

Some platforms route by **port**, not by `Host` header — Fly.io is the common
case. Every hostname they terminate arrives on one internal port, so the two
listeners above cannot each be given a hostname of their own.

Set `SINGLE_LISTENER=1` and Exhibit serves both surfaces from `ADDR`, picking
between them by the `Host` header: requests for `RENDER_ORIGIN`'s hostname get
the render surface, everything else gets the app. `RENDER_ADDR` is unused.

```bash
ADDR=:8080
SINGLE_LISTENER=1
APP_ORIGIN=https://exhibit.example.com
RENDER_ORIGIN=https://artifacts.example.com   # must be a different hostname
```

Both hostnames must resolve to the instance and both need a certificate. What
does not change is the requirement itself: the two origins stay two origins,
because that split is the artifact sandbox boundary and not a routing detail.
Exhibit **refuses to start** if `SINGLE_LISTENER` is set and the two origins
share a hostname — with one listener there is nothing left to tell them apart,
and serving artifacts from the app's own origin would put the API inside the
sandbox.

Anything arriving under an unrecognized hostname — the platform's own
`*.fly.dev` name, a health check by IP — gets the app surface, which is
authenticated. Only the exact `RENDER_ORIGIN` hostname reaches the renderer.

### 5.2 Fly.io

`fly.toml` at the repo root deploys a single machine with a volume at `/data`.
It sets `SINGLE_LISTENER=1`, so one internal port serves both origins and the
server picks between them by `Host` (§5.1). Fly's proxy routes by port and
cannot map two hostnames to two ports, which is the whole reason that flag
exists.

The committed file carries nothing specific to any one deployment: no app name,
no region, no hostnames. Pass the app name to every command with `-a`, and set
everything else with `fly secrets set`. That way you deploy from a clean clone
without editing a tracked file, and `git status` stays empty.

Pick your two hostnames first. They have to differ, and the server exits at
startup if they are missing or the same.

```bash
APP=exhibit                            # your app name
fly apps create "$APP"
fly volumes create exhibit_data --size 1 --region sea -a "$APP"

fly certs add exhibit.example.com   -a "$APP"    # the app
fly certs add artifacts.example.com -a "$APP"    # the renderer
# add the DNS records fly prints for each, then wait for both to go green:
fly certs list -a "$APP"

fly secrets set -a "$APP" \
  APP_ORIGIN="https://exhibit.example.com" \
  RENDER_ORIGIN="https://artifacts.example.com" \
  AUTH_TOKEN="$(openssl rand -hex 32)" \
  EXHIBIT_SECRET="$(openssl rand -hex 32)"

fly deploy -a "$APP"
fly scale count 1 -a "$APP"            # confirm; never raise this
```

The first boot creates an `admin` account with a default password and logs a
warning about it. Change it before you put anything in the library:

```bash
fly ssh console -a "$APP" -C "/server user passwd admin"
```

#### Why the origins are secrets

They are not secret. `fly secrets set` is simply the only per-deployment
mechanism Fly persists. `fly deploy -e APP_ORIGIN=...` applies to that one
deploy, so the next `fly deploy` without the same flags drops it and the
instance comes back up on its localhost defaults, serving links that point
nowhere.

To avoid that failure being quiet, `SINGLE_LISTENER` requires both origins to
be set explicitly. Miss one and the machine refuses to boot, and the log says
which:

```
level=ERROR msg="single listener" err="RENDER_ORIGIN: is not set"
```

Read them back any time with `fly secrets list` for the names, or
`fly ssh console -C env` for the values.

#### Set EXHIBIT_SECRET yourself

Leave it unset and Exhibit generates one into `/data/secret.key` on first boot.
That survives deploys and restarts, so most of the time you will not notice.
It does not survive the volume. Destroy and recreate the volume and the new
secret cannot decrypt anything the old one sealed, which means every stored
agent provider key is gone with no way to recover it.

Setting it as a Fly secret moves it off the volume, so a rebuilt volume is an
empty library rather than a broken one. Rotate it and you invalidate stored
agent keys, so treat it as permanent once the instance holds anything.

#### One machine, and only one

SQLite in WAL mode is a single writer against a local volume, and a Fly volume
attaches to one machine. A second machine would get its own volume and its own
database, so you would have two libraries that never converge, and each would
serve half your requests. Keep `fly scale count 1` and do not add a second
volume.

`fly.toml` stops the machine when it goes idle (`auto_stop_machines = "stop"`,
`min_machines_running = 0`) and lets the proxy start it again on the next
request. Both settings are the switch: `min_machines_running` is a floor on
running machines in the primary region, so leaving it at 1 on a single-machine
app makes that machine the floor and nothing ever stops.

What you trade away is the first request after an idle period, which waits for
a boot. On a library that publishes shares that request is often somebody
else's, arriving on a link with no account behind it. Set
`auto_stop_machines = "off"` if you would rather pay for the idle time than
hand anyone a cold start.

An idle stop is a real shutdown, so it takes any in-flight agent turn with it:
sessions live in memory, and a transcript is only written when a turn settles.
av-rcwx covers draining on signal and persisting mid-turn.

#### Backups

Fly volume snapshots are the zero-effort baseline and cover the machine dying.
They do not cover deleting the volume, and they are not point-in-time. §6 covers
Litestream, which streams the database to a bucket. Note it backs up the
database only. Artifact bodies live on the same volume unless you set
`BLOB_S3_BUCKET` (§7), so a restore from Litestream alone gives you every row
and none of the files.

## 6. Backups (optional)

`docker-compose.yml` includes a `replication` profile that runs Litestream
(streams the SQLite WAL to S3). You'll need your own `litestream.yml` (not
included) and `LITESTREAM_ACCESS_KEY_ID`/`LITESTREAM_SECRET_ACCESS_KEY`/
`LITESTREAM_BUCKET_URL`. Skip this to start — plain SQLite on a mounted volume
is fine until you need it.

> [!IMPORTANT]
> **Litestream backs up the database, and nothing else.** It streams the SQLite
> WAL, which holds your titles, tags, collections, shares, state and the *ids*
> of your artifact bodies — not the bodies. Restoring from Litestream alone
> gives you a library whose every row survives and whose files do not, which is
> not a recovered library. Back up your blobs too: the `blobs/` directory under
> `DATA_DIR`, or (if you moved them to a bucket, §7) that bucket.

## 7. Where artifact bodies live

By default, on disk: `DATA_DIR/blobs`, one file per artifact body and one more
per gallery widget. Nothing to configure, and this is the right answer for a
single machine.

Set `BLOB_S3_BUCKET` and they go to an S3-compatible bucket instead. That is
worth doing when the instance is not one machine — a container with no durable
volume, more than one replica, or a deploy that would otherwise be a volume
migration — and when you would rather back up one bucket than a bucket and a
directory (see the note in §6). Unset, none of this exists: no bucket, no
credential, no behaviour different from before the option was added.

```bash
BLOB_S3_BUCKET=exhibit
BLOB_S3_ENDPOINT=http://minio:9000     # omit for AWS S3; no scheme means https
BLOB_S3_ACCESS_KEY_ID=...
BLOB_S3_SECRET_ACCESS_KEY=...
```

The Compose file's `replication-local` profile runs MinIO, which is the easiest
way to try this locally. Start MinIO and create the bucket *before* the app —
the app verifies the bucket at startup and exits if it cannot reach it, so
bringing both up together races MinIO's boot:

```bash
docker compose --profile replication-local up -d minio
# create the bucket, in MinIO's console at http://localhost:9001 or with mc
docker compose up -d app
```

Any S3-compatible provider works; nothing AWS-specific is used.

Notes:

- **The bucket must already exist.** The service checks it at startup and
  refuses to start if it cannot reach it, rather than discovering the problem
  when someone's first upload fails. The check is a `HEAD` on the bucket, so the
  credential needs to be allowed that as well as get/put/delete on objects.
- **Leave both key variables unset** to use the ambient AWS credential chain
  (`AWS_*` environment variables, `~/.aws/credentials`, an instance role) —
  which is how a deployment with a role attached should be configured.
- **Set the bucket, or set none of these.** Any other `BLOB_S3_*` variable
  without `BLOB_S3_BUCKET` is refused at startup rather than quietly read as
  "filesystem, then" — otherwise a typo in the bucket name would put your
  artifact bodies on local disk while you believed they were in the bucket.
- **`BLOB_S3_ENDPOINT` is a host, not a URL with a path.** A scheme is allowed
  and selects TLS; a path is refused, because the SDK addresses buckets from the
  host and would silently ignore it.
- **`BLOB_S3_PREFIX`** namespaces the keys if the bucket holds anything else,
  such as your Litestream backups.
- **There is no migration between the two.** Switching an existing instance to a
  bucket means copying `DATA_DIR/blobs` into it first (`mc mirror`, `aws s3
  sync`, or `rclone`); the filenames are the keys, so a straight copy is all it
  takes. Artifacts whose bodies did not come along will render as "artifact body
  not found".

---

More detail: [architecture.md](./architecture.md) (why two origins),
[security.md](./security.md) (CSP/sandbox policy), [agent.md](./agent.md) (the
AI agent sidecar).

## 8. What is using the disk

Exhibit records how many bytes each stored blob is when it writes it, so
"what is actually using my disk" has an answer that does not involve
`du` and does not involve guessing whose library a file belongs to:

```
docker compose exec app /server storage usage
```

```
owner 1        41.3 MiB  128 blobs
owner 2         2.1 MiB  9 blobs
on disk        43.0 MiB  135 blobs stored
```

Each person also sees their own figure on `/profile`. Nothing on the instance
refuses anything because of it — there is no quota, and uploads do not stop.

The last line is not the sum of the ones above it, and the gap is deliberate.
A file two people's artifacts share is counted in full against each of them —
that is what each would have to store on their own — while the disk holds it
once. Per-owner figures answer "what is this person holding"; `on disk`
answers "what is on this volume".

**Upgrading an existing instance needs no action.** The first start after the
upgrade measures the files already stored and records their lengths, logging
`backfilled blob sizes` when it does. It reads each file once, so a large
library makes that start slower; every later start skips it entirely.

If the numbers look wrong — a crash between writing a file and recording its
length, a restore from a backup, a file replaced by hand — re-measure them:

```
docker compose exec app /server storage recompute
```

That reads every stored blob, so it is a command you run deliberately rather
than something the server does on a timer. It is safe to run on a live
instance, safe to run twice, and only ever replaces a recorded length with the
length the bytes actually have. A blob it cannot read keeps the length already
recorded for it and is reported on the line, so a backend hiccup cannot
silently shrink somebody's total.

## 9. What each account is allowed

Every account carries an **entitlement**: a plan label, a storage limit, and an
opaque reference for whatever outside Exhibit keeps track of it. All three are
set by an admin on `/admin/users`, next to the password reset and the disable
switch, and by nothing else — an entitlement a person can raise on themselves
is not a limit, so `/profile` has no control for any of it.

On a self-hosted instance this is the whole feature and it needs no external
anything: a household or small-team instance can give one person a larger
allowance than another, which is otherwise unaskable.

**Limits are off unless you switch them on.** With `ENTITLEMENTS_ENABLED`
unset — the default, and what every existing instance has — every account
resolves to unlimited and nothing is ever refused. An upgrade must never put a
ceiling in front of somebody who did not ask for one.

Switching them on requires a default:

```
ENTITLEMENTS_ENABLED=true
ENTITLEMENTS_DEFAULT_STORAGE_BYTES=5368709120     # 5 GiB
ENTITLEMENTS_DEFAULT_PLAN=included                # optional label
```

**Turning the switch on without a default is a startup failure**, not a
warning. The alternative is an instance that boots looking configured, on which
every account that nobody has provisioned is unlimited — the same posture
`LOGIN_USERNAME` without `LOGIN_PASSWORD_HASH` takes, and for the same reason:
a half-configured gate should fail where you are watching rather than quietly
not be a gate.

**Nothing refuses anything yet.** There is no quota: the entitlement is
recorded, resolvable and visible, and no write stops because of it. What
switching limits on changes today is that the instance requires a default, and
that the admin page states which of the two states it is in. The storage figure
those limits will be compared against is the one in §8.

### Setting them by hand

An admin uses the **Entitlement** button on each row of `/admin/users`. The
storage field is bytes exactly, and leaving it empty is a real answer: it
removes this account's own ceiling and puts them back on the instance default.
It does not make them unlimited — that is an instance-wide state, not something
one account can be given.

### Setting them from something else

Whatever decides *why* an account is on a given plan lives outside Exhibit
entirely. There is no integration to configure, no callback URL, and nothing in
this repo that knows about any such system. What there is instead is the
ordinary API:

```
curl -X PATCH https://exhibit.example.com/api/admin/users/7 \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"plan":"household","storage_limit_bytes":10737418240,"entitlement_ref":"acct-7"}'
```

That is the same route and the same credential the admin page uses — the single
write path, with no second door cut for a machine. `GET /api/admin/users`
returns these three fields in the same places, so reading an account, editing
the object and sending it back does what it looks like it does. Three things
about the shape are worth knowing:

- **Every field is optional**, and one you leave out is left alone. A caller
  that sends only `plan` does not reset a ceiling you raised by hand.
- **`"storage_limit_bytes": null` clears** this account's own ceiling, putting
  it back on the instance default. That is how a downgrade is expressed, and it
  is deliberately different from leaving the field out.
- **`entitlement_ref` is opaque.** Exhibit stores it and never reads it — no
  parsing, no joins, no behaviour depends on its value. It is here rather than
  only in your own system because it is durable with the account: the account
  outlives that system being rebuilt, replaced, or dropped.

### Seeing the ones that differ

Anything maintaining these from outside can fall out of step with itself — a
downgrade it failed to deliver leaves somebody on a raised ceiling
indefinitely. Keeping them current is that system's job; seeing them is not, so
the accounts carrying an entitlement of their own are listed on `/admin/users`,
and over the API:

```
curl -H "Authorization: Bearer $AUTH_TOKEN" \
  "https://exhibit.example.com/api/admin/users?entitlement=custom"
```

Everyone absent from that list is on the instance default.

**Deleting an account removes its entitlement with it.** These are columns on
the account's own row, so there is nothing left behind and nothing to clean up
— and someone who signs in again afterwards arrives as a new account on the
default, not on their old allowance.

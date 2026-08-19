---
id: av-lvxq
status: closed
deps: []
links: [av-xath]
created: 2026-08-19T05:23:08Z
type: chore
priority: 2
assignee: Max Omdal
tags: [deployment, infra, docs]
---
# Add a fly.toml so Exhibit deploys to Fly.io

Exhibit ships a Docker image and a documented docker-compose example; there is no config for a PaaS, so putting an instance on Fly.io today means reverse-engineering the compose file. Add a committed `fly.toml` (plus the deployment.md section that makes it usable) so a clone plus the documented `fly` commands produces a working instance: one machine, one volume at /data, credentials via `fly secrets`.

## Design

1. TWO ORIGINS VS ONE FLY PROXY — the hard part. The security model requires APP_ORIGIN and RENDER_ORIGIN to be different hostnames (architecture.md §4, §3.2). The server binds two listeners (ADDR :8080 for app/API, RENDER_ADDR :8081 for the render surface — cmd/server/main.go:179-184) and expects an operator proxy to map a hostname to each. Fly's proxy routes by port, not by Host header, so a single [http_service] on 443 cannot split two hostnames on its own. Pick and state the resolution before writing the file. Options as understood today (verify against current Fly docs — do not take this summary as authoritative):
  (a) one service on 443 to one internal port, with a Host-header demux in front of the two handlers inside the server. Only option that keeps both origins on default HTTPS with no port in share links; costs a code change (ticket it separately if it is not trivial).
  (b) two services on two external ports — 443 for the app, e.g. 8443 for render — with RENDER_ORIGIN carrying the port. Still a distinct origin, so the sandbox boundary holds, but every share link carries a nonstandard port and egress-restricted networks will block it.
  (c) two Fly apps. Rejected: a volume attaches to one app, and both surfaces read the same SQLite file and blob directory.
  Recommendation: (a). Whichever is chosen, the file's comments and the docs must say so.

2. ONE MACHINE, ALWAYS. SQLite in WAL mode is a single writer against a local volume, and a Fly volume attaches to one machine. The config must pin one instance — no scaling past it — and decide auto_stop/auto_start deliberately: stopping is cheap and safe here, but adds cold-start latency to a share link somebody else opens.

3. VOLUME AND SECRET KEY. [mounts] at /data with DATA_DIR=/data. Note that EXHIBIT_SECRET, when unset, is generated once into $DATA_DIR/secret.key (internal/secrets), so it survives on the volume — but destroying and recreating the volume takes every stored agent provider key with it. Setting it as a Fly secret is what makes that survivable, and the docs should say why rather than listing it as one more variable.

4. SECRETS VS [env]. The committed file carries only non-secret values (ADDR, RENDER_ADDR, DATA_DIR, LOG_LEVEL). AUTH_TOKEN, EXHIBIT_SECRET, LOGIN_PASSWORD_HASH and OIDC_CLIENT_SECRET go through `fly secrets set` and must never appear in the file. APP_ORIGIN/RENDER_ORIGIN are per-deployment: commented placeholders, set for real after `fly certs add`.

5. VM SIZE. The runtime image is node:22-bookworm-slim with the pi agent harness installed globally (Dockerfile), not distroless. Size [[vm]] deliberately instead of inheriting the smallest default, and say what the surface costs.

6. HEALTH CHECK. There is no health endpoint today; a TCP check on the app port is the honest option. Do not invent /healthz here — that is its own ticket if it is wanted.

7. DOCS. deployment.md §5 (Reverse proxy / TLS) gains a Fly.io subsection or peer section: fly launch --no-deploy, volume create, certs add for both hostnames, secrets set, deploy. Per the project's docs rule, describe only what the committed file actually does.

## Acceptance Criteria

1. `fly.toml` is committed at the repo root and deploys from a clean clone using only the documented commands.
2. The chosen origin routing (design §1) is implemented, and stated in both the file's comments and deployment.md. On the deployed instance APP_ORIGIN and RENDER_ORIGIN are different origins, and an artifact renders in its iframe with its per-artifact CSP header present.
3. Exactly one machine runs, and the config cannot silently scale past one.
4. Data survives `fly deploy` and a machine restart: artifacts, artifact state, and stored agent provider keys are all still there afterwards.
5. No secret value appears in the committed file; deployment.md names which values are `fly secrets set` and why EXHIBIT_SECRET is one of them.
6. deployment.md documents the path end to end, including `fly certs add` for both hostnames.
7. Verified once on a real Fly app: ingest an artifact, render it, restart the machine, confirm it is still there.


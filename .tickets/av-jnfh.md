---
id: av-jnfh
status: open
deps: []
links: [av-kmwj]
created: 2026-08-31T05:17:16Z
type: feature
priority: 2
assignee: Max Omdal
tags: [security, render, ui]
---
# Redirected requests: an approved origin that forwards elsewhere has no path to approval

A request to an allowlisted origin that redirects to a different origin is
blocked at the second hop, and the user has no way to approve the destination.

CSP re-checks every redirect hop, so `img-src https://picsum.photos` permits
hop 1 and refuses hop 2 to `fastly.picsum.photos`. The violation report names
the URL the artifact *asked for*, never the one it was sent to, because a
policy must not become a way to probe where a cross-origin redirect leads.

av-kmwj made the runtime prompt stop asking about this case, because asking was
worse than useless: the origin is already allowed, so Allow was a no-op, and
Allow reloads the frame, which re-fired the identical violation and re-opened
the prompt. It now raises the capability banner (`redirected-origin`) with an
explanation instead. That stops the loop but leaves the user with no way to
make the artifact work short of already knowing the destination host or
opening the devtools Network panel.

Reproduces with any picsum.photos artifact: the images never load.

## Design

## The frame cannot resolve it. This was measured, not assumed.

Every avenue from inside the render document, under the artifact's own CSP
(Chrome, av-kmwj investigation):

| attempt                                | result                                        |
|----------------------------------------|-----------------------------------------------|
| fetch no-cors + redirect:'manual'      | TypeError, no response object                  |
| fetch cors + redirect:'manual'         | TypeError                                      |
| fetch cors + redirect:'follow'         | TypeError                                      |
| fetch no-cors + redirect:'follow'      | type:"opaque", url:"", redirected:FALSE, status:0 |
| XMLHttpRequest                          | error, responseURL:"", status:0                |
| PerformanceResourceTiming              | name = the INITIAL url; redirectStart/End = 0  |
| securitypolicyviolation                | blockedURI = the pre-redirect URL              |

The fourth row is the one that settles it: the request *was* redirected and the
response reports `redirected: false`. That is the opaque response filter, i.e.
the same-origin policy, not our sandbox and not our CSP. A top-level page on
any site gets the same nothing. Removing the sandbox changes no cell in that
table. So "resolve it in the frame" is not an option that exists.

## Two shapes that do work, neither free

**A. Server resolves the destination.** The frame reports the full blocked URL;
the host asks the server to follow it with `internal/snapshot`'s existing
bounded Fetcher (dial-time SSRF guard that covers redirect targets and every
DNS answer, MaxRedirects 5, 10s timeout, proxying disabled), reading only the
Location chain and never a body. The prompt then names the real destination and
Allow writes it.

`Fetch` and `FetchWithCap` are not that: both read the response into an
`Asset`. Header-only resolution needs its own method over the Fetcher's own
`http.Client`, so it keeps the redirect policy, timeout, proxy-off transport
and dial-time guard while closing each response unread. Test it against a
target whose body is large enough — or slow enough — that reading it would
show up.

Cost: a new authenticated route that makes an outbound request to an
artifact-chosen URL at *view* time. It is strictly weaker than a capability the
API already grants (`POST /api/artifacts` with `url` fetches an arbitrary
user-supplied URL and stores the body), and the guard is the same one. But it
cannot prove the URL came from a real violation -- the host could send
anything -- so the honest claim is "an authenticated owner can ask the server
where a public URL redirects", and it wants a per-owner rate limit.

Rejected for now: the owner does not want view-time server fetches.

**B. Subdomain (wildcard) approval.** Measured on two pages differing only in
the CSP header, using google.com/favicon.ico which 301s to www.google.com:

    img-src https://google.com
      -> loaded:false, violations:["https://google.com/favicon.ico"]

    img-src https://google.com https://*.google.com
      -> loaded:true, naturalWidth:32, violations:[]

Both entries are needed: `*.google.com` alone does not match the apex, so a
wildcard *replacing* the origin breaks hop 1. The shape is "keep the approved
origin, add a sibling wildcard for its subdomains".

No outbound fetch, no new endpoint, no SSRF surface. The cost is the one
av-i7hd argued against on purpose: the approval UI stops showing exactly what
is granted, and one click covers hosts the user never saw. Scoping that would
have to be part of the design -- offered only in the redirect case, only for
the registrable domain of an already-approved origin, stored as its own
decision row so it stays visible and revocable, and rendered as "also allow
subdomains of picsum.photos" rather than raw `*.` syntax.

## Why ingest cannot pre-resolve this instead

The obvious cheaper fix is resolving redirects at ingest, where the approval
gate already lives. It does not reach this case: artifacts build such URLs at
runtime (`'https://picsum.photos/id/' + pc.id + '/640/480'`), so the scanner
only ever sees the literal prefix `https://picsum.photos/id/`, which does not
redirect the way a real image URL does. Only a running frame holds a concrete
URL to resolve.

## Not a bug fix

Both shapes change policy rather than correct an error, which is why this is
its own ticket. B in particular reverses a decision av-i7hd took deliberately,
and that reversal needs to be argued on its own terms rather than smuggled in
behind a prompt fix.

## Acceptance Criteria

1. A request to an approved origin that redirects to an unapproved one has a
   path to approval that does not require the user to already know the
   destination host or to open devtools.
2. Whatever is granted is visible: the allowlist editor shows the user exactly
   what a subdomain approval covers, or exactly which destination origin was
   added, and either is revocable like any other decision.
3. The rule stays enforced at the single write path, and a wildcard grant is
   **its own validated type**, not a loosening of `origin.NormalizeOrigin`.
   NormalizeOrigin is unchanged, so ordinary allowlist input that is not an
   origin — anything carrying a `*` included — still returns a 400 naming the
   value, exactly as av-i7hd made it. The grant type has an explicit grammar
   and admits nothing outside it: scheme, `*.`, a registrable domain, and an
   optional explicit non-default port. Rejected outright: paths, queries,
   fragments, userinfo, IP literals, a bare public suffix (`*.co.uk` grants a
   whole TLD), and `*` anywhere but the leftmost label. The store validates the
   same grammar for the reason it already re-runs NormalizeOrigin — so no
   future caller can bypass the handler — and the store, the allowlist editor,
   and the CSP builder all speak that one type, so a grant stays scoped,
   visible, and revocable end to end.
4. If the server-resolve shape is chosen, the outbound fetch goes through
   internal/snapshot's guarded Fetcher, reads no response body, and is rate
   limited per owner; security.md states plainly what the route lets an
   authenticated caller do.
5. The capability banner av-kmwj added stops being the terminal state for this
   case, and its copy is updated to point at the new path.


---
id: av-qo05
status: closed
deps: []
links: [av-4wyq, av-qwld, av-utap]
created: 2026-08-16T17:36:19Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-g2dx
tags: [frontend, gallery, account, multi-user]
---
# Profile page — gallery entry point and account surface at /profile

The gallery header has no way to reach anything about *you*. It carries the logo, a search row below it, an admin link (av-utap, admins only), and a primary "Add artifact" button — nothing that names who is signed in or leads anywhere they can act on their own account.

This story adds that entry point and the page behind it.

**Base branch: `integration/multi-user`.** This depends on identity existing (`users`, sessions, the consolidated `Principal`) and on the settings furniture av-utap already built — `settingsHeader`, `settings.css`, the `.card` / `.settings-h3` / `.settings-label` vocabulary. None of that is on `main` yet, so branch from `integration/multi-user`, not `main`.

## Header change

Two controls in the top-right, both icon-only and the same size, so neither dominates:

- **Add artifact** shrinks from a labelled primary button to an icon-only square (`ph-plus`), still linking to `/new`. It stops being the visually loudest thing on the page; it does not stop being the primary creation route.
- **Profile** is new: a static person icon (`ph-user` / `ph-user-circle`) linking to `/profile`. Static means static — no dropdown, no menu, no avatar fetch. It is a link.

Losing the visible "Add artifact" label costs discoverability, so both controls need a real `aria-label` and a `title`, and the empty-library state already carries the labelled "Add your first artifact" link that teaches the affordance to a new user.

The admin "Accounts" link stays where it is and keeps its label — it is conditional and rare, and collapsing it to a third anonymous glyph would make three icons say nothing.

## The page

`GET /profile`, server-rendered like every other app page: stdlib `html/template` in `internal/api/templates/`, Phosphor icons, static CSS under `web/gallery/`, no CDN and no framework. It reuses `settingsHeader` and `settings.css` rather than inventing a second settings look — `/admin/users` and `/profile` are two pages of one surface, sharing furniture and nothing else. Authority stays on the route: `/admin/users` is `adminOnly`, `/profile` needs only a session and acts solely on the caller's own account.

Structured as **sections** (`.card` blocks, one `h3` each) from the start, even though only one has content today. The whole point of av-g2dx is that this page grows — the BYO agent key, sessions, export — and a page laid out as sections takes each of those as an addition rather than a redesign.

### Section: Account

Displays who is signed in. There is no `username` column: a local account signs in with the login name in `users.email` (admin.tmpl labels it "Login name" and renders it as `.Name`), and an IdP identity has `external_id` plus whatever `email` the provider gave. Resolve one display name the same way admin.go already does and show it — do not invent a second naming rule for the same person. Say which kind of sign-in it is (password vs identity provider), mirroring the admin table, because that determines what this page can and cannot do for them.

Note `Principal` carries `OwnerID`, `Kind`, `ReadOnly`, `Grant` — no display name. The handler loads the user row; the name does not arrive for free in page data.

### Section: Delete account

The only action on the page for now. It erases everything the server holds for this user — every artifact and its body, all state, tags, collections, shares, origin decisions, agent keys and transcripts, sessions, and the `users` row.

The behaviour, the schema walk, and the wording are **av-4wyq's**, not this ticket's; that story also carries the real blocker (av-7jcq — `Blob.Store` has no `Delete`, so today "delete" leaves every artifact body on disk). What this ticket owns is the section that hosts it: the heading, the danger-zone treatment, and the plain statement that this is permanent and that deleting here does not remove the identity from the provider.

If av-4wyq is not ready when this lands, ship the section with the control disabled and the explanation visible rather than shipping a button that half-deletes.

## Design

## Why a page and not a menu

A dropdown under the profile icon would put account actions one click closer and would be the wrong shape for what goes on them. The things av-g2dx enumerates — deletion, the agent key, active sessions, export — all need explanation, confirmation, or a table beside them. None fits a menu item, and a menu that leads to a page anyway is a menu that exists only to be dismissed.

## Two icons, one size

The mockup's point is proportion, not minimalism: the primary button currently reads as the loudest element in a header whose actual subject is the library below it. Shrinking it to match the profile icon makes the header a place you leave from rather than a place that shouts. Keep the two visually peer-level — same box, same weight — so neither reads as the "real" one.

## Invariants this page inherits

Two walks already fail on an undeclared route, so this page cannot ship unexamined:

- **`pagecredential.go` (av-5imk)** decides what credential a page embeds. `/profile` should embed nothing beyond what it needs — the deletion call is a mutation and goes through the JSON API like every other client, so the page needs the same page credential the other authenticated pages carry, declared explicitly.
- **`csrf_test.go` (av-ke2m)** requires every GET route to be declared a read. `GET /profile` is a read; the deletion is not a GET.

## Not in scope

- Deletion semantics, the owner-scoped table walk, the live-share count in the confirmation — all av-4wyq.
- Moving the BYO agent key here — its own story once this page exists.
- Administering anyone else's account — av-sz4e / av-utap, a different authority.
- Avatars, display-name editing, profile photos. "Profile" here means account, not persona.

## Relationship to av-qwld

av-qwld ("User settings page — the surface itself") is the abstract version of this ticket, written before av-utap shipped `settingsHeader` and `settings.css`. This story names the route, the entry point, and the section layout, so it supersedes it. Close av-qwld when this lands, and re-point av-4wyq's dependency from av-qwld to this ticket.

## Acceptance Criteria

- Gallery header renders "Add artifact" as an icon-only control linking to /new, sized to match the new profile control, with an accessible name that still says "Add artifact".
- A static profile icon in the gallery header links to /profile. It is an anchor — no menu, no JS.
- The admin "Accounts" link is unchanged, still conditional on IsAdmin, still labelled.
- GET /profile renders for any signed-in user, 302s (or whatever the app's existing unauthenticated behaviour is) for anyone who is not, and needs no admin role.
- The page uses the settingsHeader partial and settings.css — no second settings stylesheet, no duplicated header markup.
- The page is laid out in sections; adding a second section requires no change to the first.
- The Account section shows the signed-in user's display name, resolved by the same rule admin.go uses, and states whether they sign in with a password or an identity provider.
- The Delete account section exists, is visually marked as destructive, and states in plain text that deletion is permanent and does not remove the identity from the identity provider.
- If av-4wyq has not landed, the delete control is present but disabled with a visible reason; it never invokes a partial deletion.
- GET /profile is declared a read in csrf_test.go and its page credential is declared in pagecredential.go — both walks pass.
- Rendered on a phone-width viewport, the two header icons and the search row below them do not overlap or wrap awkwardly; any field on /profile sizes from the --field-font-size token (av-3qmf).
- A screenshot of the new header and the /profile page is captured with shot-scraper and kept out of git (scratchpad only).

<<<<<<< HEAD

## Notes

**2026-08-16T17:46:22Z**

CORRECTION to the Account section (2026-08-16, after reading integration/multi-user).

The body says 'resolve one display name the same way admin.go already does'. Having read it, that instruction is not safe to follow literally. newAdminUserView (internal/api/admin.go:163) is 'Name: u.Email' and nothing more, and users.email is NOT NULL DEFAULT '' (migration 013) — it exists as a portable second key beside external_id, not as something an identity provider guarantees.

Consequence: for an SSO identity with no email, admin.go renders one blank cell in a list of rows, which is cosmetic. /profile renders its Account section empty, because that name IS the section. This page needs a fallback admin.go never needed — fall back to external_id, or state the sign-in route ('Signed in via your identity provider') when there is no better name. Decide it here and, if it generalises, let admin.go adopt it rather than the reverse.

Also confirmed, so the implementer does not go looking: Store.GetUser(ctx, id) already exists (internal/store/store.go:294). This story needs no store work and no shared-helper extraction.
=======
>>>>>>> efa8713 (tk: av-qo05 — profile page entry point and account surface at /profile)

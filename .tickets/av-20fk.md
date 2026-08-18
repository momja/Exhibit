---
id: av-20fk
status: in_progress
deps: []
links: [av-vnkt, av-ghvs, av-c5aq, av-8gyd, av-3pq6, av-52ll, av-fw1b]
created: 2026-08-17T03:24:22Z
type: feature
priority: 1
assignee: Max Omdal
tags: [snapshot, render, storage, agent]
---
# Store large runtime assets out of line: URL-addressed blobs instead of inlined data: URIs

The snapshot vendorer's runtime pass (av-ghvs) base64s each vendored binary payload into the artifact body: up to 16 MiB fetched, ~21 MiB in the document (`internal/snapshot/fetcher.go:51`). Four costs follow, and none of them is the one the design was solving for:

- **The agent cannot edit these artifacts.** `get_artifact` hands `a.body` verbatim to the model (`internal/agent/ext/exhibit.ts:209`), so a single wasm tool is ~21 MB of base64 in context, and `update_artifact` requires sending all of it back to change one line. Expensive, and it serves no purpose — the model can do nothing with those bytes.
- **The edit page is slow**, for the same reason: CodeMirror loads the whole body.
- **Every render re-transfers it.** The render document is `Cache-Control: no-store` by necessity (inlined state + per-artifact CSP), so those megabytes cross the wire on every single view and are gzipped from scratch each time.
- **The agent preview loop pays that cost per iteration.** Every successful agent save emits `exhibit_artifact_saved`, which htmx-swaps the preview pane and reloads the iframe (`architecture.md` §3.7). Ten iterations on a wasm artifact is ~210 MB transferred and ten full compressions of a payload that never changed — plus the same ~21 MB through the model on each `get_artifact` and each `update_artifact`.
- **No deduplication.** The same ffmpeg.wasm ingested five times is five copies.

Inlining was chosen for a narrow reason: CORS (av-ghvs). A page served from its own site fetches `/app.wasm` same-origin and needs no CORS headers, so source sites do not send them; relocated to the render origin the read fails. That argument does not carry over to assets served from *our* origin, where we set the response headers.

## Design

Move each vendored runtime payload into its own blob, addressed by URL, and leave the artifact body a small, valid HTML file.

**Scope: the runtime pass only — `.wasm`, `.data`, `.bin`, `.mem`.** The vendorer has two passes and this ticket touches one of them.

| Pass | Handles | Per-asset cap | Substitutes by |
|---|---|---|---|
| `InlineHTMLAssets` + CSS | `<img>`, `srcset`, icon `<link>`, `<script src>`, `url()`, `@import` | `MaxAssetBytes`, 5 MiB | rewriting the markup |
| `InlineRuntimeAssets` | fetch literals with binary extensions | `MaxInlineAssetBytes`, 16 MiB | manifest + `fetch` interception |

Images, fonts, stylesheets, and scripts keep being inlined as `data:` URIs. The split is forced rather than chosen: an `<img src>` is not loaded through `window.fetch`, so there is no wrapper to hook, and externalizing it means writing a render-origin URL into the stored body — which destroys the property this ticket depends on, that the body keeps its original literals and an agent rewrite cannot break asset loading. Doing that rewrite at render time instead would mean parsing and re-rendering the whole document on every request.

The markup pass has the same problem and is tracked separately as [[av-oz40]]. It is not the lesser case it first appears: the per-asset cap is smaller (5 MiB) but nothing limits how *many* assets it inlines short of the 48 MiB total, so an image-heavy page can put more base64 into an agent's context than a single wasm payload does. It is split out because it needs the other substitution mechanism — markup references are rewritten at ingest rather than intercepted, since nothing loads an `<img src>` through `window.fetch` — while sharing this ticket's table, route, CSP argument, lifecycle, and export path.

**Naming, because "asset" is already taken.** The codebase uses it in the pass-1 sense (`InlineHTMLAssets`, `MaxAssetBytes`, the `Asset` struct). Define the new table as holding **any out-of-line asset**, with the runtime pass as its only producer today. Then the broad name is accurate rather than misleading, and extending to markup assets later adds a producer instead of migrating a schema.

**Ingest records assets; it no longer rewrites the body.** `InlineRuntimeAssets` (`internal/snapshot/runtime.go`) keeps its discovery half — walk `<script>` text, take fetch-call literals via `scanner.FetchRefs`, keep binary-asset extensions, fetch through the bounded `Fetcher` — and drops its injection half entirely. Each payload is stored as its own blob with a row recording (artifact, absolute source URL, assetID, size, content type). **The stored body keeps its original fetch literals, unmodified.**

That is the shape change from the first draft of this ticket: there is no ingest-time body transform at all, so the vendoring machinery becomes entirely render-time.

**Render injects the manifest, as part of the preamble.** `injectPreamble` (`internal/render/render.go:1118`) gains the manifest — built from the assets table for the artifact being served — alongside the storage shim, the capability bridges, and the `data:` fetch shim. Substitution is still *interception, not source rewriting*: a `window.fetch` wrapper matching on the resolved absolute URL, which is what survives minification and catches runtime-constructed URLs. It just resolves to an asset URL and delegates to native `fetch`, instead of decoding base64 into a `Response`.

Four things follow, and together they are why this is worth the change:

- **An agent cannot clobber it.** The manifest is not in the body, so a wholesale body rewrite — the normal operation in the agent preview loop — cannot break asset loading. Under the previous design the manifest script sat in the body as the first child of `<head>` and was exactly as rewritable as everything else around it.
- **The assets table is the single source of truth**, rather than being copied into every stored body.
- **It collapses the two independent `fetch` wrappers** that `architecture.md` §3.2 currently has to explain as separate concerns — the preamble's `data:` shim and the vendorer's manifest wrapper. Today their ordering is incidental (the preamble injects after `<head>`, the manifest was inserted before `head.FirstChild`, so the manifest wrapper happens to wrap the shim); in the preamble it is explicit. Both are ordering-sensitive in the same way: a wrapper only shadows `fetch` for callers that run after it, so the manifest must install before any artifact script.
- **Export becomes symmetric** ([[av-vnkt]]): materialize from the assets table, the same source render reads.

Widget renders (`/w/:id`) get the manifest too. The `WIDGET = true` narrowing exists to drop *authority* — the capability bridges — and an asset manifest grants none; it resolves the artifact's own bytes.

**Serving.** New route on the render surface: `GET /a/<artifactID>/assets/<assetID>`.

- `Access-Control-Allow-Origin: *`, credentials off. The frame is opaque-origin, so its `Origin` is `null` and `*` is the only value that works. This is precisely the header the source site would not send, and the reason inlining was needed.
- Real `Content-Type`, with `application/wasm` forced for `.wasm` — the constraint `runtimeDataURI` already encodes, since `WebAssembly.instantiateStreaming` rejects anything else. Serving it as a header rather than a `data:` URI is strictly better: genuine streaming compilation instead of decoding a 21 MB string in the tab.
- `Cache-Control: public, max-age=31536000, immutable`. **This is the one route on the render surface that is not `no-store`**, and that is safe because it is the one route carrying no state and no per-request policy: an immutable, content-addressed byte blob.

**Credential: a high-entropy random `assetID`, not a render token.** The render token (av-c5aq) is short-lived, so putting it in the asset URL changes that URL on every render and destroys the caching that motivates this change. A random per-asset id is stable for the artifact's lifetime and is itself the capability — the same posture a share link already has. The route additionally verifies the asset belongs to the artifact in the path.

*This is the main security review point of the ticket.* It reintroduces one untokened route on `RENDER_ORIGIN`, which av-c5aq deliberately closed. The argument that it is acceptable: the id is unguessable, the response is opaque binary content, it exposes no state, no policy, and no enumeration, and reading it requires already knowing both the artifact id and the asset id. If that argument does not hold up, the fallback is a token and the loss of cross-view caching — the change is still worth making for the agent and editor costs alone.

**CSP.** The render surface emits, per artifact:

```
connect-src … https://render.example.com/a/<artifactID>/assets/
```

as a **system source**, in the same unconditional bucket as `data:` and `blob:` (architecture.md §3.2) — never a `decision='allow'` row, never written to `artifact_network_origins`, never shown in the allowlist editor. The justification is that these are the same bytes that sit in the document today under `data:`; only the addressing changed, and the question the bucket rule actually asks — can the artifact reach content the user did not approve, or send anything to a third party — has the same answer either way: no. A fully vendored wasm app's user-visible footprint stays empty.

Two constraints fall out: **the asset route must never redirect** (CSP drops path matching across a redirect, turning a scoped grant into a silent failure), and the path must stay scoped to the artifact's own id so one artifact cannot read another's assets.

**Storage.** Assets are blobs in the existing `Blob` store, so the operator's backup story (Litestream + the blob dir) is unchanged. Content-addressed **per owner, never globally**: dedup within one library only. Global dedup makes account deletion able to strip bytes out of another owner's artifact unless refcounting is exactly right in the delete path; per-owner addressing removes that failure mode by construction, costs duplicate storage only on multi-user instances, and costs nothing on a single-user self-host. Assets are deleted with their artifact via `Blob.Delete` (av-7jcq).

**Lifecycle: what makes an asset deletable.** Moving the manifest out of the body means the body is never consulted to build it, so nothing in the body answers "is this asset still referenced".

Be precise about why a body scan cannot answer it either, because the obvious argument is wrong. Every asset *originates* from a literal — manifest entries come from `scanner.FetchRefs` alone — so there is no such thing as an asset vendored behind a URL the scan could not see. Creation is fully covered.

The gap is in **consumption**. The wrapper matches at call time on the resolved URL (`new URL(raw, document.baseURI).href`), not on the call site, so an asset vendored from `fetch("/app.wasm")` is still served to a later body calling `fetch(base + "/" + name + ".wasm")` that resolves to the same URL. Agent rewrites and ordinary refactors do exactly that. A scan showing the literal is gone therefore does not show that nothing fetches it.

That is the narrow reason. The broad one is that a scan-based GC is not needed: the three decidable cases below cover the real churn, what remains is one narrow case (a feature edited out of a kept artifact), the leak is bounded, and getting it wrong silently destroys a payload whose source site may have rotted — unrecoverable for a pasted artifact. Reclaiming occasional megabytes is not worth that trade at any level of scan accuracy. So **only decidable questions may authorize a delete**:

These are **two questions, not one**, and conflating them is the easiest way to get this wrong.

**Is the asset row deletable?** Three cases say yes; a fourth is permanently undecidable.

| Case | Deletable? |
|---|---|
| The artifact is deleted | Yes — certain |
| A newer generation supersedes this one, and no retained version references the old one | Yes — generations are recorded |
| The user deletes it in the edit page's asset panel | Yes — explicit |
| The body no longer fetches it, but none of the above applies | **Never concluded** — a vanished literal does not mean a vanished fetch |

The last row is a leak accepted on purpose. The asset panel (below) is how a human resolves it, because a human can know what the code does and this system cannot.

**Why the second row is decidable and the fourth is not.** Both look like "is this still used?", but they ask different questions, and only one is about data we wrote down.

Every asset row carries a `generation_id`, and every body version records the generation it was created with — written at creation, when it is known with certainty. So the check is a lookup: `SELECT COUNT(*) FROM artifact_versions WHERE generation_id = ?`. Before version history exists ([[av-3pq6]]) the population is one and the check degenerates to "is this the current generation".

The claim that count supports is deliberately weak: *no retained body is associated with this generation.* It is a statement about the document's **lifetime**, not its contents — when no stored body came with those assets, nothing can fetch them regardless of what any code said, because there is no such document left to run. The fourth row asks about a body we are *keeping*, so answering it means interpreting JavaScript we did not write, where a URL may be assembled from parts, from config, or from another response. One reads our own bookkeeping; the other reads user content. Only the first may authorize a delete.

**Is the blob deletable?** Exactly when **no row references it** — not when *a* row was deleted, but when the *last* one was. So enqueueing for deletion ([[av-8gyd]]) is conditional and happens inside the same transaction: drop the row, count remaining rows for that blob id, enqueue only on zero.

This is load-bearing rather than defensive. Per-owner content addressing means two artifacts in one library legitimately share a blob, so an unconditional enqueue on artifact deletion would silently strip the payload out of the surviving artifact.

**Generations.** Each ingest or refetch that produces assets mints a generation id; asset rows carry it. Render injects only the current generation's manifest. When a new generation supersedes an old one the old *set* becomes deletable as a unit — the body it belonged to has been replaced too, so nothing can still reference it. This is what keeps [[av-b17a]]'s refetch path from accumulating a full asset set per refetch, forever. An ordinary body PATCH does **not** mint a generation and does not touch assets: the body may still fetch them, and we cannot know otherwise.

**When it runs: inline, in the operation that supersedes.** There is no scheduler and no background reclaim. The transaction that mints a new generation is the one thing that knows exactly what it superseded, so it does the delete — rows inside the transaction, blobs unlinked after commit (that order, so a crash leaves unreferenced bytes rather than rows pointing at bytes that are gone). Artifact deletion, account deletion, and a user deleting an asset from the edit panel follow the same inline pattern. The only cost is I/O on a request that is already doing network fetches.

Two consequences to accept deliberately:

- **An in-flight render can 404.** A view opened seconds before a refetch may request an asset that was just deleted. It is already displaying a superseded body; a reload fixes it. A grace period would close that window, but at the price of a `superseded_at` column and a background tick — a whole mechanism for a few seconds of exposure. If refetch ever wants to be undoable ([[av-b17a]], [[av-3pq6]]), that grace period is where it attaches, and it should be added for *that* reason rather than this one.
- **A crash between commit and unlink leaks blobs.** [[av-8gyd]] closes this with a deletion queue: the same transaction that drops the rows records the blob ids as pending, and the queue is drained at startup. No scan, no manual step, and a drainer bug can only touch bytes something already condemned.

**Versioning changes the rule, and the link must be recorded now.** If artifact source gains version history ([[av-3pq6]]), a retained old version still fetches its old assets, so "superseded" stops meaning "dead". The rule becomes: *an asset generation is deletable when no retained version references it.* That stays decidable — it is a count over recorded generations, not an inference about code — but only if each body version records the generation it uses. Record that link when generations are introduced here, even though nothing reads it yet; retrofitting it later means guessing which assets belonged to versions written before the column existed, and there is no way to guess correctly.

**This needs no pruning policy, because generations are rare.** A generation is minted by ingest and refetch only — a body edit does not produce one. Fifty edits give fifty body versions and one generation. Content-address the generation on top of that, so a refetch returning identical bytes reuses the existing one rather than minting a duplicate, and growth is bounded by how often the upstream binary genuinely changes. That is the same mitigation [[av-3pq6]] already specifies for version bodies, applied to generations.

Note the direction of the interaction: this ticket makes [[av-3pq6]]'s "keep all versions" *cheaper*. Its retention section worries specifically that vendored snapshot bodies can be multi-MB; after this change they are small text, and the payload lives in one generation shared by every version referencing it. So keep all versions, keep any generation a retained version references, and add a bytes budget only if it ever hurts in practice — which is av-3pq6's stated position already. The asset panel covers the exception.

**The asset panel** handles the case generations cannot: the user removed the feature that used a 14 MB payload and wants the space back. Only they can know that, so the panel is where they say it.

A collapsible panel on the artifact **edit** page, beside the security panel and the state inspector, and modelled on the latter (av-hg5f): the shell renders server-side and the contents are fetched on first open, since asset metadata is cold data the rest of the page never needs. It lists each asset — **source URL**, size, content type — plus the artifact's total, with a delete control per row. Deleting calls an authenticated API route that drops the row and, if that was the last reference, enqueues the blob ([[av-8gyd]]).

**The scan earns a place here as an advisory.** Flag an asset whose source URL no longer appears as a literal in the current body — "no reference found in the current source". It cannot authorize a delete (see above), but it is a genuinely useful starting point for the judgement the panel exists to support, and it is the scan doing what it already does everywhere else in this system: transparency, not enforcement (PRD §6.2). Word it as an observation, never as a recommendation.

Two things it must get right, because this is the one control here that can break a working artifact:

- **Deleting an asset still in use breaks the artifact at render** — the fetch leaves the manifest, reaches the network, and fails. The source URL is therefore the primary column, not a detail: it is what the user matches against their own code to decide. Not a confirmation dialog's job; the row itself has to carry enough to decide.
- **Say what the recovery is.** For a URL-ingested artifact a refetch re-vendors the assets, so a mistake is undoable. For a pasted artifact it is not, and the panel should not imply otherwise.

Splittable into a child ticket if this one gets too large, but not droppable — without it the design ships storage a user can neither see nor reclaim.

**Rejected: GC by render observation.** Having the manifest wrapper report which assets it actually served, and reaping the unused, superficially matches "observe, don't predict" (architecture.md §1.4). It does not: that principle is about refusing to predict behaviour in order to *gate policy*, whereas this would use observation to authorize *destruction*, where being wrong is unrecoverable. An artifact nobody has opened this month is not an artifact whose assets are unused, and distinguishing the two needs per-artifact render counting. It also adds a reporting channel out of the sandbox for no other reason.

**Agent surface.** `get_artifact` returns the body — now small, and now the *original* page source with no injected machinery in it — plus asset metadata in its meta block: id, source URL, size, content type, no bytes. The agent needs to know an asset exists so it understands why a bare `fetch('/app.wasm')` in the source it is reading actually resolves; it has no use for the bytes. With the manifest render-injected, the agent has nothing it *can* break here, which is what makes the preview loop safe to iterate in.

**Out of scope, deliberately:**

- **Export** — [[av-vnkt]] owns the invariant (the URL form is internal; the file is canonical and is materialized at every exit). Not a blocker for this ticket; it needs to exist before an exported artifact is promised to anyone.
- **Migrating existing artifacts.** Bodies already carrying an ingest-injected manifest of `data:` payloads keep working unchanged: they have no asset rows, so the preamble's manifest is empty and their in-body wrapper still resolves its own entries. The two representations coexist without a branch. Extracting the old ones is a one-shot maintenance command and a separate ticket, because it rewrites stored bodies and deserves its own blast radius.
- **Raising `MaxInlineAssetBytes`.** The current 16 MiB was chosen because of what it becomes *in the body* ("base64 to ~21 MiB"). That rationale disappears here, so the cap should be revisited — but as its own decision, not smuggled into this change.

## Acceptance Criteria

- A URL ingest with `snapshot: true` of a page fetching a `.wasm` at runtime stores the payload as its own blob. **The stored body contains no base64 payload and no injected manifest** — its fetch literals are byte-identical to the fetched page's.
- The artifact renders and instantiates its wasm module in the sandboxed gallery iframe, with an empty user allowlist — the same end state as today, verified in a real browser and not only in a unit test.
- `GET /a/<artifactID>/assets/<assetID>` serves the bytes with `Access-Control-Allow-Origin: *`, the correct `Content-Type` (`application/wasm` for `.wasm`), and an immutable long-lived `Cache-Control`. A second view of the same artifact does not re-transfer the asset.
- The route never redirects, and refuses an assetID that belongs to a different artifact.
- The emitted CSP carries the per-artifact asset path in `connect-src`; `artifact_network_origins` gains no row, and the allowlist editor shows nothing new.
- The manifest installs before any artifact script, and before the preamble's `data:` fetch shim wraps `fetch`. Ordering is asserted by a test, not left to injection-site coincidence.
- `get_artifact` returns a body small enough to edit, plus asset metadata (id, source URL, size, content type) and no asset bytes.
- **An agent that replaces the body wholesale — including one that emits a document with no `<head>` — leaves the artifact still loading its assets.** This is the preview-loop guarantee that render-time injection buys, and the reason it is worth doing.
- Re-rendering after an agent save re-fetches the render document but **not** the asset: the cache-busting stamp is on the document URL, and the assetID is stable across body rewrites.
- Deleting an artifact deletes its assets. Deleting one owner's account leaves another owner's artifacts renderable, asserted by a test.
- **Two artifacts in one library sharing a blob:** deleting one leaves the other rendering, with its bytes intact. The blob is enqueued for deletion only when the last referencing row goes. This is the test that catches an unconditional enqueue.
- A refetch that produces a new asset set leaves the superseded set deletable and deletes it; repeated refetches do not accumulate asset sets.
- An ordinary body PATCH never deletes an asset, **including one whose fetch literal no longer appears in the body** — pinned by a test, because this is the case a future "helpful" cleanup would get wrong.
- The edit page's asset panel lists each asset with its source URL, size, and content type, plus the artifact's total; its contents load on first open, not with the page. Deleting a row removes the asset, and the artifact still renders when the deleted asset was genuinely unused.
- Deletability of a generation is answered by a count over recorded generation ids, never by inspecting body content — asserted by a test in which the body's fetch literal is gone but the generation is still referenced, and the asset survives.
- Each asset row records the generation that created it, and the schema can express which body version a generation belongs to — even though version history ([[av-3pq6]]) does not exist yet and nothing reads the link.
- Artifacts already carrying inlined `data:` payloads continue to render unchanged.
- `architecture.md` §3.4a is rewritten: the runtime pass records assets rather than inlining them, and the manifest moves into the render preamble described in §3.2 — where it also replaces the note explaining the two independent `fetch` wrappers. §3.1 gains the asset route, §3.3 the assets table. `technical_stack.md` §4 and §7 follow.


## Notes

**2026-08-17T03:33:01Z**

Design amended: assets are recorded at ingest but the manifest is injected at render time, in the preamble, rather than written into the stored body. Motivated by the agent preview loop — a wholesale body rewrite can no longer break asset loading. Also makes the assets table the single source of truth, collapses the two independent fetch wrappers architecture.md 3.2 documents as separate, and makes av-vnkt's export symmetric with render.

**2026-08-17T03:35:52Z**

Added asset lifecycle. Orphaning is real: with the manifest render-injected, the body never answers 'is this asset referenced', and it must not be asked to — scanning for surviving fetch literals would delete assets behind runtime-constructed URLs. GC only on decidable questions: superseded ingest generations (automatic) and blob-with-no-row (operator sweep). User-visible asset panel on the edit page for intentional reclaim. Rejected GC-by-render-observation.

**2026-08-17T04:39:45Z**

Versioning (av-3pq6) changes the GC rule from 'superseded' to 'no retained version references this generation' — still decidable, but only if body versions record their generation. Record that link now; it cannot be reconstructed later.

**2026-08-17T04:44:04Z**

No automatic pruning policy: generations are minted only by ingest/refetch (never by edits) and content-addressed, so growth is bounded by real upstream changes. Manual asset panel covers the rest. Full-scan reclamation dropped from av-8gyd — deletion queue only.

**2026-08-17T05:39:26Z**

Clarified deletability as two separate questions: (1) is the ROW deletable — artifact deleted / generation superseded with no retained version / explicit user delete, and never 'the body stopped fetching it'; (2) is the BLOB deletable — only when the last referencing row is gone, refcounted inside the delete transaction. Shared blobs between two artifacts of one owner made this load-bearing.

**2026-08-17T05:45:50Z**

Clarified two things that were load-bearing but only implied. (1) Generation deletability is a count over recorded generation ids — a claim about the document's lifetime (no retained body came with these assets, so nothing can fetch them) rather than about its contents, which is why it is decidable where 'does the JS still fetch this URL' is not. (2) Defined the asset panel concretely: edit page, beside the state inspector, lazy-loaded, source URL as the primary column since it is what a user matches against their code, and refetch as the stated recovery for URL-ingested artifacts.

**2026-08-17T05:48:58Z**

Corrected the rationale for refusing scan-based GC. The earlier framing (a runtime-constructed URL might hide an asset from the scan) was wrong: manifest entries come only from scanner.FetchRefs literals, so every asset originates from a literal and creation is fully covered. The real gap is consumption — the wrapper matches resolved URLs at call time, so a rewritten body can consume an asset whose original literal is gone. The broader reason stands regardless: scan GC is unnecessary, and being wrong destroys an unrecoverable payload. Added the scan back as a non-authoritative advisory in the asset panel.

**2026-08-17T05:53:49Z**

Scoped explicitly: this covers the runtime pass only (.wasm/.data/.bin/.mem). Markup-referenced assets (images, fonts, CSS, scripts) stay inlined as data: URIs, because they are not loaded through window.fetch — there is no wrapper to hook, so externalizing them requires writing render-origin URLs into the stored body, which is exactly the property this ticket protects. Also flagged that 'asset' collides with the existing pass-1 meaning in the codebase; the table should be defined as any out-of-line asset with the runtime pass as its only current producer.

**2026-08-17T06:01:22Z**

Split out av-oz40 for the markup pass (images/fonts/CSS/scripts). Correcting the earlier scoping note: that pass is not less acute for the agent — it has no cap on asset count short of the 48 MiB total, so an image-heavy page can exceed a single wasm payload. It needs rewriting rather than interception, which is why it is a sibling rather than part of this ticket.

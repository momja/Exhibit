---
id: av-20fk
status: open
deps: []
links: [av-vnkt, av-ghvs, av-c5aq]
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

**Lifecycle: what makes an asset deletable.** Moving the manifest out of the body means the body is never consulted to build it, so nothing in the body answers "is this asset still referenced". Nothing can be allowed to: a scan for surviving `fetch` literals is exactly the static analysis this architecture refuses to trust (`scanner.LiteralRefs` is a hint, not analysis), and a runtime-constructed URL never appears as a literal — so a scan-based GC deletes assets that are still in use. Data loss is a worse outcome than leaked bytes, so the rule is that **only decidable questions may authorize a delete**:

| Question | Decidable? | Use |
|---|---|---|
| Is there an asset row pointing at this blob? | Yes — rows and blobs are both ours | Operator-level orphan sweep |
| Was this row created by a superseded ingest? | Yes — generations are recorded | Automatic GC |
| Does the current body still fetch this URL? | **No** — needs JS analysis | **Never** GC on this basis |

**Generations.** Each ingest or refetch that produces assets mints a generation id; asset rows carry it. Render injects only the current generation's manifest. When a new generation supersedes an old one the old *set* becomes deletable as a unit — the body it belonged to has been replaced too, so nothing can still reference it. This is what keeps [[av-b17a]]'s refetch path from accumulating a full asset set per refetch, forever. An ordinary body PATCH does **not** mint a generation and does not touch assets: the body may still fetch them, and we cannot know otherwise.

**An asset panel on the edit page** handles the case generations cannot: the user removed the feature that used a 14 MB payload, and wants the space back. It lists each asset — source URL, size, content type — with a delete control, and the artifact's total. It is the same shape as the state inspector beside it (av-hg5f): show what is stored, let the owner remove it, on the surface built for that. This is what "the user has full control over their artifacts" (PRD §1) requires once artifacts carry bytes they cannot see. Splittable into a child ticket if this one gets too large, but not droppable — without it the design ships storage a user can neither see nor reclaim.

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
- A refetch that produces a new asset set leaves the superseded set deletable and deletes it; repeated refetches do not accumulate asset sets.
- An ordinary body PATCH never deletes an asset, **including one whose fetch literal no longer appears in the body** — pinned by a test, because this is the case a future "helpful" cleanup would get wrong.
- The edit page lists the artifact's assets with source URL, size, and content type, plus a total, and can delete one.
- Artifacts already carrying inlined `data:` payloads continue to render unchanged.


## Notes

**2026-08-17T03:33:01Z**

Design amended: assets are recorded at ingest but the manifest is injected at render time, in the preamble, rather than written into the stored body. Motivated by the agent preview loop — a wholesale body rewrite can no longer break asset loading. Also makes the assets table the single source of truth, collapses the two independent fetch wrappers architecture.md 3.2 documents as separate, and makes av-vnkt's export symmetric with render.

**2026-08-17T03:35:52Z**

Added asset lifecycle. Orphaning is real: with the manifest render-injected, the body never answers 'is this asset referenced', and it must not be asked to — scanning for surviving fetch literals would delete assets behind runtime-constructed URLs. GC only on decidable questions: superseded ingest generations (automatic) and blob-with-no-row (operator sweep). User-visible asset panel on the edit page for intentional reclaim. Rejected GC-by-render-observation.

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
- **No deduplication.** The same ffmpeg.wasm ingested five times is five copies.

Inlining was chosen for a narrow reason: CORS (av-ghvs). A page served from its own site fetches `/app.wasm` same-origin and needs no CORS headers, so source sites do not send them; relocated to the render origin the read fails. That argument does not carry over to assets served from *our* origin, where we set the response headers.

## Design

Move each vendored runtime payload into its own blob, addressed by URL, and leave the artifact body a small, valid HTML file.

**Ingest.** `InlineRuntimeAssets` (`internal/snapshot/runtime.go`) keeps its current structure — substitution stays *interception, not source rewriting*, because that is what survives minification and runtime-constructed URLs. Only the manifest *value* changes: instead of a `data:` URI it becomes `RENDER_ORIGIN/a/<artifactID>/assets/<assetID>`. The `window.fetch` wrapper gets simpler as a result — it rewrites the URL and delegates to native `fetch` rather than decoding base64 into a `Response`.

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

**Agent surface.** `get_artifact` returns the body — now small — plus asset metadata in its meta block: id, size, content type, no bytes. The agent needs to know an asset exists so it does not treat the manifest script as dead code; it has no use for the bytes. Note a side benefit: today an agent that clobbers the manifest destroys the payload with it, while under this design the bytes survive in storage and the reference is recoverable.

**Widgets.** `/w/:id` renders through the same path and shares its artifact's assets; asset access is scoped to the artifact, not to which document is being served.

**Out of scope, deliberately:**

- **Export** — [[av-vnkt]] owns the invariant (the URL form is internal; the file is canonical and is materialized at every exit). Not a blocker for this ticket; it needs to exist before an exported artifact is promised to anyone.
- **Migrating existing artifacts.** Bodies already carrying inlined `data:` payloads keep working unchanged. Extracting them is a one-shot maintenance command and a separate ticket, because it rewrites stored bodies and deserves its own blast radius.
- **Raising `MaxInlineAssetBytes`.** The current 16 MiB was chosen because of what it becomes *in the body* ("base64 to ~21 MiB"). That rationale disappears here, so the cap should be revisited — but as its own decision, not smuggled into this change.

## Acceptance Criteria

- A URL ingest with `snapshot: true` of a page fetching a `.wasm` at runtime stores the payload as its own blob; the artifact body contains a manifest of URLs and no base64 payload.
- The artifact renders and instantiates its wasm module in the sandboxed gallery iframe, with an empty user allowlist — the same end state as today, verified in a real browser and not only in a unit test.
- `GET /a/<artifactID>/assets/<assetID>` serves the bytes with `Access-Control-Allow-Origin: *`, the correct `Content-Type` (`application/wasm` for `.wasm`), and an immutable long-lived `Cache-Control`. A second view of the same artifact does not re-transfer the asset.
- The route never redirects, and refuses an assetID that belongs to a different artifact.
- The emitted CSP carries the per-artifact asset path in `connect-src`; `artifact_network_origins` gains no row, and the allowlist editor shows nothing new.
- `get_artifact` returns a body small enough to edit, plus asset metadata (id, size, content type) and no asset bytes. An agent round-trip — read, modify one line, write — preserves the assets.
- Deleting an artifact deletes its assets. Deleting one owner's account leaves another owner's artifacts renderable, asserted by a test.
- Artifacts already carrying inlined `data:` payloads continue to render unchanged.


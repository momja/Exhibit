---
id: av-52ll
status: in_progress
deps: []
links: [av-8gyd, av-20fk]
created: 2026-08-18T05:56:25Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-1in5
tags: [hosted, backend, storage]
---
# S3-compatible blob backend behind the Blob interface

`blob.FSStore` (`internal/blob/blob.go:46`) is the only implementation of the `Blob` interface, and `cmd/server/main.go:84` wires it unconditionally. Artifact bodies, widget documents, and — once [[av-20fk]] lands — URL-addressed runtime assets are all files under `DATA_DIR`.

For a hosted service that is the binding constraint. It pins the service to one machine's disk, makes every deploy a volume migration, and leaves backup half-solved: the Litestream profile in `docker-compose.yml` streams the SQLite WAL to a bucket, so a restore recovers every row and none of the bytes those rows point at. A library of artifacts whose bodies no longer exist is not a recovered library.

The interface was designed for this from the start (`architecture.md` §3.3) — this ticket is writing the implementation it was designed for, not changing the design.

## Design

**Nothing above `Blob` changes.** Three methods, `Put`/`Get`/`Delete`, and handlers already speak only to the interface. If any caller needs to know which backend is behind it, the backend is wrong.

**`Delete` is already specified for this.** The interface's contract is idempotent — a missing id is success — and the doc comment on `internal/blob/blob.go` says outright that this is the contract *because* S3's `DeleteObject` answers success for a missing key, and that defining it the other way would force a `HEAD` before every delete. That reasoning is now being cashed in, so the S3 implementation must not reintroduce the existence check the contract exists to avoid.

**Selection is configuration, and absent means filesystem.** An env var names the bucket, and unset keeps `FSStore` exactly as today. A self-hoster gets no new required configuration and no new dependency.

**Streaming, not buffering.** `Put` takes an `io.Reader` and `Get` returns an `io.ReadCloser`. A snapshot with a vendored wasm payload can be tens of megabytes; an implementation that reads a body fully into memory to satisfy an SDK turns each of those into a per-request allocation on a shared host.

**MinIO counts as done.** The target is S3-compatible, not AWS — the compose file already offers MinIO for exactly this. Testing against MinIO is what keeps it honest.

**Interaction with the assets work.** [[av-20fk]] introduces URL-addressed asset blobs with refcounting and [[av-8gyd]] a deletion queue. Both go through the same interface, so neither needs to know about this — but the deletion queue's reclamation becomes a network call rather than an `unlink`, which is worth confirming it tolerates.

## Acceptance Criteria

- With the bucket variable unset, blob storage is `FSStore` and behavior is identical to today.
- With it set, artifact ingest, render, widget save, refetch and delete all work against the bucket with no handler changes.
- `Delete` of an id that was never stored succeeds, with no existence check ahead of it.
- Bodies stream in both directions; no code path reads a whole blob into memory to store or serve it.
- The full suite passes against MinIO.
- `docs/deployment.md` and `architecture.md` §3.3 record that the blob backend is now selectable, and that Litestream covers the database only.


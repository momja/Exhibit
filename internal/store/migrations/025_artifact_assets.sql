-- +goose Up
-- Version 25 for the reason 024 records: this file was numbered 20 while 18
-- was the highest, and a migration numbered below the ledger's high-water mark
-- stops goose before it runs anything.
--
-- Out-of-line artifact assets (av-20fk): the binary payloads a page fetches at
-- run time — wasm modules, Emscripten .data heaps — stored as blobs of their
-- own instead of base64 inside the artifact body.
--
-- The body used to carry them as data: URIs, at ~1.33x their size. That made a
-- single 16 MiB payload ~21 MB of base64 in the agent's context on every read
-- and every write, unreadable in CodeMirror, and re-transferred on every render
-- because the render document is necessarily Cache-Control: no-store.
--
-- id is random and unguessable, and is the credential: the asset route serves
-- these bytes without a render token, because a short-lived token in the URL
-- would change it on every render and destroy the caching that motivates the
-- whole change. Reading one therefore requires already knowing both the
-- artifact id and this id.
--
-- source_url is the absolute URL the page asks for at run time. It is what the
-- render manifest is keyed by, not the blob id and not anything derived from
-- the bytes, because the wrapper matches on the URL the artifact actually
-- requests.
--
-- generation_id groups every asset produced by one ingest or refetch. It is
-- what makes "is this still needed" answerable without reading the artifact's
-- code: a generation is deletable when no retained body version references it,
-- which is a count over ids we recorded ourselves. Deletability is never
-- inferred from whether a fetch literal still appears in the body — a URL can
-- be assembled at run time, and a wrapper matching resolved URLs will serve an
-- asset whose original literal is long gone.
--
-- version_seq is that link, recorded now and read by nothing yet. Artifact
-- version history (av-3pq6) does not exist, so today there is exactly one live
-- generation per artifact and the check degenerates to "is this the current
-- one". The column is here because it cannot be reconstructed later: retrofit
-- it after versions exist and there is no way to know which generation a body
-- written before the column belonged to.
CREATE TABLE IF NOT EXISTS artifact_assets (
    id            TEXT PRIMARY KEY,
    artifact_id   TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    generation_id TEXT NOT NULL,
    version_seq   INTEGER,
    source_url    TEXT NOT NULL,
    blob_id       TEXT NOT NULL,
    content_type  TEXT NOT NULL,
    size_bytes    INTEGER NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- The render path's query: every asset of one artifact, to build the manifest.
CREATE INDEX IF NOT EXISTS idx_artifact_assets_artifact
    ON artifact_assets (artifact_id);

-- The refcount's query. Content addressing is per owner, so two artifacts in
-- one library legitimately share a blob, and deleting one must not take the
-- bytes out from under the other. Without this index that check is a scan on
-- every delete.
CREATE INDEX IF NOT EXISTS idx_artifact_assets_blob
    ON artifact_assets (blob_id);

-- Generation GC reads by (artifact, generation) when a refetch supersedes a set.
CREATE INDEX IF NOT EXISTS idx_artifact_assets_generation
    ON artifact_assets (artifact_id, generation_id);

-- +goose Down
DROP INDEX IF EXISTS idx_artifact_assets_generation;
DROP INDEX IF EXISTS idx_artifact_assets_blob;
DROP INDEX IF EXISTS idx_artifact_assets_artifact;
DROP TABLE IF EXISTS artifact_assets;

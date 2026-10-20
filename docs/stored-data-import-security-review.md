# Stored-data import security review

Reviewed on 2026-10-20 by a separate read-only security agent, against the full
import change and the remediation working tree based on `784950c`. The review
used the current [OWASP Top 10:2025 categories](https://top10.owasp.org/2025/),
source inspection, and focused local attack reproductions. It did not attack the
live demo or external applications. No outstanding new security finding remained
after the follow-up checks.

## Findings and fixes

- **P2: deleted asset IDs could be rebound.** Removing the final current/history
  reference previously removed the only ID-to-content identity. An import could
  then reuse that ID for different bytes or MIME at an anonymously readable URL
  advertised as immutable. Migration 034 backfills current and historical
  identities and records future identities separately from payload references.
  These records survive deletion and do not keep payloads alive. Reproductions
  now reject changed bytes/MIME with 400 while restoring identical exports.
- **P2: bulk state imports could monopolize the database.** A 1,188,891-byte
  JSON object with 100,000 keys occupied SQLite's sole connection for 4–5.6
  seconds and made an unrelated tenant's read time out. The parser now validates
  one flat string-valued object incrementally and rejects duplicate or more than
  10,000 keys before database access. The store repeats the cardinality check,
  uses prepared inserts, and applies a five-second deadline. The original
  reproduction now receives 413 in about 28 ms, preserving saved state.

## OWASP coverage

| Category | Inspection and result |
| --- | --- |
| A01 Broken Access Control | Owner checks before parsing and within replacement transactions; foreign owners, recipients, anonymous/public visitors, and scoped agents cannot import. Existing session CSRF policy applies. No new bypass found. |
| A02 Security Misconfiguration | Checked origin separation, MIME, escaped templates/DOM, upload limits, and ZIP paths. Existing active-content and same-site CSRF limits are recorded below. |
| A03 Software Supply Chain Failures | No new dependency or third-party archive parser. The checked local Go 1.25.7 exceeds 1.25.6, the fixed version for the relevant [ZIP advisory GO-2026-4342](https://pkg.go.dev/vuln/GO-2026-4342). |
| A04 Cryptographic Failures | Per-owner SHA-256 blob addressing and public asset URL capabilities; no new cryptographic defect found. |
| A05 Injection | Parameterized SQL, escaped template metadata and `textContent`, strict state values; source URLs remain metadata and cause no network fetch. No new SQL, DOM, command, or SSRF injection found. |
| A06 Insecure Design | Bulk-state resource issue fixed; asset quota uses the final replacement set while retaining shared/history charges. |
| A07 Authentication Failures | Existing session, service-token, and scoped-agent middleware reused; no new credentials or authentication bypass. |
| A08 Software or Data Integrity Failures | Deleted-ID rebinding fixed. ZIP manifest, IDs, paths, sizes, duplicates, and CRCs are checked before staging; replacement is atomic. |
| A09 Security Logging and Alerting Failures | Existing structured request/error logging applies; import contents are not logged. Existing backend-error/debug-query disclosure limits remain. |
| A10 Mishandling of Exceptional Conditions | Checked malformed input, transaction rollback, staged-blob cleanup, cancellation, and resource bounds. No additional actionable defect found. |

## Verified boundaries

- The global HTTP request ceiling still applies. Asset archives have at most
  200 asset rows, 96 MiB expanded payloads, 48 MiB distinct payloads, and a 1 MiB
  manifest. The two-row allowance covers markup/runtime collection of one fetched
  URL. Identical payload buffers are shared after CRC checks.
- Archives are never extracted to disk. Traversal, absolute/backslash paths,
  directories, symlinks, duplicate manifests, malformed metadata, and bad CRCs
  are refused. A ZIP inside an asset remains opaque; imports never fetch source
  URLs.
- Follow-up tests covered unauthorized requests, changed deleted asset bytes and
  MIME, exact restoration, the original state resource reproduction, quota
  refusals, failed writes, and transaction rollback. Committed tests cover
  migration backfill, retained identities, cardinality boundaries, complete
  snapshot exports, and inspector Save/import coordination.

## Residual limits

- Migration 034 cannot reconstruct an identity deleted before migration with no
  surviving reference. Current and historical identities are backfilled.
- Asset URLs are public bearer capabilities, and previously cached bytes cannot
  be revoked by removing a share. Minimal URL identity records survive artifact
  and account deletion to prevent reassignment; they retain opaque IDs, content
  addresses, and MIME, but no payload, state, title, or source URL.
- HTML/SVG assets can execute active content when opened directly without the
  document renderer's CSP. Snapshot ingest already allowed equivalent MIME and
  content; uploads add another path into that existing architecture.
- Existing SameSite=Lax CSRF protection does not isolate sibling origins under
  the same site. State remains outside blob quotas; backend errors and debug
  query logging retain their existing disclosure limits.
- This review is not exhaustive fuzzing, load testing, or a complete dependency
  audit. Toolchain observations describe the checked environment, not every
  future deployment.

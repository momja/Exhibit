import { readFileSync } from 'node:fs';

// A green `go test` can include skipped browser tests. Require the existing
// Stimulus integrations to actually pass, without changing their local opt-in.
const required = new Set(['TestActionMenuBrowser', 'TestGalleryActionMenuDuplicationBrowser']);
for (const line of readFileSync(new URL('./artifacts/go-tests.jsonl', import.meta.url), 'utf8').split('\n').filter(Boolean)) {
  const event = JSON.parse(line);
  if (event.Package !== 'github.com/momja/Exhibit/internal/api' || !required.has(event.Test)) continue;
  if (event.Action === 'skip' || event.Action === 'fail') throw new Error(`${event.Test} ${event.Action}: required browser coverage did not pass.`);
  if (event.Action === 'pass') required.delete(event.Test);
}
if (required.size) throw new Error(`Missing required browser results: ${[...required].join(', ')}`);
console.log('Both required Stimulus Chromium integrations executed and passed.');

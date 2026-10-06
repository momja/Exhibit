/* The edit page's Versions panel: restoring asks first, names what it will do,
 * sends exactly one request, and reloads only when it worked.
 *
 * Loads the built, embedded copy of versions.js — the bytes the browser is
 * served.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript, recordingApi } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const VERSIONS_JS = path.join(here, "../../internal/api/assets/gallery/versions.js");

function load({ confirmAnswer = true, responses = [] } = {}) {
  const api = recordingApi(responses);
  const seen = { confirms: [], reloads: 0 };
  const page = loadPageScript(VERSIONS_JS, {
    ID: "art 1",
    apiFetch: api.apiFetch,
    confirm: (msg) => { seen.confirms.push(msg); return confirmAnswer; },
    location: { reload() { seen.reloads++; } }
  });

  // Stands in for one server-rendered restore button and the row it sits in.
  const clickRestore = (seq) => {
    const btn = page.byId("restore-" + seq);
    btn.dataset.seq = String(seq);
    return page.byId("versions-rows").dispatchEvent({
      type: "click",
      target: { closest: (sel) => (sel === '[data-action="restore"]' ? btn : null) }
    }).then(() => page.settle?.());
  };
  return { ...page, api, seen, clickRestore, restoreButton: (seq) => page.byId("restore-" + seq) };
}

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

test("restoring asks first, naming the version and what is kept", async () => {
  const t = load();
  await t.clickRestore(3);
  assert.equal(t.seen.confirms.length, 1);
  assert.match(t.seen.confirms[0], /Restore v3\?/);
  assert.match(t.seen.confirms[0], /code and its saved data/);
  assert.match(t.seen.confirms[0], /kept as its own version/);
});

test("a confirmed restore posts to the version's restore route and reloads", async () => {
  const t = load();
  await t.clickRestore(3);
  await tick();
  assert.deepEqual(t.api.calls.map((c) => [c.method, c.path]),
    [["POST", "/api/artifacts/art%201/versions/3/restore"]]);
  assert.equal(t.seen.reloads, 1);
});

test("declining sends nothing and leaves the button alone", async () => {
  const t = load({ confirmAnswer: false });
  await t.clickRestore(3);
  await tick();
  assert.equal(t.api.calls.length, 0);
  assert.equal(t.seen.reloads, 0);
  assert.equal(t.restoreButton(3).disabled, undefined);
});

test("a refused restore says why, does not reload, and can be retried", async () => {
  const t = load({
    responses: [{ ok: false, status: 409, json: async () => ({ error: "that version is already the current one" }) }]
  });
  await t.clickRestore(3);
  await tick();
  assert.equal(t.seen.reloads, 0);
  assert.equal(t.restoreButton(3).disabled, false, "the button comes back");
  assert.match(t.byId("versions-status").textContent, /already the current one/);
});

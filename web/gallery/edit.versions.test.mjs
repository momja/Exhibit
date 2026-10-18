/* The edit page's Versions panel: restoring asks first, names what it will do,
 * sends exactly one request, and reloads only when it worked — and looking at a
 * version first is the markup's swap, with script owning only what closing means.
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

  // One listener on the panel serves every button in it, so a click is
  // delivered to the panel with the target a browser would resolve: whichever
  // button the page's markup put under the pointer.
  const click = (action, button) => page.byId("versions-panel").dispatchEvent({
    type: "click",
    target: { closest: (sel) => (sel === '[data-action="' + action + '"]' ? button : null) }
  }).then(() => page.settle?.());

  // Stands in for one server-rendered restore button — the row's or the
  // viewer's, which are the same markup — and what it does.
  const clickRestore = (seq) => {
    const btn = page.byId("restore-" + seq);
    btn.dataset.seq = String(seq);
    return click("restore", btn);
  };
  return { ...page, api, seen, click, clickRestore, restoreButton: (seq) => page.byId("restore-" + seq) };
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

// --- Looking at a version first ----------------------------------------------------

// The viewer is a fragment swapped into a slot above the list; its "Back to
// current" button means, on this page, that the slot is emptied.
test("Back to current empties the viewer and sends nothing", async () => {
  const t = load();
  const slot = t.byId("version-viewer-slot");
  slot.innerHTML = '<div class="version-viewer">v2, running</div>';

  await t.click("close-version-viewer", t.byId("close-viewer"));
  await tick();

  assert.equal(slot.innerHTML, "");
  assert.equal(t.api.calls.length, 0, "looking, and stopping looking, write nothing");
  assert.equal(t.seen.confirms.length, 0);
});

// The viewer carries the restore beside what is being looked at, and it is the
// same button the rows carry, so it must do the same thing the same way — ask,
// name the version, send one request — rather than a second code path.
test("restoring from the viewer is the same restore as the row's", async () => {
  const t = load();
  await t.clickRestore(2);
  await tick();

  assert.match(t.seen.confirms[0], /Restore v2\?/);
  assert.deepEqual(t.api.calls.map((c) => [c.method, c.path]),
    [["POST", "/api/artifacts/art%201/versions/2/restore"]]);
});

// A press on anything else in the panel — View among it, whose whole request is
// in its markup — is not script's to answer.
test("a click that is neither close nor restore does nothing", async () => {
  const t = load();
  await t.byId("versions-panel").dispatchEvent({ type: "click", target: { closest: () => null } });
  await tick();
  assert.equal(t.api.calls.length, 0);
  assert.equal(t.seen.confirms.length, 0);
});

test("a viewer opened from far down the list is brought into view", async () => {
  const t = load();
  const slot = t.byId("version-viewer-slot");
  let shown = null;
  slot.scrollIntoView = (opts) => { shown = opts; };

  await slot.dispatchEvent({ type: "htmx:after:swap" });

  // Copied out of the vm's realm, whose Object.prototype is not this file's.
  assert.deepEqual({ ...shown }, { block: "nearest" });
});

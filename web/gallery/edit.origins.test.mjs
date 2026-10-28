/* av-kmwj — "Forget" on the edit page's blocked-origins list.
 *
 * The other half of the ticket's revocability requirement: a "don't ask again"
 * answer must have a way out, and PATCH cannot express one (it replaces the
 * allow set and deliberately leaves block rows alone), so Save has to reach the
 * per-origin DELETE. That split between two write paths is exactly the kind of
 * thing a substring assertion on the file would not notice going wrong.
 *
 * The last test covers a neighbour on the same Save: the location select
 * (av-f446), which must ship only when it was touched.
 *
 * Loads the built, embedded copy of edit.js — the bytes the browser is served.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript, recordingApi } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const EDIT_JS = path.join(here, "../../internal/api/assets/gallery/edit.js");

const ID = "art-1";
const BLOCKED = "https://tracker.example.com";

// Loads edit.js with the bootstrap globals its page template renders, plus the
// one delegated container the blocked-origins section needs. Rows are looked up
// through `closest`, which is what the real handler calls on the click target.
async function loadEdit({ blocked = [BLOCKED], allowlist = [], footprint = [], responses = [] } = {}) {
  const api = recordingApi(responses);
  const page = loadPageScript([], {
    TOKEN: "", READ_ONLY: false, ID, INITIAL_SOURCE_FOOTPRINT: footprint,
    allowlist, unapproved: footprint.filter(origin => !allowlist.includes(origin) && !blocked.includes(origin)), blocked,
    downloadsApproved: false, clipboardApproved: false, linksApproved: false,
    cameraApproved: false, microphoneApproved: false, geolocationApproved: false,
    apiFetch: api.apiFetch,
    alert: () => {}, confirm: () => true, CustomEvent: class { constructor(type) { this.type = type; } }
  }, {
    // save() refuses an empty body before it reaches anything else, so the
    // fields it reads have to hold what the page would have rendered.
    "title": { value: "A tool" },
    "body": { value: "<html><body>hi</body></html>" },
    "scan-result": { style: {} }
  });

  const module = await page.loadModule(EDIT_JS, ['save', 'saveEditorChanges', 'artifactDirty', 'artifactDraft', 'approveOrigins', 'finishEdit', 'discardArtifact']);
  Object.assign(page.context, module);

  // Stands in for one server-rendered .allowlist-row: the button the user
  // clicks, and the row element it resolves to.
  const clickRow = (action, origin) => {
    const row = page.byId("row-" + origin);
    row.dataset.origin = origin;
    const btn = page.byId("btn-" + action + "-" + origin);
    btn.dataset.action = action;
    btn.closest = selector => selector === '.allowlist-row' || selector === '#blocked-rows' ? row : null;
    return page.byId("origin-lists").dispatchEvent({ type: "click", target: {
      closest: (sel) => (sel === 'button[data-action]' ? btn : null)
    } });
  };

  return { ...page, api, clickRow };
}

const originCalls = (api) => api.calls.filter((c) => c.path.includes("/origins"));
const saveButton = (page) => page.context.save;

test("Forget deletes the decision through the per-origin route", async () => {
  const page = await loadEdit();
  page.clickRow("forget", BLOCKED);
  await saveButton(page)();

  const [call] = originCalls(page.api);
  assert.ok(call, "Save must apply the forget; PATCH cannot express it");
  assert.equal(call.method, "DELETE");
  assert.equal(call.path,
    `/api/artifacts/${ID}/origins?origin=${encodeURIComponent(BLOCKED)}`);
});

for (const [selectID, field] of [['link-select', 'links_approved'], ['cam-select', 'camera_approved'], ['mic-select', 'microphone_approved'], ['geo-select', 'geolocation_approved']]) {
  test(`Save persists an explicit ${field} revocation after toggling back`, async () => {
    const page = await loadEdit({blocked: []});
    for (const value of ['true', 'false']) {
      await page.byId(selectID).dispatchEvent({type: 'change', target: {value}});
    }
    assert.equal(page.context.artifactDirty(), true);
    assert.equal(await page.context.saveEditorChanges(), true);
    const [request] = page.api.calls.filter(call => call.method === 'PATCH');
    assert.equal(JSON.parse(request.body)[field], false);
    assert.equal(page.context.artifactDirty(), false);
  });
}

test('approving source origins reconciles the blocked and undecided groups', async () => {
  const fresh = 'https://new.example.com';
  const page = await loadEdit({footprint: [BLOCKED], responses: [
    {ok: true, json: async () => ({footprint_changed: true, network_footprint: [BLOCKED, fresh]})},
    {ok: true}
  ]});
  page.byId('body').value += '<script src="' + fresh + '/lib.js"></script>';
  assert.equal(await page.context.saveEditorChanges(), false);
  page.document.querySelectorAll = selector => selector === '.al-origin:checked' ? [{value: BLOCKED}, {value: fresh}] : [];
  await page.context.approveOrigins();
  const draft = page.context.artifactDraft();
  assert.deepEqual(Array.from(draft.allowlist), [BLOCKED, fresh]);
  assert.deepEqual(Array.from(draft.blocked), []);
  assert.deepEqual(Array.from(draft.unapproved), []);
  assert.equal(page.context.artifactDirty(), false);
});

test('keeping new origins blocked leaves them available as undecided after saving', async () => {
  const fresh = 'https://new.example.com';
  const page = await loadEdit({blocked: [], responses: [
    {ok: true, json: async () => ({footprint_changed: true, network_footprint: [fresh]})}
  ]});
  page.byId('body').value += '<script src="' + fresh + '/lib.js"></script>';
  assert.equal(await page.context.saveEditorChanges(), false);
  page.context.finishEdit();
  assert.deepEqual(Array.from(page.context.artifactDraft().unapproved), [fresh]);
  assert.equal(page.context.artifactDirty(), false);
});

test('forgetting a referenced block returns it to undecided after saving', async () => {
  const page = await loadEdit({footprint: [BLOCKED]});
  await page.clickRow('forget', BLOCKED);
  assert.equal(await page.context.saveEditorChanges(), true);
  assert.deepEqual(Array.from(page.context.artifactDraft().blocked), []);
  assert.deepEqual(Array.from(page.context.artifactDraft().unapproved), [BLOCKED]);
  assert.equal(page.context.artifactDirty(), false);
});

test('discard after a failed Forget restores the last confirmed block', async () => {
  const page = await loadEdit({footprint: [BLOCKED], responses: [
    {ok: true, json: async () => ({})}, {ok: false, status: 500}
  ]});
  await page.clickRow('forget', BLOCKED);
  assert.equal(await page.context.saveEditorChanges(), false);
  page.context.discardArtifact();
  assert.deepEqual(Array.from(page.context.artifactDraft().blocked), [BLOCKED]);
  assert.deepEqual(Array.from(page.context.artifactDraft().unapproved), []);
  assert.equal(page.context.artifactDirty(), false);
});

test('origin button changes immediately update the saved status', async () => {
  const page = await loadEdit({blocked: []});
  const {default: Controller} = await page.loadModule(path.join(here, '../../internal/api/assets/gallery/unsaved-changes-controller.js'));
  const controller = Object.create(Controller.prototype);
  Object.defineProperty(controller, 'statusTarget', {value: page.byId('dirty-status')});
  page.document.body.addEventListener('exhibit:editor-changed', () => controller.refresh());
  controller.refresh();
  assert.equal(page.byId('dirty-status').textContent, 'All changes saved');
  page.byId('al-add-input').value = 'https://cdn.example.com';
  await page.byId('al-add-btn').click();
  assert.equal(page.byId('dirty-status').textContent, 'Unsaved changes');
  assert.equal(page.api.calls.length, 0, 'button changes are still local until Save');
});

test('a permission edit made while source saves remains unsaved', async () => {
  let finish;
  const response = new Promise(resolve => {finish = resolve;});
  const page = await loadEdit({blocked: [], responses: [response]});
  page.byId('title').value = 'Submitted title';
  const saving = page.context.saveEditorChanges();
  await new Promise(resolve => setImmediate(resolve));
  page.byId('al-add-input').value = 'https://later.example.com';
  await page.byId('al-add-btn').click();
  finish({ok: true, json: async () => ({})});
  assert.equal(await saving, false);
  assert.deepEqual(JSON.parse(page.api.calls[0].body).network_allowlist, []);
  assert.deepEqual(Array.from(page.context.artifactDraft().allowlist), ['https://later.example.com']);
  assert.equal(page.context.artifactDirty(), true);
});

test('reversing an unsaved Allow restores the existing block decision', async () => {
  const page = await loadEdit({footprint: [BLOCKED]});
  await page.clickRow('allow', BLOCKED);
  await page.clickRow('remove', BLOCKED);
  assert.deepEqual(Array.from(page.context.artifactDraft().blocked), [BLOCKED]);
  assert.equal(page.context.artifactDirty(), false);
});

test('an unchanged source save retains the known network footprint', async () => {
  const page = await loadEdit({blocked: [], allowlist: [BLOCKED], footprint: [BLOCKED], responses: [
    {ok: true, json: async () => ({network_footprint: [], footprint_changed: false})}
  ]});
  await page.clickRow('remove', BLOCKED);
  assert.equal(await page.context.saveEditorChanges(), true);
  assert.deepEqual(Array.from(page.context.artifactDraft().unapproved), [BLOCKED]);
  assert.equal(page.context.artifactDirty(), false);
});

test('normalized origin saves keep later local additions unsaved', async () => {
  let finish;
  const response = new Promise(resolve => {finish = resolve;});
  const page = await loadEdit({blocked: [], responses: [response]});
  page.byId('al-add-input').value = 'https://CDN.example.com/';
  await page.byId('al-add-btn').click();
  const saving = page.context.saveEditorChanges();
  await new Promise(resolve => setImmediate(resolve));
  page.byId('al-add-input').value = 'https://later.example.com';
  await page.byId('al-add-btn').click();
  finish({ok: true, json: async () => ({artifact: {network_allowlist: ['https://cdn.example.com']}})});
  assert.equal(await saving, false);
  assert.deepEqual(Array.from(page.context.artifactDraft().allowlist), ['https://CDN.example.com/', 'https://later.example.com']);
  page.context.discardArtifact();
  assert.deepEqual(Array.from(page.context.artifactDraft().allowlist), ['https://cdn.example.com']);
  assert.equal(page.context.artifactDirty(), false);
});

test('a forgotten block re-allowed with a normalized spelling keeps its new grant', async () => {
  const page = await loadEdit({footprint: [BLOCKED], responses: [
    {ok: true, json: async () => ({artifact: {network_allowlist: [BLOCKED]}})}
  ]});
  await page.clickRow('forget', BLOCKED);
  page.byId('al-add-input').value = 'https://TRACKER.example.com/';
  await page.byId('al-add-btn').click();
  assert.equal(await page.context.saveEditorChanges(), true);
  assert.deepEqual(originCalls(page.api), []);
  assert.deepEqual(Array.from(page.context.artifactDraft().allowlist), [BLOCKED]);
  assert.equal(page.context.artifactDirty(), false);
});

test("the forget lands after the PATCH, so a failed save changes nothing", async () => {
  const page = await loadEdit({
    responses: [{ ok: false, status: 400, json: async () => ({ error: "nope" }) }]
  });
  page.clickRow("forget", BLOCKED);
  await saveButton(page)();

  assert.deepEqual(originCalls(page.api), [],
    "a rejected PATCH must leave every origin decision as it was");
});

test("a failed forget is reported rather than passed off as a clean save", async () => {
  const page = await loadEdit({
    responses: [
      { ok: true, status: 200, json: async () => ({}) },              // the PATCH
      { ok: false, status: 500, json: async () => ({}) }               // the DELETE
    ]
  });
  page.clickRow("forget", BLOCKED);
  await saveButton(page)();

  assert.match(page.byId("status").textContent, /could not forget/,
    "the row is already gone from the panel; silence would show it as undecided");
});

test("an origin forgotten and then re-allowed keeps the allow", async () => {
  // Forget moves it out of `blocked`; Allow puts it into the allowlist, which
  // the PATCH writes as an allow decision. Deleting it afterwards would undo
  // the newer of the two answers.
  const page = await loadEdit();
  page.clickRow("forget", BLOCKED);
  page.context.allowlist.push(BLOCKED);
  await saveButton(page)();

  assert.deepEqual(originCalls(page.api), []);
});

// The bootstrap value goes stale the moment the viewer approves location in
// another tab, so an untouched select must not ship it: a Save that wrote it
// back would revoke a newer approval nobody asked to revoke.
test("the location select ships only when it was touched", async () => {
  const untouched = await loadEdit({ blocked: [] });
  await saveButton(untouched)();
  const [first] = untouched.api.calls.filter((c) => c.method === "PATCH");
  assert.equal("geolocation_approved" in JSON.parse(first.body), false);

  const touched = await loadEdit({ blocked: [] });
  await touched.byId("geo-select").dispatchEvent({ type: "change", target: { value: "true" } });
  await saveButton(touched)();
  const [second] = touched.api.calls.filter((c) => c.method === "PATCH");
  assert.equal(JSON.parse(second.body).geolocation_approved, true);
  assert.match(touched.byId("security-summary-text").textContent, /location: always allow/);
});

/* av-kmwj — "Forget" on the edit page's blocked-origins list.
 *
 * The other half of the ticket's revocability requirement: a "don't ask again"
 * answer must have a way out, and PATCH cannot express one (it replaces the
 * allow set and deliberately leaves block rows alone), so Save has to reach the
 * per-origin DELETE. That split between two write paths is exactly the kind of
 * thing a substring assertion on the file would not notice going wrong.
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
function loadEdit({ blocked = [BLOCKED], allowlist = [], responses = [] } = {}) {
  const api = recordingApi(responses);
  const page = loadPageScript(EDIT_JS, {
    TOKEN: "", READ_ONLY: false, ID,
    allowlist, unapproved: [], blocked,
    downloadsApproved: false, clipboardApproved: false, linksApproved: false,
    cameraApproved: false, microphoneApproved: false,
    apiFetch: api.apiFetch,
    alert: () => {}, confirm: () => true
  }, {
    // save() refuses an empty body before it reaches anything else, so the
    // fields it reads have to hold what the page would have rendered.
    "title": { value: "A tool" },
    "body": { value: "<html><body>hi</body></html>" },
    "scan-result": { style: {} }
  });

  // Stands in for one server-rendered .allowlist-row: the button the user
  // clicks, and the row element it resolves to.
  const clickRow = (action, origin) => {
    const row = page.byId("row-" + origin);
    row.dataset.origin = origin;
    const btn = page.byId("btn-" + action + "-" + origin);
    btn.dataset.action = action;
    btn.closest = () => row;
    return page.byId("blocked-rows").dispatchEvent({ type: "click", target: {
      closest: (sel) => (sel === `[data-action="${action}"]` ? btn : null)
    } });
  };

  return { ...page, api, clickRow };
}

const originCalls = (api) => api.calls.filter((c) => c.path.includes("/origins"));
const saveButton = (page) => page.context.save;

test("Forget deletes the decision through the per-origin route", async () => {
  const page = loadEdit();
  page.clickRow("forget", BLOCKED);
  await saveButton(page)();

  const [call] = originCalls(page.api);
  assert.ok(call, "Save must apply the forget; PATCH cannot express it");
  assert.equal(call.method, "DELETE");
  assert.equal(call.path,
    `/api/artifacts/${ID}/origins?origin=${encodeURIComponent(BLOCKED)}`);
});

test("the forget lands after the PATCH, so a failed save changes nothing", async () => {
  const page = loadEdit({
    responses: [{ ok: false, status: 400, json: async () => ({ error: "nope" }) }]
  });
  page.clickRow("forget", BLOCKED);
  await saveButton(page)();

  assert.deepEqual(originCalls(page.api), [],
    "a rejected PATCH must leave every origin decision as it was");
});

test("a failed forget is reported rather than passed off as a clean save", async () => {
  const page = loadEdit({
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
  const page = loadEdit();
  page.clickRow("forget", BLOCKED);
  page.context.allowlist.push(BLOCKED);
  await saveButton(page)();

  assert.deepEqual(originCalls(page.api), []);
});

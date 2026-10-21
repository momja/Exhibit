/* Drives the built tags.js through the edit page's Tags panel interactions. */
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript, recordingApi } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const TAGS_JS = path.join(here, "../../internal/api/assets/gallery/tags.js");

const ID = "art-1";

async function loadTags({ responses = [], initial = {}, confirmed = true } = {}) {
  const api = recordingApi(responses);
  const events = [];
  let reloads = 0;
  const page = loadPageScript([], {
    ID, TOKEN: "", READ_ONLY: false,
    apiFetch: api.apiFetch,
    confirm: () => confirmed,
    CustomEvent
  }, {
    "tags-panel": { dataset: { artifactId: ID } },
    "tag-create-error": { hidden: true },
    "tag-edit-modal": { hidden: true },
    "tag-add-error": { hidden: true },
    "tag-edit-error": { hidden: true },
    ...initial
  });
  page.window.apiFetch = api.apiFetch;
  await page.loadModule(TAGS_JS);
  page.window.location.reload = () => { reloads++; };
  page.document.body.addEventListener("exhibit:tags-changed", (e) => events.push(e.type));

  // Simulates a click on a control inside the delegated panel.
  const clickControl = (control, matches) => page.byId("tags-panel").dispatchEvent({
    type: "click",
    target: { id: "", closest: (sel) => (matches(sel) ? control : null) }
  });
  const clickAction = (action, dataset) =>
    clickControl({ dataset: { action, ...dataset } }, (sel) => sel === "[data-action]");
  const clickById = (id) => clickControl({ id }, (sel) => sel === "#" + id);

  return { ...page, api, events, clickAction, clickById, reloads: () => reloads };
}

test("remove detaches the tag from this artifact and re-renders the panel", async () => {
  const t = await loadTags();
  await t.clickAction("detach-tag", { tagId: "tag-1" });
  assert.deepEqual(t.api.calls.map((c) => [c.method, c.path]), [["DELETE", "/api/tags/tag-1/artifacts/art-1"]]);
  assert.deepEqual(t.events, ["exhibit:tags-changed"]);
  assert.equal(t.reloads(), 0, "a reload would drop the page's unsaved editors");
});

test("adding an existing tag attaches it, and nothing is created", async () => {
  const t = await loadTags({ initial: {
    "tag-add-select": { value: "tag-2" }, "tag-add-name": { value: "Unsubmitted new tag" }
  } });
  await t.clickById("tag-add-confirm");
  assert.deepEqual(t.api.calls.map((c) => [c.method, c.path]), [["POST", "/api/tags/tag-2/artifacts/art-1"]]);
  assert.deepEqual(t.events, ["exhibit:tags-changed"]);
});

test("create new makes the tag with its name and color, then attaches it", async () => {
  const t = await loadTags({
    initial: {
      "tag-add-select": { value: "" },
      "tag-add-name": { value: "  charts " },
      "tag-add-color-hex": { value: "#ff0000" }
    },
    responses: [{ ok: true, status: 201, json: async () => ({ id: "new-tag" }) }]
  });
  await t.clickById("tag-create-confirm");
  assert.deepEqual(t.api.calls.map((c) => [c.method, c.path]), [
    ["POST", "/api/tags"],
    ["POST", "/api/tags/new-tag/artifacts/art-1"]
  ]);
  assert.deepEqual(JSON.parse(t.api.calls[0].body), { name: "charts", color: "#ff0000" });
  assert.deepEqual(t.events, ["exhibit:tags-changed"]);
});

test("add with nothing chosen sends nothing and says why", async () => {
  const t = await loadTags({ initial: { "tag-add-select": { value: "" } } });
  await t.clickById("tag-add-confirm");
  assert.equal(t.api.calls.length, 0);
  assert.equal(t.byId("tag-add-error").hidden, false);
  assert.deepEqual(t.events, []);
});

test("a refused create is reported and never attaches", async () => {
  const t = await loadTags({
    initial: { "tag-add-select": { value: "" }, "tag-add-name": { value: "dup" }, "tag-add-color-hex": { value: "" } },
    responses: [{ ok: false, status: 409, json: async () => ({ error: "tag exists" }) }]
  });
  await t.clickById("tag-create-confirm");
  assert.equal(t.api.calls.length, 1);
  assert.equal(t.byId("tag-create-error").textContent, "tag exists");
  assert.equal(t.byId("tag-create-confirm").disabled, false);
});

test("the edit dialog renames the tag library-wide and re-renders the panel", async () => {
  const t = await loadTags();
  await t.clickAction("edit-tag", { tagId: "tag-1", tagName: "charts", tagColor: "#ffffff" });
  const modal = t.byId("tag-edit-modal");
  assert.equal(modal.hidden, false);
  assert.equal(t.byId("tag-edit-name").value, "charts");

  t.byId("tag-edit-name").value = "graphs";
  await modal.dispatchEvent({ type: "click", target: { closest: (sel) => (sel === "#tag-edit-save" ? {} : null) } });
  assert.deepEqual(t.api.calls.map((c) => [c.method, c.path]), [["PATCH", "/api/tags/tag-1"]]);
  assert.deepEqual(JSON.parse(t.api.calls[0].body), { name: "graphs", color: "#ffffff" });
  assert.equal(modal.hidden, true);
  assert.deepEqual(t.events, ["exhibit:tags-changed"]);
});

test("deleting a tag asks first, and a declined confirm sends nothing", async () => {
  const t = await loadTags({ confirmed: false });
  await t.clickAction("edit-tag", { tagId: "tag-1", tagName: "charts", tagColor: "#ffffff" });
  const modal = t.byId("tag-edit-modal");
  await modal.dispatchEvent({ type: "click", target: { closest: (sel) => (sel === "#tag-edit-delete" ? {} : null) } });
  assert.equal(t.api.calls.length, 0);
  assert.equal(modal.hidden, false);
});

test("creating validates its own name even when an existing tag is selected", async () => {
  const t = await loadTags({ initial: {
    "tag-add-select": { value: "tag-2" }, "tag-add-name": { value: "  " }
  } });
  await t.clickById("tag-create-confirm");
  assert.equal(t.api.calls.length, 0);
  assert.equal(t.byId("tag-create-error").textContent, "Name is required.");
  assert.equal(t.byId("tag-add-error").hidden, true);
});

test("Enter in the new tag name creates without consulting the dropdown", async () => {
  const t = await loadTags({
    initial: { "tag-add-select": { value: "tag-2" }, "tag-add-name": { value: "new" } },
    responses: [{ ok: true, json: async () => ({ id: "new-tag" }) }]
  });
  let prevented = false;
  await t.byId("tags-panel").dispatchEvent({ type: "keydown", key: "Enter",
    target: { id: "tag-add-name" }, preventDefault() { prevented = true; } });
  assert.equal(prevented, true);
  assert.deepEqual(t.api.calls.map(c => c.path), ["/api/tags", "/api/tags/new-tag/artifacts/art-1"]);
});

test("attachment errors after creation survive the panel refresh", async () => {
  const t = await loadTags({
    initial: { "tag-add-name": { value: "new" } },
    responses: [
      { ok: true, json: async () => ({ id: "new-tag" }) },
      { ok: false, json: async () => ({ error: "Could not attach" }) }
    ]
  });
  await t.clickById("tag-create-confirm");
  assert.equal(t.byId("tag-add-error").textContent, "Could not attach");
  assert.deepEqual(t.events, ["exhibit:tags-changed"]);
});

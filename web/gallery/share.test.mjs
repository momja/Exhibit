/* av-6xjd — the owner's share panel, driven the way a browser drives it.
 *
 * The Go tests can assert that an id is in the rendered markup and that the
 * server answers a request correctly. They cannot assert that clicking the
 * public-link toggle sends a DELETE rather than a POST, that a bulk submit is
 * one request rather than three, or that "Replace link" is a single call and
 * not a delete followed by a create — and each of those passes a markup
 * assertion while being wrong. That gap is what let a whole feature reach a
 * merged PR without reaching main (see gallery_script_test.go).
 *
 * Loads internal/api/assets/gallery/share.js — the built, embedded copy the
 * browser is served — so an edit under web/gallery that was never rebuilt
 * fails here rather than shipping.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript, recordingApi } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const SHARE_JS = path.join(here, "../../internal/api/assets/gallery/share.js");

const ID = "art-1";
const LINK_ID = "share-link-row-id";

// loadShare renders the panel for its owner. `link` is the share id the
// template puts on the toggle when the artifact already has a public link —
// its absence is what "the link is off" looks like in the markup. `embed`
// is the widget snippet the template renders when the artifact has both a
// link and a widget (av-ei5h); empty is the no-widget panel.
function loadShare({ link = "", embed = "", responses = [], confirmed = true, grantCount = 0 } = {}) {
  const api = recordingApi(responses);
  const events = [];
  const copied = [];
  const page = loadPageScript([SHARE_JS], {
    ID,
    TOKEN: "",
    READ_ONLY: false,
    apiFetch: api.apiFetch,
    confirm: () => confirmed,
    CustomEvent,
    navigator: { clipboard: { writeText: (t) => { copied.push(t); return Promise.resolve(); } } }
  }, {
    // The template renders the overlay hidden (av-esa1). Seeding it here is
    // what the harness's `initial` is for: without it every test would start
    // with the modal already open, and "it opens when clicked" would be a
    // claim the harness had made true before the script ran.
    "share-panel": { hidden: true },
    "share-add-input": { value: "" },
    "share-link-toggle": { dataset: link ? { shareId: link } : {} },
    "share-link-url": { value: "http://render.test/s/" + link, select() {} },
    "share-widget-embed": { value: embed, select() {} },
    // data-grant-count is the template's render of len(.Shares.Grants) —
    // what setStateMode reads to decide whether switching away from 'shared'
    // needs a confirm (av-tlpv).
    "share-state-mode": { dataset: { grantCount: String(grantCount) } }
  });

  // The panel fires exhibit:shares-changed and htmx re-fetches the fragment.
  // There is no htmx here, so the event is what is asserted: it is the whole
  // contract between this script and the server-rendered list.
  page.document.body.addEventListener("exhibit:shares-changed", (e) => events.push(e.type));

  const panel = page.byId("share-panel");
  const click = (id) => panel.dispatchEvent({ type: "click", target: { id } });
  const change = (id, props) =>
    panel.dispatchEvent({ type: "change", target: Object.assign({ id }, props) });
  const clickRevoke = (shareID) => {
    const btn = page.byId("revoke-" + shareID);
    btn.dataset.shareId = shareID;
    return panel.dispatchEvent({
      type: "click",
      target: { closest: (sel) => (sel === '[data-action="revoke"]' ? btn : null) }
    });
  };

  return { ...page, api, events, copied, click, change, clickRevoke };
}

const paths = (api) => api.calls.map((c) => c.method + " " + c.path);

// --- granting -----------------------------------------------------------

test("several names go out as ONE request, and every one gets a result", async () => {
  const page = loadShare({
    responses: [{
      ok: true, status: 200, json: async () => ({
        results: [
          { name: "alice", status: "granted", share_id: "s1" },
          { name: "bob", status: "already_shared" },
          { name: "tpyo", status: "unknown" }
        ]
      })
    }]
  });
  page.byId("share-add-input").value = "alice, bob\ntpyo";

  await page.click("share-add-btn");

  assert.deepEqual(paths(page.api), ["POST /api/shares"],
    "'gave friends access, dropped the link' is one gesture; three requests would " +
    "make one bad name fail some of them and not others");
  assert.deepEqual(JSON.parse(page.api.calls[0].body),
    { artifact_id: ID, recipients: ["alice", "bob", "tpyo"] },
    "commas and newlines both separate — a list pasted out of a chat does not " +
    "arrive in the one format the field asked for");

  const rows = page.byId("share-add-results").children;
  assert.equal(rows.length, 3, "per name, not per request");
  assert.equal(rows[0].children[0].textContent, "alice");
  assert.match(rows[2].children[1].textContent, /no account with that name/,
    "a name nobody holds must be named back: 'added' for a typo leaves the owner " +
    "believing their friend has access when the friend has none");
  assert.deepEqual(page.events, ["exhibit:shares-changed"]);
});

test("an empty field sends nothing", async () => {
  const page = loadShare();
  page.byId("share-add-input").value = "   ";

  await page.click("share-add-btn");

  assert.deepEqual(page.api.calls, []);
});

test("Enter in the field submits it, since typing names and reaching for a button is two gestures", async () => {
  const page = loadShare();
  page.byId("share-add-input").value = "alice";

  await page.byId("share-panel").dispatchEvent({
    type: "keydown", key: "Enter", target: { id: "share-add-input" }, preventDefault() {}
  });

  assert.deepEqual(paths(page.api), ["POST /api/shares"]);
});

test("a revoke names the share row, not the person", async () => {
  const page = loadShare();

  await page.clickRevoke("grant-7");

  assert.deepEqual(paths(page.api), ["DELETE /api/shares/grant-7"]);
  assert.deepEqual(page.events, ["exhibit:shares-changed"]);
});

// --- the public link ----------------------------------------------------

test("the toggle mints on and deletes off — the same control, both directions", async () => {
  const on = loadShare();
  await on.change("share-link-toggle", { checked: true });
  assert.deepEqual(paths(on.api), ["POST /api/shares"]);
  assert.deepEqual(JSON.parse(on.api.calls[0].body), { artifact_id: ID },
    "minting is not a rotation, and it does not ask for one — replace on an " +
    "artifact that has no link would still work, but sending it here would mean " +
    "the toggle and the rotate button were the same request");

  const off = loadShare({ link: LINK_ID });
  await off.change("share-link-toggle", { checked: false });
  assert.deepEqual(paths(off.api), ["DELETE /api/shares/" + LINK_ID],
    "off deletes the row and the URL dies; nothing accumulates a list of links " +
    "whose destinations nobody remembers");
});

test("Replace is one request, so the artifact is never briefly linkless", async () => {
  const page = loadShare({ link: LINK_ID });

  await page.click("share-link-replace");

  assert.deepEqual(paths(page.api), ["POST /api/shares"],
    "a delete followed by a create leaves no link at all when the second half fails");
  assert.deepEqual(JSON.parse(page.api.calls[0].body), { artifact_id: ID, replace: true });
  assert.match(page.byId("share-status").textContent, /old one no longer works/,
    "the whole reason this is a button and not two toggles is that the user must " +
    "know the previous URL stopped working");
});

test("Replace asks first, because the old link stops working immediately", async () => {
  const page = loadShare({ link: LINK_ID, confirmed: false });

  await page.click("share-link-replace");

  assert.deepEqual(page.api.calls, []);
  assert.deepEqual(page.events, []);
});

test("Copy puts the link on the clipboard from the app origin", async () => {
  const page = loadShare({ link: LINK_ID });

  await page.click("share-link-copy");

  assert.deepEqual(page.copied, ["http://render.test/s/" + LINK_ID]);
  assert.deepEqual(page.api.calls, [], "copying decides nothing, so it writes nothing");
});

test("Copy puts the widget embed snippet on the clipboard", async () => {
  const snippet = '<iframe src="http://render.test/s/' + LINK_ID + '/widget" width="320" height="132" style="border:0"></iframe>';
  const page = loadShare({ link: LINK_ID, embed: snippet });

  await page.click("share-widget-copy");

  assert.deepEqual(page.copied, [snippet]);
  assert.deepEqual(page.api.calls, [], "copying decides nothing, so it writes nothing");
});

// --- whose data ---------------------------------------------------------

test("the state mode applies immediately, through the artifact PATCH", async () => {
  const page = loadShare();

  await page.change("share-state-mode", { value: "shared" });

  assert.deepEqual(paths(page.api), ["PATCH /api/artifacts/" + ID]);
  assert.deepEqual(JSON.parse(page.api.calls[0].body), { share_state_mode: "shared" });
  assert.deepEqual(page.events, ["exhibit:shares-changed"],
    "the re-render is what confirms the stored value; a select left showing the " +
    "click rather than the answer would lie about a rejected write");
});

// av-tlpv — switching away from 'shared' resolves every grantee off the
// owner's rows and onto their own (StatePrincipal, internal/store/store.go).
// Nothing is deleted, but a grantee's own row was never written, so their
// next visit looks like the artifact forgot everything they did there.

test("switching to 'own' with grantees asks first, naming how many people it affects", async () => {
  const page = loadShare({ grantCount: 3 });

  await page.change("share-state-mode", { value: "own" });

  assert.deepEqual(paths(page.api), ["PATCH /api/artifacts/" + ID]);
  assert.deepEqual(JSON.parse(page.api.calls[0].body), { share_state_mode: "own" });
});

test("cancelling that confirm sends nothing, and still re-renders the select from the server", async () => {
  const page = loadShare({ grantCount: 3, confirmed: false });

  await page.change("share-state-mode", { value: "own" });

  assert.deepEqual(page.api.calls, [],
    "a cancelled switch must not reach the API — the mode the server holds is unchanged");
  assert.deepEqual(page.events, ["exhibit:shares-changed"],
    "the select shows whatever the click implied until something re-renders it from " +
    "the stored value; a cancel has no PATCH response to do that, so it asks for the " +
    "same re-render a successful save gets");
});

test("switching to 'own' with nobody else on the artifact asks nothing", async () => {
  const page = loadShare({ grantCount: 0 });

  await page.change("share-state-mode", { value: "own" });

  assert.deepEqual(paths(page.api), ["PATCH /api/artifacts/" + ID],
    "there is nobody whose data the switch could orphan");
});

test("switching to 'shared' never asks, whatever the confirm would answer", async () => {
  const page = loadShare({ grantCount: 3, confirmed: false });

  await page.change("share-state-mode", { value: "shared" });

  assert.deepEqual(paths(page.api), ["PATCH /api/artifacts/" + ID],
    "only 'own' can orphan anyone; 'shared' just adds everyone to the owner's board");
});

// --- failure ------------------------------------------------------------

test("a refused write is reported, not passed off as done", async () => {
  const page = loadShare({
    link: LINK_ID,
    responses: [{ ok: false, status: 403, json: async () => ({}) }]
  });

  await page.change("share-link-toggle", { checked: false });

  assert.match(page.byId("share-status").textContent, /Could not remove the link/);
  assert.deepEqual(page.events, ["exhibit:shares-changed"],
    "the panel still re-renders on a failure, so what it shows is the server's " +
    "state rather than the one the click implied");
});

/* av-esa1 — the panel is a modal now, so how it opens and closes is behaviour
 * rather than markup. The Go template test can see that #share-open is in the
 * page; only running the script proves the button is wired to the overlay, and
 * that Escape and the backdrop reach it too.
 *
 * The backdrop case is the one worth having. It is the difference between
 * `e.target === panel` and a check that also matches clicks inside the dialog,
 * and getting it wrong throws away whatever the owner had half-typed in the
 * username field the moment they click a label. */

test("the panel starts closed and the toolbar button opens it", () => {
  const page = loadShare();
  const panel = page.byId("share-panel");
  assert.equal(panel.hidden, true, "the modal must hold no layout until asked for");

  page.byId("share-open").dispatchEvent({ type: "click", target: { id: "share-open" } });
  assert.equal(panel.hidden, false);
});

test("Done, the backdrop, and Escape each close it — a click inside does not", () => {
  const page = loadShare();
  const panel = page.byId("share-panel");
  const open = () =>
    page.byId("share-open").dispatchEvent({ type: "click", target: { id: "share-open" } });

  open();
  page.byId("share-close").dispatchEvent({ type: "click", target: { id: "share-close" } });
  assert.equal(panel.hidden, true, "the Done button closes it");

  open();
  panel.dispatchEvent({ type: "click", target: panel });
  assert.equal(panel.hidden, true, "a click on the backdrop closes it");

  open();
  page.pressKey("Escape");
  assert.equal(panel.hidden, true, "Escape closes it");

  open();
  panel.dispatchEvent({ type: "click", target: { id: "share-add-submit" } });
  assert.equal(panel.hidden, false, "a click inside the dialog must not close it");
});

test("Escape does nothing while the panel is already closed", () => {
  const page = loadShare();
  page.pressKey("Escape");
  assert.equal(page.byId("share-panel").hidden, true);
});

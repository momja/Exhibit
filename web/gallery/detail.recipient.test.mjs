/* av-awr4 — the recipient's read-only viewer, driven as a browser drives it.
 *
 * The acceptance criterion this file exists for is AC#6: nothing in a
 * recipient's session issues POST /origins or PATCH /api/artifacts/:id. That
 * cannot be established by reading markup, and reading markup is exactly how
 * the same claim would be got wrong — every dialog this page can raise is still
 * in the document for a recipient (one definition apiece, resolved by id from
 * five different bridges), and what changes is only whether anything opens one.
 * So the test loads the shipped detail.js and plays the artifact's side of the
 * postMessage boundary: a blocked origin, a download, a clipboard call, an
 * external link, a getUserMedia. Every recorded API call is then inspected.
 *
 * The other half is AC#4, and it is the reason the ticket exists rather than a
 * detail of it. A shared artifact is frozen at whatever its owner approved. A
 * recipient cannot widen that and should not be asked to — but if nothing says
 * so, the tool simply does nothing and the visitor learns that Exhibit is
 * flaky. The explanation is the feature, so it is asserted as one.
 *
 * Two layers are exercised, because the claim is held in two places and either
 * alone would be a weaker statement. detail.js decides that no prompt opens;
 * api.js refuses the write regardless of what opened. The last test drives the
 * shipped api.js directly, which is where the second half lives.
 *
 * Like detail.net.test.mjs this loads internal/api/assets/gallery/*.js — the
 * built, embedded copies the browser is served — so an edit under web/gallery
 * that was never rebuilt fails here rather than shipping.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript, recordingApi } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const ASSETS = path.join(here, "../../internal/api/assets/gallery");
const DETAIL_JS = [
  path.join(ASSETS, "network-prompt.js"),
  path.join(ASSETS, "state-api.js"),
  path.join(ASSETS, "detail.js")
];
const API_JS = path.join(ASSETS, "api.js");

const ID = "art-1";
const OPEN_URL = "/artifacts/art-1/open";

// What detail.tmpl renders: every dialog and banner starts hidden. Without it
// "the prompt did not open" would be a claim the harness itself made false.
const HIDDEN = {
  "net-modal": { hidden: true },
  "dl-modal": { hidden: true },
  "clip-modal": { hidden: true },
  "link-modal": { hidden: true },
  "media-modal": { hidden: true },
  "capability-warning-banner": { hidden: true },
  "origin-blocked-banner": { hidden: true }
};

// loadDetail renders the page for one visitor. readOnly is the whole
// difference between an owner and a recipient — the server narrows it per
// artifact (pageCredentials.forArtifactOwnedBy), and everything below turns on
// it. approvals are the OWNER's capability grants, which travel with the
// artifact and are not the visitor's to make.
function loadDetail({ readOnly, approvals = {} } = {}) {
  const api = recordingApi();
  const opened = [];
  const page = loadPageScript(DETAIL_JS, {
    TOKEN: "",
    READ_ONLY: readOnly,
    ID,
    SOURCE_URL: "",
    OPEN_URL,
    downloadsApproved: false,
    clipboardApproved: false,
    linksApproved: false,
    cameraApproved: false,
    microphoneApproved: false,
    ...approvals,
    apiFetch: api.apiFetch,
    open: (url) => { opened.push(url); return null; }
  }, HIDDEN);
  return { ...page, api, opened };
}

const loadRecipient = (approvals) => loadDetail({ readOnly: true, approvals });
// Every "a recipient is not asked" assertion is paired with the owner's page,
// because a gate that refused everybody would satisfy the whole file and take
// the feature with it (AC#7).
const loadOwner = () => loadDetail({ readOnly: false });

const blocked = (origin, directive = "connect-src") => ({
  __avNetwork: true, artifactId: ID, origin, directive
});
const download = () => ({
  __avDownload: true, artifactId: ID, filename: "export.csv",
  mime: "text/csv", bytes: new ArrayBuffer(4)
});
const clipboardRead = () => ({ __avClipboard: true, artifactId: ID, id: "c1", op: "read" });
const externalLink = () => ({ __avNavigate: true, artifactId: ID, url: "https://example.com/docs" });
const cameraRequest = () => ({ __avMedia: true, artifactId: ID, id: "m1", video: true, audio: false });

// --- AC#4: explain, do not ask ----------------------------------------

test("a blocked origin is explained by name, and never prompted for", async () => {
  const { byId, api, postFromFrame } = loadRecipient();

  await postFromFrame(blocked("https://api.example.com"));

  assert.equal(byId("net-modal").hidden, true,
    "the runtime prompt writes an origin decision, which is the owner's to make");
  assert.equal(byId("origin-blocked-banner").hidden, false,
    "a silent block is the failure this ticket exists to prevent");
  assert.equal(
    byId("origin-blocked-headline").textContent,
    "This tool tried to reach https://api.example.com. Its owner has not allowed that."
  );
  assert.deepEqual(api.calls, [], "explaining decides nothing, so it writes nothing");
});

test("the explanation names every blocked origin, not just the first", async () => {
  const { byId, postFromFrame } = loadRecipient();

  await postFromFrame(blocked("https://api.example.com"));
  await postFromFrame(blocked("https://cdn.example.com", "script-src-elem"));

  assert.equal(
    byId("origin-blocked-headline").textContent,
    "This tool tried to reach 2 origins its owner has not allowed.",
    "a tool reaching several origins has several things wrong with it, and naming " +
    "one leaves the visitor reporting a list they cannot see the end of"
  );
  const listed = byId("origin-blocked-list").children.map((row) => row.children[0].textContent);
  assert.deepEqual(listed, ["https://api.example.com", "https://cdn.example.com"]);
});

test("the same origin reported twice is listed once", async () => {
  const { byId, postFromFrame } = loadRecipient();

  await postFromFrame(blocked("https://api.example.com"));
  await postFromFrame(blocked("https://api.example.com", "img-src"));

  assert.equal(byId("origin-blocked-list").children.length, 1);
  assert.match(byId("origin-blocked-headline").textContent, /reach https:\/\/api\.example\.com\./);
});

test("the origin reaches the banner as text, never as markup", async () => {
  const { byId, postFromFrame } = loadRecipient();

  const hostile = 'https://evil.example.com/"><img src=x onerror=alert(1)>';
  await postFromFrame(blocked(hostile));

  assert.equal(byId("origin-blocked-list").children[0].children[0].textContent, hostile,
    "the value comes out of the artifact's own blocked request; it is content, not markup");
});

test("the owner is still asked, so the gate is about ownership and not about the feature", async () => {
  const { byId, postFromFrame } = loadOwner();

  await postFromFrame(blocked("https://api.example.com"));

  assert.equal(byId("net-modal").hidden, false, "the owner can widen their own allowlist");
  assert.equal(byId("net-origin").textContent, "https://api.example.com");
  assert.equal(byId("origin-blocked-banner").hidden, true,
    "and is not also told they cannot, which would be a page contradicting itself");
});

// --- AC#5: capability attempts fail the way they always have ----------

test("an unapproved download is dropped without a prompt", async () => {
  const { byId, api, postFromFrame } = loadRecipient();

  await postFromFrame(download());

  assert.equal(byId("dl-modal").hidden, true);
  assert.deepEqual(api.calls, []);
});

test("an unapproved clipboard call is rejected into the frame rather than left hanging", async () => {
  const { byId, api, frame, postFromFrame } = loadRecipient();

  await postFromFrame(clipboardRead());

  assert.equal(byId("clip-modal").hidden, true);
  const [reply] = frame.contentWindow.posted.filter((m) => m.__avClipboardResult);
  assert.ok(reply, "an unanswered clipboard promise is a hang, which is worse than a denial");
  assert.equal(reply.ok, false);
  assert.equal(reply.id, "c1");
  assert.deepEqual(api.calls, []);
});

test("an unapproved external link opens nothing and prompts for nothing", async () => {
  const { byId, api, opened, postFromFrame } = loadRecipient();

  await postFromFrame(externalLink());

  assert.equal(byId("link-modal").hidden, true);
  assert.deepEqual(opened, []);
  assert.deepEqual(api.calls, []);
});

test("an unapproved camera request is denied, not offered", async () => {
  const { byId, api, frame, postFromFrame } = loadRecipient();

  await postFromFrame(cameraRequest());

  assert.equal(byId("media-modal").hidden, true,
    "Allow would open a tab on an approval this visitor cannot record");
  const [reply] = frame.contentWindow.posted.filter((m) => m.__avMediaResult);
  assert.ok(reply, "getUserMedia must settle; a pending promise looks like a hang");
  assert.equal(reply.ok, false);
  assert.equal(reply.name, "NotAllowedError");
  assert.deepEqual(api.calls, []);
});

// The gate is about *granting*, never about using. An approval the owner
// already made belongs to the artifact, and the bridge spends it for whoever
// is looking — otherwise a shared tool would lose the capability its owner
// deliberately turned on, which is a second silent breakage rather than a fix.
test("an approval the owner already made still works for the recipient", async () => {
  const { api, opened, postFromFrame } = loadRecipient({ linksApproved: true });

  await postFromFrame(externalLink());

  assert.deepEqual(opened, ["https://example.com/docs"],
    "the owner allowed links on this artifact; that decision travels with it");
  assert.deepEqual(api.calls, [], "and spending an existing approval writes nothing");
});

// --- AC#6: the blanket claim, over one whole session ------------------

test("a recipient's session issues no API call at all across every bridge", async () => {
  const { byId, api, postFromFrame } = loadRecipient();

  await postFromFrame(blocked("https://api.example.com"));
  await postFromFrame(blocked("https://cdn.example.com", "script-src-elem"));
  await postFromFrame(download());
  await postFromFrame(clipboardRead());
  await postFromFrame(externalLink());
  await postFromFrame(cameraRequest());

  // The state bridge is deliberately absent from this battery: a recipient's
  // storage write does reach apiFetch, and it is api.js that refuses it (last
  // test). That is a real gap rather than a tidy one — see the state-bridge
  // comment in detail.js — and hiding it inside this assertion would make the
  // file claim more than it has.
  assert.deepEqual(api.calls, [],
    "every route these bridges write to is owner-scoped and would answer 404; " +
    "sending them is how a recipient learns the tool is broken instead of that " +
    "it is not theirs");
  for (const id of ["net-modal", "dl-modal", "clip-modal", "link-modal", "media-modal"]) {
    assert.equal(byId(id).hidden, true, id + " must not open for a visitor who cannot answer it");
  }
});

// Non-vacuity for the battery above: the same messages on the owner's page do
// open dialogs, and answering one does write. Without this a detail.js that
// stopped wiring the bridges at all would pass the whole file.
test("the owner's session prompts and writes", async () => {
  const { byId, api, postFromFrame } = loadOwner();

  await postFromFrame(blocked("https://api.example.com"));
  await byId("net-allow").click();
  await postFromFrame(download());
  assert.equal(byId("dl-modal").hidden, false, "the owner is asked before the first download");
  await byId("dl-allow").click();

  const paths = api.calls.map((c) => c.method + " " + c.path);
  assert.ok(paths.includes(`POST /api/artifacts/${ID}/origins`), paths.join(", "));
  assert.ok(paths.includes(`PATCH /api/artifacts/${ID}`), paths.join(", "));
});

// The second layer, and the one that holds whatever a page script does. api.js
// is where READ_ONLY is spent, so a write from a recipient's page never leaves
// the browser — including the state write-through, which is the one the
// bridges above do still attempt. It is refused rather than 404'd: the state
// routes are owner-scoped in SQL and av-lrae pins that a grant does not widen
// them, so until av-v991 decides whose board a shared artifact is, a recipient
// reads their inlined rows and writes none.
test("api.js refuses a read-only visitor's writes before the network sees them", async () => {
  const sent = [];
  const { window } = loadPageScript([API_JS], {
    TOKEN: "",
    READ_ONLY: true,
    fetch: (path, opts) => { sent.push({ path, opts }); return Promise.resolve({ ok: true }); }
  });

  const refused = await window.apiFetch("/api/artifacts/" + ID + "/state", {
    method: "PUT", body: JSON.stringify({ key: "k", value: "v" })
  });
  assert.equal(refused.ok, false);
  assert.equal(refused.status, 403);

  const patched = await window.apiFetch("/api/artifacts/" + ID, {
    method: "PATCH", body: JSON.stringify({ downloads_approved: true })
  });
  assert.equal(patched.status, 403);

  assert.deepEqual(sent, [], "a refused write must not reach the network at all");

  // Reads still go, or the page could not render anything it fetches.
  await window.apiFetch("/api/artifacts/" + ID);
  assert.equal(sent.length, 1);
});

/* av-v991 — state liveness, driven the way a browser drives it.
 *
 * Two halves of one channel meet on this page and neither can be established by
 * reading a file. detail.js decides WHEN to ask the server and what to do with
 * the answer; the render preamble decides what an incoming map does to the
 * shim's cache. So this file plays both sides: it loads the shipped detail.js
 * over the test DOM, and it runs the shipped preamble's applyStateSync by
 * extracting it from the render document Go emits — because "the diff fires a
 * storage event per changed key" is a claim about that code, not about a
 * substring in it.
 *
 * The honest ceiling is asserted too, in the last test. Liveness shrinks the
 * window in which two writes collide; it does not merge them, and a file that
 * only demonstrated the happy path would read as if it did.
 *
 * Like its siblings this loads internal/api/assets/gallery/*.js — the built,
 * embedded copies the browser is served — so an edit under web/gallery that was
 * never rebuilt fails here rather than shipping.
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

const ID = "art-1";
const OPEN_URL = "/artifacts/art-1/open";
const STATE_URL = "/api/artifacts/art-1/state";

const HIDDEN = {
  "net-modal": { hidden: true },
  "dl-modal": { hidden: true },
  "clip-modal": { hidden: true },
  "link-modal": { hidden: true },
  "media-modal": { hidden: true },
  "capability-warning-banner": { hidden: true },
  "origin-blocked-banner": { hidden: true },
  "state-changed-banner": { hidden: true }
};

// stateResponse answers one GET /state with a map, then falls back to the
// recorder's default. Queued rather than fixed, so a test can show the server
// changing between two polls.
const stateResponse = (state) => ({ ok: true, status: 200, json: async () => state });

function loadDetail({ readOnly = false, stateWritable = true, shared = false, responses = [] } = {}) {
  const api = recordingApi(responses);
  const page = loadPageScript(DETAIL_JS, {
    TOKEN: "",
    READ_ONLY: readOnly,
    STATE_WRITABLE: stateWritable,
    SHARED_STATE: shared,
    ID,
    SOURCE_URL: "",
    OPEN_URL,
    downloadsApproved: false,
    clipboardApproved: false,
    linksApproved: false,
    cameraApproved: false,
    microphoneApproved: false,
    apiFetch: api.apiFetch,
    apiStateFetch: api.apiStateFetch
  }, HIDDEN);
  return { ...page, api };
}

const stateWrite = (key, value) => ({ __avState: true, artifactId: ID, op: "set", key, value });
const synced = (changed, live) => ({ __avStateSynced: true, artifactId: ID, changed, live });

// --- the write half, which is what av-awr4 found missing -----------------

test("a state write goes out through the state path, not the ordinary one", async () => {
  const { api, postFromFrame } = loadDetail({ readOnly: true });

  await postFromFrame(stateWrite("todo", '["milk"]'));

  assert.equal(api.calls.length, 1, "a recipient's saved data is theirs to write");
  assert.equal(api.calls[0].method, "PUT");
  assert.equal(api.calls[0].path, STATE_URL);
  assert.equal(api.calls[0].via, "apiStateFetch",
    "through apiStateFetch, the one exception READ_ONLY carves out — apiFetch would refuse it");
});

// --- when the host asks --------------------------------------------------

test("coming back to the tab resyncs, for every artifact", async () => {
  const { api, setVisibility } = loadDetail({ shared: false, responses: [stateResponse({})] });

  await setVisibility("hidden");
  assert.deepEqual(api.calls, [], "a tab nobody is looking at asks nothing");

  await setVisibility("visible");
  assert.deepEqual(api.calls.map((c) => c.method + " " + c.path), ["GET " + STATE_URL],
    "the cross-device case: the phone wrote while the laptop was asleep, and this " +
    "is the moment the person looks at the laptop again");
});

test("only a shared artifact polls", async () => {
  const solo = loadDetail({ shared: false, responses: [stateResponse({})] });
  await solo.tickIntervals();
  assert.deepEqual(solo.api.calls, [],
    "nobody else writes your own rows, so a poll would spend a request every few " +
    "seconds forever to discover nothing");

  const board = loadDetail({ shared: true, responses: [stateResponse({ position: "e4" })] });
  await board.tickIntervals();
  assert.deepEqual(board.api.calls.map((c) => c.method + " " + c.path), ["GET " + STATE_URL]);
});

test("a visitor with no principal never polls and never asks", async () => {
  const { api, tickIntervals, setVisibility } = loadDetail({
    readOnly: true, stateWritable: false, shared: true, responses: [stateResponse({})]
  });

  await tickIntervals();
  await setVisibility("visible");

  assert.deepEqual(api.calls, [],
    "an anonymous viewer's frame was rendered with no state inlined; polling would " +
    "be a 401 every few seconds");
});

test("a resync in flight with a local write waits", async () => {
  // The write's response is queued first, so the PUT is still unresolved when
  // the interval fires: a refetch landing there would push the pre-write server
  // copy back into the frame and visibly undo what the user just typed.
  const { api, frame, postFromFrame, tickIntervals } = loadDetail({
    shared: true,
    responses: [new Promise(() => {}), stateResponse({ position: "e4" })]
  });

  postFromFrame(stateWrite("position", "e5"));
  await tickIntervals();

  assert.equal(api.calls.length, 1, "the PUT went; the poll stood down");
  assert.equal(api.calls[0].method, "PUT");
  assert.deepEqual(frame.contentWindow.posted.filter((m) => m.__avStateSync), []);
});

test("the fetched map is posted into the frame", async () => {
  const { frame, tickIntervals } = loadDetail({
    shared: true, responses: [stateResponse({ position: "e4" })]
  });

  await tickIntervals();

  const [sync] = frame.contentWindow.posted.filter((m) => m.__avStateSync);
  assert.ok(sync, "the frame is where the state has to end up; the host only carries it");
  assert.equal(sync.artifactId, ID);
  assert.deepEqual(sync.state, { position: "e4" });
});

// --- the reload control, and when it stays out of the way ----------------

test("an artifact that ignores 'storage' is offered a reload", async () => {
  const { byId, postFromFrame } = loadDetail({ shared: true });

  await postFromFrame(synced(2, false));

  assert.equal(byId("state-changed-banner").hidden, false,
    "the shim can keep getItem fresh; it cannot re-render an artifact that read " +
    "storage once at startup");
});

test("an artifact that listens is left alone", async () => {
  const { byId, postFromFrame } = loadDetail({ shared: true });

  await postFromFrame(synced(3, true));

  assert.equal(byId("state-changed-banner").hidden, true,
    "it already updated itself; saying so would be noise reporting work that is done");
});

test("a resync that changed nothing says nothing", async () => {
  const { byId, postFromFrame } = loadDetail({ shared: true });

  await postFromFrame(synced(0, false));

  assert.equal(byId("state-changed-banner").hidden, true);
});

test("the reload is the visitor's to press, and goes through /open", async () => {
  const { byId, frame, postFromFrame } = loadDetail({ shared: true });

  await postFromFrame(synced(1, false));
  assert.equal(frame.src, "", "nothing reloads on its own: a half-typed input would die with the frame");

  await byId("state-changed-reload").click();

  assert.match(frame.src, /^\/artifacts\/art-1\/open\?r=\d+$/,
    "the frame's own src carries a token minted when this page loaded, and this " +
    "banner is pressed on pages that have been open a while");
  assert.equal(byId("state-changed-banner").hidden, true);
});

test("a sync report from anywhere but our own frame is ignored", async () => {
  const { byId, postFromFrame } = loadDetail({ shared: true });

  await postFromFrame(synced(1, false), { postMessage() {} });

  assert.equal(byId("state-changed-banner").hidden, true,
    "the frame's origin is opaque, so identity is the source window and nothing else");
});

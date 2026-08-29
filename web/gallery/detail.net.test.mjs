/* av-kmwj — the runtime network-permission prompt, end to end on the client.
 *
 * This is the acceptance test the first attempt at this feature did not have.
 * It drives the shipped detail.js through the whole loop the ticket specifies:
 * the artifact frame reports a CSP-blocked origin, the host prompts in app
 * chrome, the user answers, and the answer reaches the API and the frame. The
 * Go tests cover the two server halves (the route, and what the render surface
 * inlines); nothing but this covers the wiring between them.
 *
 * It loads internal/api/assets/gallery/detail.js — the built, embedded copy the
 * browser is actually served — so a change to web/gallery/detail.js that was
 * never rebuilt fails here rather than shipping.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript, recordingApi } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const DETAIL_JS = path.join(here, "../../internal/api/assets/gallery/detail.js");

const ID = "art-1";
const OPEN_URL = "/artifacts/art-1/open";

// Loads detail.js with the bootstrap globals its page template renders.
function loadDetail({ readOnly = false, responses = [] } = {}) {
  const api = recordingApi(responses);
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
    apiFetch: api.apiFetch
  }, {
    // What detail.tmpl renders: every capability dialog starts hidden.
    "net-modal": { hidden: true },
    "dl-modal": { hidden: true },
    "clip-modal": { hidden: true },
    "link-modal": { hidden: true },
    "media-modal": { hidden: true }
  });
  return { ...page, api };
}

const report = (origin, directive = "connect-src") => ({
  __avNetwork: true, artifactId: ID, origin, directive
});

const originCalls = (api) => api.calls.filter((c) => c.path.includes("/origins"));

// Found in a browser, not by a unit test: on a cold load the frame's document
// often finishes before detail.js runs, so its `load` event has already fired
// and a listener attached afterwards never hears it. Announcing only on `load`
// left the frame's buffered reports stranded and the prompt simply never
// appeared — intermittently, with cache state deciding.
test("the host announces itself immediately, not only on a future frame load", () => {
  const { frame } = loadDetail();

  assert.ok(frame.contentWindow.posted.some((m) => m.__avHostReady === true),
    "a frame that already loaded fires no second load event; the ping must not wait for one");

  // And still on load, for the frames that do load after this script runs.
  const count = frame.contentWindow.posted.length;
  frame.dispatchEvent({ type: "load" });
  assert.ok(frame.contentWindow.posted.length > count);
});

test("a blocked origin is prompted for in app chrome, not in the frame", async () => {
  const { byId, postFromFrame } = loadDetail();

  assert.equal(byId("net-modal").hidden, true, "no prompt before anything is blocked");
  await postFromFrame(report("https://cdn.example.com", "script-src-elem"));

  assert.equal(byId("net-modal").hidden, false);
  assert.equal(byId("net-origin").textContent, "https://cdn.example.com",
    "the origin is set as text, never as markup");
  assert.equal(byId("net-directive").textContent, "Blocked by script-src-elem");
});

test("Allow records an allow decision and reloads the frame under the new CSP", async () => {
  const { byId, frame, api, postFromFrame } = loadDetail();
  const before = frame.src;

  await postFromFrame(report("https://cdn.example.com"));
  await byId("net-allow").click();

  const [call] = originCalls(api);
  assert.ok(call, "Allow must write through the per-origin route");
  assert.equal(call.method, "POST");
  assert.equal(call.path, `/api/artifacts/${ID}/origins`);
  assert.deepEqual(JSON.parse(call.body), {
    origin: "https://cdn.example.com", decision: "allow", source: "runtime"
  });

  assert.notEqual(frame.src, before, "the frame must refetch, or the widened CSP never applies");
  assert.ok(frame.src.startsWith(OPEN_URL + "?r="),
    `the reload goes through the token-minting open route, got ${frame.src}`);
  assert.equal(byId("net-modal").hidden, true, "the prompt closes once the answer is recorded");

  // The reload re-runs the artifact, so anything still blocked reports itself
  // again — the load handler clears the status line rather than leaving
  // "Applying…" on screen forever.
  assert.equal(byId("al-status").textContent, "Applying…");
  frame.dispatchEvent({ type: "load" });
  assert.equal(byId("al-status").textContent, "");
});

test("a failed write leaves the prompt open rather than claiming success", async () => {
  const { byId, frame, postFromFrame } = loadDetail({
    responses: [{ ok: false, status: 500, json: async () => ({}) }]
  });
  const before = frame.src;

  await postFromFrame(report("https://cdn.example.com"));
  await byId("net-allow").click();

  assert.equal(byId("net-modal").hidden, false, "the user must be able to try again");
  assert.equal(frame.src, before, "no reload: the CSP did not change");
  assert.match(byId("al-status").textContent, /Failed to save/);
});

test("Don't ask again records a block decision and never an allow", async () => {
  const { byId, frame, api, postFromFrame } = loadDetail();
  const before = frame.src;

  await postFromFrame(report("https://tracker.example.com"));
  await byId("net-never").click();

  const [call] = originCalls(api);
  assert.equal(JSON.parse(call.body).decision, "block");
  assert.equal(byId("net-modal").hidden, true);
  assert.equal(frame.src, before,
    "a block changes no policy, so there is nothing to reload for");
});

test("Block once dismisses without writing anything", async () => {
  const { byId, api, postFromFrame } = loadDetail();

  await postFromFrame(report("https://cdn.example.com"));
  byId("net-once").click();

  assert.equal(byId("net-modal").hidden, true);
  assert.deepEqual(originCalls(api), [], "dismissing decides nothing");
});

test("Escape dismisses the prompt", async () => {
  const { byId, api, postFromFrame, pressKey } = loadDetail();

  await postFromFrame(report("https://cdn.example.com"));
  pressKey("Escape");

  assert.equal(byId("net-modal").hidden, true);
  assert.deepEqual(originCalls(api), []);
});

test("several blocked origins queue rather than overwrite each other", async () => {
  const { byId, api, postFromFrame } = loadDetail();

  await postFromFrame(report("https://a.example.com"));
  await postFromFrame(report("https://b.example.com"));
  assert.equal(byId("net-origin").textContent, "https://a.example.com",
    "the second report must not steal the dialog out from under the first");

  await byId("net-never").click();
  assert.equal(byId("net-modal").hidden, false);
  assert.equal(byId("net-origin").textContent, "https://b.example.com");

  await byId("net-never").click();
  assert.equal(byId("net-modal").hidden, true);
  assert.deepEqual(
    originCalls(api).map((c) => JSON.parse(c.body).origin),
    ["https://a.example.com", "https://b.example.com"]
  );
});

test("Allow drops the queue, because the reload re-reports whatever is still blocked", async () => {
  const { byId, postFromFrame } = loadDetail();

  await postFromFrame(report("https://a.example.com"));
  await postFromFrame(report("https://b.example.com"));
  await byId("net-allow").click();

  assert.equal(byId("net-modal").hidden, true,
    "asking about b now would ask again the moment the fresh load reports it");
});

test("a message from anything but the artifact frame is ignored", async () => {
  const { byId, postFromFrame } = loadDetail();

  await postFromFrame(report("https://evil.example.com"), { notTheFrame: true });
  assert.equal(byId("net-modal").hidden, true,
    "an opaque frame's origin is the string 'null', so identity is the source window");
});

test("a report for another artifact is ignored", async () => {
  const { byId, postFromFrame } = loadDetail();

  await postFromFrame({ __avNetwork: true, artifactId: "someone-else", origin: "https://x.example.com" });
  assert.equal(byId("net-modal").hidden, true);
});

test("a read-only visitor is not asked a question they cannot answer", async () => {
  const { byId, api, postFromFrame } = loadDetail({ readOnly: true });

  await postFromFrame(report("https://cdn.example.com"));
  assert.equal(byId("net-modal").hidden, true);
  assert.deepEqual(originCalls(api), []);
});

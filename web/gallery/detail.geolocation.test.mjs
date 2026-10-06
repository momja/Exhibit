/* av-f446: the host half of the geolocation gate, driven as a browser drives it.
 *
 * The frame cannot read a location (it carries no allow= delegation), so the
 * whole feature is this page's decision: prompt, persist geolocation_approved,
 * open the top-level render where the approval is spent, and answer the
 * frame's request on every path. An unanswered request is an artifact waiting
 * forever on a callback, which is the failure the gate exists to remove, so
 * most of what follows asserts that an answer went back.
 *
 * Loads the built copies under internal/api/assets/gallery, like the other
 * detail.*.test.mjs files, so an edit under web/gallery that was never rebuilt
 * fails here. The frame end of the channel is render.geolocation.test.mjs.
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

const HIDDEN = {
  "net-modal": { hidden: true },
  "dl-modal": { hidden: true },
  "clip-modal": { hidden: true },
  "link-modal": { hidden: true },
  "media-modal": { hidden: true },
  "geo-modal": { hidden: true },
  "capability-warning-banner": { hidden: true },
  "origin-blocked-banner": { hidden: true }
};

// tabs records the placeholder tabs the Allow handlers claim with
// window.open('', '_blank'); opened records any other window.open.
function loadDetail({ readOnly = false, approvals = {}, responses = [] } = {}) {
  const api = recordingApi(responses);
  const tabs = [];
  const opened = [];
  const page = loadPageScript(DETAIL_JS, {
    TOKEN: "",
    READ_ONLY: readOnly,
    STATE_WRITABLE: true,
    SHARED_STATE: false,
    ID,
    SOURCE_URL: "",
    OPEN_URL,
    downloadsApproved: false,
    clipboardApproved: false,
    linksApproved: false,
    cameraApproved: false,
    microphoneApproved: false,
    geolocationApproved: false,
    ...approvals,
    apiFetch: api.apiFetch,
    apiStateFetch: api.apiStateFetch,
    CustomEvent,
    open: (url) => {
      if (url === "") {
        const tab = { location: null, opener: "the library tab", closed: false, close() { this.closed = true; } };
        tabs.push(tab);
        return tab;
      }
      opened.push(url);
      return null;
    }
  }, HIDDEN);
  return { ...page, api, tabs, opened };
}

const geoRequest = (id) => ({ __avGeolocation: true, artifactId: ID, id });
const answers = (frame) => frame.contentWindow.posted.filter((m) => m.__avGeolocationResult);
const patches = (api) => api.calls.filter((c) => c.method === "PATCH").map((c) => JSON.parse(c.body));

test("an unapproved request prompts, and is not answered until the user decides", async () => {
  const { byId, frame, api, postFromFrame } = loadDetail();

  await postFromFrame(geoRequest(1));

  assert.equal(byId("geo-modal").hidden, false);
  assert.equal(byId("geo-block").focused, true, "keyboard users land on the decision");
  assert.deepEqual(answers(frame), []);
  assert.deepEqual(api.calls, []);
});

test("Allow persists location alone, opens the top-level render, and answers the frame", async () => {
  const { byId, frame, api, tabs, postFromFrame } = loadDetail();

  await postFromFrame(geoRequest(1));
  await byId("geo-allow").click();

  assert.deepEqual(patches(api), [{ geolocation_approved: true }],
    "approving location writes that flag and no device flag");
  assert.equal(tabs.length, 1, "the tab is claimed in the click's own task");
  assert.equal(tabs[0].opener, null, "the artifact's top-level render gets no handle on the library tab");
  assert.equal(tabs[0].location, OPEN_URL);
  assert.equal(byId("geo-modal").hidden, true);
  assert.deepEqual(answers(frame).map((a) => [a.id, a.banner]), [[1, false]],
    "no banner: the user is already looking at the place the approval works");
});

test("after Allow, the next request raises the banner instead of a second prompt", async () => {
  const { byId, frame, api, postFromFrame } = loadDetail();

  await postFromFrame(geoRequest(1));
  await byId("geo-allow").click();
  await postFromFrame(geoRequest(2));

  assert.equal(byId("geo-modal").hidden, true);
  const second = answers(frame).find((a) => a.id === 2);
  assert.ok(second, "the frame's request is answered");
  assert.equal(second.banner, true);
  assert.equal(patches(api).length, 1, "nothing left to persist");
});

test("an artifact approved before this page loaded is answered with the banner", async () => {
  const { byId, frame, api, postFromFrame } = loadDetail({ approvals: { geolocationApproved: true } });

  await postFromFrame(geoRequest(7));

  assert.equal(byId("geo-modal").hidden, true);
  assert.deepEqual(answers(frame).map((a) => [a.id, a.banner]), [[7, true]]);
  assert.deepEqual(api.calls, []);
});

test("Block answers with a denial and persists nothing", async () => {
  const { byId, frame, api, tabs, postFromFrame } = loadDetail();

  await postFromFrame(geoRequest(1));
  await byId("geo-block").click();

  assert.equal(byId("geo-modal").hidden, true);
  assert.deepEqual(answers(frame).map((a) => [a.id, a.banner, a.error]), [[1, false, "User denied Geolocation"]]);
  assert.deepEqual(api.calls, []);
  assert.deepEqual(tabs, []);
});

test("Escape is a Block", async () => {
  const { byId, frame, pressKey, postFromFrame } = loadDetail();

  await postFromFrame(geoRequest(1));
  pressKey("Escape");

  assert.equal(byId("geo-modal").hidden, true);
  assert.equal(answers(frame).length, 1);
});

test("a newer request displaces the pending one, which still gets its answer", async () => {
  const { byId, frame, postFromFrame } = loadDetail();

  await postFromFrame(geoRequest(1));
  await postFromFrame(geoRequest(2));
  assert.deepEqual(answers(frame).map((a) => a.id), [1],
    "getCurrentPosition then watchPosition is common, and the first must not wait forever");
  assert.equal(byId("geo-modal").hidden, false, "one prompt covers the newest request");

  await byId("geo-allow").click();
  assert.deepEqual(answers(frame).map((a) => a.id), [1, 2]);
});

test("a request displaced while the PATCH is in flight is not answered by opening a tab", async () => {
  const { byId, frame, tabs, postFromFrame } = loadDetail();

  await postFromFrame(geoRequest(1));
  // The PATCH is still a pending promise when the frame asks again.
  const clicking = byId("geo-allow").click();
  await postFromFrame(geoRequest(2));
  await clicking;

  assert.equal(tabs.length, 1);
  assert.equal(tabs[0].closed, true, "the tab claimed for request 1 is closed, not navigated");
  assert.equal(tabs[0].location, null);
  assert.deepEqual(answers(frame).map((a) => a.id), [1], "request 1 was answered when it was displaced");
  assert.equal(byId("geo-modal").hidden, false, "request 2 is still on screen");
});

test("a failed PATCH closes the placeholder tab and keeps the prompt", async () => {
  const { byId, frame, tabs, postFromFrame } = loadDetail({
    responses: [{ ok: false, status: 500, json: async () => ({}) }]
  });

  await postFromFrame(geoRequest(1));
  await byId("geo-allow").click();

  assert.equal(tabs[0].closed, true);
  assert.equal(tabs[0].location, null);
  assert.equal(byId("geo-modal").hidden, false, "the user can still choose Block");
  assert.deepEqual(answers(frame), []);
});

test("a message from anything but the artifact frame is ignored", async () => {
  const { byId, frame, postFromFrame } = loadDetail();

  await postFromFrame(geoRequest(1), { postMessage() {} });
  await postFromFrame({ ...geoRequest(1), artifactId: "someone-else" });

  assert.equal(byId("geo-modal").hidden, true);
  assert.deepEqual(answers(frame), []);
});

test("a recipient is answered with a denial, never offered the prompt", async () => {
  const { byId, frame, api, postFromFrame } = loadDetail({ readOnly: true });

  await postFromFrame(geoRequest(1));

  assert.equal(byId("geo-modal").hidden, true, "the approval is the owner's to make");
  assert.deepEqual(answers(frame).map((a) => [a.id, a.banner]), [[1, false]]);
  assert.deepEqual(api.calls, []);
});

test("a recipient of an approved artifact is pointed at the top-level render", async () => {
  const { frame, api, postFromFrame } = loadDetail({ readOnly: true, approvals: { geolocationApproved: true } });

  await postFromFrame(geoRequest(1));

  assert.deepEqual(answers(frame).map((a) => [a.id, a.banner]), [[1, true]],
    "the owner's approval travels with the artifact");
  assert.deepEqual(api.calls, []);
});

// The camera gate's Allow now goes through the same helper, so it gets one
// end-to-end pass here: the refactor must not have changed what it does.
test("the camera gate's Allow still persists, opens the render, and answers", async () => {
  const { byId, frame, api, tabs, postFromFrame } = loadDetail();

  await postFromFrame({ __avMedia: true, artifactId: ID, id: "m1", video: true, audio: false });
  assert.equal(byId("media-modal").hidden, false);
  await byId("media-allow").click();

  assert.deepEqual(patches(api), [{ camera_approved: true }]);
  assert.equal(tabs[0].location, OPEN_URL);
  assert.equal(tabs[0].opener, null);
  const [reply] = frame.contentWindow.posted.filter((m) => m.__avMediaResult);
  assert.equal(reply.id, "m1");
  assert.equal(reply.name, "NotSupportedError");
  assert.equal(byId("media-modal").hidden, true);
});

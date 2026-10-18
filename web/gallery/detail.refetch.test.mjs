/* av-reo3, av-b17a — "Update from source" on the detail page.
 *
 * The refetch response reports new origins and never approves them, so the
 * page is the only place a user learns what the new content contacts. These
 * tests drive the shipped detail.js through one refetch each and check what
 * the status line says, and whether the page reloads out from under it.
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
  path.join(ASSETS, "detail.js")
];

// Runs one refetch against a queued response and returns what the page did.
async function refetchWith(body) {
  const api = recordingApi([{ ok: true, status: 200, json: async () => body }]);
  const page = loadPageScript(DETAIL_JS, {
    TOKEN: "",
    READ_ONLY: false,
    STATE_WRITABLE: true,
    SHARED_STATE: false,
    ID: "art-1",
    SOURCE_URL: "https://source.example.com/tool.html",
    OPEN_URL: "/artifacts/art-1/open",
    downloadsApproved: false,
    clipboardApproved: false,
    linksApproved: false,
    cameraApproved: false,
    microphoneApproved: false,
    apiFetch: api.apiFetch,
    apiStateFetch: api.apiStateFetch,
    confirm: () => true,
    CustomEvent
  });
  let reloads = 0;
  page.window.location.reload = () => { reloads++; };
  await page.context.refetchSource();
  const status = page.byId("al-status");
  return { api, status, reloads: () => reloads };
}

const artifact = { id: "art-1" };

test("an unchanged source says so and does not reload", async () => {
  const { api, status, reloads } = await refetchWith({
    artifact, updated: false, network_footprint: [], footprint_changed: false,
    undecided_origins: []
  });

  assert.equal(api.calls[0].path, "/api/artifacts/art-1/refetch");
  assert.equal(status.textContent, "✓ Already up to date.");
  assert.equal(reloads(), 0);
});

test("an update with nothing to report reloads straight away", async () => {
  const { status, reloads } = await refetchWith({
    artifact,
    updated: true,
    network_footprint: ["https://api.example.com"],
    footprint_changed: true,
    undecided_origins: []
  });

  assert.equal(reloads(), 1, "every new origin is already decided, so there is nothing to read");
  assert.match(status.textContent, /reloading/);
});

test("only undecided origins are named, and the reload waits for the user", async () => {
  const { status, reloads } = await refetchWith({
    artifact,
    updated: true,
    network_footprint: ["https://approved.example.com", "https://new.example.com"],
    footprint_changed: true,
    undecided_origins: ["https://new.example.com"]
  });

  assert.match(status.textContent, /https:\/\/new\.example\.com/);
  assert.doesNotMatch(status.textContent, /approved\.example\.com/,
    "an origin already decided needs no approval");
  assert.equal(reloads(), 0, "the page must not reload while the user is reading");

  const [button] = status.children;
  assert.equal(button.textContent, "Reload");
  await button.click();
  assert.equal(reloads(), 1);
});

test("assets that could not be vendored are reported", async () => {
  const { status, reloads } = await refetchWith({
    artifact,
    updated: true,
    network_footprint: [],
    footprint_changed: false,
    undecided_origins: [],
    snapshot: { applied: true, failures: [{ ref: "img/a.png", kind: "status" }] }
  });

  assert.match(status.textContent, /1 asset could not be saved into the file/);
  assert.equal(reloads(), 0);
});

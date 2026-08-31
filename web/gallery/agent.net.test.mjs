/* av-6xvs — the agent chat page hosts the runtime network prompt.
 *
 * The gap this closes: /agent embeds the same render document behind the same
 * sandbox on the same app origin as the detail page, so it has the trusted
 * chrome the prompt needs — and it had none of the prompt, so an artifact
 * reaching an unapproved origin while being built there failed silently.
 *
 * The two pages now share network-prompt.js, and what is worth testing here is
 * exactly what they do NOT share: this page's frame is replaced by an htmx swap
 * after every save, its artifact id does not exist until the agent makes one,
 * and its reload goes through the fragment rather than an src reassignment.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript, recordingApi } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const ASSETS = path.join(here, "../../internal/api/assets/gallery");
// The order agent.tmpl loads them in.
const AGENT_JS = [
  path.join(ASSETS, "api.js"),
  path.join(ASSETS, "state-api.js"),
  path.join(ASSETS, "network-prompt.js"),
  path.join(ASSETS, "agent.js")
];

const ID = "art-1";

// artifact is declared by agent.tmpl's inline bootstrap, not by agent.js:
// `let artifact = {id,title}` when editing an existing one, else null.
function loadAgent({ responses = [], artifact = { id: ID, title: "Artifact" } } = {}) {
  const api = recordingApi(responses);
  const page = loadPageScript(AGENT_JS, {
    // The full bootstrap agent.tmpl renders. BYOK true keeps the key markup in
    // play, which is the shape a self-hosted instance has.
    TOKEN: "", READ_ONLY: false, BYOK: true, artifact,
    apiFetch: api.apiFetch, apiEventSource: () => ({ close() {} }),
    EventSource: function () { return { close() {} }; },
    CustomEvent: class { constructor(t, o) { this.type = t; Object.assign(this, o); } },
    matchMedia: () => ({ matches: false, addEventListener() {} }),
    alert: () => {}, confirm: () => true
  }, {
    "net-modal": { hidden: true },
    "key-modal": { hidden: true }
  });
  return { ...page, api };
}

const report = (origin, directive = "img-src") => ({
  __avNetwork: true, artifactId: ID, origin, directive
});
const originCalls = (api) => api.calls.filter((c) => c.path.includes("/origins"));

test("the agent page installs the prompt against its own preview frame", () => {
  const { byId } = loadAgent();
  // Installed at all: the module refuses to install twice and no-ops without
  // the dialog, so a page that forgot the partial would silently have nothing.
  assert.ok(byId("net-modal"), "the shared dialog must be present");
  assert.equal(byId("net-modal").hidden, true);
});

test("a blocked origin in the preview opens the prompt", async () => {
  const page = loadAgent();
  // The preview frame is #pv-frame here, not the detail page's bare <iframe>.
  const frame = page.byId("pv-frame");
  frame.contentWindow = { posted: [], postMessage(m) { this.posted.push(m); } };

  await page.postFromFrame(report("https://cdn.example.com"), frame.contentWindow);

  assert.equal(page.byId("net-modal").hidden, false);
  assert.equal(page.byId("net-origin").textContent, "https://cdn.example.com");
  assert.equal(page.byId("net-directive").textContent, "Blocked by img-src");
});

test("Allow writes the decision and re-renders the pane rather than reassigning src", async () => {
  const page = loadAgent();
  const frame = page.byId("pv-frame");
  frame.contentWindow = { posted: [], postMessage(m) { this.posted.push(m); } };
  const before = frame.src;

  await page.postFromFrame(report("https://cdn.example.com"), frame.contentWindow);
  await page.byId("net-allow").click();

  const [call] = originCalls(page.api);
  assert.ok(call, "Allow must write through the per-origin route");
  assert.equal(call.method, "POST");
  assert.equal(call.path, `/api/artifacts/${ID}/origins`);
  assert.deepEqual(JSON.parse(call.body), {
    origin: "https://cdn.example.com", decision: "allow", source: "runtime"
  });
  // The pane is re-rendered by htmx, which mints a fresh render token in the
  // fragment; reassigning src here would reuse the stale one.
  assert.equal(frame.src, before, "the pane reloads through the fragment, not the src");
  assert.equal(page.byId("net-modal").hidden, true);
});

// A chat session that has not created anything yet has no artifact to decide
// about, and the id the route would need does not exist. Prompting would ask
// about an origin with nowhere to record the answer.
test("no prompt before the agent has created an artifact", async () => {
  const page = loadAgent({ artifact: null });
  const frame = page.byId("pv-frame");
  frame.contentWindow = { posted: [], postMessage() {} };

  await page.postFromFrame(report("https://cdn.example.com"), frame.contentWindow);
  assert.equal(page.byId("net-modal").hidden, true);
  assert.deepEqual(originCalls(page.api), []);
});

test("a report for a frame that is not the preview is ignored", async () => {
  const page = loadAgent();
  const frame = page.byId("pv-frame");
  frame.contentWindow = { posted: [], postMessage() {} };

  await page.postFromFrame(report("https://evil.example.com"), { notTheFrame: true });
  assert.equal(page.byId("net-modal").hidden, true);
});

test("the host announces itself to each swapped-in preview frame", () => {
  const page = loadAgent();
  const frame = page.byId("pv-frame");
  frame.contentWindow = { posted: [], postMessage(m) { this.posted.push(m); } };

  // htmx replaces the frame on every agent save, so the handshake has to run
  // again — a violation raised before the page noticed the new frame would
  // otherwise be lost, which is the failure the handshake exists to remove.
  page.byId("pane-preview").dispatchEvent({ type: "htmx:afterSwap" });

  assert.ok(frame.contentWindow.posted.some((m) => m.__avHostReady === true),
    "each new preview frame must be told the host is listening");
});

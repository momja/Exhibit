/* av-y7td — the chat page's History pane.
 *
 * Its contents are server-rendered fragments that htmx swaps into #history, so
 * what script owns is small and easy to get wrong: there is no history until
 * there is an artifact, and while the pane is open it takes the place of the
 * messages and the composer.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript, recordingApi } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const ASSETS = path.join(here, "../../internal/api/assets/gallery");
const AGENT_JS = [
  path.join(ASSETS, "api.js"),
  path.join(ASSETS, "state-api.js"),
  path.join(ASSETS, "network-prompt.js"),
  path.join(ASSETS, "agent.js")
];

function loadAgent({ artifact = null } = {}) {
  const api = recordingApi([]);
  return loadPageScript(AGENT_JS, {
    TOKEN: "", READ_ONLY: false, BYOK: true, artifact,
    apiFetch: api.apiFetch, apiEventSource: () => ({ close() {} }),
    EventSource: function () { return { close() {} }; },
    CustomEvent: class { constructor(t, o) { this.type = t; Object.assign(this, o); } },
    matchMedia: () => ({ matches: false, addEventListener() {} }),
    alert: () => {}, confirm: () => true
  }, { "net-modal": { hidden: true }, "key-modal": { hidden: true } });
}

test("a page that is not about an artifact has no history to offer", () => {
  const { byId } = loadAgent({ artifact: null });
  assert.equal(byId("history-btn").hidden, true);
});

test("a page opened on an artifact offers its history", () => {
  const { byId } = loadAgent({ artifact: { id: "art-1", title: "Counter" } });
  assert.equal(byId("history-btn").hidden, false);
});

test("opening the history pane takes over the chat, and closing hands it back", () => {
  const { byId, context } = loadAgent({ artifact: { id: "art-1", title: "Counter" } });
  const chat = byId("pane-chat");
  assert.equal(chat.classList.contains("history-open"), false);

  context.openHistory();
  assert.equal(chat.classList.contains("history-open"), true);

  context.closeHistory();
  assert.equal(chat.classList.contains("history-open"), false);
});

test("the button appears once the agent has made an artifact", () => {
  const { byId, context } = loadAgent({ artifact: null });
  assert.equal(byId("history-btn").hidden, true);

  // The save event sets `artifact` and refreshes the preview, which is when the
  // page learns there is something to have a history of.
  context.artifact = { id: "new-1", title: "Made" };
  context.refreshPreview();
  assert.equal(byId("history-btn").hidden, false);
});

// --- Continuing a conversation (av-b4yh) --------------------------------------

const json = (status, body) => ({ ok: status < 400, status, json: async () => body });
const STARTED = json(201, { id: "conv-1", sse_ticket: "t1" });
const posts = (api, needle) => api.calls.filter((c) => c.method === "POST" && c.path.includes(needle));

const KEY_STATUS = json(200, { configured: true, provider: "exhibit-mock", model: "exhibit-mock-1" });

// The page asks for the key's status as it boots, which takes the first queued
// response and leaves a call in the log: both are accounted for here, so a test
// sees only what continuing a conversation did.
async function loadWith(responses) {
  const api = recordingApi([KEY_STATUS, ...responses]);
  const page = loadPageScript(AGENT_JS, {
    TOKEN: "", READ_ONLY: false, BYOK: true, artifact: { id: "art-1", title: "Counter" },
    apiFetch: api.apiFetch, apiEventSource: () => ({ close() {} }),
    EventSource: function () { return { close() {} }; },
    CustomEvent: class { constructor(t, o) { this.type = t; Object.assign(this, o); } },
    matchMedia: () => ({ matches: false, addEventListener() {} }),
    alert: () => {}, confirm: () => true
  }, { "net-modal": { hidden: true }, "key-modal": { hidden: true } });
  await new Promise((resolve) => setTimeout(resolve, 0));
  api.calls.length = 0;
  page.byId("history-messages").innerHTML = '<div class="msg user">make it purple</div>';
  page.context.openHistory();
  return { ...page, api };
}

test("rolling back and continuing starts the conversation first, then returns the artifact to the version", async () => {
  const page = await loadWith([STARTED, json(200, {})]);
  await page.context.resumeConversation("conv-1", 2);

  assert.deepEqual(page.api.calls.map((c) => [c.method, c.path]), [
    ["POST", "/api/agent/sessions"],
    ["POST", "/api/artifacts/art-1/versions/2/restore"]
  ]);
  assert.deepEqual(JSON.parse(page.api.calls[0].body), { artifact_id: "art-1", resume_session_id: "conv-1" });

  // The conversation is on screen, with the pane handed back to the chat.
  assert.ok(page.byId("messages").innerHTML.includes("make it purple"));
  assert.equal(page.byId("pane-chat").classList.contains("history-open"), false);
});

test("continuing without rolling back never touches the artifact", async () => {
  const page = await loadWith([STARTED]);
  await page.context.resumeConversation("conv-1", 0);

  assert.equal(posts(page.api, "/restore").length, 0);
  assert.equal(posts(page.api, "/api/agent/sessions").length, 1);
  assert.equal(page.byId("pane-chat").classList.contains("history-open"), false);
});

test("a refusal to start costs nothing: no rollback, and the pane stays for another try", async () => {
  const page = await loadWith([json(412, { error: "no API key configured" })]);
  page.byId("resume-status");
  await page.context.resumeConversation("conv-1", 2);

  assert.equal(posts(page.api, "/restore").length, 0, "nothing was rolled back");
  assert.equal(page.byId("resume-status").textContent, "no API key configured");
  assert.equal(page.byId("pane-chat").classList.contains("history-open"), true);
  assert.equal(page.byId("key-modal").hidden, false, "a missing key opens the key dialog");
});

test("a rollback that fails ends the session it started and says the conversation was not continued", async () => {
  const page = await loadWith([STARTED, json(500, { error: "boom" }), json(204, {})]);
  await page.context.resumeConversation("conv-1", 2);

  assert.equal(page.api.calls.filter((c) => c.method === "DELETE" && c.path === "/api/agent/sessions/conv-1").length, 1,
    "no agent is left running for a conversation that was not continued");
  assert.match(page.byId("resume-status").textContent, /Could not roll back: boom/);
  assert.match(page.byId("resume-status").textContent, /not continued/);
  assert.equal(page.byId("pane-chat").classList.contains("history-open"), true);
});

// 409 is the artifact already being at that version — nothing to put back, and
// no reason to stop the person getting on with it.
test("an artifact already at that version is not a failure", async () => {
  const page = await loadWith([STARTED, json(409, { error: "that version is already the current one" })]);
  await page.context.resumeConversation("conv-1", 2);

  assert.equal(page.byId("pane-chat").classList.contains("history-open"), false);
  assert.equal(page.byId("resume-status").textContent, "Starting…", "no error was shown");
});

test("the page's live conversation gives way to the one being continued", async () => {
  const page = await loadWith([json(201, { id: "live-1", sse_ticket: "a" }), json(201, { id: "conv-1", sse_ticket: "b" }), json(204, {})]);
  // A prompt starts this page's own session.
  await page.context.ensureSession();
  page.api.calls.length = 0;

  await page.context.resumeConversation("conv-1", 0);
  assert.ok(page.api.calls.some((c) => c.method === "DELETE" && c.path === "/api/agent/sessions/live-1"),
    "the session it replaces is closed");
});

test("it will not switch conversations while the agent is working", async () => {
  const page = await loadWith([STARTED]);
  page.context.setStreaming(true);
  await page.context.resumeConversation("conv-1", 2);

  assert.equal(page.api.calls.length, 0);
  assert.match(page.byId("resume-status").textContent, /still working/);
});

test("the buttons in the fragment are handled by the pane they are swapped into", async () => {
  const page = await loadWith([STARTED, json(200, {})]);
  const button = { dataset: { resume: "conv-1", rollback: "2" } };
  // What a browser hands a delegated listener: the click's target, which
  // resolves to the button it landed on.
  await page.byId("history").click({ closest: (sel) => (sel === "[data-resume]" ? button : null) });
  await new Promise((resolve) => setTimeout(resolve, 0));

  assert.equal(posts(page.api, "/api/agent/sessions").length, 1);
  assert.equal(posts(page.api, "/versions/2/restore").length, 1);
});

test("a click that is not on a resume button does nothing", async () => {
  const page = await loadWith([STARTED]);
  await page.byId("history").click({ closest: () => null });
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(page.api.calls.length, 0);
});

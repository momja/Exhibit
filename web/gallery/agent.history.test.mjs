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

// The page opened on an artifact, booted: it has asked for the key's status,
// which takes the first queued response and leaves a call in the log. The
// responses given are what the page is answered with from then on.
async function boot(responses) {
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
  return { ...page, api };
}

// A booted page with History open on a conversation, and the boot's own call
// accounted for, so a test sees only what continuing a conversation did.
async function loadWith(responses) {
  const page = await boot(responses);
  page.api.calls.length = 0;
  page.byId("history-messages").innerHTML = '<div class="msg user">make it purple</div>';
  page.context.openHistory();
  return page;
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

// --- A new conversation is the default (av-b4yh) --------------------------------
// Nothing the page does on its own continues a kept conversation. Continuing is
// asked for by name — `resume_session_id`, sent from resumeConversation alone,
// behind the Continue buttons in the History pane — so these pin the paths that
// are *not* that: opening the page, sending the first message, and sending the
// next one after a session ended. Each is a way a chat could quietly pick an
// earlier conversation back up, and none does.

const sessionStarts = (api) => api.calls.filter((c) => c.method === "POST" && c.path === "/api/agent/sessions");

test("opening the page on an artifact that has history starts nothing", async () => {
  const page = await boot([]);
  // Looking at the pane is not a request for anything either; its contents are
  // fragments the markup fetches, and the pane itself is a class on the chat.
  page.context.openHistory();
  page.context.closeHistory();

  assert.deepEqual(page.api.calls.map((c) => [c.method, c.path]), [["GET", "/api/agent/key"]],
    "the key's status is all the page asked for: no session, and no conversation continued");
});

test("the first message begins a new conversation, whatever has been kept", async () => {
  const page = await boot([STARTED]);
  page.byId("input").value = "now make it red";
  await page.context.send();

  const starts = sessionStarts(page.api);
  assert.equal(starts.length, 1);
  assert.deepEqual(JSON.parse(starts[0].body), { artifact_id: "art-1" },
    "it names the artifact and no conversation to continue");
  assert.equal(posts(page.api, "/api/agent/sessions/conv-1/prompt").length, 1, "and the message goes to it");
});

test("when a session ends under the page the next message starts a new one", async () => {
  const page = await boot([STARTED, json(201, { id: "conv-2", sse_ticket: "t2" })]);
  await page.context.ensureSession();
  // The server closed it (idle, or restarted): the page says the next message
  // starts a new one, and it must mean that rather than continuing the ended one.
  page.context.handleAgentEvent({ type: "exhibit_session_closed" });
  page.api.calls.length = 0;

  page.byId("input").value = "and a little bigger";
  await page.context.send();

  const starts = sessionStarts(page.api);
  assert.equal(starts.length, 1);
  assert.deepEqual(JSON.parse(starts[0].body), { artifact_id: "art-1" });
  assert.equal(posts(page.api, "/api/agent/sessions/conv-2/prompt").length, 1);
});

// --- Looking at an earlier version (av-vw7r) ------------------------------------------
// "View vN" in the History card is an htmx swap into the preview pane — the card's
// markup carries the whole request — so what script owns is small: what "Back to
// current" means, and a conversation that is continued ending a view. The frame
// in the pane is deliberately not #pv-frame, which is what keeps this page's
// bridges from hearing it; that is a property of the fragment's markup and is
// pinned by the Go template tests, where the markup is real.

// Counts the pane being asked to re-render for the artifact as it is: the same
// event an agent save fires, which the pane's hx-trigger turns into a fetch.
function countRefreshes(page) {
  const seen = { n: 0 };
  page.document.body.addEventListener("exhibit:artifact-saved", () => { seen.n++; });
  return seen;
}

test("Back to current re-renders the pane for the artifact as it is", async () => {
  const page = await boot([]);
  const refreshes = countRefreshes(page);

  await page.byId("pane-preview").click({
    closest: (sel) => (sel === '[data-action="close-version-viewer"]' ? { dataset: {} } : null)
  });

  assert.equal(refreshes.n, 1);
  assert.equal(page.api.calls.filter((c) => c.method !== "GET").length, 0, "looking and stopping looking write nothing");
});

test("a click elsewhere in the pane leaves it alone", async () => {
  const page = await boot([]);
  const refreshes = countRefreshes(page);
  await page.byId("pane-preview").click({ closest: () => null });
  assert.equal(refreshes.n, 0);
});

// A version that was only looked at gives way when its conversation is
// continued: the pane would otherwise sit showing v2 beside a chat that is about
// the artifact as it is now (or, after a rollback, about the new v5).
test("continuing a conversation ends a view of a version", async () => {
  const page = await boot([STARTED]);
  const refreshes = countRefreshes(page);
  page.context.viewingVersion = () => true;
  page.byId("history-messages").innerHTML = '<div class="msg user">make it purple</div>';

  await page.context.resumeConversation("conv-1", 0);

  assert.equal(refreshes.n, 1, "the pane returns to the artifact");
  assert.equal(posts(page.api, "/restore").length, 0, "and the artifact itself was not touched");
});

test("continuing when no version is showing does not reload the pane", async () => {
  const page = await boot([STARTED]);
  const refreshes = countRefreshes(page);
  page.context.viewingVersion = () => false;

  await page.context.resumeConversation("conv-1", 0);

  assert.equal(refreshes.n, 0, "nothing changed, so the artifact in the pane is not restarted");
});

test("a rollback re-renders the pane once, whether or not a version was showing", async () => {
  for (const viewing of [true, false]) {
    const page = await boot([STARTED, json(200, {})]);
    const refreshes = countRefreshes(page);
    page.context.viewingVersion = () => viewing;

    await page.context.resumeConversation("conv-1", 2);

    assert.equal(refreshes.n, 1, "viewing=" + viewing);
  }
});

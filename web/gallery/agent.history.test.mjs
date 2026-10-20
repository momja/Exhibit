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
  path.join(ASSETS, "state-api.js"),
  path.join(ASSETS, "network-prompt.js"),

];

async function loadAgent({ artifact = null } = {}) {
  const api = recordingApi([]);
  const page = loadPageScript(AGENT_JS, {
    TOKEN: "", READ_ONLY: false, BYOK: true, artifact,
    apiFetch: api.apiFetch, apiEventSource: () => ({ close() {} }),
    EventSource: function () { return { close() {} }; },
    CustomEvent: class { constructor(t, o) { this.type = t; Object.assign(this, o); } },
    matchMedia: () => ({ matches: false, addEventListener() {} }),
    alert: () => {}, confirm: () => true
  }, { "net-modal": { hidden: true }, "key-modal": { hidden: true } });
  const bindings = await page.loadModule(path.join(ASSETS, 'agent.js'),
    ['ensureSession', 'handleAgentEvent', 'refreshPreview', 'resumeConversation', 'setStreaming', 'viewingVersion']);
  Object.assign(page.context, page.window, bindings);
  return page;
}

test("a page that is not about an artifact has no history to offer", async () => {
  const { byId } = await loadAgent({ artifact: null });
  assert.equal(byId("history-btn").hidden, true);
});

test("a page opened on an artifact offers its history", async () => {
  const { byId } = await loadAgent({ artifact: { id: "art-1", title: "Counter" } });
  assert.equal(byId("history-btn").hidden, false);
});

test("opening the history pane takes over the chat, and closing hands it back", async () => {
  const { byId, context } = await loadAgent({ artifact: { id: "art-1", title: "Counter" } });
  const chat = byId("pane-chat");
  assert.equal(chat.classList.contains("history-open"), false);

  context.openHistory();
  assert.equal(chat.classList.contains("history-open"), true);

  context.closeHistory();
  assert.equal(chat.classList.contains("history-open"), false);
});

test("the button appears once the agent has made an artifact", async () => {
  const { byId, context } = await loadAgent({ artifact: null });
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
  const bindings = await page.loadModule(path.join(ASSETS, 'agent.js'),
    ['ensureSession', 'handleAgentEvent', 'refreshPreview', 'resumeConversation', 'setStreaming', 'viewingVersion']);
  Object.assign(page.context, page.window, bindings);
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

test('the chat composer stays usable while questions wait and sends its message as a reply', async () => {
  const page = await boot([STARTED]);
  await page.context.ensureSession();
  page.context.handleAgentEvent({ type: 'agent_start' });
  page.context.handleAgentEvent({ type: 'exhibit_questions', requestId: 'q1', pending: true });
  assert.equal(page.byId('input').disabled, false);
  let reply;
  page.context.document.addEventListener('exhibit:question-message', event => {
    reply = event.detail;
    event.detail.response = Promise.resolve(true);
  });
  page.byId('input').value = 'Make a shopping list instead.';
  await page.context.send();
  assert.equal(reply.message, 'Make a shopping list instead.');
  assert.equal(reply.requestId, 'q1');
  assert.equal(reply.sessionId, 'conv-1');
  assert.equal(page.byId('input').value, '');
  assert.equal(posts(page.api, '/prompt').length, 0);
});

test('a rejected chat reply preserves the draft for retry', async () => {
  const page = await boot([STARTED]);
  await page.context.ensureSession();
  page.context.handleAgentEvent({ type: 'exhibit_questions', requestId: 'q1', pending: true });
  page.context.document.addEventListener('exhibit:question-message', event => {
    event.detail.response = Promise.resolve(false);
  });
  page.byId('input').value = 'A different direction';
  await page.context.send();
  assert.equal(page.byId('input').value, 'A different direction');
  assert.equal(page.byId('send-btn').disabled, false);
});

test('question replies keep attachments on failure and consume them only after acceptance', async () => {
  const page = await boot([STARTED, json(202, {})]);
  await page.context.ensureSession();
  page.context.handleAgentEvent({ type: 'exhibit_questions', requestId: 'q1', pending: true });
  const frame = page.byId('pv-frame');
  frame.contentWindow = {};
  await page.postFromFrame({ __exSnippet: 'captured', descriptor: { selector: '#counter', tag: 'BUTTON', outerHTML: '<button>Count</button>' }, image: { data: 'base64', mimeType: 'image/png' } }, frame.contentWindow);
  const replies = [];
  page.context.document.addEventListener('exhibit:question-message', event => {
    replies.push(event.detail);
    event.detail.response = Promise.resolve(replies.length > 1);
  });
  page.byId('input').value = 'Make this button larger';
  await page.context.send();
  assert.equal(page.byId('input').value, 'Make this button larger');
  await page.context.send();
  assert.equal(page.byId('input').value, '');
  for (const reply of replies) {
    assert.equal(reply.images[0].data, 'base64');
    assert.equal(reply.images[0].mime_type, 'image/png');
    assert.ok(reply.snippets[0].includes('<button>Count</button>'));
  }
  page.context.handleAgentEvent({ type: 'exhibit_questions', requestId: 'q1', pending: false });
  page.byId('input').value = 'And make the heading bold';
  await page.context.send();
  const next = JSON.parse(posts(page.api, '/prompt')[0].body);
  assert.deepEqual(next.images, []);
  assert.deepEqual(next.snippets, []);
});

test('an attachment added while a reply is sending remains available for the next message', async () => {
  const page = await boot([STARTED, json(202, {})]);
  await page.context.ensureSession();
  page.context.handleAgentEvent({ type: 'exhibit_questions', requestId: 'q1', pending: true });
  const frame = page.byId('pv-frame');
  frame.contentWindow = {};
  const capture = selector => page.postFromFrame({ __exSnippet: 'captured', descriptor: { selector, tag: 'BUTTON' } }, frame.contentWindow);
  await capture('#first');
  let finish;
  page.context.document.addEventListener('exhibit:question-message', event => {
    event.detail.response = new Promise(resolve => { finish = resolve; });
  });
  page.byId('input').value = 'Make the first button larger';
  const sending = page.context.send();
  await capture('#second');
  finish(true);
  await sending;
  page.context.handleAgentEvent({ type: 'exhibit_questions', requestId: 'q1', pending: false });
  page.byId('input').value = 'Now make the second one green';
  await page.context.send();
  const next = JSON.parse(posts(page.api, '/prompt')[0].body);
  assert.equal(next.snippets.length, 1);
  assert.ok(next.snippets[0].includes('#second'));
});

test('Enter cannot resend a chat reply while its successful response is still being displayed', async () => {
  const page = await boot([STARTED]);
  await page.context.ensureSession();
  page.context.handleAgentEvent({ type: 'exhibit_questions', requestId: 'q1', pending: true });
  let finish, replies = 0;
  page.context.document.addEventListener('exhibit:question-message', event => {
    replies++;
    event.detail.response = new Promise(resolve => { finish = resolve; });
  });
  page.byId('input').value = 'A different direction';
  const sending = page.context.send();
  await page.context.document.dispatchEvent({
    type: 'exhibit:question-resolved', detail: { sessionId: 'conv-1', requestId: 'q1' }
  });
  await page.context.send();
  assert.equal(replies, 1);
  assert.equal(posts(page.api, '/prompt').length, 0);
  finish(true);
  await sending;
});

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

test("a rollback that fails detaches its candidate and says the conversation was not continued", async () => {
  const page = await loadWith([STARTED, json(500, { error: "boom" }), json(204, {})]);
  await page.context.resumeConversation("conv-1", 2);

  assert.equal(page.api.calls.filter((c) => c.method === "DELETE" && c.path === "/api/agent/sessions/conv-1").length, 0,
    "an unclaimed candidate is left to the server lease rather than deleting a sidecar another tab can claim");
  assert.match(page.byId("resume-status").textContent, /Could not roll back: boom/);
  assert.match(page.byId("resume-status").textContent, /not continued/);
  assert.equal(page.byId("pane-chat").classList.contains("history-open"), true);
});

// 409 is the artifact already being at that version — nothing to put back, and
// no reason to stop the person getting on with it.
test("an artifact already at that version is not a failure", async () => {
  const page = await loadWith([STARTED, json(409, { error: "that version is already the current one", code: "already_current" })]);
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
  page.document.body.addEventListener("exhibit:artifact-saved", async () => { seen.n++; });
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
  page.byId('version-viewer');
  page.byId("history-messages").innerHTML = '<div class="msg user">make it purple</div>';

  await page.context.resumeConversation("conv-1", 0);

  assert.equal(refreshes.n, 1, "the pane returns to the artifact");
  assert.equal(posts(page.api, "/restore").length, 0, "and the artifact itself was not touched");
});

test("continuing when no version is showing does not reload the pane", async () => {
  const page = await boot([STARTED]);
  const refreshes = countRefreshes(page);
  const lookup = page.document.getElementById;
  page.document.getElementById = id => id === 'version-viewer' ? null : lookup(id);

  await page.context.resumeConversation("conv-1", 0);

  assert.equal(refreshes.n, 0, "nothing changed, so the artifact in the pane is not restarted");
});

test("a rollback re-renders the pane once, whether or not a version was showing", async () => {
  for (const viewing of [true, false]) {
    const page = await boot([STARTED, json(200, {})]);
    const refreshes = countRefreshes(page);
    const lookup = page.document.getElementById;
    page.document.getElementById = id => id === 'version-viewer' && !viewing ? null : lookup(id);

    await page.context.resumeConversation("conv-1", 2);

    assert.equal(refreshes.n, 1, "viewing=" + viewing);
  }
});

test("a missing historical blob is a failed rollback even with HTTP 409", async () => {
  const page = await loadWith([STARTED, json(409, { error: "that version's source is no longer stored" }), json(204, {})]);
  await page.context.resumeConversation("conv-1", 2);
  assert.match(page.byId("resume-status").textContent, /Could not roll back.*not continued/);
  assert.equal(page.byId("pane-chat").classList.contains("history-open"), true);
});

// Streaming writes only the server template's content slot, preserving the
// action row through deltas and creating a separate row for the next block.
test('streamed response blocks keep their own message action shells', async () => {
  const { byId, context } = await loadAgent();
  const shells = [];
  byId('agent-message-template').content = { firstElementChild: {
    cloneNode() {
      const content = { textContent: '' };
      const shell = { content, querySelector: () => content, actions: ['copy'] };
      shells.push(shell);
      return shell;
    }
  } };
  const update = (type, delta) => context.handleAgentEvent({
    type: 'message_update', assistantMessageEvent: { type, delta }
  });
  update('text_delta', 'First');
  update('text_delta', '\nsecond');
  assert.equal(shells.length, 1);
  assert.equal(shells[0].content.textContent, 'First\nsecond');
  assert.deepEqual(shells[0].actions, ['copy']);
  assert.ok(byId('messages').children.includes(shells[0]));
  update('text_end');
  update('text_delta', 'Another block');
  assert.equal(shells.length, 2);
  assert.equal(shells[1].content.textContent, 'Another block');
  assert.equal(shells[0].content.textContent, 'First\nsecond');
});

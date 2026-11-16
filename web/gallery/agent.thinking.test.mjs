/* Exh-s8yu — the agent's working indicator is the server's boiling pencil.
 *
 * The page script decides when the indicator shows; the markup, baked frames
 * included, is the agent-thinking-template the server renders. So thinking_start
 * must clone that template rather than build an icon, and the turn moving on
 * must take the clone away again.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript, recordingApi, nativeDialog, permissionDialog } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const ASSETS = path.join(here, "../../internal/api/assets/gallery");

async function loadAgent() {
  const api = recordingApi([]);
  const page = loadPageScript([path.join(ASSETS, "state-api.js")], {
    TOKEN: "", READ_ONLY: false, BYOK: true, artifact: null,
    apiFetch: api.apiFetch, apiEventSource: () => ({ close() {} }),
    EventSource: function () { return { close() {} }; },
    CustomEvent: class { constructor(t, o) { this.type = t; Object.assign(this, o); } },
    matchMedia: () => ({ matches: false, addEventListener() {} }),
    alert: () => {}, confirm: () => true
  }, { "net-modal": { hidden: true }, "key-modal": { hidden: true } });
  nativeDialog(page, page.byId("key-modal"));
  permissionDialog(page, "net-modal");
  const bindings = await page.loadModule(path.join(ASSETS, "agent.js"), ["handleAgentEvent"]);
  Object.assign(page.context, page.window, bindings);

  // The server-rendered template: every clone is recorded, and knows whether
  // it has been taken off the page.
  const clones = [];
  page.byId("agent-thinking-template").content = { firstElementChild: {
    className: "thinking is-boiling",
    cloneNode(deep) {
      const clone = { className: this.className, deep, removed: false, remove() { this.removed = true; } };
      clones.push(clone);
      return clone;
    }
  } };
  const content = { textContent: "", querySelector: () => null };
  page.byId("agent-message-template").content = { firstElementChild: {
    cloneNode: () => ({ querySelector: () => content })
  } };
  return { ...page, clones };
}

const thinkingStart = { type: "message_update", assistantMessageEvent: { type: "thinking_start" } };

test("thinking_start shows a clone of the server's indicator", async () => {
  const { byId, context, clones } = await loadAgent();
  context.handleAgentEvent(thinkingStart);
  assert.equal(clones.length, 1);
  assert.equal(clones[0].deep, true, "a deep clone carries the baked frames with it");
  assert.equal(clones[0].className, "thinking is-boiling");
  assert.ok(byId("messages").children.includes(clones[0]));
});

test("a second thinking_start in the same turn shows no second indicator", async () => {
  const { context, clones } = await loadAgent();
  context.handleAgentEvent(thinkingStart);
  context.handleAgentEvent(thinkingStart);
  assert.equal(clones.length, 1);
});

test("the indicator goes when the turn moves on, and comes back for the next", async () => {
  const { context, clones } = await loadAgent();
  context.handleAgentEvent(thinkingStart);
  context.handleAgentEvent({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "Done." } });
  assert.equal(clones[0].removed, true, "the answer arriving ends the thinking");

  context.handleAgentEvent(thinkingStart);
  assert.equal(clones.length, 2);
  context.handleAgentEvent({ type: "agent_settled" });
  assert.equal(clones[1].removed, true, "the settled turn takes it away");
});

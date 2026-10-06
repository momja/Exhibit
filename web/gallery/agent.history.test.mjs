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

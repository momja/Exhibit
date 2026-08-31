/* A DOM small enough to read, real enough to run a page script against.
 *
 * The gallery's page scripts are plain files served verbatim from the app
 * origin, with no bundler and no module system, so there is nothing to import
 * and mock. This loads one of them into a `vm` context over the stubs below,
 * which is what lets a test drive the *shipped* bytes — the same file a browser
 * gets — through a whole interaction rather than grepping it for a substring.
 *
 * It is deliberately not jsdom. What these scripts do is narrow (look an
 * element up by id, listen, set textContent, toggle `hidden`, post a message
 * between two windows) and a full DOM implementation would be a large
 * dependency for a workspace whose entire build step is copying files. The
 * shortcut has a cost worth knowing: elements are created on demand rather than
 * parsed from the page's template, so this cannot catch an id the template
 * spells differently from the script. The Go template tests cover that half —
 * they assert the ids exist in the rendered markup.
 */
import { readFileSync } from "node:fs";
import vm from "node:vm";

// An element. Created on first lookup and cached, so a script and its test
// address the same object without either declaring the page's markup.
function makeElement(id) {
  const listeners = {};
  const el = {
    id,
    hidden: false,
    textContent: "",
    title: "",
    src: "",
    value: "",
    dataset: {},
    children: [],
    style: {},
    focused: false,
    className: "",
    classList: {
      _set: new Set(),
      add(c) { this._set.add(c); },
      remove(c) { this._set.delete(c); },
      contains(c) { return this._set.has(c); },
      toggle(c, on) { if (on) this.add(c); else this.remove(c); }
    },
    addEventListener(type, fn) { (listeners[type] ||= []).push(fn); },
    removeEventListener(type, fn) {
      listeners[type] = (listeners[type] || []).filter((f) => f !== fn);
    },
    // Returns a promise settling when every handler has, which a browser has
    // no need for and a test does: these handlers are async (they write a
    // decision through the API before touching the DOM), so a test that only
    // dispatched would assert against the state before the answer landed.
    dispatchEvent(ev) {
      return Promise.all((listeners[ev.type] || []).map((fn) => fn(ev)));
    },
    // A click carries `target` so the delegated handlers (which read
    // e.target.closest) see what a browser would give them.
    click(target) {
      return el.dispatchEvent({ type: "click", target: target || el, id: el.id });
    },
    focus() { el.focused = true; },
    setAttribute() {},
    getAttribute() { return null; },
    appendChild(child) { el.children.push(child); return child; },
    remove() {},
    querySelector() { return null; },
    querySelectorAll() { return []; },
    closest() { return null; },
    listenerCount(type) { return (listeners[type] || []).length; }
  };
  return el;
}

// Builds a window/document pair plus the artifact frame, and returns the
// handles a test needs to act as either side of the postMessage boundary.
// initial gives an element the state its template renders it with — chiefly
// `hidden` on the modal overlays. Without it every dialog would start visible,
// and "the prompt is not showing yet" would be a claim the harness itself made
// false.
// scriptPaths is one path or several, run in order in one shared context —
// the page loads them as sibling <script> tags, so a module one of them
// depends on (network-prompt.js under detail.js) has to be listed first here
// exactly as it is in the template.
export function loadPageScript(scriptPaths, globals = {}, initial = {}) {
  const elements = new Map();
  const byId = (id) => {
    if (!elements.has(id)) elements.set(id, makeElement(id));
    return elements.get(id);
  };
  for (const [id, props] of Object.entries(initial)) Object.assign(byId(id), props);

  const frame = makeElement("artifact-frame");
  // The frame's window is the identity every bridge in detail.js checks
  // (`e.source === frame.contentWindow`), since a sandboxed frame's origin is
  // the string "null" and proves nothing.
  frame.contentWindow = { posted: [], postMessage(msg) { this.posted.push(msg); } };

  const documentListeners = {};
  const windowListeners = {};

  const document = {
    body: makeElement("body"),
    getElementById: byId,
    // The only selector these scripts use is 'iframe'.
    querySelector(sel) { return sel === "iframe" ? frame : null; },
    querySelectorAll() { return []; },
    createElement: (tag) => makeElement(tag),
    addEventListener(type, fn) { (documentListeners[type] ||= []).push(fn); },
    removeEventListener(type, fn) {
      documentListeners[type] = (documentListeners[type] || []).filter((f) => f !== fn);
    },
    get activeElement() { return null; }
  };

  const window = {
    document,
    addEventListener(type, fn) { (windowListeners[type] ||= []).push(fn); },
    removeEventListener(type, fn) {
      windowListeners[type] = (windowListeners[type] || []).filter((f) => f !== fn);
    },
    open() {},
    location: { hash: "", reload() {} }
  };

  // Page scripts reach for both the bare name and the window-prefixed one
  // (`apiFetch(...)` beside `window.matchMedia(...)`), so a supplied global has
  // to exist on both or the harness fails for a reason the page never would.
  Object.assign(window, globals);

  const context = vm.createContext({
    window,
    document,
    location: window.location,
    URL,
    Response,
    Blob,
    Date,
    console,
    setTimeout,
    clearTimeout,
    encodeURIComponent,
    JSON,
    Promise,
    ...globals
  });
  context.globalThis = context;
  for (const path of [].concat(scriptPaths)) {
    vm.runInContext(readFileSync(path, "utf8"), context, { filename: path });
  }

  // Delivers a message as if the artifact frame had posted it. `source`
  // defaults to the frame's window, so a test that wants to prove the identity
  // check works passes something else.
  const postFromFrame = (data, source = frame.contentWindow) =>
    Promise.all(
      (windowListeners.message || []).map((fn) => fn({ data, source, origin: "null" }))
    );

  const pressKey = (key) =>
    (documentListeners.keydown || []).forEach((fn) => fn({ key }));

  return { byId, frame, window, document, context, postFromFrame, pressKey };
}

// A stand-in for api.js's apiFetch that records every call and answers with
// whatever the test queued. Defaults to success, so a test only spells out the
// response it cares about.
export function recordingApi(responses = []) {
  const calls = [];
  const queue = [...responses];
  const apiFetch = (path, opts = {}) => {
    calls.push({ path, method: opts.method || "GET", body: opts.body });
    const next = queue.shift();
    return Promise.resolve(next || { ok: true, status: 200, json: async () => ({}) });
  };
  return { apiFetch, calls };
}

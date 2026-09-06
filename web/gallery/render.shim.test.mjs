/* av-v991 — the frame half of the state channel, run rather than read.
 *
 * The storage shim is a Go string literal (internal/render/render.go's
 * shimTemplate) that becomes the first script of every render document. Nothing
 * on the Go side can assert what it DOES: a Go test can check that a substring
 * is in the emitted bytes, which is exactly how "the diff fires a storage event
 * per changed key" would be got wrong. So this file lifts the shipped template
 * out of render.go, fills in the values the render surface fills in, and runs it
 * over a frame-shaped environment — the same trick testdom.mjs plays for a page
 * script, one origin further in.
 *
 * It is the counterpart to detail.statesync.test.mjs, which drives the host end
 * of the same postMessage channel. Neither file proves the pair works; together
 * they cover both ends of it, and the message shapes are written out in each so
 * a change to one fails the other.
 *
 * Reading render.go rather than a rendered fixture is deliberate: a fixture is a
 * copy, and a copy of security-sensitive code is a thing that goes stale
 * silently. The extraction below is narrow enough to fail loudly if the literal
 * moves.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import vm from "node:vm";

const here = path.dirname(fileURLToPath(import.meta.url));
const RENDER_GO = path.join(here, "../../internal/render/render.go");

const ARTIFACT_ID = "art-1";
const API_ORIGIN = "https://app.test";

// shimSource returns the shim exactly as injectPreamble would compose it, minus
// the surrounding <script> tags. Go raw strings carry no escapes, so the first
// backtick after the opener closes the literal; the format verbs are then
// filled in the order render.go's Sprintf fills them.
function shimSource({ cache = {}, anonymous = false, widget = false } = {}) {
  const go = readFileSync(RENDER_GO, "utf8");
  const opener = "const shimTemplate = `";
  const start = go.indexOf(opener);
  assert.notEqual(start, -1, "shimTemplate moved; this test extracts it by name");
  const from = start + opener.length;
  const end = go.indexOf("`", from);
  assert.notEqual(end, -1, "shimTemplate's raw string literal is unterminated");

  const values = [
    JSON.stringify(ARTIFACT_ID),   // %q ARTIFACT_ID
    JSON.stringify(API_ORIGIN),    // %q API_ORIGIN
    String(widget),                // %t WIDGET
    String(anonymous),             // %t ANONYMOUS
    "[]",                          // %s ALLOWED_ORIGINS
    "[]",                          // %s BLOCKED_ORIGINS
    JSON.stringify(cache),         // %s the inlined state
    ""                             // %s the capability bridges — the widget
                                   //    narrowing, which keeps this harness to
                                   //    the storage half it is about
  ];
  let i = 0;
  return go.slice(from, end)
    .replace(/<\/?script>/g, "")
    .replace(/%[qts]/g, () => values[i++]);
}

// A frame: a window whose parent is somebody else, which is the condition the
// shim installs its host-facing halves under.
function loadShim(opts = {}) {
  const posted = [];
  const listeners = {};
  const dispatched = [];

  // Nothing satisfies this, which is the point: StorageEventInit types
  // storageArea as `Storage?`, so a plain object throws on construction. That
  // is measured browser behaviour (Chromium 149: "Failed to convert value to
  // 'Storage'"), and the shim is written to avoid it — this stub is what makes
  // re-adding the field fail here instead of in a browser.
  class Storage {}
  class StorageEvent {
    constructor(type, init = {}) {
      if ("storageArea" in init && init.storageArea !== null &&
          !(init.storageArea instanceof Storage)) {
        throw new TypeError(
          "Failed to construct 'StorageEvent': Failed to convert value to 'Storage'.");
      }
      this.type = type;
      this.key = init.key === undefined ? null : init.key;
      this.oldValue = init.oldValue === undefined ? null : init.oldValue;
      this.newValue = init.newValue === undefined ? null : init.newValue;
      this.url = init.url || "";
      this.constructedAsStorageEvent = true;
    }
  }

  const parent = { postMessage: (msg) => posted.push(msg) };
  const window = {
    parent,
    onstorage: null,
    addEventListener(type, fn) { (listeners[type] ||= []).push(fn); },
    removeEventListener(type, fn) {
      listeners[type] = (listeners[type] || []).filter((f) => f !== fn);
    },
    dispatchEvent(ev) {
      dispatched.push(ev);
      (listeners[ev.type] || []).forEach((fn) => fn(ev));
      return true;
    }
  };

  const context = vm.createContext({
    window, Object, JSON, String, Promise, console,
    Event, StorageEvent, Storage,
    location: { href: "https://render.test/a/" + ARTIFACT_ID },
    document: { addEventListener() {}, createElement: () => ({}) }
  });
  context.globalThis = context;
  vm.runInContext(shimSource(opts), context, { filename: "shimTemplate" });

  // Delivers a message the way the host frame does: from the parent window, on
  // the app origin.
  const fromHost = (data, { origin = API_ORIGIN, source = parent } = {}) =>
    Promise.all((listeners.message || []).map((fn) => fn({ data, origin, source })));

  // Messages the shim built came out of the vm's realm, so their prototype is
  // not this file's Object.prototype and a strict deep-equal against a literal
  // fails on that alone. Copying the own properties out is what makes the
  // comparison about the message rather than about which realm made it.
  const flat = (msg) => ({ ...msg });

  return { window, posted, dispatched, fromHost, flat, storage: window.localStorage };
}

const sync = (state) => ({ __avStateSync: true, artifactId: ARTIFACT_ID, state });

// --- the diff -------------------------------------------------------------

test("a resync applies adds, changes and deletes to the live cache", async () => {
  const { storage, fromHost } = loadShim({ cache: { keep: "1", change: "old", drop: "x" } });

  await fromHost(sync({ keep: "1", change: "new", added: "y" }));

  assert.equal(storage.getItem("keep"), "1");
  assert.equal(storage.getItem("change"), "new");
  assert.equal(storage.getItem("added"), "y");
  assert.equal(storage.getItem("drop"), null,
    "a key the server no longer holds is gone here too, or the frame keeps a ghost forever");
  assert.equal(storage.length, 3);
});

test("one storage event per changed key, and none for the unchanged", async () => {
  const { dispatched, fromHost } = loadShim({ cache: { keep: "1", change: "old", drop: "x" } });

  await fromHost(sync({ keep: "1", change: "new", added: "y" }));

  const byKey = Object.fromEntries(dispatched.map((e) => [e.key, e]));
  assert.deepEqual(Object.keys(byKey).sort(), ["added", "change", "drop"],
    "'keep' did not change, so reporting it would be a lie about what happened");

  assert.equal(byKey.change.oldValue, "old");
  assert.equal(byKey.change.newValue, "new");
  assert.equal(byKey.added.oldValue, null, "an added key had no previous value");
  assert.equal(byKey.added.newValue, "y");
  assert.equal(byKey.drop.newValue, null, "a removal is newValue null, per the platform");
  assert.equal(byKey.drop.oldValue, "x");

  for (const ev of dispatched) {
    assert.equal(ev.type, "storage");
    assert.equal(ev.constructedAsStorageEvent, true,
      "a real StorageEvent, so an artifact written to the platform contract needs " +
      "nothing new from us — and so the storageArea trap stays caught here");
  }
});

// The whole reason the clear() fix had to land first: `store = {}` rebinds the
// local and leaves the resync writing into an object nothing reads, so an
// artifact that ever calls clear() would go permanently deaf.
test("the cache survives clear(), so a later resync still reaches the artifact", async () => {
  const { storage, fromHost } = loadShim({ cache: { note: "old" } });

  storage.clear();
  assert.equal(storage.length, 0);

  await fromHost(sync({ note: "from somebody else" }));

  assert.equal(storage.getItem("note"), "from somebody else",
    "after a clear the shim must still be reading the object the resync writes");
  assert.equal(storage.length, 1);
});

test("clear() reports itself to the host, as it always did", () => {
  const { storage, posted, flat } = loadShim({ cache: { note: "old" } });
  storage.clear();
  assert.deepEqual(posted.filter((m) => m.__avState).map(flat),
    [{ __avState: true, artifactId: ARTIFACT_ID, op: "clear" }]);
});

// --- what the frame reports back -----------------------------------------

test("the frame reports how much changed and whether anything is listening", async () => {
  const { window, posted, fromHost, flat } = loadShim({ cache: {} });

  await fromHost(sync({ a: "1", b: "2" }));
  const [first] = posted.filter((m) => m.__avStateSynced);
  assert.deepEqual(flat(first), {
    __avStateSynced: true, artifactId: ARTIFACT_ID, changed: 2, live: false
  }, "nothing listened, so the host offers a reload");

  window.addEventListener("storage", () => {});
  await fromHost(sync({ a: "1", b: "3" }));
  const last = posted.filter((m) => m.__avStateSynced).pop();
  assert.equal(last.changed, 1);
  assert.equal(last.live, true,
    "an artifact that listens updated itself; the host must not also nag about it");
});

test("an onstorage handler counts as listening too", async () => {
  const { window, posted, fromHost } = loadShim({ cache: {} });
  window.onstorage = () => {};

  await fromHost(sync({ a: "1" }));

  assert.equal(posted.filter((m) => m.__avStateSynced).pop().live, true,
    "the older spelling of the same contract; missing it would show a reload " +
    "banner over an artifact that is already up to date");
});

test("a resync that changed nothing is reported as nothing", async () => {
  const { posted, fromHost } = loadShim({ cache: { a: "1" } });

  await fromHost(sync({ a: "1" }));

  assert.equal(posted.filter((m) => m.__avStateSynced).pop().changed, 0);
});

// --- who the frame will listen to ----------------------------------------

test("a sync from the wrong origin or the wrong window is ignored", async () => {
  const { storage, fromHost } = loadShim({ cache: { note: "mine" } });

  await fromHost(sync({ note: "injected" }), { origin: "https://evil.test" });
  await fromHost(sync({ note: "injected" }), { source: { postMessage() {} } });
  await fromHost({ __avStateSync: true, artifactId: "someone-else", state: { note: "injected" } });

  assert.equal(storage.getItem("note"), "mine",
    "the host is a real origin, so unlike the messages this frame sends, both " +
    "halves of its identity are checkable — and both are checked");
});

test("an anonymous render accepts no resync at all", async () => {
  const { storage, fromHost, posted } = loadShim({ cache: {}, anonymous: true });

  await fromHost(sync({ note: "somebody's" }));

  assert.equal(storage.getItem("note"), null,
    "a viewer with no principal has no rows to keep in step; their frame is " +
    "rendered with none inlined and persists none either");
  assert.deepEqual(posted.filter((m) => m.__avStateSynced), []);
});

// The honest ceiling, written down as a test so nobody reads liveness as
// collaboration. Two people appending to one JSON blob under one key still lose
// an item: the shim delivers the other person's whole value, and whichever
// write lands second is the one that survives. Liveness shrinks the window from
// "until somebody reloads" to seconds; it does not merge, and cannot here.
test("a resync replaces a value; it does not merge one", async () => {
  const { storage, fromHost } = loadShim({ cache: { list: '["milk"]' } });

  // Both people added an item to the list they each last read.
  storage.setItem("list", '["milk","bread"]');
  await fromHost(sync({ list: '["milk","eggs"]' }));

  assert.equal(storage.getItem("list"), '["milk","eggs"]',
    "last write wins, whole value at a time — 'bread' is gone, and no channel " +
    "at this layer could have saved it");
});

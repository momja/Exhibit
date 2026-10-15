/* av-vw7r — the host half of the version viewer's "nothing here is saved" notice,
 * run rather than read.
 *
 * The render shim in a version view refuses every write and, in place of one,
 * posts a single empty notice (render.shim.test.mjs drives that end). The viewer
 * partial carries the banner, hidden, with its sentence already written; this is
 * the listener in components.js that un-hides it. Neither file proves the pair
 * works — together they cover both ends, and the message's shape is spelled out in
 * each so a change to one fails the other.
 *
 * Loads the built, embedded copy of components.js — the bytes the browser is
 * served — over a document that holds only what this block looks at: the viewer's
 * frame and its banner. Elements are made by hand because the questions are about
 * identity (which window sent it) and effect (what it was allowed to change), and
 * a parsed page would add nothing to either.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const COMPONENTS_JS = path.join(here, "../../internal/api/assets/gallery/components.js");

const NOTICE = { __avVersionUnsaved: true };

// A page with a version viewer in it, unless told otherwise. The viewer is looked
// up by selector at the moment a message arrives — an htmx swap replaces it — so
// the document answers the same selector the script asks, and `closeViewer()` is
// the swap that leaves nothing to find.
function load({ viewer = true } = {}) {
  const listeners = {};
  const versionWindow = { name: "the version viewer's frame" };
  const liveWindow = { name: "the live artifact frame" };
  const banner = { hidden: true };
  const other = { hidden: true };
  let present = viewer;

  const document = {
    readyState: "complete",
    addEventListener() {},
    querySelectorAll() { return []; },
    querySelector(sel) {
      return present && sel === "#version-viewer .version-viewer-frame"
        ? { contentWindow: versionWindow }
        : null;
    },
    getElementById(id) {
      if (id === "version-viewer-unsaved") return present ? banner : null;
      if (id === "some-other-banner") return other;
      return null;
    }
  };
  const window = {
    addEventListener(type, fn) { (listeners[type] ||= []).push(fn); }
  };
  vm.runInNewContext(fs.readFileSync(COMPONENTS_JS, "utf8"), {
    document, window, IntersectionObserver: undefined, CustomEvent: class {}
  });

  // A frame's origin is the string "null" when it is sandboxed, so the only
  // identity a message carries is the window it came from.
  const post = (data, source = versionWindow) =>
    (listeners.message || []).forEach((fn) => fn({ data, source, origin: "null" }));
  return {
    banner, other, versionWindow, liveWindow, post,
    closeViewer() { present = false; }
  };
}

test("the banner is hidden until the version's frame reports a refused write", () => {
  const t = load();
  assert.equal(t.banner.hidden, true, "looking at a version is not an attempt to change it");

  t.post(NOTICE);

  assert.equal(t.banner.hidden, false, "the person is told, in the page's own chrome");
});

test("a notice from any other window is not a version's to send", () => {
  const t = load();

  // The live artifact frame sits on the chat page beside the viewer; it speaks the
  // bridges' language, and this is not one of them. A hand-forged notice from it
  // would otherwise tell the person that the artifact they are looking at is
  // unsaved when it is not.
  t.post(NOTICE, t.liveWindow);
  t.post(NOTICE, null);
  t.post(NOTICE, { name: "somebody else" });

  assert.equal(t.banner.hidden, true);
});

test("only the notice's own shape reveals it", () => {
  const t = load();

  t.post(undefined);
  t.post(null);
  t.post("__avVersionUnsaved");
  t.post({});
  t.post({ __avVersionUnsaved: "true" });
  t.post({ __avVersionUnsaved: 1 });
  t.post({ __avState: true, op: "set", key: "k", value: "v" });

  assert.equal(t.banner.hidden, true,
    "a write message is not a notice, and a version frame's writes are not heard");
});

test("a notice that arrives after the viewer has gone is ignored", () => {
  const t = load();
  t.closeViewer();

  t.post(NOTICE);

  assert.equal(t.banner.hidden, true, "it was a version's notice, and there is no version showing");
});

test("a page with no viewer in it takes no notice", () => {
  const t = load({ viewer: false });

  assert.doesNotThrow(() => t.post(NOTICE));
  assert.equal(t.banner.hidden, true);
});

// The reason it is safe to listen to a frame that runs somebody's code: whatever
// the notice carries, its effect is one fixed thing.
test("a forged notice can do one thing, which is show the notice", () => {
  const t = load();

  t.post({ ...NOTICE, id: "some-other-banner", target: "#pv-frame", op: "set", key: "k", value: "v", html: "<img onerror=x>" });

  assert.equal(t.banner.hidden, false);
  assert.equal(t.other.hidden, true, "nothing in the message chooses what is revealed");
});

import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const WIDGETS_JS = path.join(here, "../../internal/api/assets/gallery/gallery-widgets.js");

// Models the shipped card: the iframe arrives inside a <template>, so the well
// owns no browsing context until the module clones one in. `frame` reads null
// until that happens, which is what the "no browsing context offscreen" tests
// assert on.
async function loadWidgets({ intersection = true } = {}) {
  let visibility;
  let removals;
  class IntersectionObserver {
    constructor(callback, options) {
      this.callback = callback;
      this.options = options;
      this.observed = new Set();
      visibility = this;
    }
    observe(node) { this.observed.add(node); }
    unobserve(node) { this.observed.delete(node); }
    disconnect() { this.observed.clear(); }
  }
  class MutationObserver {
    constructor(callback) { this.callback = callback; removals = this; }
    observe() {}
    disconnect() { this.disconnected = true; }
  }
  const page = loadPageScript([], {
    Event, IntersectionObserver: intersection ? IntersectionObserver : undefined, MutationObserver
  });
  const root = page.byId("gallery");
  root.isConnected = true;
  root.parentNode = page.document.body;
  const widgets = [];
  root.querySelectorAll = (selector) => selector === "[data-gallery-widget]"
    ? widgets.filter((widget) => widget.well.isConnected).map((widget) => widget.well) : [];

  function makeFrame(index, events) {
    const frame = page.byId(`frame-${index}`);
    frame.contentWindow = {};
    frame.src = `https://render.test/w/${index}?t=original-token`;
    frame.addEventListener("exhibit:widget-dispose", () => events.push("dispose"));
    frame.addEventListener("exhibit:widget-mounted", () => events.push("mount"));
    frame.remove = () => { frame.parentNode = null; frame.contentWindow = null; };
    return frame;
  }

  function addWidget() {
    const index = widgets.length;
    const well = page.byId(`well-${index}`);
    const events = [];
    let frame = null;
    const template = { content: { cloneNode: () => makeFrame(index, events) } };
    well.isConnected = true;
    well.dataset.widgetReload = `/artifacts/widget-${index}/widget/open`;
    well.matches = (selector) => selector === "[data-gallery-widget]";
    well.querySelector = (selector) => {
      if (selector === ".card-widget-frame") return frame && frame.parentNode === well ? frame : null;
      if (selector === "template.card-widget-frame-template") return frame && frame.parentNode === well ? null : template;
      return null;
    };
    // The module inserts by appending either a clone of the template's content
    // or the frame node it already holds; both land in the well.
    well.append = (node) => {
      if (node) frame = node;
      frame.parentNode = well;
      frame.contentWindow ||= {};
    };
    const widget = { well, events, get frame() { return frame; } };
    widgets.push(widget);
    return widget;
  }
  const first = addWidget();
  const { initializeGalleryWidgets } = await page.loadModule(WIDGETS_JS);
  initializeGalleryWidgets(root);
  return {
    root, first, addWidget,
    visibility, removals,
    intersect(widget, visible) {
      visibility.callback([{ target: widget.well, isIntersecting: visible }]);
    },
    process(widget) {
      return root.dispatchEvent({ type: "htmx:after:process", target: widget.well });
    },
    remove(widget) {
      widget.well.isConnected = false;
      removals.callback([{ removedNodes: [widget.well] }]);
    }
  };
}

// The whole point of the template: a card that has not come near the viewport
// costs a parsed element and nothing else. A space switch inserts every card at
// once, so this is what stops it paying for 24 browsing contexts.
test("a card off the viewport owns no browsing context", async () => {
  const page = await loadWidgets();
  assert.equal(page.first.frame, null, "the template's iframe must not be materialised");
  assert.deepEqual(page.first.events, []);
  assert.equal(page.visibility.observed.size, 1, "the card is still watched for intersection");
});

test("nearby widgets materialise their frame once and keep the direct render URL", async () => {
  const page = await loadWidgets();
  page.intersect(page.first, true);
  const { frame } = page.first;
  const originalWindow = frame.contentWindow;
  page.intersect(page.first, true);
  await page.process(page.first);

  assert.equal(frame.contentWindow, originalWindow);
  assert.equal(frame.src, "https://render.test/w/0?t=original-token");
  assert.equal(page.first.well.classList.contains("has-frame"), true, "the live frame covers the monogram");
  assert.deepEqual(page.first.events, ["mount"], "repeated intersection does not mount twice");
  assert.equal(page.visibility.options.rootMargin, "400px 0px");
});

test("offscreen widgets lose their document and reload with a fresh credential on re-entry", async () => {
  const page = await loadWidgets();
  page.intersect(page.first, true);
  const { well, events } = page.first;
  const frame = page.first.frame;
  const oldWindow = frame.contentWindow;
  well.classList.add("widget-failed");
  page.intersect(page.first, false);

  assert.equal(frame.parentNode, null);
  assert.equal(frame.contentWindow, null);
  assert.equal(well.classList.contains("has-frame"), false, "the monogram is visible while unloaded");
  assert.equal(well.classList.contains("widget-failed"), false);
  page.intersect(page.first, true);

  assert.equal(frame.parentNode, well);
  assert.notEqual(frame.contentWindow, oldWindow);
  assert.equal(frame.src, well.dataset.widgetReload, "the old render token must never be reused");
  assert.equal(well.classList.contains("has-frame"), true);
  page.intersect(page.first, true);
  assert.deepEqual(events, ["mount", "dispose", "mount"], "repeated intersection does not reload a live frame");
});

test('a protected widget drops its old srcdoc before loading the fresh trusted wrapper', async () => {
  const page = await loadWidgets();
  page.intersect(page.first, true);
  const { well, frame } = page.first;
  frame.dataset.widgetWrapper = '';
  frame.srcdoc = 'old trusted wrapper';
  frame.removeAttribute = name => { if (name === 'srcdoc') delete frame.srcdoc; };
  page.intersect(page.first, false);
  page.intersect(page.first, true);
  assert.equal(frame.srcdoc, undefined);
  assert.equal(frame.src, well.dataset.widgetReload);
});

test("paging observes new widgets and search releases removed ones", async () => {
  const page = await loadWidgets();
  const added = page.addWidget();
  await page.process(added);
  assert.equal(page.visibility.observed.size, 2);
  page.intersect(added, true);
  page.intersect(added, false);
  page.remove(added);
  page.intersect(added, true); // A callback queued before unobserve.

  assert.equal(page.visibility.observed.size, 1);
  assert.equal(added.frame.parentNode, null, "a removed search result must not be resurrected");
  assert.deepEqual(added.events, ["mount", "dispose"]);
});

test("replacing the entire gallery releases every frame and observer", async () => {
  const page = await loadWidgets();
  page.intersect(page.first, true);
  const frame = page.first.frame;
  page.root.isConnected = false;
  page.removals.callback([{ removedNodes: [page.root] }]);

  assert.equal(page.visibility.observed.size, 0);
  assert.equal(page.removals.disconnected, true);
  assert.equal(frame.parentNode, null);
});

test("browsers without IntersectionObserver materialise every frame and leave the fetch to lazy loading", async () => {
  const page = await loadWidgets({ intersection: false });
  assert.equal(page.first.frame.parentNode, page.first.well);
  assert.equal(page.first.frame.src, "https://render.test/w/0?t=original-token");
  assert.equal(page.first.well.classList.contains("has-frame"), true);
  assert.equal(page.visibility, undefined);
});

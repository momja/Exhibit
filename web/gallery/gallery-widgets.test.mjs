import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const WIDGETS_JS = path.join(here, "../../internal/api/assets/gallery/gallery-widgets.js");

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

  function addWidget() {
    const well = page.byId(`well-${widgets.length}`);
    const frame = page.byId(`frame-${widgets.length}`);
    well.isConnected = true;
    well.dataset.widgetReload = `/artifacts/widget-${widgets.length}/widget/open`;
    well.classList.add("has-frame");
    well.matches = (selector) => selector === "[data-gallery-widget]";
    well.querySelector = () => frame.parentNode === well ? frame : null;
    frame.parentNode = well;
    frame.contentWindow = {};
    frame.src = `https://render.test/w/${widgets.length}?t=original-token`;
    const events = [];
    frame.addEventListener("exhibit:widget-dispose", () => events.push("dispose"));
    frame.addEventListener("exhibit:widget-mounted", () => events.push("mount"));
    frame.remove = () => { frame.parentNode = null; frame.contentWindow = null; };
    well.append = () => { frame.parentNode = well; frame.contentWindow = {}; };
    const widget = { well, frame, events };
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

test("nearby widgets retain their initial frame and direct render URL", async () => {
  const page = await loadWidgets();
  const { frame } = page.first;
  const originalWindow = frame.contentWindow;
  page.intersect(page.first, true);
  page.intersect(page.first, true);
  await page.process(page.first);

  assert.equal(frame.contentWindow, originalWindow);
  assert.equal(frame.src, "https://render.test/w/0?t=original-token");
  assert.deepEqual(page.first.events, []);
  assert.equal(page.visibility.observed.size, 1);
  assert.equal(page.visibility.options.rootMargin, "400px 0px");
});

test("offscreen widgets lose their document and reload with a fresh credential on re-entry", async () => {
  const page = await loadWidgets();
  const { well, frame, events } = page.first;
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
  assert.deepEqual(events, ["dispose", "mount"], "repeated intersection does not reload a live frame");
});

test("paging observes new widgets and search releases removed ones", async () => {
  const page = await loadWidgets();
  const added = page.addWidget();
  await page.process(added);
  assert.equal(page.visibility.observed.size, 2);
  page.intersect(added, false);
  page.remove(added);
  page.intersect(added, true); // A callback queued before unobserve.

  assert.equal(page.visibility.observed.size, 1);
  assert.equal(added.frame.parentNode, null, "a removed search result must not be resurrected");
  assert.deepEqual(added.events, ["dispose"]);
});

test("replacing the entire gallery releases every frame and observer", async () => {
  const page = await loadWidgets();
  page.root.isConnected = false;
  page.removals.callback([{ removedNodes: [page.root] }]);

  assert.equal(page.visibility.observed.size, 0);
  assert.equal(page.removals.disconnected, true);
  assert.equal(page.first.frame.parentNode, null);
});

test("browsers without IntersectionObserver retain native lazy loading", async () => {
  const page = await loadWidgets({ intersection: false });
  assert.equal(page.first.frame.parentNode, page.first.well);
  assert.equal(page.visibility, undefined);
});

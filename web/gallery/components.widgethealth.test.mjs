import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";
import { loadPageScript } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const COMPONENTS_JS = path.join(here, "../../internal/api/assets/gallery/components.js");

function loadWidgetHealth(t) {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const observers = new Map();
  class IntersectionObserver {
    constructor(callback) { this.callback = callback; }
    observe(frame) { this.frame = frame; observers.set(frame, this); }
    disconnect() { observers.delete(this.frame); }
  }
  const page = loadPageScript([], { CustomEvent, IntersectionObserver });
  const cards = [];
  const results = page.byId("gallery-results");
  page.document.readyState = "complete";
  results.querySelectorAll = (selector) =>
    selector === ".card-widget-frame" ? cards.map((card) => card.frame) : [];
  page.document.querySelectorAll = results.querySelectorAll;
  vm.runInContext(readFileSync(COMPONENTS_JS, "utf8"), page.context, { filename: COMPONENTS_JS });

  function addCard() {
    const root = page.byId(`card-${cards.length}`);
    const well = page.byId(`well-${cards.length}`);
    const frame = page.byId(`frame-${cards.length}`);
    frame.contentWindow = {};
    frame.closest = (selector) => selector === ".card-widget" ? well : null;
    root.querySelectorAll = (selector) => selector === ".card-widget-frame" ? [frame] : [];
    const card = { root, frame, failed: () => well.classList.contains("widget-failed") };
    cards.push(card);
    return card;
  }

  // htmx 4 processes each inserted root. The swap event instead targets the
  // surviving request source, or the first inserted card after outerHTML.
  async function swap(batch, source = batch[0].root) {
    for (const card of batch) {
      await page.document.dispatchEvent({ type: "htmx:after:process", target: card.root });
    }
    await page.document.dispatchEvent({ type: "htmx:after:swap", target: source });
  }

  return {
    ...page, results, addCard, swap,
    pendingObservations: () => observers.size,
    intersectionCallback: (card) => observers.get(card.frame)?.callback,
    reveal(card) {
      observers.get(card.frame)?.callback([{ target: card.frame, isIntersecting: true }]);
    },
    tick: (milliseconds) => t.mock.timers.tick(milliseconds)
  };
}

test("every appended widget falls back after silence without disturbing a ready widget", async (t) => {
  const page = loadWidgetHealth(t);
  const existing = page.addCard();
  await page.swap([existing]);
  page.reveal(existing);
  await page.postFromFrame({ __avWidget: true, status: "ready" }, existing.frame.contentWindow);

  const appended = Array.from({ length: 24 }, () => page.addCard());
  await page.swap(appended);
  appended.forEach(page.reveal);
  page.tick(5999);
  assert.ok(appended.every((card) => !card.failed()));
  page.tick(1);

  assert.equal(appended.filter((card) => card.failed()).length, 24,
    "the first card's swap event must not leave the rest of its batch unwatched");
  assert.equal(existing.failed(), false, "paging must not restart a ready widget's deadline");
});

test("reprocessing a gallery preserves deadlines and waits for offscreen widgets", async (t) => {
  const page = loadWidgetHealth(t);
  const visible = page.addCard();
  const offscreen = page.addCard();
  await page.swap([visible, offscreen]);
  page.reveal(visible);
  page.tick(3000);
  await page.document.dispatchEvent({ type: "htmx:after:process", target: page.results });
  page.tick(3000);

  assert.equal(visible.failed(), true, "reprocessing must not postpone a running deadline");
  assert.equal(offscreen.failed(), false, "an offscreen widget has not had a chance to answer");
  page.reveal(offscreen);
  page.tick(5999);
  assert.equal(offscreen.failed(), false);
  page.tick(1);
  assert.equal(offscreen.failed(), true);
});

test("search results get health watchers when the swap event targets the search form", async (t) => {
  const page = loadWidgetHealth(t);
  const results = Array.from({ length: 3 }, () => page.addCard());
  await page.swap(results, page.byId("search-form"));
  results.forEach(page.reveal);
  page.tick(6000);

  assert.ok(results.every((card) => card.failed()),
    "the request source need not contain any of the new widget frames");
});

test("a widget ready before becoming visible never starts a later failure deadline", async (t) => {
  const page = loadWidgetHealth(t);
  const card = page.addCard();
  await page.swap([card]);
  const queuedIntersection = page.intersectionCallback(card);
  await page.postFromFrame({ __avWidget: true, status: "ready" }, card.frame.contentWindow);

  assert.equal(page.pendingObservations(), 0);
  queuedIntersection([{ target: card.frame, isIntersecting: true }]);
  page.tick(6000);
  assert.equal(card.failed(), false);
});

test("a ready report before frame initialization survives later htmx processing", async (t) => {
  const page = loadWidgetHealth(t);
  const card = page.addCard();
  await page.postFromFrame({ __avWidget: true, status: "ready" }, card.frame.contentWindow);
  await page.swap([card]);
  page.reveal(card);
  page.tick(6000);

  assert.equal(card.failed(), false);
  assert.equal(page.pendingObservations(), 0);
});

test("unloading cancels health work and remounting starts one fresh watcher", async (t) => {
  const page = loadWidgetHealth(t);
  const card = page.addCard();
  await page.swap([card]);
  page.reveal(card);
  page.tick(3000);
  await card.frame.dispatchEvent({ type: "exhibit:widget-dispose" });
  page.tick(6000);

  assert.equal(card.failed(), false);
  assert.equal(card.frame.dataset.widgetWatched, undefined);
  assert.equal(card.frame.listenerCount("__avresolved"), 0);
  assert.equal(card.frame.listenerCount("exhibit:widget-dispose"), 0);
  assert.equal(page.pendingObservations(), 0);
  await page.document.dispatchEvent({ type: "exhibit:widget-mounted", target: card.frame });
  assert.equal(card.frame.listenerCount("__avresolved"), 1);
  page.reveal(card);
  page.tick(6000);
  assert.equal(card.failed(), true);
});

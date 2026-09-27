/* The capability popover's Manage link must survive a tap in Safari.
 *
 * Safari does not focus an anchor on click, so tapping "Manage security
 * settings" blurs the trigger toward nothing: focusout arrives with a null
 * relatedTarget BEFORE the click. The popover used to close on that, which
 * hid it (pointer-events:none) before the click was dispatched, so the link
 * never navigated on iOS. These tests replay that event order.
 *
 * Loads internal/api/assets/gallery/components.js — the built, embedded copy
 * the browser is served — so an edit that was never rebuilt fails here.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const COMPONENTS_JS = path.join(here, "../../internal/api/assets/gallery/components.js");

// A tree just deep enough for components.js: closest() by class or
// attribute, contains(), and a classList on the wrap.
function el(name, { classes = [], attrs = {} } = {}) {
  const node = {
    name, parent: null, children: [], attrs: { ...attrs },
    cls: new Set(classes),
    classList: {
      contains: (c) => node.cls.has(c),
      toggle: (c, on) => { on ? node.cls.add(c) : node.cls.delete(c); },
    },
    setAttribute(k, v) { node.attrs[k] = v; },
    append(...kids) { for (const k of kids) { k.parent = node; node.children.push(k); } return node; },
    matches(sel) {
      if (sel.startsWith(".")) return node.cls.has(sel.slice(1));
      if (sel.startsWith("[")) return sel.slice(1, -1) in node.attrs;
      return false;
    },
    closest(sel) {
      for (let n = node; n; n = n.parent) if (n.matches(sel)) return n;
      return null;
    },
    contains(other) {
      for (let n = other; n; n = n.parent) if (n === node) return true;
      return false;
    },
    querySelector(sel) {
      for (const k of node.children) {
        if (k.matches(sel)) return k;
        const hit = k.querySelector(sel);
        if (hit) return hit;
      }
      return null;
    },
  };
  return node;
}

function load() {
  const trigger = el("trigger", { classes: ["capability-cluster"], attrs: { "data-capability-trigger": "" } });
  const manage = el("a", { classes: ["capability-popover-manage"] });
  const popover = el("popover", { classes: ["capability-popover"] }).append(manage);
  const wrap = el("wrap", { classes: ["capability-wrap"] }).append(trigger, popover);
  const elsewhere = el("elsewhere");
  const body = el("body").append(wrap, elsewhere);

  const listeners = {};
  const document = {
    readyState: "complete",
    addEventListener(type, fn) { (listeners[type] ||= []).push(fn); },
    querySelectorAll(sel) {
      const out = [];
      const walk = (n) => { if (sel === ".capability-wrap.is-open" ? n.cls.has("capability-wrap") && n.cls.has("is-open") : false) out.push(n); n.children.forEach(walk); };
      walk(body);
      return out;
    },
    querySelector(sel) { return this.querySelectorAll(sel)[0] || null; },
  };
  const fire = (type, ev) => (listeners[type] || []).forEach((fn) => fn(ev));
  vm.runInNewContext(fs.readFileSync(COMPONENTS_JS, "utf8"), {
    document,
    window: { addEventListener() {} },
    IntersectionObserver: undefined,
    CustomEvent: class {},
  });
  return { trigger, manage, wrap, elsewhere, fire, isOpen: () => wrap.cls.has("is-open") };
}

test("tapping Manage in Safari leaves the popover open for the link's click", () => {
  const p = load();
  p.fire("click", { target: p.trigger });
  assert.ok(p.isOpen(), "trigger click opens the popover");

  // Safari: the trigger blurs toward nothing, then the click hits the link.
  p.fire("focusout", { target: p.trigger, relatedTarget: null });
  assert.ok(p.isOpen(), "a focusout naming no destination must not hide the link mid-tap");
});

test("tabbing out of the popover still closes it", () => {
  const p = load();
  p.fire("click", { target: p.trigger });
  p.fire("focusout", { target: p.manage, relatedTarget: p.elsewhere });
  assert.ok(!p.isOpen());
});

test("tabbing from the trigger to the Manage link keeps it open", () => {
  const p = load();
  p.fire("click", { target: p.trigger });
  p.fire("focusout", { target: p.trigger, relatedTarget: p.manage });
  assert.ok(p.isOpen());
});

test("tapping elsewhere still dismisses it", () => {
  const p = load();
  p.fire("click", { target: p.trigger });
  p.fire("focusout", { target: p.trigger, relatedTarget: null });
  p.fire("click", { target: p.elsewhere });
  assert.ok(!p.isOpen());
});

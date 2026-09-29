/* Drives the built index.js through the gallery's `/` search shortcut (av-6mdw). */
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const INDEX_JS = path.join(here, "../../internal/api/assets/gallery/index.js");

function loadIndex() {
  // The harness makes bare elements; the shortcut tells a field from the page
  // by tagName, so the search box needs the one a browser would give it.
  const page = loadPageScript([INDEX_JS], { TOKEN: "", READ_ONLY: false }, {
    "search-input": { tagName: "INPUT" }
  });
  const input = page.byId("search-input");

  // Presses a key the way a browser delivers it: at whatever element has focus,
  // with modifiers, and with a preventDefault the test can check was called.
  const press = (key, { target = page.document.body, ...mods } = {}) => {
    const ev = { target, prevented: false, preventDefault() { ev.prevented = true; }, ...mods };
    page.pressKey(key, ev);
    return ev;
  };
  return { ...page, input, press };
}

test("/ from the page focuses search, selects the query, and keeps the slash out of it", () => {
  const { input, press } = loadIndex();
  const ev = press("/");
  assert.equal(input.focused, true);
  assert.equal(input.selected, true, "typing after / should replace the old query");
  assert.equal(ev.prevented, true, "without preventDefault the slash lands in the box");
});

test("/ typed into a field belongs to that field", () => {
  const { input, press } = loadIndex();
  const fields = [
    { tagName: "INPUT" },
    { tagName: "TEXTAREA" },
    { tagName: "SELECT" },
    { tagName: "DIV", isContentEditable: true }
  ];
  for (const target of fields) {
    const ev = press("/", { target });
    assert.equal(ev.prevented, false, `stole a slash from ${target.tagName}`);
  }
  assert.equal(input.focused, false);
});

test("a slash typed into the search box itself is a character, not the shortcut", () => {
  const { input, press } = loadIndex();
  const ev = press("/", { target: input });
  assert.equal(ev.prevented, false);
  assert.equal(input.selected, undefined, "reselecting would overwrite the query mid-typing");
});

test("Ctrl, Cmd and Alt with / are left to whatever else binds them", () => {
  const { input, press } = loadIndex();
  for (const mod of ["ctrlKey", "metaKey", "altKey"]) {
    const ev = press("/", { [mod]: true });
    assert.equal(ev.prevented, false, `${mod}+/ was intercepted`);
  }
  assert.equal(input.focused, false);
});

test("Shift is allowed, since some layouts type / as Shift+7", () => {
  const { input, press } = loadIndex();
  press("/", { shiftKey: true });
  assert.equal(input.focused, true);
});

test("a / another handler already consumed is not taken twice", () => {
  const { input, press } = loadIndex();
  press("/", { defaultPrevented: true });
  assert.equal(input.focused, false);
});

test("other keys do nothing", () => {
  const { input, press } = loadIndex();
  for (const key of ["?", "s", "Escape", "Enter"]) press(key);
  assert.equal(input.focused, false);
});

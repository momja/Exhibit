/* Drives the built index.js through the gallery's `/` search shortcut (av-6mdw). */
import { test } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadPageScript } from "./testdom.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const INDEX_JS = path.join(here, "../../internal/api/assets/gallery/index.js");

async function loadIndex({ multiple = false } = {}) {
  // The harness makes bare elements; the shortcut tells a field from the page
  // by tagName, so the search box needs the one a browser would give it.
  const page = loadPageScript([], { TOKEN: "", READ_ONLY: false, Event }, {
    "search-input": { tagName: "INPUT" }
  });
  page.window.location.href = 'https://app.test/';
  function galleryFixture(prefix) {
    const root = page.byId(prefix);
    const input = page.byId(prefix + '-input');
    input.tagName = 'INPUT';
    const form = page.byId(prefix + '-form');
    const clear = page.byId(prefix + '-clear');
    const error = page.byId(prefix + '-error');
    root.querySelector = (selector) => ({
      '[data-gallery-search]': form, '[data-gallery-query]': input,
      '[data-gallery-clear]': clear, '[data-gallery-search-error]': error
    })[selector] || null;
    root.closest = () => root;
    form.closest = (selector) => selector === '[data-gallery]' ? root : null;
    return { root, input, form, clear, error };
  }
  const first = galleryFixture('first-gallery');
  const second = multiple ? galleryFixture('second-gallery') : null;
  const { root, input, form, clear } = first;
  page.document.querySelectorAll = () => second ? [root, second.root] : [root];
  await page.loadModule(INDEX_JS);

  // Presses a key the way a browser delivers it: at whatever element has focus,
  // with modifiers, and with a preventDefault the test can check was called.
  const press = (key, { target = page.document.body, ...mods } = {}) => {
    if (!target.closest) target.closest = () => null;
    const ev = { target, prevented: false, preventDefault() { ev.prevented = true; }, ...mods };
    page.pressKey(key, ev);
    return ev;
  };
  return { ...page, root, form, clear, input, press, second };
}

test("/ from the page focuses search, selects the query, and keeps the slash out of it", async () => {
  const { input, press } = await loadIndex();
  const ev = press("/");
  assert.equal(input.focused, true);
  assert.equal(input.selected, true, "typing after / should replace the old query");
  assert.equal(ev.prevented, true, "without preventDefault the slash lands in the box");
});

test("/ typed into a field belongs to that field", async () => {
  const { input, press } = await loadIndex();
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

test("a slash typed into the search box itself is a character, not the shortcut", async () => {
  const { input, press } = await loadIndex();
  const ev = press("/", { target: input });
  assert.equal(ev.prevented, false);
  assert.equal(input.selected, undefined, "reselecting would overwrite the query mid-typing");
});

test("Ctrl, Cmd and Alt with / are left to whatever else binds them", async () => {
  const { input, press } = await loadIndex();
  for (const mod of ["ctrlKey", "metaKey", "altKey"]) {
    const ev = press("/", { [mod]: true });
    assert.equal(ev.prevented, false, `${mod}+/ was intercepted`);
  }
  assert.equal(input.focused, false);
});

test("Shift is allowed, since some layouts type / as Shift+7", async () => {
  const { input, press } = await loadIndex();
  press("/", { shiftKey: true });
  assert.equal(input.focused, true);
});

test("a / another handler already consumed is not taken twice", async () => {
  const { input, press } = await loadIndex();
  press("/", { defaultPrevented: true });
  assert.equal(input.focused, false);
});

test("other keys do nothing", async () => {
  const { input, press } = await loadIndex();
  for (const key of ["?", "s", "Escape", "Enter"]) press(key);
  assert.equal(input.focused, false);
});

function htmxEvent(type, ctx) {
  return { type, detail: { ctx }, defaultPrevented: false,
    preventDefault() { this.defaultPrevented = true; } };
}

test("changing search discards an old response during the debounce and body read", async () => {
  const { root, form, input } = await loadIndex();
  form.closest = (selector) => selector === '[data-gallery]' ? root : null;
  const ctx = { sourceElement: form, request: { action: '/partials/gallery-grid?q=', signal: { aborted: false } } };
  await root.dispatchEvent(htmxEvent('htmx:before:request', ctx));
  input.value = 'new';
  await input.dispatchEvent({ type: 'input' });
  for (const name of ['htmx:before:response', 'htmx:after:request', 'htmx:before:swap']) {
    const event = htmxEvent(name, ctx);
    await root.dispatchEvent(event);
    assert.equal(event.defaultPrevented, true, name);
  }
  await form.dispatchEvent({ type: 'submit' }); // cancel debounce timer
});

test("an old sentinel cannot start a page for the previous query", async () => {
  const { root, form, input } = await loadIndex();
  input.value = 'new';
  await input.dispatchEvent({ type: 'input' });
  const source = { closest: (selector) => selector === '[data-gallery]' ? root : null };
  const event = htmxEvent('htmx:before:request', {
    sourceElement: source, request: { action: '/partials/gallery-grid?offset=24' }
  });
  await root.dispatchEvent(event);
  assert.equal(event.defaultPrevented, true);
  await form.dispatchEvent({ type: 'submit' });
});

test("request revisions belong to one gallery, even with the same query", async () => {
  const first = await loadIndex({ multiple: true });
  const { second } = first;
  const ctx = { sourceElement: first.form, request: { action: '/partials/first' } };
  first.form.closest = (selector) => selector === '[data-gallery]' ? first.root : null;
  await first.root.dispatchEvent(htmxEvent('htmx:before:request', ctx));
  second.input.value = 'other';
  await second.input.dispatchEvent({ type: 'input' });
  const event = htmxEvent('htmx:before:response', ctx);
  await first.root.dispatchEvent(event);
  assert.equal(event.defaultPrevented, false);
  await second.form.dispatchEvent({ type: 'submit' });
});

test('an artifact change clears a filtered gallery and invalidates in-flight old search results', async () => {
  const { root, form, input } = await loadIndex();
  input.value = 'old title';
  await input.dispatchEvent({ type: 'input' });
  const ctx = { sourceElement: form, request: { action: '/partials/gallery-grid?q=old%20title', signal: { aborted: false } } };
  await root.dispatchEvent(htmxEvent('htmx:before:request', ctx));
  let requested = 0;
  form.addEventListener('exhibit:gallery-search', () => requested++);
  await root.dispatchEvent({ type: 'exhibit:artifacts-changed' });
  assert.equal(input.value, '');
  assert.equal(requested, 1);
  const stale = htmxEvent('htmx:before:swap', ctx);
  await root.dispatchEvent(stale);
  assert.equal(stale.defaultPrevented, true);
});

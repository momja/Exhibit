// One in-memory snapshot per unfiltered tab. The lifetime is shorter than
// the credentials embedded in the server-rendered widget templates.
export const gallerySnapshotMaxAge = 60_000;

export function widgetSource(value) {
  return value?.replace(/\?t=o=[\w.=-]+/g, '?t=');
}

function parseFragment(html) {
  const template = document.createElement('template');
  template.innerHTML = html;
  return template.content;
}

function comparableRegions(html) {
  const fragment = parseFragment(html);
  for (const template of fragment.querySelectorAll('template.card-widget-frame-template')) {
    const frame = template.content.firstElementChild;
    for (const attr of ['src', 'srcdoc']) {
      const value = frame.getAttribute(attr);
      if (value) frame.setAttribute(attr, widgetSource(value));
    }
  }
  return [...fragment.children];
}

export function gallerySpaceSnapshot(space) {
  const content = space.cloneNode(true);
  for (const well of content.querySelectorAll('[data-gallery-widget].has-frame')) {
    well.querySelector('.card-widget-frame')?.remove();
    well.classList.remove('has-frame', 'widget-failed');
  }
  const search = document.getElementById('gallery-search-region').cloneNode(true);
  search.setAttribute('hx-swap-oob', 'outerHTML:#gallery-search-region');
  for (const root of [content, search]) {
    root.querySelectorAll('[data-htmx-powered]').forEach(node => node.removeAttribute('data-htmx-powered'));
  }
  return document.querySelector('title').outerHTML + content.outerHTML + search.outerHTML;
}

export class GallerySpaceCache {
  #entries = new Map();

  read(path) {
    const entry = this.#entries.get(path);
    if (entry && performance.now() - entry.savedAt < gallerySnapshotMaxAge) return entry.html;
    this.#entries.delete(path);
  }

  save(path, html) {
    this.#entries.set(path, { html, savedAt: performance.now() });
  }

  matches(path, html) {
    const cached = this.read(path);
    if (!cached) return false;
    if (cached === html) return true;
    // Widget URLs carry new credentials on each response. Their renewal
    // doesn't change the rendered tab or require restarting live iframes.
    const before = comparableRegions(cached);
    const after = comparableRegions(html);
    return before.length === after.length && before.every((region, i) => region.isEqualNode(after[i]));
  }

  appendPage(path, action, html) {
    const cached = this.read(path);
    if (!cached) return;
    const fragment = parseFragment(cached);
    const sentinel = fragment.querySelector('[data-gallery-page]');
    if (sentinel?.getAttribute('hx-get') !== action) return;
    // Keep the original timestamp. Appending a page doesn't renew the widget
    // credentials of cards that were already in the snapshot.
    sentinel.closest('.gallery-more').replaceWith(parseFragment(html));
    const template = document.createElement('template');
    template.content.append(fragment);
    this.#entries.get(path).html = template.innerHTML;
  }

  delete(path) {
    this.#entries.delete(path);
  }

  clear() {
    this.#entries.clear();
  }
}

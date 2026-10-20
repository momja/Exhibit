// Shared behavior for server-defined galleries. Endpoints and paging URLs live
// in their templates; this module owns only typing, shortcuts and request races.
import { initializeGalleryWidgets } from './gallery-widgets.js';
import { initializeCardActions } from './gallery-actions.js';

export function initializeGallery(root) {
  initializeGalleryWidgets(root);
  initializeCardActions(root);
  const form = root.querySelector('[data-gallery-search]');
  const input = root.querySelector('[data-gallery-query]');
  const clear = root.querySelector('[data-gallery-clear]');
  const searchError = root.querySelector('[data-gallery-search-error]');
  let query = input.value.trim();
  let revision = 0;
  let timer;
  const requests = new WeakMap();

  function search() {
    clearTimeout(timer);
    form.dispatchEvent(new Event('exhibit:gallery-search', { bubbles: true }));
  }

  function syncClear() {
    clear.hidden = input.value.length === 0;
  }

  input.addEventListener('input', () => {
    syncClear();
    if (query === input.value.trim()) return;
    query = input.value.trim();
    revision++;
    clearTimeout(timer);
    timer = setTimeout(search, 220);
  });
  form.addEventListener('submit', () => clearTimeout(timer));
  clear.addEventListener('click', () => {
    input.value = '';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.focus();
    search();
  });
  syncClear();

  // hx-sync coordinates searches and pages within this gallery. Revisions
  // also cover the debounce interval and a response already reading its body:
  // neither may swap old results or update the URL after the query changes.
  root.addEventListener('htmx:before:request', (event) => {
    const { ctx } = event.detail;
    if (ctx.sourceElement.closest('[data-gallery]') !== root) return;
    const requestQuery = (new URL(ctx.request.action, window.location.href).searchParams.get('q') || '').trim();
    if (requestQuery !== query) {
      event.preventDefault();
      return;
    }
    requests.set(ctx, revision);
    searchError.hidden = true;
    const error = ctx.sourceElement.closest('.gallery-more')?.querySelector('[data-gallery-load-error]');
    if (error) error.hidden = true;
  });
  for (const name of ['htmx:before:response', 'htmx:after:request', 'htmx:before:swap']) {
    root.addEventListener(name, (event) => {
      const { ctx } = event.detail;
      if (requests.has(ctx) && requests.get(ctx) !== revision) event.preventDefault();
    });
  }
  root.addEventListener('htmx:error', (event) => {
    const { ctx } = event.detail;
    if (!requests.has(ctx) || requests.get(ctx) !== revision || ctx.request.signal.aborted) return;
    const error = ctx.sourceElement.closest('.gallery-more')?.querySelector('[data-gallery-load-error]');
    (error || searchError).hidden = false;
  });

  root.addEventListener('click', (event) => {
    if (event.target.closest('a, button, .card-actions, [data-capability-trigger], .capability-popover')) return;
    const card = event.target.closest('.card');
    if (card?.dataset.href) window.location.href = card.dataset.href;
  });
}

export function initializeGalleries() {
  const galleries = [...document.querySelectorAll('[data-gallery]')];
  galleries.forEach(initializeGallery);
  document.addEventListener('keydown', (event) => {
    if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey || event.defaultPrevented) return;
    const target = event.target;
    if (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return;
    const gallery = target.closest('[data-gallery]') || galleries[0];
    if (!gallery) return;
    event.preventDefault();
    const input = gallery.querySelector('[data-gallery-query]');
    input.focus();
    input.select();
  });
}

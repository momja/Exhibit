import { Controller } from '../stimulus/stimulus.js';
import { initializeGallery } from './gallery.js';
import { eventScope } from './event-scope.js';
import { GallerySpaceCache, gallerySpaceSnapshot, gallerySnapshotMaxAge } from './gallery-space-cache.js';
import { canKeepGalleryViewport, captureGalleryViewport, restoreGalleryViewport } from './gallery-viewport.js';

const spacePaths = new Set(['/', '/search', '/shared', '/shared-by-me', '/activity']);
const tabSelector = '.gallery-spaces [data-gallery-space-link]';
const scrollIdleMs = 150;

function tabPath(url) {
  return !url.search && spacePaths.has(url.pathname) ? url.pathname : null;
}

function spaceURL(path, count) {
  // The server's existing editor-return parameter can render the whole
  // loaded range in one response, including an anchor on a later page.
  return count > 24 ? path + '?visible=' + Math.min(count, 480) : path;
}

// The controller, header and account island live for the whole document.
// htmx owns requests, history and markup; this boundary invalidates work that
// belongs to a space we are leaving, including responses still reading a body.
export default class extends Controller {
  static targets = ['space'];

  get spaceElement() {
    return this.hasSpaceTarget ? this.spaceTarget : null;
  }

  // The tab that was clicked is inside the region the response replaces, so its
  // aria-current cannot move until that response lands. Answer the click now:
  // point at the destination and dim what is still on screen. Content still
  // waits for the server — this only stops the tab bar from looking dead.
  markPending(link) {
    if (!link) return;
    for (const other of this.spaceLinks()) other.removeAttribute('aria-current');
    link.setAttribute('aria-current', 'page');
    this.element.setAttribute('data-gallery-pending', '');
    this.spaceElement?.setAttribute('aria-busy', 'true');
  }

  // The response arrived and its markup is in place: its own aria-current is
  // authoritative, so only the shell's pending flag is cleared here.
  clearPending() {
    this.element.removeAttribute('data-gallery-pending');
    this.spaceElement?.removeAttribute('aria-busy');
  }

  // The swap was refused, so the old region is still what is displayed — and
  // this controller took its aria-current away. Put it back where the URL that
  // is still on screen says it belongs.
  restorePendingLink() {
    if (!this.element.hasAttribute('data-gallery-pending')) return;
    const showing = new URL(this.currentURL, location.href).pathname;
    for (const link of this.spaceLinks()) {
      if (new URL(link.getAttribute('href'), location.href).pathname === showing) link.setAttribute('aria-current', 'page');
      else link.removeAttribute('aria-current');
    }
  }

  spaceLinks() {
    return this.element.querySelectorAll('[data-gallery-space-link]');
  }

  spaceTargetConnected() {
    this.gallery?.disconnect();
    this.gallery = initializeGallery(this.element);
  }

  connect() {
    this.events = eventScope();
    this.requests = new Map();
    this.cache = new GallerySpaceCache();
    this.viewports = new Map();
    this.pointers = new Set();
    this.lastScroll = -Infinity;
    this.revision = 0;
    this.currentURL = location.pathname + location.search;
    const initialTab = tabPath(new URL(location.href));
    if (initialTab && this.element.querySelector(tabSelector)) {
      this.cache.save(initialTab, gallerySpaceSnapshot(this.spaceElement));
    }
    const on = (name, callback) => this.events.on(document, name, callback);
    on('htmx:before:history:restore', () => this.leave());
    on('htmx:before:request', event => this.beforeRequest(event));
    for (const name of ['htmx:before:response', 'htmx:after:request']) {
      on(name, event => this.beforeResponse(event));
    }
    on('htmx:before:swap', event => this.beforeSwap(event));
    on('htmx:after:swap', event => this.afterSwap(event.detail.ctx));
    on('htmx:finally:swap', event => this.restoreNativeAnchoring(event.detail.ctx));
    on('htmx:finally:request', event => this.finished(event));
    on('htmx:after:history:update', () => {
      this.currentURL = location.pathname + location.search;
    });
    // htmx already preserves Ctrl/Cmd/Shift clicks. Leave Alt clicks to the
    // browser too, before htmx's click handler can prevent their default.
    this.events.on(this.element, 'click', event => {
      if (event.altKey && event.target.closest('[data-gallery-space-link]')) event.stopPropagation();
    }, true);
    const moved = () => {
      this.lastScroll = performance.now();
      this.retryRefresh();
    };
    for (const name of ['scroll', 'wheel']) this.events.on(window, name, moved, { passive: true });
    this.events.on(document, 'pointerdown', event => this.pointers.add(event.pointerId), true);
    for (const name of ['pointerup', 'pointercancel']) {
      this.events.on(document, name, event => {
        this.pointers.delete(event.pointerId);
        this.retryRefresh();
      }, true);
    }
    this.events.on(window, 'blur', () => {
      this.pointers.clear();
      this.retryRefresh();
    });
    for (const name of ['click', 'keydown', 'focusout', 'close']) {
      this.events.on(document, name, () => this.retryRefresh(), true);
    }
    this.events.on(this.element, 'input', () => this.cancelRefresh());
    for (const name of ['exhibit:artifacts-changed', 'exhibit:capabilities-changed']) {
      this.events.on(this.element, name, () => {
        this.cache.clear();
        this.cancelRefresh();
      });
    }
  }

  leave() {
    this.revision++;
    this.navigating = true;
    this.cancelRefresh();
    this.gallery?.suspend();
    for (const ctx of this.requests.keys()) ctx.request.abort?.();
  }

  beforeRequest(event) {
    const { ctx } = event.detail;
    if (event.defaultPrevented || !this.element.contains(ctx.sourceElement)) return;
    if (ctx.galleryRefresh) {
      if (ctx.galleryRefresh !== this.refresh) event.preventDefault();
      else ctx.galleryRefresh.request = ctx;
      return;
    }
    const url = new URL(ctx.request.action, location.href);
    const path = url.pathname;
    const navigation = url.origin === location.origin && spacePaths.has(path);
    const from = this.currentURL;
    const tab = tabPath(url);
    const tabClick = navigation && ctx.sourceElement.matches(tabSelector);
    let cached;
    if (navigation) {
      // htmx history defaults to outerSync, which retains controller roots.
      // Replace this region so every space gets a fresh component lifecycle.
      ctx.swap = 'outerHTML';
      // htmx 4 history supplies its own request options, omitting the core
      // HX-Request header. Keep fragment rendering and session expiry on
      // the same safe path as link navigation.
      ctx.request.headers['HX-Request'] = 'true';
      ctx.request.headers['HX-Target'] = 'div#gallery-content';
      this.leave();
      this.element.querySelector('[data-gallery-navigation-error]').hidden = true;
      // Only a deliberate tab click anticipates a destination. History
      // traversal, retries and programmatic navigation get no optimistic
      // state: the browser has already moved the URL for those.
      if (ctx.sourceElement.matches?.('[data-gallery-space-link]')) this.markPending(ctx.sourceElement);
      if (tabClick && tab && tab !== from) {
        cached = this.cache.read(tab);
        if (cached) {
          ctx.fetch = () => new Response(cached, { headers: { 'Content-Type': 'text/html' } });
        } else {
          const fetch = ctx.fetch;
          ctx.fetch = (_, options) => fetch(spaceURL(tab, this.viewports.get(tab)?.count), options);
        }
        // A widened result range is only a request detail, not a new URL.
        ctx.push = tab;
      }
    } else if (this.navigating) {
      event.preventDefault();
      return;
    } else if (ctx.target?.matches('[data-gallery-results]')) {
      this.cancelRefresh();
    }
    this.requests.set(ctx, {
      revision: this.revision, navigation, from, tab, cached,
      page: !navigation && ctx.sourceElement.matches('[data-gallery-page]'),
      focusSearch: navigation && path === '/search' && ctx.sourceElement.matches('[data-gallery-space-link]'),
    });
  }

  beforeResponse(event) {
    const { ctx } = event.detail;
    if (ctx.galleryRefresh) {
      if (ctx.galleryRefresh !== this.refresh || ctx.request.signal?.aborted ||
          (ctx.response.status !== 200 && ctx.response.status !== 401)) event.preventDefault();
      return;
    }
    const request = this.requests.get(ctx);
    if (!request) return;
    if (request.revision !== this.revision || ctx.request.signal?.aborted) {
      event.preventDefault();
    } else if (request.navigation && ctx.response.status >= 400 && ctx.response.status !== 401) {
      // A failed destination cannot replace a complete, working space with
      // an error body or change its URL. 401 retains the session HX-Refresh.
      event.preventDefault();
      request.failed = true;
    }
  }

  beforeSwap(event) {
    this.beforeResponse(event);
    if (event.defaultPrevented) return;
    const { ctx, tasks } = event.detail;
    if (ctx.galleryRefresh) {
      this.prepareRefresh(event, ctx.galleryRefresh, tasks);
      return;
    }
    const request = this.requests.get(ctx);
    if (!request?.navigation) return;
    const outgoing = tabPath(new URL(request.from, location.href));
    if (outgoing && outgoing !== request.tab) {
      this.viewports.set(outgoing, captureGalleryViewport(this.spaceElement));
    }
    this.prepareSpaceSwap(ctx, tasks);
  }

  prepareSpaceSwap(ctx, tasks) {
    // htmx's settle delay can paint the new grid before scroll restoration.
    // Keep all parts of this swap, including the header controls, in one turn.
    for (const { swapSpec } of tasks) {
      swapSpec.settle = 0;
      swapSpec.transition = false;
    }
    const style = document.documentElement.style;
    ctx.galleryOverflowAnchor = style.overflowAnchor;
    style.overflowAnchor = 'none';
  }

  restoreNativeAnchoring(ctx) {
    if (ctx.galleryOverflowAnchor === undefined) return;
    document.documentElement.style.overflowAnchor = ctx.galleryOverflowAnchor;
    delete ctx.galleryOverflowAnchor;
  }

  afterSwap(ctx) {
    const refresh = ctx.galleryRefresh;
    if (refresh === this.refresh && refresh?.viewport) {
      restoreGalleryViewport(this.spaceElement, refresh.viewport);
      this.element.dispatchEvent(new CustomEvent('exhibit:gallery-scroll-restored'));
      this.cache.save(refresh.path, ctx.text);
      this.refresh = null;
      return;
    }
    const request = this.requests.get(ctx);
    if (!request || request.revision !== this.revision) return;
    if (request.navigation) {
      const viewport = request.tab && request.from !== request.tab && this.viewports.get(request.tab);
      restoreGalleryViewport(this.spaceElement, viewport);
      this.element.dispatchEvent(new CustomEvent('exhibit:gallery-scroll-restored', { detail: { revealChrome: true } }));
      if (request.tab && !request.cached && ctx.response.status === 200) {
        this.cache.save(request.tab, ctx.text);
      }
    } else if (ctx.response.status === 200) {
      const path = tabPath(new URL(location.href));
      if (path && request.page) this.cache.appendPage(path, ctx.request.action, ctx.text);
      else if (ctx.target?.matches('[data-gallery-results]')) this.cache.delete(location.pathname);
    }
  }

  refreshTab(path) {
    const refresh = { path };
    this.refresh = refresh;
    window.htmx.ajax('GET', spaceURL(path, this.viewports.get(path)?.count), {
      source: this.spaceElement,
      target: this.spaceElement,
      swap: 'outerHTML',
      galleryRefresh: refresh,
    }).finally(() => {
      if (this.refresh === refresh && !refresh.pending) this.cancelRefresh();
    });
  }

  prepareRefresh(event, refresh, tasks) {
    const main = tasks.find(task => task.type === 'main');
    const incoming = main?.fragment.firstElementChild;
    if (incoming?.id !== this.spaceElement.id || !tasks.every(task =>
      task.target?.isConnected && task.fragment.firstElementChild &&
      task.swapSpec.style === 'outerHTML')) {
      event.preventDefault();
      return;
    }
    const { ctx } = event.detail;
    if (refresh.receivedAt === undefined) {
      refresh.receivedAt = performance.now();
      if (this.cache.matches(refresh.path, ctx.text)) {
        this.cache.save(refresh.path, ctx.text);
        event.preventDefault();
        return;
      }
    }
    const scrolling = performance.now() - this.lastScroll < scrollIdleMs;
    const busy = scrolling || this.pointers.size > 0 ||
      [...this.requests.values()].some(request => request.revision === this.revision) ||
      this.spaceElement.contains(document.activeElement) ||
      document.getElementById('gallery-search-region').contains(document.activeElement) ||
      this.element.querySelector('dialog[open], .card [aria-expanded="true"]');
    if (!busy) {
      const viewport = captureGalleryViewport(this.spaceElement);
      this.prepareSpaceSwap(ctx, tasks);
      if (canKeepGalleryViewport(tasks, incoming, viewport)) {
        refresh.viewport = viewport;
        return;
      }
      // Wait for the reader to move to a part of the tab we can update
      // without displacing their current view.
    }
    refresh.pending = ctx;
    event.preventDefault();
    if (scrolling) this.retryRefresh();
  }

  retryRefresh() {
    const refresh = this.refresh;
    if (!refresh?.pending) return;
    clearTimeout(this.refreshTimer);
    this.refreshTimer = setTimeout(() => {
      if (this.refresh !== refresh) return;
      if (performance.now() - refresh.receivedAt >= gallerySnapshotMaxAge) {
        this.cancelRefresh();
        this.refreshTab(refresh.path);
      } else {
        window.htmx.swap(refresh.pending);
      }
    }, Math.max(0, this.lastScroll + scrollIdleMs - performance.now()));
  }

  cancelRefresh() {
    clearTimeout(this.refreshTimer);
    this.refresh?.request?.request.abort?.();
    this.refresh = null;
  }

  finished(event) {
    const { ctx } = event.detail;
    const request = this.requests.get(ctx);
    this.requests.delete(ctx);
    if (!request || request.revision !== this.revision) return;
    if (!request.navigation) {
      this.retryRefresh();
      return;
    }
    this.navigating = false;
    if (request.failed || ctx.status.startsWith('error:')) {
      // History traversal moves the URL before fetching. Keep the previous
      // coherent view and URL on failure, with a notice for an explicit retry.
      history.replaceState(history.state, '', this.currentURL);
      this.restorePendingLink();
      this.clearPending();
      this.element.querySelector('[data-gallery-navigation-error]').hidden = false;
    } else {
      this.clearPending();
      this.currentURL = location.pathname + location.search;
      // A Search-tab click (including the / shortcut) carries its focus intent
      // through the async swap. History restores keep their own focus context.
      if (request.focusSearch) {
        const input = this.element.querySelector('[data-gallery-query]');
        input.focus({ preventScroll: true });
        input.select();
      }
      if (request.cached) this.refreshTab(request.tab);
    }
  }

  disconnect() {
    this.cancelRefresh();
    this.gallery?.disconnect();
    this.events.disconnect();
    for (const ctx of this.requests.keys()) ctx.request.abort?.();
  }
}

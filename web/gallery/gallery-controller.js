import { Controller } from '../stimulus/stimulus.js';
import { initializeGallery } from './gallery.js';
import { eventScope } from './event-scope.js';

const spacePaths = new Set(['/', '/search', '/shared', '/shared-by-me', '/activity']);

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
    this.revision = 0;
    this.currentURL = location.pathname + location.search;
    const on = (name, callback) => this.events.on(document, name, callback);
    on('htmx:before:history:restore', () => this.leave());
    on('htmx:before:request', event => this.beforeRequest(event));
    for (const name of ['htmx:before:response', 'htmx:after:request', 'htmx:before:swap']) {
      on(name, event => this.beforeResponse(event));
    }
    on('htmx:finally:request', event => this.finished(event));
    on('htmx:after:history:update', () => {
      this.currentURL = location.pathname + location.search;
    });
    // htmx already preserves Ctrl/Cmd/Shift clicks. Leave Alt clicks to the
    // browser too, before htmx's click handler can prevent their default.
    this.events.on(this.element, 'click', event => {
      if (event.altKey && event.target.closest('[data-gallery-space-link]')) event.stopPropagation();
    }, true);
  }

  leave() {
    this.revision++;
    this.navigating = true;
    this.gallery?.suspend();
    for (const ctx of this.requests.keys()) ctx.request.abort?.();
  }

  beforeRequest(event) {
    const { ctx } = event.detail;
    if (!this.element.contains(ctx.sourceElement)) return;
    const path = new URL(ctx.request.action, location.href).pathname;
    const navigation = spacePaths.has(path);
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
    } else if (this.navigating) {
      event.preventDefault();
      return;
    }
    this.requests.set(ctx, {
      revision: this.revision, navigation,
      focusSearch: navigation && path === '/search' && ctx.sourceElement.matches('[data-gallery-space-link]'),
    });
  }

  beforeResponse(event) {
    const { ctx } = event.detail;
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

  finished(event) {
    const { ctx } = event.detail;
    const request = this.requests.get(ctx);
    this.requests.delete(ctx);
    if (!request?.navigation || request.revision !== this.revision) return;
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
      window.scrollTo(0, 0);
      // A Search-tab click (including the / shortcut) carries its focus intent
      // through the async swap. History restores keep their own focus context.
      if (request.focusSearch) {
        const input = this.element.querySelector('[data-gallery-query]');
        input.focus({ preventScroll: true });
        input.select();
      }
    }
  }

  disconnect() {
    this.gallery?.disconnect();
    this.events.disconnect();
    for (const ctx of this.requests.keys()) ctx.request.abort?.();
  }
}

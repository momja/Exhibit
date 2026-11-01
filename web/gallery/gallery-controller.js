import { Controller } from '../stimulus/stimulus.js';
import { initializeGallery } from './gallery.js';
import { eventScope } from './event-scope.js';

const spacePaths = new Set(['/', '/shared', '/shared-by-me']);

// The controller, header and account island live for the whole document.
// htmx owns requests, history and markup; this boundary invalidates work that
// belongs to a space we are leaving, including responses still reading a body.
export default class extends Controller {
  static targets = ['space'];

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
    } else if (this.navigating) {
      event.preventDefault();
      return;
    }
    this.requests.set(ctx, { revision: this.revision, navigation });
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
      this.element.querySelector('[data-gallery-navigation-error]').hidden = false;
    } else {
      this.currentURL = location.pathname + location.search;
      window.scrollTo(0, 0);
    }
  }

  disconnect() {
    this.gallery?.disconnect();
    this.events.disconnect();
    for (const ctx of this.requests.keys()) ctx.request.abort?.();
  }
}

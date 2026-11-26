// Keep widget documents near the viewport. The card and its server-rendered
// iframe node survive paging; removing the iframe destroys its live document.
//
// The frame arrives inside a <template> (cardWidget), so a card owns no
// browsing context until this module clones one in. That is deliberate: a
// gallery is inserted wholesale, and building one context per card dominated
// the cost of every library space switch.
export function initializeGalleryWidgets(root) {
  const selector = '[data-gallery-widget]';
  const frameSelector = '.card-widget-frame';
  const templateSelector = 'template.card-widget-frame-template';
  if (typeof IntersectionObserver !== 'function') {
    materialiseAll(root);
    return;
  }
  const widgets = new Map();
  const visibility = new IntersectionObserver((entries) => {
    for (const entry of entries) {
      const widget = widgets.get(entry.target);
      if (!widget || !entry.target.isConnected) continue;
      if (entry.isIntersecting) mount(entry.target, widget);
      else unmount(entry.target, widget);
    }
  }, { rootMargin: '400px 0px' });

  function unmount(well, widget) {
    if (!widget.frame || widget.frame.parentNode !== well) return;
    widget.frame.dispatchEvent(new Event('exhibit:widget-dispose'));
    widget.frame.remove();
    well.classList.remove('has-frame', 'widget-failed');
    widget.reload = true;
  }

  // Clones the server-rendered frame out of its template. The clone lands in
  // the well, which is what gives it a document and starts the load.
  function insertFrame(well, widget) {
    const template = well.querySelector(templateSelector);
    if (!template) return;
    well.append(template.content.cloneNode(true));
    widget.frame = well.querySelector(frameSelector);
  }

  function mount(well, widget) {
    if (widget.frame?.parentNode === well) return;
    if (!widget.frame) {
      insertFrame(well, widget);
      if (!widget.frame) return;
    } else {
      // Re-entry may be hours later. The app-origin redirect mints a fresh
      // render credential on demand rather than reusing the page's old token.
      if (widget.reload) {
        if (widget.frame.dataset.widgetWrapper !== undefined) widget.frame.removeAttribute('srcdoc');
        widget.frame.src = well.dataset.widgetReload;
      }
      well.append(widget.frame);
    }
    well.classList.add('has-frame');
    widget.frame.dataset.galleryMountedAt = performance.now();
    widget.frame.dispatchEvent(new Event('exhibit:widget-mounted', { bubbles: true }));
  }

  function visit(node, callback) {
    if (node.matches?.(selector)) callback(node);
    node.querySelectorAll?.(selector).forEach(callback);
  }

  function register(well) {
    if (widgets.has(well)) return;
    const frame = well.querySelector(frameSelector);
    if (!frame && !well.querySelector(templateSelector)) return;
    widgets.set(well, { frame, reload: false });
    visibility.observe(well);
  }

  function release(well) {
    const widget = widgets.get(well);
    if (!widget) return;
    unmount(well, widget);
    visibility.unobserve(well);
    widgets.delete(well);
  }

  // Search removes cards; the observer and cached iframe must not retain them.
  // Watching the parent also handles replacement of this entire gallery.
  const removals = new MutationObserver((records) => {
    if (!root.isConnected) {
      widgets.forEach((_, well) => release(well));
      visibility.disconnect();
      removals.disconnect();
      return;
    }
    for (const record of records) {
      for (const node of record.removedNodes) {
        visit(node, (well) => { if (!well.isConnected) release(well); });
      }
    }
  });
  removals.observe(root, { childList: true, subtree: true });
  if (root.parentNode) removals.observe(root.parentNode, { childList: true });
  const process = event => visit(event.target, register);
  root.addEventListener('htmx:after:process', process);
  visit(root, register);
  return () => {
    root.removeEventListener('htmx:after:process', process);
    removals.disconnect();
    widgets.forEach((_, well) => release(well));
    visibility.disconnect();
  };

  // No observer to hang the lifecycle on: clone every frame in now and let
  // each frame's own loading="lazy" decide when to fetch it. Widgets keep
  // their fallback monogram either way.
  function materialiseAll(scope) {
    scope.querySelectorAll(selector).forEach((well) => {
      const widget = { frame: well.querySelector(frameSelector), reload: false };
      if (!widget.frame) insertFrame(well, widget);
      if (!widget.frame) return;
      well.classList.add('has-frame');
    });
  }
}

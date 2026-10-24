// Keep widget documents near the viewport. The card and its server-rendered
// iframe node survive paging; removing the iframe destroys its live document.
export function initializeGalleryWidgets(root) {
  if (typeof IntersectionObserver !== 'function') return;

  const selector = '[data-gallery-widget]';
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
    if (widget.frame.parentNode !== well) return;
    widget.frame.dispatchEvent(new Event('exhibit:widget-dispose'));
    widget.frame.remove();
    well.classList.remove('has-frame', 'widget-failed');
    widget.reload = true;
  }

  function mount(well, widget) {
    if (widget.frame.parentNode === well) return;
    // Re-entry may be hours later. The app-origin redirect mints a fresh
    // render credential on demand rather than reusing the page's old token.
    if (widget.reload) {
      if (widget.frame.dataset.widgetWrapper !== undefined) widget.frame.removeAttribute('srcdoc');
      widget.frame.src = well.dataset.widgetReload;
    }
    well.classList.add('has-frame');
    well.append(widget.frame);
    widget.frame.dispatchEvent(new Event('exhibit:widget-mounted', { bubbles: true }));
  }

  function visit(node, callback) {
    if (node.matches?.(selector)) callback(node);
    node.querySelectorAll?.(selector).forEach(callback);
  }

  function register(well) {
    if (widgets.has(well)) return;
    const frame = well.querySelector('.card-widget-frame');
    if (!frame) return;
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
  root.addEventListener('htmx:after:process', (event) => visit(event.target, register));
  visit(root, register);
}

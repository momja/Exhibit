const itemSelector = '.card[data-href], [data-gallery-anchor]';
const resultItems = '[data-gallery-results] > :is(' + itemSelector + ')';
const itemKey = item => item.dataset.href || item.dataset.galleryAnchor;

export function captureGalleryViewport(space) {
  const { scrollY: top, innerHeight } = window;
  const anchors = [];
  if (top > 0) {
    const header = document.querySelector('[data-app-header]');
    const visibleTop = Math.max(0, header?.getBoundingClientRect().bottom ?? 0);
    for (const item of space.querySelectorAll(itemSelector)) {
      const rect = item.getBoundingClientRect();
      if (rect.bottom > visibleTop && rect.top < innerHeight) {
        anchors.push({ key: itemKey(item), offset: rect.top });
      }
    }
  }
  return { top, anchors, count: space.querySelectorAll(resultItems).length };
}

function findAnchor(space, viewport) {
  const items = new Map(Array.from(space.querySelectorAll(itemSelector), item => [itemKey(item), item]));
  for (const { key, offset } of viewport.anchors) {
    if (items.has(key)) return { item: items.get(key), offset };
  }
  return null;
}

export function scrollGalleryTo(top) {
  window.scrollTo({ top, behavior: 'instant' });
}

export function restoreGalleryViewport(space, viewport) {
  const anchor = viewport && findAnchor(space, viewport);
  scrollGalleryTo(anchor
    ? window.scrollY + anchor.item.getBoundingClientRect().top - anchor.offset
    : viewport?.top ?? 0);
}

// Measure the incoming server markup in its real layout. A surviving item
// isn't sufficient if the new page is too short to scroll it into position.
// The old nodes stay connected and the measurement is undone in this task,
// before htmx commits the swap or the browser can paint.
export function canKeepGalleryViewport(tasks, incoming, viewport) {
  if (!viewport.top) return true;
  if (!findAnchor(incoming, viewport)) return false;
  const staged = tasks.map(({ target, fragment }) => ({
    target,
    display: target.style.display,
    replacement: fragment.firstElementChild.cloneNode(true),
  }));
  try {
    for (const { target, replacement } of staged) {
      replacement.inert = true;
      target.before(replacement);
      target.style.display = 'none';
    }
    const space = staged.find(({ replacement }) => replacement.id === incoming.id).replacement;
    const anchor = findAnchor(space, viewport);
    const page = document.scrollingElement;
    const wanted = window.scrollY + anchor.item.getBoundingClientRect().top - anchor.offset;
    return wanted >= -0.5 && wanted <= page.scrollHeight - page.clientHeight + 0.5;
  } finally {
    for (const { target, display, replacement } of staged) {
      replacement.remove();
      target.style.display = display;
    }
    if (window.scrollY !== viewport.top) scrollGalleryTo(viewport.top);
  }
}

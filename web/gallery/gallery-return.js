// Preserve library context as data, never cached markup. The server renders
// the loaded result count again; this tab restores the matching scroll offset.
function contextKey(url) {
  const key = new URL(url);
  key.searchParams.delete('visible');
  key.searchParams.sort();
  return 'exhibit:gallery-return:' + key.pathname + key.search;
}

export function initializeGalleryReturn(root) {
  if (new URL(location.href).searchParams.has('visible')) {
    try {
      const saved = JSON.parse(sessionStorage.getItem(contextKey(location.href)));
      if (saved && Date.now() - saved.when < 30 * 60 * 1000) {
        requestAnimationFrame(() => window.scrollTo(0, saved.scroll));
      }
    } catch { /* Storage may be disabled; search and filters still survive. */ }
  }
  root.addEventListener('click', event => {
    const link = event.target.closest('a[href]');
    if (!link || !/\/artifacts\/[^/]+\/edit$/.test(new URL(link.href).pathname)) return;
    const destination = new URL(location.href);
    destination.hash = '';
    destination.searchParams.set('visible', Math.min(480, root.querySelectorAll('.card').length));
    const edit = new URL(link.href);
    edit.searchParams.set('return_to', destination.pathname + destination.search);
    link.href = edit.href;
    try {
      sessionStorage.setItem(contextKey(destination.href), JSON.stringify({when: Date.now(), scroll: window.scrollY}));
    } catch { /* Returning remains an ordinary link when storage is unavailable. */ }
  }, true);
}

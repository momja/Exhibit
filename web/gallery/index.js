/* Gallery index (library) page script. Served from the app origin at
 * /assets/gallery/index.js. The page's inline bootstrap <script> defines the
 * per-request globals this file reads before it loads:
 *   TOKEN / READ_ONLY - this visitor's API credential, decided server-side
 *                       per request (av-5imk); spent via api.js's apiFetch
 */

// Eager search: filter the gallery as the user types instead of waiting for a
// submit. A debounced fetch re-asks the server-rendered gallery page with the
// current query and swaps only the .grid contents, so search stays authoritative
// (it runs the same FTS query as the form did) while the delegated card
// handler stays untouched. The empty query lists all.
(function() {
  const input = document.getElementById('search-input');
  const clear = document.getElementById('search-clear');
  if (!input) return;
  let timer = null;
  let lastQ = input.value.trim();
  syncClear();
  input.addEventListener('input', function() {
    syncClear();
    clearTimeout(timer);
    timer = setTimeout(runSearch, 220);
  });
  input.addEventListener('keydown', function(e) { if (e.key === 'Enter') { e.preventDefault(); clearTimeout(timer); runSearch(); } });
  if (clear) clear.addEventListener('click', function() { input.value = ''; syncClear(); input.focus(); clearTimeout(timer); runSearch(); });
  function syncClear() { if (clear) clear.hidden = !input.value; }
  function runSearch() {
    const q = input.value.trim();
    if (q === lastQ) return;
    lastQ = q;
    const grid = document.querySelector('.grid');
    if (!grid) return;
    grid.classList.add('grid-loading');
    const url = q ? '/?q=' + encodeURIComponent(q) : '/';
    fetch(url, { headers: { 'X-Requested-With': 'gallery-search' }, credentials: 'same-origin' })
      .then(function(r) { return r.ok ? r.text() : Promise.reject(r.statusText); })
      .then(function(html) {
        const doc = new DOMParser().parseFromString(html, 'text/html');
        const fresh = doc.querySelector('.grid');
        grid.innerHTML = fresh ? fresh.innerHTML : '';
        grid.classList.remove('grid-loading');
        if (history.replaceState) history.replaceState(null, '', url);
      })
      .catch(function() { grid.classList.remove('grid-loading'); });
  }
})();

// Clicking anywhere on a card opens that artifact's detail/viewer page — the
// card itself is the way in. Clicks on an interactive child (the title or
// Edit link, or the capability cluster, whose own click toggles its popover
// in components.js) are left alone so those keep their own behavior.
// The 'Open' card action was removed; this is the single open affordance per
// card.
document.addEventListener('click', function(e) {
  if (e.target.closest('a, button, .card-actions, [data-capability-trigger], .capability-popover')) return;
  const card = e.target.closest('.card');
  if (!card || !card.dataset.href) return;
  window.location.href = card.dataset.href;
});

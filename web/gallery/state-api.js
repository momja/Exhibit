/* Shared state-endpoint URL construction — served from the app origin at
 * /assets/gallery/state-api.js, loaded by every page that touches artifact
 * state: detail.tmpl and agent.tmpl (the storage-bridge listeners, which
 * write through on the sandboxed artifact's behalf) and edit.tmpl (the state
 * inspector).
 *
 * This exists as one definition on purpose (av-hh1o). The three callers each
 * built the delete URL themselves, and the same defect was in all three: a
 * state key went into the URL as a PATH SEGMENT, and encodeURIComponent does
 * not escape '.', so a key of ".." was resolved away by the browser's URL
 * parser before the request was sent —
 *
 *     /api/artifacts/abc/state/..   ->   /api/artifacts/abc/
 *
 * which is the artifact delete route. An artifact calling
 * localStorage.removeItem('..') could therefore make the host frame delete
 * the artifact with the host's own bearer token. Keys of "" and "." collapsed
 * the same way to a trailing slash and silently 404'd instead of deleting.
 *
 * The fix is structural rather than an escaping patch: the key travels as a
 * QUERY VALUE, which has no segment structure to normalize. That also makes
 * the empty-string key representable (there is no empty path segment, but
 * there is an empty query value) and stops a long key from overflowing the
 * request line on delete when it was settable via the PUT body.
 */
(function() {
  var base = function(artifactID) {
    return '/api/artifacts/' + encodeURIComponent(artifactID) + '/state';
  };

  window.ExhibitState = {
    // Read/write endpoint for the whole state map.
    url: base,

    // Delete endpoint. Pass a key to remove one row; omit it entirely to
    // erase all state. The server discriminates on the PRESENCE of the `key`
    // parameter, not its value, so deleting the empty-string key (legitimate:
    // '' is a valid Web Storage key) stays distinct from erase-all. Callers
    // must therefore pass no argument for erase-all rather than passing ''.
    deleteURL: function(artifactID, key) {
      if (key === undefined) return base(artifactID);
      return base(artifactID) + '?key=' + encodeURIComponent(key);
    }
  };
})();

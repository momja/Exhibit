/* The page's API credential, and the one place it is spent (av-5imk).
 *
 * A server-rendered page authenticates its own fetches with whatever the
 * request that rendered it was entitled to — never with the process's static
 * token by default. The server states that credential as TOKEN in the page's
 * bootstrap <script> (internal/api/pagecredential.go):
 *
 *   - TOKEN '' — a session-authenticated browser uses its revocable cookie.
 *     Handing it a bearer token would give it a second, operator-strength
 *     credential that logging out cannot revoke.
 *   - TOKEN set — on a no-login instance, the visitor is the operator and
 *     the static token is the page's credential. When API auth is disabled,
 *     this value is empty too.
 *
 * READ_ONLY separately describes the page's gallery or artifact: a granted
 * recipient may read another owner's artifact but may not change it. Writes
 * are refused here rather than sent to be refused by the server.
 * STATE_WRITABLE says whether this visitor may write the artifact's saved
 * data, which a granted recipient may do even on a read-only artifact
 * (av-v991). apiStateFetch below spends that authority.
 *
 * Page scripts call apiFetch, never fetch-plus-a-hand-built header, so each
 * call site uses the same credential and local write policy.
 */
(function() {
  // TOKEN and READ_ONLY are read at call time rather than captured at load
  // time, so this file carries no ordering requirement against the inline
  // bootstrap that declares them — only the requirement that a page which
  // calls the API declares them at all. `typeof` covers a page (the 404) that
  // loads no bootstrap and makes no API call either.
  function token() {
    return typeof TOKEN === 'string' ? TOKEN : '';
  }

  function readOnly() {
    // Space switches retain this credential client. Authority for the active
    // gallery travels in its server-rendered region, never a stale bootstrap.
    var space = document.querySelector('[data-gallery-read-only]');
    if (space) return space.dataset.galleryReadOnly !== 'false';
    return typeof READ_ONLY === 'boolean' ? READ_ONLY : false;
  }

  // Whether this visitor may write the artifact's own saved state (av-v991).
  // Decided server-side alongside the two above, and false unless the page says
  // otherwise — a page that never declares it writes no state, which is the
  // fail-closed reading.
  function stateWritable() {
    return typeof STATE_WRITABLE === 'boolean' ? STATE_WRITABLE : false;
  }

  // apiHeaders builds the headers for one API call: the caller's, plus the
  // bearer token when this page was given one.
  function apiHeaders(extra) {
    var headers = Object.assign({}, extra || {});
    var t = token();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    return headers;
  }

  // A refused write answers in the shape the server would have used, so the
  // `if (!r.ok)` branch every caller already has reports it in that page's own
  // words. Degrading to read-only must not look like a network failure.
  function refused(message) {
    return new Response(
      JSON.stringify({error: message || 'This Gallery is read-only — sign in to make changes.'}),
      {status: 403, statusText: 'Forbidden', headers: {'Content-Type': 'application/json'}}
    );
  }

  function isWrite(method) {
    return method !== 'GET' && method !== 'HEAD';
  }

  // send performs one credentialed call. It is deliberately the only place a
  // request is actually built: the two entry points below differ in what they
  // REFUSE, never in what they send, so a second refusal rule can never come
  // with a second credential rule attached.
  function send(path, opts) {
    opts = Object.assign({}, opts || {});
    opts.headers = apiHeaders(opts.headers);
    // Only the app supplies a frame's current policy. A guest state message
    // cannot choose or omit this header, including on recipient writes.
    var statePath = /^\/api\/artifacts\/([^/]+)\/state(?:\/import)?(?:\?|$)/.exec(path);
    if (statePath) {
      var root = document.querySelector('[data-relay-host-artifact-id-value]');
      var frameId = root?.dataset.relayHostArtifactIdValue;
      var policy = root?.dataset.relayHostPolicyValue;
      if (!frameId && typeof ID === 'string' && typeof NAVIGATION_POLICY === 'string') {
        frameId = ID;
        policy = NAVIGATION_POLICY;
      }
      if (frameId && typeof policy === 'string' && statePath[1] === encodeURIComponent(frameId)) {
        for (var name of Object.keys(opts.headers)) {
          if (name.toLowerCase() === 'x-exhibit-relay-policy') delete opts.headers[name];
        }
        opts.headers['X-Exhibit-Relay-Policy'] = policy;
      }
    }
    var hasContentType = Object.keys(opts.headers).some(function(k) {
      return k.toLowerCase() === 'content-type';
    });
    if (opts.body !== undefined && opts.body !== null && !hasContentType) {
      opts.headers['Content-Type'] = 'application/json';
    }
    return fetch(path, opts);
  }

  // apiFetch is fetch against this app's API, credentialed for this visitor.
  // A JSON body gets its Content-Type here so callers stop repeating it.
  window.apiFetch = function(path, opts) {
    var method = ((opts && opts.method) || 'GET').toUpperCase();
    if (readOnly() && isWrite(method)) return Promise.resolve(refused());
    return send(path, opts);
  };

  // apiStateFetch is the artifact-state write path, and the only place a
  // READ_ONLY visitor's write is allowed out (av-v991).
  //
  // The two flags answer different questions and this is where that shows.
  // READ_ONLY says "this artifact is not yours to change" — its body, its
  // allowlist, its capability approvals. The data an artifact saves while
  // somebody uses it is not the artifact; it belongs to whoever typed it, and
  // refusing it here would leave a granted tool forgetting everything on every
  // reload, which is not a shared tool at all. So a recipient's page is
  // READ_ONLY and STATE_WRITABLE at once, and an anonymous reader is neither.
  //
  // The exception is gated on a server-supplied flag rather than on a route
  // pattern this file would have to recognise, and it is spent by exactly one
  // caller: the host frame's storage bridge. The server refuses the same set
  // again — the state routes are authenticated, and the store resolves the
  // artifact through the grant predicate — so nothing here is what holds the
  // line; it stops the page sending a write that would only be refused.
  //
  // Reads do not come through here. A GET is not a write, so the resync uses
  // apiFetch like anything else.
  window.apiStateFetch = function(path, opts) {
    var method = ((opts && opts.method) || 'GET').toUpperCase();
    if (isWrite(method) && !stateWritable()) {
      return Promise.resolve(refused('This artifact cannot save data for you.'));
    }
    return send(path, opts);
  };

  // This POST validates an existing viewer capability and grants nothing.
  // Recipients may use it without acquiring any owner editing authority.
  window.apiNavigationCheck = function(artifactId, destination, policy) {
    return send('/api/artifacts/' + encodeURIComponent(artifactId) + '/navigation-check', {
      method: 'POST',
      headers: { 'X-Exhibit-Relay-Policy': policy },
      body: JSON.stringify({ url: destination })
    });
  };

  // apiEventSource is the one API call apiFetch cannot make: EventSource sets
  // no headers, so whatever credential the stream carries has to travel in the
  // query string. What travels there is a *ticket* — single-use, seconds-lived,
  // bound to one session, and minted by the caller over an ordinary
  // authenticated request — never the service token, which a URL would leak
  // into this service's debug request log, the operator's proxy log, and
  // browser history (av-rgp1).
  //
  // A page holding no token needs no ticket either: EventSource sends cookies
  // on a same-origin stream, so the session authenticates it and nothing at all
  // appears in the URL. Which of the two applies is this function's to know, so
  // callers pass a ticket and stay out of it.
  window.apiEventSource = function(path, ticket) {
    if (!token()) return new EventSource(path);
    return new EventSource(path + (path.indexOf('?') === -1 ? '?' : '&') +
      'ticket=' + encodeURIComponent(ticket || ''));
  };
})();

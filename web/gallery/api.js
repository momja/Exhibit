/* The page's API credential, and the one place it is spent (av-5imk).
 *
 * A server-rendered page authenticates its own fetches with whatever the
 * request that rendered it was entitled to — never with the process's static
 * token by default. The server decides which of three cases applies
 * (internal/api/pagecredential.go) and states it in the page's bootstrap
 * <script> as TOKEN and READ_ONLY:
 *
 *   - TOKEN '', READ_ONLY false — a session-authenticated browser. The session
 *     cookie is a real per-user credential the browser attaches to every
 *     same-origin request on its own, so no Authorization header is needed and
 *     none is sent. Handing the page a bearer token here would give it a
 *     second, operator-strength credential that logging out cannot revoke.
 *   - TOKEN '', READ_ONLY true — an anonymous visitor on a public instance.
 *     There is no credential, and writes are refused here rather than sent to
 *     be refused by the server.
 *   - TOKEN set, READ_ONLY false — a single-user instance with no identity
 *     provider. The static token is the only credential such an instance has,
 *     and its page visitor is the operator who holds it anyway.
 *
 * A fourth value, STATE_WRITABLE, rides beside them and is not a fourth case:
 * it says whether this visitor may write the artifact's own saved data, which
 * a granted recipient may do on an artifact they may not otherwise change
 * (av-v991). apiStateFetch below is where it is spent, and it is the only
 * refusal in this file that is not READ_ONLY's.
 *
 * Page scripts therefore call apiFetch, never fetch-plus-a-hand-built header.
 * That is the point: the three cases are distinguished once, here, and a call
 * site cannot get them wrong individually. A new `'Authorization': 'Bearer ' +
 * TOKEN` anywhere else is the defect this file exists to remove.
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
      JSON.stringify({error: message || 'This library is read-only — sign in to make changes.'}),
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

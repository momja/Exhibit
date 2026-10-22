// Installed only on current, authenticated artifact documents. The bridge is
// dormant outside Exhibit's frame, and holds no API or connector credentials.
(function () {
  'use strict';
  if (window.parent === window) return;
  var config = __EXHIBIT_RELAY_CONFIG__;
  var nativeFetch = window.fetch;
  var pending = new Map();
  var sequence = 0;
  var session = Math.random().toString(36).slice(2);
  var channel = new MessageChannel();
  var post = channel.port1.postMessage.bind(channel.port1);

  function encode(bytes) {
    var parts = [];
    for (var offset = 0; offset < bytes.length; offset += 8192) {
      parts.push(String.fromCharCode.apply(null, bytes.subarray(offset, offset + 8192)));
    }
    return btoa(parts.join(''));
  }

  channel.port1.onmessage = function (event) {
    var data = event.data;
    if (!data || data.__exhibitRelayResponse !== true || !pending.has(data.id)) return;
    pending.get(data.id)(data);
  };
  // Created before guest scripts. The port belongs to this document and dies
  // when it navigates; a replacement document cannot inherit the WindowProxy's
  // authority. The host listens before it starts loading this frame.
  window.parent.postMessage({ __exhibitRelayChannel: true }, config.app_origin, [channel.port2]);

  function connectionRequest(action, connectionId) {
    if (pending.size >= 16) return Promise.reject(new TypeError('Too many pending Exhibit requests.'));
    var id = session + '-connection-' + (++sequence);
    return new Promise(function (resolve, reject) {
      var timer = setTimeout(function () { pending.delete(id); reject(new TypeError('Exhibit connection request timed out.')); }, 15000);
      pending.set(id, function (data) {
        clearTimeout(timer); pending.delete(id);
        if (data.error) reject(new TypeError(data.error)); else resolve(data.connection);
      });
      post({ __exhibitConnection: true, id: id, action: action, connection_id: connectionId });
    });
  }
  var exhibit = window.exhibit || {};
  exhibit.connections = Object.freeze({
    list: function () { return connectionRequest('list'); },
    get: function (connectionId) { return connectionRequest('standin', String(connectionId)); }
  });
  window.exhibit = exhibit;

  window.fetch = function (input, init) {
    var request;
    try {
      var source = input instanceof Request ? input.url : String(input);
      var target = new URL(source, document.baseURI);
      var mode = init && init.mode !== undefined ? init.mode : (input instanceof Request ? input.mode : 'cors');
      if (mode === 'no-cors' || target.origin === location.origin ||
          target.protocol !== 'https:' || config.origins.indexOf(target.origin) < 0) {
        return nativeFetch.call(window, input, init);
      }
      if (mode === 'same-origin') return Promise.reject(new TypeError('A same-origin request cannot reach another origin.'));
      request = new Request(input, init);
    } catch (error) {
      return Promise.reject(error);
    }
    if (request.signal.aborted) return Promise.reject(new DOMException('The operation was aborted.', 'AbortError'));
    if (pending.size >= 16) return Promise.reject(new TypeError('Too many pending Exhibit relay requests.'));
    var id = session + '-' + (++sequence);
    return new Promise(function (resolve, reject) {
      var timer;
      function cleanup() {
        clearTimeout(timer);
        pending.delete(id);
        request.signal.removeEventListener('abort', abort);
      }
      function abort() {
        cleanup();
        post({ __exhibitRelayAbort: true, id: id });
        reject(new DOMException('The operation was aborted.', 'AbortError'));
      }
      pending.set(id, function (data) {
        cleanup();
        if (data.error) { reject(new TypeError(data.error)); return; }
        try {
          var result = data.response;
          var raw = atob(result.body_base64);
          var bytes = Uint8Array.from(raw, function (character) { return character.charCodeAt(0); });
          var empty = request.method === 'HEAD' || result.status === 204 || result.status === 205 || result.status === 304;
          var response = new Response(empty ? null : bytes, { status: result.status, statusText: result.status_text, headers: result.headers });
          Object.defineProperty(response, 'url', { value: result.url });
          Object.defineProperty(response, 'redirected', { value: result.url !== request.url });
          resolve(response);
        } catch (error) { reject(new TypeError('Invalid Exhibit relay response.')); }
      });
      request.signal.addEventListener('abort', abort, { once: true });
      // Human consent can take time. The actual HTTP exchange has its own
      // fifteen-second deadline, separate from this bridge deadline.
      timer = setTimeout(function () {
        cleanup();
        post({ __exhibitRelayAbort: true, id: id });
        reject(new TypeError('Exhibit relay permission or request timed out.'));
      }, 180000);
      var bodyPromise = request.body ? request.arrayBuffer() : Promise.resolve(new ArrayBuffer(0));
      bodyPromise.then(function (buffer) {
        if (!pending.has(id)) return;
        if (buffer.byteLength > 1048576) throw new TypeError('Exhibit relay requests are limited to 1 MiB.');
        var headers = {};
        request.headers.forEach(function (value, key) { headers[key] = value; });
        post({
          __exhibitRelay: true, id: id,
          request: { url: request.url, method: request.method, headers: headers, body_base64: encode(new Uint8Array(buffer)), cache: request.cache }
        });
      }).catch(function (error) { cleanup(); reject(error); });
    });
  };
})();

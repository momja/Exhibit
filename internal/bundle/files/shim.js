// Runs only in the opaque artifact frame. Credentials and the host's CSRF token
// are deliberately absent. Replies are bytes, never markup on the host page.
(() => {
  const nativeFetch = window.fetch.bind(window);
  const pending = new Map();
  const policy = __EXHIBIT_POLICY__;
  let sequence = 0;
  function host(type, data) {
    if (parent === window) return Promise.reject(new Error("Open this app through its runner's home page."));
    const id = String(++sequence);
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { pending.delete(id); reject(new Error("The local relay timed out.")); }, 20000);
      pending.set(id, {resolve, reject, timer});
      parent.postMessage({type, id, ...data}, "*");
    });
  }
  addEventListener("message", event => {
    if (event.source !== parent || !event.data || event.data.type !== "exhibit:local:reply") return;
    const call = pending.get(event.data.id);
    if (!call) return;
    pending.delete(event.data.id);
    clearTimeout(call.timer);
    if (event.data.error) call.reject(new Error(event.data.error));
    else call.resolve(event.data.result);
  });
  function encode(bytes) {
    let text = "";
    for (let i = 0; i < bytes.length; i += 8192) text += String.fromCharCode(...bytes.subarray(i, i + 8192));
    return btoa(text);
  }
  window.fetch = async (input, init) => {
    const request = new Request(typeof input === "string" || input instanceof URL ? new URL(input, document.baseURI) : input, init);
    const url = new URL(request.url);
    if (url.protocol === "data:" || url.protocol === "blob:") return nativeFetch(request);
    if (request.signal.aborted) throw new DOMException("The operation was aborted.", "AbortError");
    const bytes = new Uint8Array(await request.arrayBuffer());
    if (bytes.byteLength > 1048576) throw new Error("Request exceeds the local relay byte limit.");
    const result = await host("exhibit:local:relay", {request: {url: request.url, method: request.method, headers: Object.fromEntries(request.headers), body_base64: encode(bytes), policy}});
    const body = Uint8Array.from(atob(result.body_base64 || ""), c => c.charCodeAt(0));
    const response = new Response([101, 204, 205, 304].includes(result.status) || request.method === "HEAD" ? null : body, {status: result.status, statusText: result.status_text, headers: result.headers});
    Object.defineProperty(response, "url", {value: result.url});
    return response;
  };
  const state = JSON.parse(__EXHIBIT_STATE_JSON__);
  let saves = Promise.resolve();
  function persist(op, key, value) {
    saves = saves.then(() => host("exhibit:local:state", {operation: {op, key, value}})).catch(error => console.warn("Exhibit could not save data:", error));
  }
  const storage = {
    getItem(key) { key = String(key); return Object.hasOwn(state, key) ? state[key] : null; },
    setItem(key, value) { key = String(key); value = String(value); Object.defineProperty(state, key, {value, writable:true, enumerable:true, configurable:true}); persist("set", key, value); },
    removeItem(key) { key = String(key); delete state[key]; persist("delete", key); },
    clear() { for (const key of Object.keys(state)) delete state[key]; persist("clear"); },
    key(index) { return Object.keys(state)[index] ?? null; },
    get length() { return Object.keys(state).length; }
  };
  Object.defineProperty(window, "localStorage", {value: storage, configurable: true});
  Object.defineProperty(window, "exhibit", {value:Object.freeze({connections:Object.freeze({list:() => host("exhibit:local:connections", {})})}), configurable:false});
})();

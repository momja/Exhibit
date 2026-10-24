// The host pins messages to its only opaque artifact frame. Response bytes are
// returned only through postMessage; they are never parsed into this document.
const frame = document.getElementById("exhibit-app");
const csrf = __EXHIBIT_CSRF__;
addEventListener("message", async event => {
  if (event.source !== frame.contentWindow || !event.data || typeof event.data.id !== "string") return;
  const message = event.data;
  let path, body, method = "POST";
  if (message.type === "exhibit:local:relay") { path = "/__exhibit/relay"; body = message.request; }
  else if (message.type === "exhibit:local:state") { path = "/__exhibit/state"; body = message.operation; }
  else if (message.type === "exhibit:local:connections") { path = "/__exhibit/connections"; method = "GET"; }
  else return;
  try {
    const response = await fetch(path, {method, headers:{"Content-Type":"application/json", "X-Exhibit-CSRF":csrf}, body:method === "POST" ? JSON.stringify(body) : undefined});
    const result = response.status === 204 ? null : await response.json();
    if (result?.reload) frame.src = "/__exhibit/render?policy=" + Date.now();
    if (!response.ok) throw new Error(result.error || "The local request failed.");
    frame.contentWindow.postMessage({type:"exhibit:local:reply", id:message.id, result}, "*");
  } catch (error) {
    frame.contentWindow.postMessage({type:"exhibit:local:reply", id:message.id, error:String(error.message || error)}, "*");
  }
});

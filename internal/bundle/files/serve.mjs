#!/usr/bin/env node
// Browser-only Exhibit bundle runner. Artifact code stays in an opaque frame.
// This file has no dependencies and works with Node 18+, Bun and Deno's Node
// compatibility mode (with --allow-net --allow-read --allow-write --allow-env).
import http from "node:http";
import https from "node:https";
import dns from "node:dns/promises";
import net from "node:net";
import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";
import zlib from "node:zlib";
import {fileURLToPath} from "node:url";

const directory = path.dirname(fileURLToPath(import.meta.url));
const manifest = JSON.parse(fs.readFileSync(path.join(directory, "exhibit.json"), "utf8"));
if (manifest.format !== "exhibit-relay" || manifest.format_version !== 1 || manifest.files?.page !== "index.html" || manifest.files?.state !== "state.json") throw new Error("Unsupported Exhibit relay bundle.");
function normalizeOrigin(value) {
  if (typeof value !== "string" || value.trim() !== value || /\s/.test(value)) throw new Error("Invalid allowlist origin.");
  const url = new URL(value);
  if (url.username || url.password || !["https:", "http:"].includes(url.protocol) || !["", "/"].includes(url.pathname) || url.search || url.hash) throw new Error("Allowlist entries must be origins.");
  url.hostname = url.hostname.toLowerCase().replace(/\.$/, "");
  if (url.protocol === "http:" && !(url.hostname === "localhost" || url.hostname.endsWith(".localhost") || url.hostname === "[::1]" || /^127\./.test(url.hostname))) throw new Error("Plaintext allowlist origin is not loopback.");
  return url.origin;
}
const allowed = new Set((manifest.network_allowlist || []).map(normalizeOrigin));
const requirements = [...(manifest.connections || [])];
for (const requirement of requirements) {
  requirement.origin = normalizeOrigin(requirement.origin);
  if (!allowed.has(requirement.origin) || !["bearer", "basic", "api-key"].includes(requirement.kind)) throw new Error("Unsupported local connection requirement; export again and reconnect using a generic connection.");
  validateLimits(requirement);
}
for (const origin of allowed) if (origin.startsWith("https:") && !requirements.some(requirement => requirement.origin === origin)) requirements.push({name:origin, origin, kind:"bearer", header:"X-API-Key"});
const page = fs.readFileSync(path.join(directory, "index.html"), "utf8");
const shim = fs.readFileSync(path.join(directory, "shim.js"), "utf8");
const hostScript = fs.readFileSync(path.join(directory, "host.js"), "utf8");
const connectScript = fs.readFileSync(path.join(directory, "connect.js"), "utf8");
const statePath = path.join(directory, "state.json");
let state = Object.assign(Object.create(null), JSON.parse(fs.readFileSync(statePath, "utf8")));
const csrf = crypto.randomBytes(32).toString("base64url");
let framePolicy = crypto.randomBytes(32).toString("base64url");
const connections = new Map();
const cache = new Map();
const connectionLocks = new Map();
const allowedHosts = new Set();
const requestHeaders = new Set(["accept", "accept-language", "content-type", "if-match", "if-none-match", "if-modified-since", "if-unmodified-since", "range", "if-range", "cache-control"]);
const responseHeaders = new Set(["content-type", "content-language", "cache-control", "etag", "last-modified", "expires", "date", "retry-after", "accept-ranges", "content-range", "vary"]);
let active = 0;
// Own agent with an explicit empty proxy environment. Node versions that
// support environment proxies must not inherit the runner's ambient proxy.
const outboundAgent = new https.Agent({keepAlive:true, maxSockets:8, maxFreeSockets:2, timeout:15000, proxyEnv:{}});

function ipv6Number(value) {
  let text = value.toLowerCase().replace(/^\[|\]$/g, "");
  if (text.includes(".")) text = text.replace(/([\d.]+)$/, tail => { const octets = tail.split(".").map(Number); return ((octets[0] << 8) | octets[1]).toString(16) + ":" + ((octets[2] << 8) | octets[3]).toString(16); });
  const halves = text.split("::");
  const left = halves[0] ? halves[0].split(":") : [];
  const right = halves[1] ? halves[1].split(":") : [];
  const groups = halves.length === 2 ? [...left, ...Array(8 - left.length - right.length).fill("0"), ...right] : left;
  if (groups.length !== 8) throw new Error("Invalid IPv6 address.");
  return groups.reduce((value, group) => (value << 16n) | BigInt(parseInt(group, 16)), 0n);
}
function publicIPv4(value) {
  const octets = typeof value === "number" ? [value >>> 24, (value >>> 16) & 255, (value >>> 8) & 255, value & 255] : value.split(".").map(Number);
  const [a, b, c] = octets;
  return !(a === 0 || a === 10 || a === 127 || a >= 224 || a === 169 && b === 254 || a === 172 && b >= 16 && b <= 31 || a === 192 && b === 168 || a === 100 && b >= 64 && b <= 127 || a === 192 && b === 0 && [0, 2].includes(c) || a === 192 && b === 88 && c === 99 || a === 198 && [18, 19].includes(b) || a === 198 && b === 51 && c === 100 || a === 203 && b === 0 && c === 113);
}
function publicAddress(address) {
  const family = net.isIP(address);
  if (family === 4) return publicIPv4(address);
  if (family !== 6 || address.includes("%")) return false;
  const value = ipv6Number(address);
  if (value >> 32n === 0xffffn) return publicIPv4(Number(value & 0xffffffffn));
  if (value >> 125n !== 1n) return false; // assigned 2000::/3 global unicast
  for (const [prefix, length] of [["2001::", 23], ["2001:db8::", 32], ["2002::", 16], ["3fff::", 20]]) if (value >> BigInt(128 - length) === ipv6Number(prefix) >> BigInt(128 - length)) return false;
  return true;
}
function numericHost(host) { return host.split(".").every(label => /^\d+$/.test(label) || /^0x[0-9a-f]+$/i.test(label)); }
function checkURL(input, origins) {
  if (typeof input !== "string" || input.trim() !== input || /[\r\n\t]/.test(input)) throw new Error("Invalid relay URL.");
  // Check the authority before WHATWG URL canonicalizes legacy numeric IPv4.
  const rawAuthority = /^https:\/\/([^/?#]+)/i.exec(input)?.[1];
  if (!rawAuthority || rawAuthority.includes("@")) throw new Error("Only HTTPS URLs without credentials are allowed.");
  const rawHost = rawAuthority.startsWith("[") ? rawAuthority.slice(1, rawAuthority.indexOf("]")) : rawAuthority.replace(/:\d+$/, "").replace(/\.$/, "").toLowerCase();
  if (!net.isIP(rawHost) && numericHost(rawHost)) throw new Error("Ambiguous numeric destination is not allowed.");
  const url = new URL(input);
  const host = url.hostname.replace(/^\[|\]$/g, "").replace(/\.$/, "").toLowerCase();
  if (url.protocol !== "https:" || url.username || url.password || url.hash || host === "localhost" || host.endsWith(".localhost") || net.isIP(host) && !publicAddress(host)) throw new Error("Destination is not publicly routable.");
  if (!origins.has(normalizeOrigin(url.origin))) throw new Error("Destination is not in this bundle's allowlist.");
  return url;
}
function selectedHeaders(headers, allowedNames) {
  const result = Object.create(null);
  const hop = new Set(String(headers.connection || headers.Connection || "").split(",").map(name => name.trim().toLowerCase()));
  for (const [name, value] of Object.entries(headers || {})) if (allowedNames.has(name.toLowerCase()) && !hop.has(name.toLowerCase())) result[name.toLowerCase()] = Array.isArray(value) ? value.join(", ") : String(value);
  return result;
}
async function exchange(url, method, headers, body, signal, authorize) {
  const host = url.hostname.replace(/^\[|\]$/g, "");
  const addresses = net.isIP(host) ? [{address:host, family:net.isIP(host)}] : await new Promise((resolve, reject) => {
    const aborted = () => reject(new Error("The relay timed out."));
    if (signal.aborted) { aborted(); return; }
    signal.addEventListener("abort", aborted, {once:true});
    dns.lookup(host, {all:true, verbatim:true}).then(resolve, reject).finally(() => signal.removeEventListener("abort", aborted));
  });
  if (!addresses.length || addresses.some(({address}) => !publicAddress(address))) throw new Error("Destination DNS includes a non-public address.");
  addresses.sort((left, right) => left.family - right.family);
  const chosen = addresses[0];
  authorize(url, headers); // After all DNS answers pass, immediately before contact.
  return new Promise((resolve, reject) => {
    const request = https.request(url, {method, headers, signal, agent:outboundAgent, family:chosen.family, autoSelectFamily:false, maxHeaderSize:65536,
      // DNS answers were checked once; the socket is pinned to that literal.
      lookup(_hostname, _options, callback) { callback(null, chosen.address, chosen.family); }
    }, response => resolve(response));
    request.on("error", reject);
    if (body.length) request.write(body);
    request.end();
  });
}
async function decodedBody(response) {
  let stream = response;
  const encoding = String(response.headers["content-encoding"] || "").toLowerCase().trim();
  if (encoding === "gzip") stream = response.pipe(zlib.createGunzip());
  else if (encoding === "deflate") stream = response.pipe(zlib.createInflate());
  else if (encoding && encoding !== "identity") { response.destroy(); throw new Error("Unsupported content encoding."); }
  response.on("error", error => stream.destroy(error));
  const chunks = []; let length = 0;
  try { for await (const chunk of stream) { length += chunk.length; if (length > 4194304) throw new Error("Decoded response exceeds the relay byte limit."); chunks.push(chunk); } }
  catch (error) { response.destroy(); stream.destroy(); throw error; }
  return Buffer.concat(chunks);
}
async function gate(input, origins = allowed, authorize = () => {}) {
  let url = checkURL(input.url, origins);
  let method = input.method || "GET";
  if (!["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"].includes(method)) throw new Error("Unsupported relay method.");
  if (typeof input.body_base64 !== "string" || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(input.body_base64)) throw new Error("Invalid relay body.");
  let body = Buffer.from(input.body_base64, "base64");
  const inputHeaders = input.headers ?? {};
  if (body.length > 1048576 || typeof inputHeaders !== "object" || Array.isArray(inputHeaders) || Object.keys(inputHeaders).length > 64) throw new Error("Relay request exceeds its limit.");
  const baseHeaders = selectedHeaders(inputHeaders, requestHeaders);
  if (Object.entries(baseHeaders).reduce((length, [name, value]) => length + Buffer.byteLength(name) + Buffer.byteLength(value) + 4, 0) > 65536) throw new Error("Relay headers exceed their byte limit.");
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 15000);
  timeout.unref?.();
  try {
    for (let hop = 0; ; hop++) {
      url = checkURL(url.href, origins);
      const headers = {...baseHeaders, "user-agent":"Exhibit-Relay/1", "accept-encoding":"gzip, deflate"};
      const response = await exchange(url, method, headers, body, controller.signal, authorize);
      if ([301, 302, 303, 307, 308].includes(response.statusCode) && response.headers.location) {
        response.destroy();
        if (hop >= 5) throw new Error("Too many redirects.");
        const location = response.headers.location;
        if (/^https:\/\//i.test(location) || location.startsWith("//")) checkURL(location.startsWith("//") ? "https:" + location : location.split("#")[0], origins);
        const next = new URL(location, url); next.hash = "";
        url = next;
        if (response.statusCode === 303 && method !== "HEAD" || [301, 302].includes(response.statusCode) && method === "POST") { method = "GET"; body = Buffer.alloc(0); delete baseHeaders["content-type"]; }
        continue;
      }
      const noBody = method === "HEAD" || [204, 304].includes(response.statusCode);
      if (noBody) response.resume();
      const data = noBody ? Buffer.alloc(0) : await decodedBody(response);
      return {status:response.statusCode, status_text:http.STATUS_CODES[response.statusCode] || "", headers:selectedHeaders(response.headers, responseHeaders), body_base64:data.toString("base64"), url:url.href};
    }
  } finally { clearTimeout(timeout); }
}

function credentialHeader(header) {
  const name = String(header || "").toLowerCase();
  return /^[a-z0-9-]{1,128}$/.test(name) && !/^(proxy-|sec-|x-forwarded-)/.test(name) && !["cookie", "set-cookie", "host", "origin", "referer", "connection", "content-length", "transfer-encoding", "te", "trailer", "upgrade", "accept-encoding", "user-agent"].includes(name);
}
function safeAccessPath(value) {
  for (let attempt = 0; attempt < 4; attempt++) {
    if (value.includes("\\") || value.split("/").some(part => [".", ".."].includes(part))) return false;
    let decoded;
    try { decoded = decodeURIComponent(value); } catch { return false; }
    if (decoded === value) return true;
    value = decoded;
  }
  return false;
}
function validateLimits(connection) {
  for (const name of ["daily_limit", "cache_minutes"]) if (connection[name] !== undefined && (!Number.isSafeInteger(connection[name]) || connection[name] < 0)) throw new Error("Enter whole nonnegative request and response reuse limits.");
  if ((connection.cache_minutes || 0) > 525600) throw new Error("Response reuse cannot exceed 525600 minutes.");
}
function prepareConnection(input) {
  const connection = {...input};
  if (connection.access_url) {
    try {
      const value = String(connection.access_url).trim();
      const rawPath = /^https:\/\/[^/?#]+([^?#]*)/.exec(value)?.[1] || "";
      const access = new URL(value);
      if (value.length > 8192 || connection.kind !== "basic" || access.protocol !== "https:" || !access.username || !access.password || access.search || access.hash || !safeAccessPath(rawPath)) throw new Error();
      const origin = normalizeOrigin(access.origin);
      if (connection.origin && connection.origin !== origin) throw new Error();
      connection.origin = origin;
      connection.username = decodeURIComponent(access.username);
      connection.password = decodeURIComponent(access.password);
      connection.base_path = decodeURIComponent(access.pathname).replace(/\/+$/, "");
      delete connection.access_url;
    } catch { throw new Error("Enter an HTTPS Basic access URL with embedded credentials at this connection's allowed origin, without query or fragment."); }
  }
  if (connection.origin !== normalizeOrigin(connection.origin) || !connection.origin.startsWith("https:") || !allowed.has(connection.origin)) throw new Error("Connection requires an allowed HTTPS origin.");
  if (connection.kind === "bearer" && !connection.secret || connection.kind === "api-key" && (!connection.secret || !credentialHeader(connection.header)) || connection.kind === "basic" && (!connection.username || !connection.password || connection.username.includes(":")) || !["bearer", "api-key", "basic"].includes(connection.kind)) throw new Error("Enter the required credentials for a generic connection; reconnect older unsupported connections.");
  if (/[\r\n]/.test((connection.secret || "") + (connection.username || "") + (connection.password || ""))) throw new Error("Credentials cannot contain line breaks.");
  validateLimits(connection);
  delete connection.data_origins; // Ignored legacy metadata, never a credential scope.
  return connection;
}
function scrubResponse(connection, response) {
  const values = [connection.secret,connection.username,connection.password];
  if (connection.username || connection.password) values.push(connection.username + ":" + connection.password, Buffer.from(connection.username + ":" + connection.password).toString("base64"));
  values.sort((left, right) => (right || "").length - (left || "").length);
  const scrub = buffer => {
    let data = buffer.toString("latin1");
    for (const value of values) if (value) for (const variation of [value, encodeURIComponent(value), new URLSearchParams({value}).toString().slice(6), JSON.stringify(value).slice(1,-1)]) data = data.split(Buffer.from(variation).toString("latin1")).join(Buffer.byteLength(variation) < 21 ? "*" : "[credential redacted]");
    if (connection.base_path) {
      const escapedPath = connection.base_path.split("/").map(encodeURIComponent).join("/");
      for (const value of [connection.base_path,escapedPath,encodeURIComponent(connection.base_path),new URLSearchParams({value:connection.base_path}).toString().slice(6)]) {
        const jsonValue = JSON.stringify(value).slice(1,-1).replace(/[<>&]/g, char => "\\u" + char.charCodeAt(0).toString(16).padStart(4,"0"));
        for (const variation of [value,jsonValue,value.replaceAll("/","\\/"),jsonValue.replaceAll("/","\\/")]) data = data.split(Buffer.from(variation).toString("latin1")).join("");
      }
    }
    return Buffer.from(data,"latin1");
  };
  response.body_base64 = scrub(Buffer.from(response.body_base64,"base64")).toString("base64");
  response.url = scrub(Buffer.from(response.url)).toString();
  for (const [name,value] of Object.entries(response.headers)) response.headers[name] = scrub(Buffer.from(value)).toString();
}
function clearConnectionCache(origin) {
  for (const [key,saved] of cache) if (saved.origin === origin) cache.delete(key);
}
for (const input of JSON.parse(process.env.EXHIBIT_CONNECTIONS || "[]")) { const connection = prepareConnection(input); connections.set(connection.origin, connection); }

const escape = value => String(value ?? "").replace(/[&<>"']/g, char => ({"&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;", "'":"&#39;"}[char]));
const json = value => JSON.stringify(value).replace(/</g, "\\u003c").replace(/>/g, "\\u003e").replace(/&/g, "\\u0026");
const style = "body{font:16px/1.5 system-ui;color:#172126;background:#f6f7f8;margin:40px auto;padding:0 20px;max-width:720px}a{color:#146350}form{padding:24px;background:white;border:1px solid #dce0e1;border-radius:12px;margin:20px 0}label{display:block;margin:14px 0}input,select,button{font:inherit;padding:9px;border:1px solid #bcc6c5;border-radius:6px;max-width:100%;box-sizing:border-box}input{display:block;width:100%}button{cursor:pointer;background:#146350;color:white;margin-right:8px}code{overflow-wrap:anywhere}.muted,small{font-size:14px;color:#52615f}h2{font-size:19px;margin:0}.message{padding:12px;background:#e2eee8}";
function connectPage(message = "") {
  const forms = requirements.map((requirement, index) => {
    const connection = connections.get(requirement.origin);
    const kind = connection?.kind || requirement.kind;
    return `<form method="post" action="/__exhibit/connect" autocomplete="off" data-connection-form><input type="hidden" name="csrf" value="${csrf}"><input type="hidden" name="index" value="${index}"><h2>${escape(requirement.name)}</h2><code>${escape(requirement.origin)}</code><p class="muted">${connection ? "Connected locally" : "No local credentials"}</p><label>Connection kind<select name="kind">${[["bearer","Bearer token"],["api-key","API key header"],["basic","Basic authentication"]].map(([value, name]) => `<option value="${value}" ${kind === value ? "selected" : ""}>${name}</option>`).join("")}</select></label><div data-kinds="bearer api-key"><label>API key or bearer token<input type="password" name="secret" autocomplete="new-password"></label></div><div data-kinds="api-key"><label>API key header<input name="header" value="${escape(requirement.header || "X-API-Key")}"></label></div><div data-kinds="basic"><label>Basic access URL (HTTPS with embedded username and password)<input type="password" name="access_url" autocomplete="new-password" placeholder="https://user:password@api.example/private-base"></label><p class="muted">Or enter the credentials separately when the API has no private base path.</p><label>Basic username<input name="username" autocomplete="off"></label><label>Basic password<input type="password" name="password" autocomplete="new-password"></label></div><label>Maximum provider requests per UTC day (optional)<input type="number" name="daily_limit" min="0" step="1" value="${escape((connection?.daily_limit ?? requirement.daily_limit) || "")}"></label><label>Reuse successful GET/HEAD responses for minutes (optional)<input type="number" name="cache_minutes" min="0" max="525600" step="1" value="${escape((connection?.cache_minutes ?? requirement.cache_minutes) || "")}"></label><small>Credentials are used only for this origin. Reconnecting replaces the current local connection. Limits and reuse apply during this runner session.</small><p><button name="action" value="connect">Connect locally</button>${connection ? '<button name="action" value="disconnect">Disconnect</button>' : ""}</p></form>`;
  }).join("");
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Local connections · ${escape(manifest.title)}</title><style>${style}</style></head><body><a href="/">← Back to app</a><h1>Local connections</h1><p>Connect ${escape(manifest.title)} on this machine. Credentials stay outside the app frame and remain only until this runner stops. Reconnect after restarting.</p><p class="muted">Connections add authentication for their own origin. The app can also call other approved origins without a connection. The app's allowed origins are: ${escape([...allowed].join(", "))}.</p>${message ? `<p class="message" role="status">${escape(message)}</p>` : ""}${forms || "<p>This app has no allowed external origins.</p>"}<script type="module">${connectScript}</script></body></html>`;
}

function send(response, status, body, type = "application/json") { response.writeHead(status, {"Content-Type":type}); response.end(body); }
function error(response, status, message) { send(response, status, JSON.stringify({error:message})); }
async function readBody(request) {
  const chunks = []; let length = 0;
  for await (const chunk of request) { length += chunk.length; if (length > 2097152) throw new Error("Request exceeds local runner byte limit."); chunks.push(chunk); }
  return Buffer.concat(chunks);
}
function authorized(request, form) {
  if (request.headers.origin !== `http://${request.headers.host}`) return false;
  const token = request.headers["x-exhibit-csrf"] || form?.get("csrf") || "";
  const actual = Buffer.from(String(token)); const expected = Buffer.from(csrf);
  return actual.length === expected.length && crypto.timingSafeEqual(actual, expected);
}
function persist(operation) {
  const next = Object.assign(Object.create(null), state);
  if (operation.op === "set" && typeof operation.key === "string" && typeof operation.value === "string") next[operation.key] = operation.value;
  else if (operation.op === "delete" && typeof operation.key === "string") delete next[operation.key];
  else if (operation.op === "clear") for (const key of Object.keys(next)) delete next[key];
  else throw new Error("Unknown saved data operation.");
  const data = JSON.stringify(next);
  if (Buffer.byteLength(data) > 8388608 || Object.keys(next).length > 10000) throw new Error("Saved data exceeds local limit.");
  fs.writeFileSync(statePath + ".tmp", data, {mode:0o600});
  fs.renameSync(statePath + ".tmp", statePath);
  state = next;
}
async function relay(input) {
  const checkPolicy = () => { if (input.policy !== framePolicy) { const failure = new Error("Connections changed; reloading the app."); failure.reload = true; throw failure; } };
  checkPolicy();
  const origin = normalizeOrigin(checkURL(input.url, allowed).origin);
  const previous = connectionLocks.get(origin) || Promise.resolve();
  let release;
  const lock = new Promise(resolve => { release = resolve; });
  connectionLocks.set(origin, lock);
  await previous;
  try {
    checkPolicy();
    const permitted = allowed;
    const original = checkURL(input.url, permitted);
    const connection = connections.get(origin);
    if ((manifest.connections || []).some(requirement => requirement.origin === origin) && !connection) { const failure = new Error("Connect this app locally using Connections in the page header."); failure.status = 409; throw failure; }
    const method = input.method || "GET";
    const selected = selectedHeaders(input.headers || {}, requestHeaders);
    const key = method + " " + input.url + " " + JSON.stringify(Object.entries(selected).sort());
    const cacheable = connection?.cache_minutes > 0 && ["GET", "HEAD"].includes(method) && !input.body_base64 && !["range","if-range","if-match","if-none-match","if-modified-since","if-unmodified-since"].some(name => selected[name]) && !/no-/i.test(selected["cache-control"] || "");
    if (cacheable) { const saved = cache.get(key); if (saved && saved.expires > Date.now()) return saved.response; }
    const publicURL = input.url;
    if (connection?.base_path) {
      const rawPath = /^https:\/\/[^/?#]+([^?#]*)/.exec(input.url)?.[1] || "";
      if (!safeAccessPath(rawPath)) throw new Error("Invalid path for this connection.");
      original.pathname = connection.base_path + "/" + original.pathname.replace(/^\/+/, "");
      input = {...input, url:original.href};
    }
    const response = await gate(input, permitted, (url, headers) => {
      checkPolicy();
      const hopOrigin = normalizeOrigin(url.origin);
      if (!permitted.has(hopOrigin)) throw new Error("Destination is not approved for this app.");
      if (!connection) return;
      if (hopOrigin !== origin) throw new Error("Connector redirects must stay at their authorized origin.");
      const hopPath = decodeURIComponent(url.pathname);
      if (connection.base_path && (!safeAccessPath(hopPath) || hopPath !== connection.base_path && !hopPath.startsWith(connection.base_path + "/"))) throw new Error("Redirect leaves the connection's private base path.");
      const day = new Date().toISOString().slice(0, 10);
      if (connection.day !== day) { connection.day = day; connection.calls = 0; }
      if (connection.daily_limit && connection.calls >= connection.daily_limit) { const failure = new Error("This connection's daily request limit has been reached."); failure.status = 429; throw failure; }
      connection.calls++;
      if (connection.kind === "bearer") headers.authorization = "Bearer " + connection.secret;
      else if (connection.kind === "basic") headers.authorization = "Basic " + Buffer.from(connection.username + ":" + connection.password).toString("base64");
      else if (connection.kind === "api-key") headers[connection.header.toLowerCase()] = connection.secret;
    });
    checkPolicy();
    if (connection?.base_path) response.url = publicURL;
    if (connection) scrubResponse(connection,response);
    if (cacheable && response.status >= 200 && response.status < 300 && response.status !== 206 && !response.headers.vary?.includes("*") && !/no-store/i.test(response.headers["cache-control"] || "")) {
      for (const [key,saved] of cache) if (saved.expires <= Date.now() || cache.size >= 128) cache.delete(key);
      cache.set(key, {response, expires:Date.now() + connection.cache_minutes * 60000, origin});
    }
    return response;
  } finally { release(); if (connectionLocks.get(origin) === lock) connectionLocks.delete(origin); }
}

const server = http.createServer(async (request, response) => {
  response.setHeader("Cache-Control", "no-store");
  response.setHeader("X-Content-Type-Options", "nosniff");
  response.setHeader("Referrer-Policy", "no-referrer");
  if (!allowedHosts.has(String(request.headers.host || "").toLowerCase())) { error(response, 403, "Unrecognized local runner host."); return; }
  try {
    const url = new URL(request.url, `http://${request.headers.host}`);
    let body, form;
    if (request.method === "POST") {
      body = await readBody(request);
      if (String(request.headers["content-type"] || "").startsWith("application/x-www-form-urlencoded")) form = new URLSearchParams(body.toString("utf8"));
      if (!authorized(request, form)) { error(response, 403, "Local request failed its origin or session check."); return; }
    }
    if (request.method === "GET" && ["/", "/index.html"].includes(url.pathname)) {
      response.setHeader("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; frame-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'");
      send(response, 200, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${escape(manifest.title)}</title><style>html,body{height:100%;margin:0;font:15px system-ui;background:#f6f7f8;color:#172126}body{display:flex;flex-direction:column}header{display:flex;align-items:center;gap:20px;padding:12px 20px;border-bottom:1px solid #ddd}header strong{flex:1}a{color:#146350}iframe{border:0;width:100%;flex:1;background:white}.label{font-size:12px;color:#5c676c}</style></head><body><header><strong>${escape(manifest.title)}</strong><span class="label">Exhibit · local relay</span><a href="/__exhibit/connect" target="_blank" rel="noopener">Connections</a></header><iframe id="exhibit-app" title="${escape(manifest.title)}" sandbox="allow-scripts" src="/__exhibit/render"></iframe><script type="module">${hostScript.replaceAll("__EXHIBIT_CSRF__", json(csrf))}</script></body></html>`, "text/html; charset=utf-8");
    } else if (request.method === "GET" && url.pathname === "/__exhibit/render") {
      const origins = [...allowed].join(" ");
      response.setHeader("Content-Security-Policy", `sandbox allow-scripts; default-src 'none'; script-src 'unsafe-inline' 'unsafe-eval' blob: data: ${origins}; style-src 'unsafe-inline' ${origins}; img-src data: blob: ${origins}; font-src data: ${origins}; media-src data: blob: ${origins}; connect-src 'none'; worker-src blob: data:; frame-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'self'`);
      const script = "<script>" + shim.replaceAll("__EXHIBIT_STATE_JSON__", json(JSON.stringify(state))).replaceAll("__EXHIBIT_POLICY__", json(framePolicy)) + "</script>";
      const head = /<head(?:\s[^>]*)?>/i.exec(page);
      send(response, 200, head ? page.slice(0, head.index + head[0].length) + script + page.slice(head.index + head[0].length) : script + page, "text/html; charset=utf-8");
    } else if (request.method === "GET" && url.pathname === "/__exhibit/connect") {
      response.setHeader("Referrer-Policy", "same-origin");
      response.setHeader("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'");
      send(response, 200, connectPage(), "text/html; charset=utf-8");
    } else if (request.method === "GET" && url.pathname === "/__exhibit/connections") {
      const list = requirements.filter(requirement => connections.has(requirement.origin)).map(requirement => { const connection = connections.get(requirement.origin); return {name:requirement.name,provider:connection.kind,kind:"native",origins:[connection.origin]}; });
      send(response, 200, JSON.stringify(list));
    } else if (request.method === "POST" && url.pathname === "/__exhibit/connect" && form) {
      const index = Number(form.get("index"));
      if (!Number.isInteger(index) || index < 0 || index >= requirements.length) { error(response, 400, "Unknown local connection."); return; }
      const requirement = requirements[index];
      let message = "Connection removed from this runner.";
      try {
        if (form.get("action") === "disconnect") connections.delete(requirement.origin);
        else {
          const limit = name => { const value = (form.get(name) || "").trim(); return value ? Number(value) : 0; };
          const connection = prepareConnection({origin:requirement.origin, kind:form.get("kind"), header:form.get("header"), secret:form.get("secret"), username:form.get("username"), password:form.get("password"), access_url:form.get("access_url"), daily_limit:limit("daily_limit"), cache_minutes:limit("cache_minutes")});
          connections.set(requirement.origin, connection);
          message = "Connected locally. Credentials remain in the runner's memory until it stops.";
        }
        clearConnectionCache(requirement.origin);
        framePolicy = crypto.randomBytes(32).toString("base64url");
      } catch (failure) { message = failure.message; }
      response.setHeader("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'");
      response.setHeader("Referrer-Policy", "same-origin");
      send(response, 200, connectPage(message), "text/html; charset=utf-8");
    } else if (request.method === "POST" && url.pathname === "/__exhibit/state") {
      persist(JSON.parse(body)); response.writeHead(204); response.end();
    } else if (request.method === "POST" && url.pathname === "/__exhibit/relay") {
      if (active >= 8) { error(response, 429, "The local relay is busy; try again."); return; }
      active++;
      try { send(response, 200, JSON.stringify(await relay(JSON.parse(body)))); }
      finally { active--; }
    } else error(response, 404, "Not found.");
  } catch (failure) { if (!response.headersSent) { if (failure.reload) send(response, 409, JSON.stringify({error:failure.message,reload:true})); else error(response, failure.status || 502, failure.status ? failure.message : "The local runner refused or could not complete this request."); } else response.destroy(); }
});
server.requestTimeout = 20000;
server.headersTimeout = 10000;
const port = Number(process.env.PORT || 8787);
const host = process.env.HOST || "127.0.0.1";
server.listen(port, host, () => {
  const boundPort = server.address().port;
  const displayHost = host.includes(":") ? `[${host}]` : host;
  allowedHosts.add(`${displayHost}:${boundPort}`.toLowerCase());
  if (["127.0.0.1", "::1", "0.0.0.0", "::"].includes(host)) for (const alias of ["127.0.0.1", "localhost", "[::1]"]) allowedHosts.add(`${alias}:${boundPort}`);
  if (process.env.EXHIBIT_PUBLIC_HOST) allowedHosts.add(process.env.EXHIBIT_PUBLIC_HOST.toLowerCase());
  console.log(`${manifest.title} is running at http://${host === "127.0.0.1" ? "localhost" : displayHost}:${boundPort}/`);
});

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
  if (!allowed.has(requirement.origin) || !["bearer", "basic", "api-key", "simplefin"].includes(requirement.kind)) throw new Error("Invalid local connection requirement.");
}
for (const origin of allowed) if (origin.startsWith("https:") && !requirements.some(requirement => requirement.origin === origin)) requirements.push({name:origin, origin, kind:"bearer", header:"X-API-Key"});
const page = fs.readFileSync(path.join(directory, "index.html"), "utf8");
const shim = fs.readFileSync(path.join(directory, "shim.js"), "utf8");
const hostScript = fs.readFileSync(path.join(directory, "host.js"), "utf8");
const connectScript = fs.readFileSync(path.join(directory, "connect.js"), "utf8");
const statePath = path.join(directory, "state.json");
const policyPath = statePath + ".policy.json";
let state = Object.assign(Object.create(null), JSON.parse(fs.readFileSync(statePath, "utf8")));
const csrf = crypto.randomBytes(32).toString("base64url");
let framePolicy = crypto.randomBytes(32).toString("base64url");
const financialOrigins = new Set(requirements.filter(requirement => requirement.kind === "simplefin").map(requirement => requirement.origin));
if (fs.existsSync(policyPath)) {
  const raw = fs.readFileSync(policyPath,"utf8");
  if (Buffer.byteLength(raw)>65536) throw new Error("Saved local financial policy exceeds its limit.");
  const policy = JSON.parse(raw);
  if (!Array.isArray(policy.financial_origins) || !policy.financial_origins.length) throw new Error("Invalid saved local financial policy.");
  for (const origin of policy.financial_origins) { const normalized = normalizeOrigin(origin); if (!allowed.has(normalized) || !normalized.startsWith("https:")) throw new Error("Saved local financial policy does not match this bundle."); financialOrigins.add(normalized); }
}
const connections = new Map();
const cache = new Map();
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
async function exchange(url, method, headers, body, signal) {
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
  if (body.length > 1048576 || !input.headers || typeof input.headers !== "object" || Object.keys(input.headers).length > 64) throw new Error("Relay request exceeds its limit.");
  const baseHeaders = selectedHeaders(input.headers, requestHeaders);
  if (Object.entries(baseHeaders).reduce((length, [name, value]) => length + Buffer.byteLength(name) + Buffer.byteLength(value) + 4, 0) > 65536) throw new Error("Relay headers exceed their byte limit.");
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 15000);
  timeout.unref?.();
  try {
    for (let hop = 0; ; hop++) {
      url = checkURL(url.href, origins);
      const headers = {...baseHeaders, "user-agent":"Exhibit-Relay/1", "accept-encoding":"gzip, deflate"};
      authorize(url, headers);
      const response = await exchange(url, method, headers, body, controller.signal);
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
function validateConnection(connection) {
  if (connection.origin !== normalizeOrigin(connection.origin) || !connection.origin.startsWith("https:") || !allowed.has(connection.origin)) throw new Error("Connection requires an allowed HTTPS origin.");
  if (connection.kind === "bearer" && !connection.secret || connection.kind === "api-key" && (!connection.secret || !credentialHeader(connection.header)) || connection.kind === "basic" && (!connection.username || !connection.password || connection.username.includes(":")) || connection.kind === "simplefin" && (!connection.username || !connection.password || !connection.endpoint) || !["bearer", "api-key", "basic", "simplefin"].includes(connection.kind)) throw new Error("Enter the required credentials for the selected kind.");
  if (/[\r\n]/.test((connection.secret || "") + (connection.username || "") + (connection.password || ""))) throw new Error("Credentials cannot contain line breaks.");
}
function scrubResponse(connection, response) {
  const values = [connection.secret,connection.username,connection.password];
  if (connection.username || connection.password) values.push(connection.username + ":" + connection.password, Buffer.from(connection.username + ":" + connection.password).toString("base64"));
  values.sort((left, right) => (right || "").length - (left || "").length);
  const scrub = buffer => {
    let data = buffer.toString("latin1");
    for (const value of values) if (value) for (const variation of [value, encodeURIComponent(value), new URLSearchParams({value}).toString().slice(6), JSON.stringify(value).slice(1,-1)]) data = data.split(Buffer.from(variation).toString("latin1")).join(Buffer.byteLength(variation) < 21 ? "*" : "[credential redacted]");
    if (connection.kind === "simplefin" && connection.endpoint) data = data.split(connection.endpoint).join("/accounts");
    return Buffer.from(data,"latin1");
  };
  response.body_base64 = scrub(Buffer.from(response.body_base64,"base64")).toString("base64");
  response.url = scrub(Buffer.from(response.url)).toString();
  for (const [name,value] of Object.entries(response.headers)) response.headers[name] = scrub(Buffer.from(value)).toString();
}
for (const connection of JSON.parse(process.env.EXHIBIT_CONNECTIONS || "[]")) { validateConnection(connection); connections.set(connection.origin, connection); }

const escape = value => String(value ?? "").replace(/[&<>"']/g, char => ({"&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;", "'":"&#39;"}[char]));
const json = value => JSON.stringify(value).replace(/</g, "\\u003c").replace(/>/g, "\\u003e").replace(/&/g, "\\u0026");
const style = "body{font:16px/1.5 system-ui;color:#172126;background:#f6f7f8;margin:40px auto;padding:0 20px;max-width:720px}a{color:#146350}form{padding:24px;background:white;border:1px solid #dce0e1;border-radius:12px;margin:20px 0}label{display:block;margin:14px 0}input,select,button{font:inherit;padding:9px;border:1px solid #bcc6c5;border-radius:6px;max-width:100%;box-sizing:border-box}input{display:block;width:100%}button{cursor:pointer;background:#146350;color:white;margin-right:8px}code{overflow-wrap:anywhere}.muted,small{font-size:14px;color:#52615f}h2{font-size:19px;margin:0}.message{padding:12px;background:#e2eee8}";
function connectPage(message = "") {
  const forms = requirements.map((requirement, index) => `<form method="post" action="/__exhibit/connect" autocomplete="off" data-connection-form><input type="hidden" name="csrf" value="${csrf}"><input type="hidden" name="index" value="${index}"><h2>${escape(requirement.name)}</h2><code>${escape(requirement.origin)}</code><p class="muted">${connections.has(requirement.origin) ? "Connected locally" : "No local credentials"}</p><label>Connection kind<select name="kind">${[["bearer","Bearer token"],["api-key","API key header"],["basic","Basic authentication"],["simplefin","SimpleFIN setup token"]].map(([kind, name]) => `<option value="${kind}" ${requirement.kind === kind ? "selected" : ""}>${name}</option>`).join("")}</select></label><div data-kinds="bearer api-key"><label>API key or bearer token<input type="password" name="secret" autocomplete="new-password"></label></div><div data-kinds="api-key"><label>API key header<input name="header" value="${escape(requirement.header || "X-API-Key")}"></label></div><div data-kinds="basic"><label>Basic username<input name="username" autocomplete="off"></label><label>Basic password<input type="password" name="password" autocomplete="new-password"></label></div><div data-kinds="simplefin"><label>SimpleFIN setup token<input type="password" name="setup" autocomplete="new-password"></label></div><small>Credentials are used only for this origin. Reconnecting replaces the current local connection.</small><p><button name="action" value="connect">Connect locally</button>${connections.has(requirement.origin) ? '<button name="action" value="disconnect">Disconnect</button>' : ""}</p></form>`).join("");
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Local connections · ${escape(manifest.title)}</title><style>${style}</style></head><body><a href="/">← Back to app</a><h1>Local connections</h1><p>Connect ${escape(manifest.title)} on this machine. Credentials stay outside the app frame and remain only until this runner stops.</p><p class="muted">This app can read connected data and send it to: ${escape([...allowed].join(", "))}. SimpleFIN restricts relay requests to its financial origin, with at most 24 provider requests per day.</p>${message ? `<p class="message" role="status">${escape(message)}</p>` : ""}${forms || "<p>This app has no allowed external origins.</p>"}<script type="module">${connectScript}</script></body></html>`;
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
async function claimSimpleFIN(setup, expectedOrigin) {
  if (!/^[A-Za-z0-9+/]+={0,2}$/.test(setup)) throw new Error("Enter a valid SimpleFIN setup token.");
  const claim = new URL(Buffer.from(setup, "base64").toString("utf8"));
  if (claim.protocol !== "https:" || claim.username || claim.password || claim.hash) throw new Error("SimpleFIN requires an HTTPS claim URL.");
  const response = await gate({url:claim.href, method:"POST", headers:{}, body_base64:""}, new Set([normalizeOrigin(claim.origin)]));
  if (response.status < 200 || response.status >= 300) throw new Error("SimpleFIN setup could not be claimed; it may already have been used.");
  const access = new URL(Buffer.from(response.body_base64, "base64").toString("utf8").trim());
  if (access.protocol !== "https:" || !access.username || !access.password || access.search || access.hash || normalizeOrigin(access.origin) !== expectedOrigin) throw new Error("SimpleFIN returned an invalid or different access origin; update exhibit.json if its origin changed.");
  return {origin:expectedOrigin, kind:"simplefin", username:decodeURIComponent(access.username), password:decodeURIComponent(access.password), endpoint:access.pathname.replace(/\/$/, "") + "/accounts", calls:0};
}
async function relay(input) {
  const checkPolicy = () => { if (input.policy !== framePolicy) { const failure = new Error("Connections changed; reloading the app with its current network policy."); failure.reload = true; throw failure; } };
  checkPolicy();
  const publicURL = input.url;
  const original = checkURL(input.url, allowed);
  const origin = normalizeOrigin(original.origin);
  const connection = connections.get(origin);
  if ([...financialOrigins].some(financialOrigin => financialOrigin !== origin)) throw new Error("This financial app may send data only to its financial origin, including after disconnect.");
  if ((manifest.connections || []).some(requirement => requirement.origin === origin) && !connection) throw new Error("Connect this app locally using Connections in the page header.");
  const key = (input.method || "GET") + " " + input.url;
  if (connection?.kind === "simplefin") {
    if (original.pathname !== "/accounts" || !["GET", "HEAD"].includes(input.method || "GET")) throw new Error("SimpleFIN permits only GET /accounts.");
    const saved = cache.get(key); if (saved && saved.expires > Date.now()) return saved.response;
    const day = new Date().toISOString().slice(0, 10);
    if (connection.day !== day) { connection.day = day; connection.calls = 0; }
    if (connection.calls >= 24) throw new Error("SimpleFIN's daily request limit has been reached.");
    connection.calls++;
    original.pathname = connection.endpoint;
    input = {...input, url:original.href};
  }
  const response = await gate(input, allowed, (url, headers) => {
    const hopOrigin = normalizeOrigin(url.origin);
    if ([...financialOrigins].some(financialOrigin => financialOrigin !== hopOrigin)) throw new Error("Financial data destinations remain restricted after disconnect.");
    if (!connection) return;
    if (connection.kind === "simplefin" && (hopOrigin !== origin || url.pathname !== connection.endpoint)) throw new Error("Financial connector redirects are restricted.");
    if (hopOrigin !== origin) return;
    if (connection.kind === "bearer") headers.authorization = "Bearer " + connection.secret;
    else if (["basic", "simplefin"].includes(connection.kind)) headers.authorization = "Basic " + Buffer.from(connection.username + ":" + connection.password).toString("base64");
    else if (connection.kind === "api-key") headers[connection.header.toLowerCase()] = connection.secret;
  });
  checkPolicy();
  if (connection?.kind === "simplefin") response.url = publicURL;
  if (connection) scrubResponse(connection,response);
  if (connection?.kind === "simplefin" && response.status >= 200 && response.status < 300) cache.set(key, {response, expires:Date.now() + 3600000});
  return response;
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
      const origins = [...(financialOrigins.size > 1 ? [] : financialOrigins.size ? financialOrigins : allowed)].join(" ");
      response.setHeader("Content-Security-Policy", `sandbox allow-scripts; default-src 'none'; script-src 'unsafe-inline' 'unsafe-eval' blob: data: ${origins}; style-src 'unsafe-inline' ${origins}; img-src data: blob: ${origins}; font-src data: ${origins}; media-src data: blob: ${origins}; connect-src 'none'; worker-src blob: data:; frame-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'self'`);
      const script = "<script>" + shim.replaceAll("__EXHIBIT_STATE_JSON__", json(JSON.stringify(state))).replaceAll("__EXHIBIT_POLICY__", json(framePolicy)) + "</script>";
      const head = /<head(?:\s[^>]*)?>/i.exec(page);
      send(response, 200, head ? page.slice(0, head.index + head[0].length) + script + page.slice(head.index + head[0].length) : script + page, "text/html; charset=utf-8");
    } else if (request.method === "GET" && url.pathname === "/__exhibit/connect") {
      response.setHeader("Referrer-Policy", "same-origin");
      response.setHeader("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'");
      send(response, 200, connectPage(), "text/html; charset=utf-8");
    } else if (request.method === "GET" && url.pathname === "/__exhibit/connections") {
      const list = requirements.filter(requirement => connections.has(requirement.origin)).map(requirement => { const connection = connections.get(requirement.origin); return {name:requirement.name,provider:connection.kind,kind:"native",origins:[connection.origin],endpoint:connection.kind === "simplefin" ? "/accounts" : ""}; });
      send(response, 200, JSON.stringify(list));
    } else if (request.method === "POST" && url.pathname === "/__exhibit/connect" && form) {
      const index = Number(form.get("index"));
      if (!Number.isInteger(index) || index < 0 || index >= requirements.length) { error(response, 400, "Unknown local connection."); return; }
      const requirement = requirements[index];
      let message = "Connection removed from this runner.";
      try {
        if (form.get("action") === "disconnect") connections.delete(requirement.origin);
        else {
          let connection = {origin:requirement.origin, kind:form.get("kind"), header:form.get("header"), secret:form.get("secret"), username:form.get("username"), password:form.get("password")};
          if (connection.kind === "simplefin") connection = await claimSimpleFIN(form.get("setup").trim(), requirement.origin);
          validateConnection(connection);
          if (connection.kind === "simplefin") {
            const financial_origins = [...new Set([...financialOrigins,connection.origin])].sort();
            fs.writeFileSync(policyPath+".tmp",JSON.stringify({financial_origins}),{mode:0o600});
            fs.renameSync(policyPath+".tmp",policyPath);
            financialOrigins.add(connection.origin);
          }
          connections.set(requirement.origin, connection);
          message = "Connected locally. Credentials remain in the runner's memory until it stops.";
        }
        cache.clear();
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
  } catch (failure) { if (!response.headersSent) { if (failure.reload) send(response, 409, JSON.stringify({error:failure.message,reload:true})); else error(response, 502, "The local runner refused or could not complete this request."); } else response.destroy(); }
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

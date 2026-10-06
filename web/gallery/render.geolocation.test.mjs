/* av-f446: the frame half of the geolocation gate, run rather than read.
 *
 * The gate is a section of bridgeScript, a Go string literal in
 * internal/render/render.go. A Go test can check that the bytes contain
 * "__avGeolocation"; it cannot check that the artifact's error callback runs
 * exactly once, that a cleared watch stays silent, or that the error an
 * artifact receives compares equal to PERMISSION_DENIED. Those are the
 * properties that decide whether a location tool hangs or reports a denial.
 *
 * So, like render.shim.test.mjs, this lifts the shipped section out of
 * render.go by its two section markers and runs it over a frame-shaped
 * environment. The extraction fails loudly if either marker moves.
 *
 * Its counterpart is detail.geolocation.test.mjs, which drives the host end of
 * the same postMessage channel. The message shapes are written out in both, so
 * a change to one end fails the other's file.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import vm from "node:vm";

const here = path.dirname(fileURLToPath(import.meta.url));
const RENDER_GO = path.join(here, "../../internal/render/render.go");

const ARTIFACT_ID = "art-1";
const API_ORIGIN = "https://app.test";

function gateSource() {
  const go = readFileSync(RENDER_GO, "utf8");
  const start = go.indexOf("// ---- Geolocation gate (av-f446) ----");
  assert.notEqual(start, -1, "the geolocation gate's section marker moved");
  const end = go.indexOf("// ---- File System Access picker polyfill", start);
  assert.notEqual(end, -1, "the section after the geolocation gate moved");
  return go.slice(start, end);
}

// What the browser exposes, as far as the gate touches it. The constructor
// throws, as the real one does, and `code` is a prototype getter that refuses
// any receiver it did not create, also as the real one does: an error built by
// Object.create would read it and throw unless the gate shadows it.
class GeolocationPositionError {
  constructor() { throw new TypeError("Illegal constructor"); }
}
Object.assign(GeolocationPositionError.prototype, {
  PERMISSION_DENIED: 1, POSITION_UNAVAILABLE: 2, TIMEOUT: 3
});
for (const name of ["code", "message"]) {
  Object.defineProperty(GeolocationPositionError.prototype, name, {
    get() { throw new TypeError("Illegal invocation"); },
    configurable: true
  });
}

// A frame: a window whose parent is the host, a navigator, and the
// capability-banner hook the preamble defines earlier in the same function.
function loadGate({ geolocation = true, positionError = true } = {}) {
  const posted = [];
  const listeners = {};
  const warnings = [];
  const parent = { postMessage(msg, target) { posted.push({ msg, target }); } };
  const window = {
    parent,
    addEventListener(type, fn) { (listeners[type] ||= []).push(fn); }
  };
  const reachedNative = [];
  const nativeGeolocation = {
    getCurrentPosition() { reachedNative.push("getCurrentPosition"); },
    watchPosition() { reachedNative.push("watchPosition"); return 99; },
    clearWatch() { reachedNative.push("clearWatch"); }
  };
  const navigator = geolocation ? { geolocation: nativeGeolocation } : {};
  const globals = {
    window, navigator, API_ORIGIN, ARTIFACT_ID,
    warnCapability: (capability, resource) => warnings.push([capability, resource])
  };
  if (positionError) globals.GeolocationPositionError = GeolocationPositionError;
  vm.runInContext(gateSource(), vm.createContext(globals), { filename: "render.go#geolocation-gate" });

  // A message as the host would send it: from the app origin, from the parent.
  const fromHost = (data, { origin = API_ORIGIN, source = parent } = {}) =>
    (listeners.message || []).forEach((fn) => fn({ data, origin, source }));
  return { navigator, nativeGeolocation, posted, warnings, reachedNative, fromHost };
}

function recorder() {
  const calls = [];
  return { calls, fn: (value) => calls.push(value) };
}

test("a request goes to the host, pinned to the app origin, and carries no options", () => {
  const { navigator, posted } = loadGate();
  const success = recorder();

  navigator.geolocation.getCurrentPosition(success.fn, () => {}, { enableHighAccuracy: true, timeout: 5 });

  assert.equal(posted.length, 1);
  assert.equal(posted[0].target, API_ORIGIN, "never broadcast: the frame posts only to the app origin");
  assert.deepEqual(Object.keys(posted[0].msg).sort(), ["__avGeolocation", "artifactId", "id"],
    "the host reads no location, so options would be data with no reader");
  assert.equal(posted[0].msg.__avGeolocation, true);
  assert.equal(posted[0].msg.artifactId, ARTIFACT_ID);
  assert.deepEqual(success.calls, []);
});

test("the host's answer runs the error callback with PERMISSION_DENIED, once", () => {
  const { navigator, posted, fromHost, warnings } = loadGate();
  const success = recorder();
  const error = recorder();

  navigator.geolocation.getCurrentPosition(success.fn, error.fn);
  const { id } = posted[0].msg;
  fromHost({ __avGeolocationResult: true, id, banner: false, error: "User denied Geolocation" });
  fromHost({ __avGeolocationResult: true, id, banner: false, error: "User denied Geolocation" });

  assert.equal(error.calls.length, 1, "one request, one answer, however many replies arrive");
  const [err] = error.calls;
  assert.equal(err.code, 1);
  assert.equal(err.code, err.PERMISSION_DENIED, "artifacts compare against the constant, not the number");
  assert.ok(err instanceof GeolocationPositionError, "and some check the type");
  assert.equal(err.message, "User denied Geolocation");
  assert.deepEqual(success.calls, [], "the gate never reports a position");
  assert.deepEqual(warnings, [], "a denial is not the already-approved case, so no banner");
});

test("the already-approved answer raises the capability banner and still settles", () => {
  const { navigator, posted, fromHost, warnings } = loadGate();
  const error = recorder();

  navigator.geolocation.getCurrentPosition(() => {}, error.fn);
  fromHost({ __avGeolocationResult: true, id: posted[0].msg.id, banner: true, error: "open it directly" });

  assert.deepEqual(warnings, [["geolocation", null]]);
  assert.equal(error.calls.length, 1);
  assert.equal(error.calls[0].message, "open it directly");
});

test("only the host may answer", () => {
  const { navigator, posted, fromHost } = loadGate();
  const error = recorder();

  navigator.geolocation.getCurrentPosition(() => {}, error.fn);
  const reply = { __avGeolocationResult: true, id: posted[0].msg.id, banner: false };
  fromHost(reply, { origin: "https://evil.test" });
  fromHost(reply, { source: {} });
  assert.deepEqual(error.calls, [], "the artifact's own frame, or anyone else, cannot settle a request");

  fromHost(reply);
  assert.equal(error.calls.length, 1);
});

test("watchPosition returns a usable id, and a cleared watch never calls back", () => {
  const { navigator, posted, fromHost } = loadGate();
  const kept = recorder();
  const cleared = recorder();

  const keptId = navigator.geolocation.watchPosition(() => {}, kept.fn);
  const clearedId = navigator.geolocation.watchPosition(() => {}, cleared.fn);
  assert.ok(Number.isInteger(keptId) && keptId > 0);
  assert.notEqual(keptId, clearedId);

  navigator.geolocation.clearWatch(clearedId);
  for (const { msg } of posted) fromHost({ __avGeolocationResult: true, id: msg.id, banner: false });

  assert.equal(kept.calls.length, 1);
  assert.deepEqual(cleared.calls, [], "the spec's rule for a cleared watch: no callback after clearWatch");
});

test("the native methods are replaced, not wrapped", () => {
  const { navigator, nativeGeolocation, reachedNative } = loadGate();

  assert.equal(navigator.geolocation, nativeGeolocation, "the object an artifact holds stays the same object");
  navigator.geolocation.getCurrentPosition(() => {});
  navigator.geolocation.watchPosition(() => {});
  navigator.geolocation.clearWatch(1);
  assert.deepEqual(reachedNative, []);
});

test("a missing success callback is the artifact's bug, reported as the native call reports it", () => {
  const { navigator, posted } = loadGate();

  assert.throws(() => navigator.geolocation.getCurrentPosition(null), {
    name: "TypeError", message: /parameter 1 is not a function/
  });
  assert.throws(() => navigator.geolocation.watchPosition(undefined), { name: "TypeError" });
  assert.deepEqual(posted, [], "nothing reaches the host for a call that never happened");
});

test("an answer to a request with no error callback is dropped quietly", () => {
  const { navigator, posted, fromHost } = loadGate();

  navigator.geolocation.getCurrentPosition(() => {});
  assert.doesNotThrow(() => fromHost({ __avGeolocationResult: true, id: posted[0].msg.id, banner: false }));
});

test("with no navigator.geolocation at all, the gate is installed as the object", () => {
  // navigator.geolocation is [SecureContext], so an http:// render origin
  // has none, and an artifact that touches it would die on property access.
  const { navigator, posted } = loadGate({ geolocation: false });

  assert.equal(typeof navigator.geolocation.getCurrentPosition, "function");
  navigator.geolocation.getCurrentPosition(() => {}, () => {});
  assert.equal(posted.length, 1);
});

test("without GeolocationPositionError the error is still shaped like one", () => {
  const { navigator, posted, fromHost } = loadGate({ positionError: false });
  const error = recorder();

  navigator.geolocation.getCurrentPosition(() => {}, error.fn);
  fromHost({ __avGeolocationResult: true, id: posted[0].msg.id, banner: false });

  const [err] = error.calls;
  assert.equal(err.code, 1);
  assert.equal(err.PERMISSION_DENIED, 1);
  assert.equal(err.message, "User denied Geolocation", "the default message is Chromium's own");
});

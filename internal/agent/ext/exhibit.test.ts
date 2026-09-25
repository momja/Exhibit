/**
 * Tool-level tests for edit_artifact (av-f5i5): registration scoping, the
 * GET -> validate -> PATCH flow, artifact_saved details (the preview
 * re-render rides on them), and no-PATCH-on-invalid.
 *
 * exhibit.ts imports bare `typebox`, which pi provides at runtime. Under
 * plain node that resolves to ext/testdata/modpath/typebox (a minimal test
 * double — schemas are opaque descriptors, enough for registration), wired
 * via NODE_PATH by ext_test.go. Run directly with e.g.
 *   NODE_PATH=./testdata/modpath node --test exhibit.test.ts
 */
import { describe, it, before } from "node:test";
import assert from "node:assert/strict";

let exhibitMod = null;

before(async () => {
	process.env.EXHIBIT_API_URL = "http://exhibit.test";
	process.env.EXHIBIT_TOKEN = "test-token";
	process.env.EXHIBIT_ARTIFACT_ID = "artifact-1";
	process.env.EXHIBIT_DATA_NONCE = "nonce1";
	exhibitMod = await import("./exhibit.ts");
});

function makePi() {
	const tools = new Map();
	return {
		tools,
		registerTool(def) {
			tools.set(def.name, def);
		},
		registerProvider() {},
	};
}

function needExhibit(t) {
	if (!exhibitMod) return t.skip("exhibit.ts failed to load");
}

/** Stub fetch routing on method+path; records every call. */
function makeFetch({ body, patchResult, widgetBody = null, widgetPutResult = null }) {
	const calls = [];
	const fetch = async (url, opts) => {
		calls.push({ url, method: opts?.method, body: opts?.body });
		const path = String(url).replace("http://exhibit.test", "");
		if (opts?.method === "GET" && path.startsWith("/api/artifacts/artifact-1?body=true")) {
			return { ok: true, text: async () => JSON.stringify({ id: "artifact-1", title: "T", network_allowlist: [], body }) };
		}
		if (opts?.method === "PATCH" && path === "/api/artifacts/artifact-1") {
			return { ok: true, text: async () => JSON.stringify(patchResult) };
		}
		if (path === "/api/artifacts/artifact-1/widget") {
			if (opts?.method === "GET") {
				if (widgetBody === null) {
					return { ok: false, status: 404, text: async () => "not found" };
				}
				return { ok: true, text: async () => JSON.stringify({ body: widgetBody }) };
			}
			if (opts?.method === "PUT") {
				return { ok: true, text: async () => JSON.stringify(widgetPutResult) };
			}
		}
		throw new Error("unexpected fetch " + opts?.method + " " + path);
	};
	return { calls, fetch };
}

const PATCH_OK = {
	artifact: { id: "artifact-1", title: "T", network_allowlist: [] },
	network_footprint: [],
	footprint_changed: false,
};

const WIDGET_PUT_OK = {
	widget_url: "http://exhibit.test/w/artifact-1",
	unapproved_origins: [],
};

describe("edit_artifact tool", () => {
	it("registers with the same scoping as write_artifact (no artifact id parameter)", async (t) => {
		needExhibit(t);
		const pi = makePi();
		await exhibitMod.default(pi);
		const tool = pi.tools.get("edit_artifact");
		assert.ok(tool, "edit_artifact registered");
		const props = tool.parameters?.properties ?? {};
		assert.ok(!("id" in props) && !("artifactId" in props) && !("artifact_id" in props) && !("path" in props), "takes no artifact id or path — the session's bound artifact is the target");
		assert.ok("edits" in props, "takes edits[]");
	});

	it("GETs the current body, PATCHes only the edited result, and reports artifact_saved", async (t) => {
		needExhibit(t);
		const pi = makePi();
		await exhibitMod.default(pi);
		const { calls, fetch } = makeFetch({
			body: "<h1>Hi</h1><p>keep me</p>",
			patchResult: PATCH_OK,
		});
		const origFetch = globalThis.fetch;
		globalThis.fetch = fetch;
		try {
			const result = await pi.tools.get("edit_artifact").execute("call-1", {
				edits: [{ oldText: "<h1>Hi</h1>", newText: "<h1>Yo</h1>" }],
			});
			const patch = calls.find((c) => c.method === "PATCH");
			assert.ok(patch, "PATCH issued");
			assert.equal(JSON.parse(patch.body).body, "<h1>Yo</h1><p>keep me</p>");
			assert.equal(result.details.exhibit, "artifact_saved", "preview re-render event fires");
			assert.equal(result.details.action, "updated");
		} finally {
			globalThis.fetch = origFetch;
		}
	});

	it("issues no PATCH when any edit is invalid", async (t) => {
		needExhibit(t);
		const pi = makePi();
		await exhibitMod.default(pi);
		const { calls, fetch } = makeFetch({ body: "<h1>Hi</h1>", patchResult: PATCH_OK });
		const origFetch = globalThis.fetch;
		globalThis.fetch = fetch;
		try {
			await assert.rejects(
				pi.tools.get("edit_artifact").execute("call-1", {
					edits: [{ oldText: "missing piece", newText: "x" }],
				}),
				/edits\[0\] matches nothing/,
			);
			assert.ok(!calls.some((c) => c.method === "PATCH"), "no PATCH issued");
		} finally {
			globalThis.fetch = origFetch;
		}
	});

	it("refuses without a bound artifact, like every other session tool", async (t) => {
		needExhibit(t);
		delete process.env.EXHIBIT_ARTIFACT_ID;
		const fresh = await import("./exhibit.ts?unbound=1");
		process.env.EXHIBIT_ARTIFACT_ID = "artifact-1";
		const pi = makePi();
		await fresh.default(pi);
		await assert.rejects(
			pi.tools.get("edit_artifact").execute("call-1", { edits: [{ oldText: "a", newText: "b" }] }),
			/call create_artifact first/,
		);
	});
});

describe("edit_widget tool", () => {
	it("registers with the same scoping as set_widget (no artifact id parameter)", async (t) => {
		needExhibit(t);
		const pi = makePi();
		await exhibitMod.default(pi);
		const tool = pi.tools.get("edit_widget");
		assert.ok(tool, "edit_widget registered");
		const props = tool.parameters?.properties ?? {};
		assert.ok(!("id" in props) && !("artifactId" in props) && !("artifact_id" in props) && !("path" in props), "takes no artifact id or path");
		assert.ok("edits" in props, "takes edits[]");
	});

	it("GETs the current tile, PUTs only the edited result, and reports widget_saved", async (t) => {
		needExhibit(t);
		const pi = makePi();
		await exhibitMod.default(pi);
		const { calls, fetch } = makeFetch({
			widgetBody: '<div class="v" id="v">—</div>',
			widgetPutResult: WIDGET_PUT_OK,
		});
		const origFetch = globalThis.fetch;
		globalThis.fetch = fetch;
		try {
			const result = await pi.tools.get("edit_widget").execute("call-1", {
				edits: [{ oldText: '<div class="v" id="v">—</div>', newText: '<div class="v" id="v">0</div>' }],
			});
			const put = calls.find((c) => c.method === "PUT");
			assert.ok(put, "PUT issued");
			assert.equal(JSON.parse(put.body).body, '<div class="v" id="v">0</div>');
			assert.equal(result.details.exhibit, "widget_saved", "tile re-render event fires");
		} finally {
			globalThis.fetch = origFetch;
		}
	});

	it("issues no PUT when any edit is invalid", async (t) => {
		needExhibit(t);
		const pi = makePi();
		await exhibitMod.default(pi);
		const { calls, fetch } = makeFetch({
			widgetBody: "<div>tile</div>",
			widgetPutResult: WIDGET_PUT_OK,
		});
		const origFetch = globalThis.fetch;
		globalThis.fetch = fetch;
		try {
			await assert.rejects(
				pi.tools.get("edit_widget").execute("call-1", {
					edits: [{ oldText: "missing piece", newText: "x" }],
				}),
				/edits\[0\] matches nothing/,
			);
			assert.ok(!calls.some((c) => c.method === "PUT"), "no PUT issued");
		} finally {
			globalThis.fetch = origFetch;
		}
	});

	it("tells the model to create the tile when the artifact has none yet", async (t) => {
		needExhibit(t);
		const pi = makePi();
		await exhibitMod.default(pi);
		const { calls, fetch } = makeFetch({
			widgetBody: null,
			widgetPutResult: WIDGET_PUT_OK,
		});
		const origFetch = globalThis.fetch;
		globalThis.fetch = fetch;
		try {
			await assert.rejects(
				pi.tools.get("edit_widget").execute("call-1", {
					edits: [{ oldText: "a", newText: "b" }],
				}),
				/create it with set_widget first/,
			);
			assert.ok(!calls.some((c) => c.method === "PUT"), "no PUT issued");
		} finally {
			globalThis.fetch = origFetch;
		}
	});
});

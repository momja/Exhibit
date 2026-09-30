/**
 * Tests for the usage-policy guardrail (av-gust): config parsing, the
 * user-words cut, both screening paths against a stubbed model registry, the
 * fail-closed cases, and the input hook's contract with the host.
 */
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import guard, { configFromEnv, userWords, screen, GUARD_SIGNAL, CLASSIFIER_BLOCK_THRESHOLD } from "./guard.ts";

const FENCE = "-----BEGIN EXHIBIT UNTRUSTED DATA abc123-----";

const cfg = { provider: "p", model: "m", apiKey: "guard-key", dataFence: FENCE, policy: "POLICY" };

const USAGE = { input: 120, output: 1, cacheRead: 0, cacheWrite: 0, totalTokens: 121, cost: { input: 0.000005, output: 0, cacheRead: 0, cacheWrite: 0, total: 0.000005 } };

function chatRegistry(replyText: string | null, { stopReason = "stop", usage = undefined as any } = {}) {
	const calls: any[] = [];
	return {
		calls,
		getModelOfType: () => undefined,
		find: (provider: string, model: string) => (provider === "p" && model === "m" ? { id: "m" } : undefined),
		classify: async () => { throw new Error("classify must not be called for a chat model"); },
		complete: async (model: any, context: any, options: any) => {
			calls.push({ model, context, options });
			return { stopReason, usage, content: replyText === null ? [] : [{ type: "text", text: replyText }] };
		},
	};
}

function classifierRegistry(result: any) {
	const calls: any[] = [];
	return {
		calls,
		getModelOfType: (type: string, provider: string, model: string) =>
			type === "classifier" && provider === "p" && model === "m" ? { id: "jev" } : undefined,
		find: () => { throw new Error("find must not be called for a classifier model"); },
		classify: async (model: any, context: any, options: any) => {
			calls.push({ model, context, options });
			return result;
		},
		complete: async () => { throw new Error("complete must not be called for a classifier model"); },
	};
}

describe("configFromEnv", () => {
	it("returns null when nothing is set", () => {
		assert.equal(configFromEnv({}), null);
	});
	it("reads all three values", () => {
		const c = configFromEnv({ EXHIBIT_GUARD_PROVIDER: " typesafe ", EXHIBIT_GUARD_MODEL: "jev-latest", EXHIBIT_GUARD_API_KEY: "k" });
		assert.equal(c?.provider, "typesafe");
		assert.equal(c?.model, "jev-latest");
		assert.equal(c?.apiKey, "k");
	});
});

describe("userWords", () => {
	it("cuts the message at the first data block", () => {
		assert.equal(userWords(`make it green\n\n${FENCE}\nlabel: x\n\nsexually explicit body text`, FENCE), "make it green\n");
	});
	it("keeps the whole message when there is no block", () => {
		assert.equal(userWords("make it green", FENCE), "make it green");
	});
	it("returns everything when no fence is configured", () => {
		assert.equal(userWords(`a\n${FENCE}\nb`, ""), `a\n${FENCE}\nb`);
	});
});

describe("screen with a chat model", () => {
	it("allows on ALLOW and passes the guard's own key", async () => {
		const reg = chatRegistry(" allow\n");
		const v = await screen(reg, cfg, "build a todo list");
		assert.equal(v.allowed, true);
		assert.equal(reg.calls[0].options.apiKey, "guard-key");
		// The message travels JSON-encoded as data, never spliced into the instruction.
		assert.equal(reg.calls[0].context.messages[0].content, JSON.stringify({ message: "build a todo list" }));
		assert.match(reg.calls[0].context.systemPrompt, /POLICY/);
	});
	it("blocks on BLOCK", async () => {
		const v = await screen(chatRegistry("BLOCK"), cfg, "x");
		assert.equal(v.allowed, false);
	});
	it("throws on an unparseable reply", async () => {
		await assert.rejects(screen(chatRegistry("Sure! Here's a story"), cfg, "x"), /unparseable/);
	});
	it("throws on a provider error", async () => {
		await assert.rejects(screen(chatRegistry(null, { stopReason: "error" }), cfg, "x"), /error/);
	});
	it("throws on an unknown model", async () => {
		await assert.rejects(screen(chatRegistry("ALLOW"), { ...cfg, model: "nope" }, "x"), /unknown guardrail model/);
	});
});

describe("screen reports usage", () => {
	it("reports the chat model's usage", async () => {
		const seen: unknown[] = [];
		await screen(chatRegistry("ALLOW", { usage: USAGE }), cfg, "x", undefined, (u) => seen.push(u));
		assert.deepEqual(seen, [USAGE]);
	});
	it("reports usage even when the reply is unparseable", async () => {
		const seen: unknown[] = [];
		await assert.rejects(screen(chatRegistry("hmm", { usage: USAGE }), cfg, "x", undefined, (u) => seen.push(u)));
		assert.deepEqual(seen, [USAGE], "the screen spent money before it answered garbage");
	});
	it("reports the classifier's usage", async () => {
		const seen: unknown[] = [];
		await screen(classifierRegistry({ stopReason: "stop", usage: USAGE, answers: { violates: { type: "bool", probability: 0.1 } } }),
			cfg, "x", undefined, (u) => seen.push(u));
		assert.deepEqual(seen, [USAGE]);
	});
	it("reports nothing when the provider returned no usage", async () => {
		const seen: unknown[] = [];
		await screen(chatRegistry("ALLOW"), cfg, "x", undefined, (u) => seen.push(u));
		assert.deepEqual(seen, []);
	});
});

describe("screen with a classifier model", () => {
	const answer = (probability: number) => ({ stopReason: "stop", answers: { violates: { type: "bool", probability } } });

	it("allows below the threshold", async () => {
		const reg = classifierRegistry(answer(CLASSIFIER_BLOCK_THRESHOLD - 0.01));
		const v = await screen(reg, cfg, "hello");
		assert.equal(v.allowed, true);
		assert.deepEqual(reg.calls[0].context.state, { message: "hello" });
		assert.equal(reg.calls[0].context.questions.violates.type, "bool");
		assert.equal(reg.calls[0].options.apiKey, "guard-key");
	});
	it("blocks at the threshold", async () => {
		const v = await screen(classifierRegistry(answer(CLASSIFIER_BLOCK_THRESHOLD)), cfg, "x");
		assert.equal(v.allowed, false);
	});
	it("throws when the classifier fails", async () => {
		await assert.rejects(screen(classifierRegistry({ stopReason: "error", errorMessage: "503" }), cfg, "x"), /503/);
	});
	it("throws when the answer carries no probability", async () => {
		await assert.rejects(screen(classifierRegistry({ stopReason: "stop", answers: {} }), cfg, "x"), /no probability/);
	});
});

describe("input hook", () => {
	function load(env: Record<string, string>) {
		const saved = { ...process.env };
		Object.assign(process.env, env);
		let handler: any = null;
		try {
			guard({ on: (event: string, h: any) => { if (event === "input") handler = h; } } as any);
		} finally {
			process.env = saved;
		}
		return handler;
	}
	const env = { EXHIBIT_GUARD_PROVIDER: "p", EXHIBIT_GUARD_MODEL: "m", EXHIBIT_GUARD_API_KEY: "guard-key" };

	function ctx(registry: any) {
		const notes: string[] = [];
		return { notes, modelRegistry: registry, ui: { notify: (msg: string) => notes.push(msg) } };
	}

	it("registers nothing without configuration", () => {
		assert.equal(load({ EXHIBIT_GUARD_PROVIDER: "", EXHIBIT_GUARD_MODEL: "", EXHIBIT_GUARD_API_KEY: "" }), null);
	});
	it("continues an allowed message silently", async () => {
		const c = ctx(chatRegistry("ALLOW"));
		const r = await load(env)({ type: "input", text: "hi", source: "rpc" }, c);
		assert.deepEqual(r, { action: "continue" });
		assert.deepEqual(c.notes, []);
	});
	it("handles a blocked message and signals the host", async () => {
		const c = ctx(chatRegistry("BLOCK"));
		const r = await load(env)({ type: "input", text: "bad", source: "rpc" }, c);
		assert.deepEqual(r, { action: "handled" });
		assert.equal(c.notes.length, 1);
		assert.ok(c.notes[0].startsWith(GUARD_SIGNAL + "blocked"));
	});
	it("fails closed when the screen errors", async () => {
		const c = ctx(chatRegistry("maybe?"));
		const r = await load(env)({ type: "input", text: "x", source: "rpc" }, c);
		assert.deepEqual(r, { action: "handled" });
		assert.ok(c.notes[0].startsWith(GUARD_SIGNAL + "error"));
	});
	it("reports usage on an allowed message, and nothing else", async () => {
		const c = ctx(chatRegistry("ALLOW", { usage: USAGE }));
		const r = await load(env)({ type: "input", text: "hi", source: "rpc" }, c);
		assert.deepEqual(r, { action: "continue" });
		assert.equal(c.notes.length, 1);
		assert.ok(c.notes[0].startsWith(GUARD_SIGNAL + "usage "));
		assert.deepEqual(JSON.parse(c.notes[0].slice((GUARD_SIGNAL + "usage ").length)), { provider: "p", model: "m", usage: USAGE });
	});
	it("reports usage before the block on a blocked message", async () => {
		const c = ctx(chatRegistry("BLOCK", { usage: USAGE }));
		await load(env)({ type: "input", text: "bad", source: "rpc" }, c);
		assert.deepEqual(c.notes.map((n) => n.split(" ")[0]), [GUARD_SIGNAL + "usage", GUARD_SIGNAL + "blocked"]);
	});
	it("skips text an extension injected", async () => {
		const c = ctx(chatRegistry("BLOCK"));
		const r = await load(env)({ type: "input", text: "x", source: "extension" }, c);
		assert.deepEqual(r, { action: "continue" });
	});
});

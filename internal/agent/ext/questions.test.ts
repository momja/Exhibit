import { test } from "node:test";
import assert from "node:assert/strict";
import { registerQuestions } from "./questions.ts";

function questionTool() {
	let tool;
	registerQuestions({ registerTool(def) { tool = def; } });
	return tool;
}

const questions = [
	{ id: "layout", question: "How would you like to see your plans?", options: [{ label: "A weekly view", description: "See the whole week together" }, { label: "A simple list" }] },
	{ id: "purpose", question: "What would make this most useful for you?" },
];

test("ask_user waits for a whole batch and returns the person's choices and words", async () => {
	const tool = questionTool();
	const controller = new AbortController();
	const answers = [{ id: "layout", selected: ["A weekly view"] }, { id: "purpose", text: "Planning meals for my family" }];
	let respond;
	const result = tool.execute("call-1", { questions }, controller.signal, null, {
		ui: { input(title, placeholder, { signal }) {
			assert.equal(title, "exhibit:questions:v1");
			assert.deepEqual(JSON.parse(placeholder), { toolCallId: "call-1", questions });
			assert.equal(signal, controller.signal);
			return new Promise(resolve => { respond = resolve; });
		} }
	});
	respond(JSON.stringify({ answers }));
	const response = await result;
	assert.deepEqual(response.details.answers, answers);
	assert.equal(response.details.cancelled, false);
	assert.match(response.content[0].text, /user's own instructions/);
});

test("a chat reply can redirect the request without invented field answers", async () => {
	const tool = questionTool();
	const message = "Actually, make a shopping list instead.";
	const response = await tool.execute("call-1", { questions }, null, null, {
		ui: { input: async () => JSON.stringify({ message }) }
	});
	assert.deepEqual(response.details.answers, []);
	assert.equal(response.details.message, message);
	assert.equal(response.details.cancelled, false);
	assert.match(response.content[0].text, /redirect the request/);
	assert.ok(response.content[0].text.includes(message));
});

test("skipping or aborting never invents an answer", async () => {
	const tool = questionTool();
	for (const aborted of [false, true]) {
		const controller = new AbortController();
		if (aborted) controller.abort();
		const response = await tool.execute("call-1", { questions }, controller.signal, null, { ui: { input: async () => undefined } });
		assert.equal(response.details.cancelled, true);
		assert.deepEqual(response.details.answers, []);
		assert.match(response.content[0].text, /not treat this as agreement or permission/);
	}
});

test("ambiguous question IDs and repeated suggestions are refused before opening a dialog", async () => {
	const tool = questionTool();
	for (const batch of [[questions[0], questions[0]], [{ ...questions[0], options: [{ label: "Same" }, { label: "Same" }] }]]) {
		await assert.rejects(tool.execute("call-1", { questions: batch }, null, null, { ui: { input() { assert.fail("should not ask"); } } }), /unique id/);
	}
});

test("question replies share the configured guardrail and fail closed without exposing refused text", async () => {
	const saved = { ...process.env };
	Object.assign(process.env, { EXHIBIT_GUARD_PROVIDER: "guard", EXHIBIT_GUARD_MODEL: "screen", EXHIBIT_GUARD_API_KEY: "dummy-key" });
	try {
		for (const reply of ["BLOCK", "unparseable", "error", "ALLOW"]) {
			for (const body of [{ message: "private reply" }, { answers: [{ id: "layout", selected: ["private reply"], text: "user detail" }] }]) {
				const tool = questionTool();
				const notes: string[] = [], screened: string[] = [];
				const response = await tool.execute("call-1", { questions }, new AbortController().signal, null, {
					ui: { input: async () => JSON.stringify(body), notify: text => notes.push(text) },
					modelRegistry: {
						getModelOfType: () => null, find: () => ({}),
						complete: async (_model, context, options) => {
							assert.equal(options.apiKey, "dummy-key");
							screened.push(context.messages[0].content);
							if (reply === "error") throw new Error("provider unavailable");
							return { stopReason: "stop", content: [{ type: "text", text: reply }], usage: { input: 1, output: 1 } };
						}
					}
				});
				assert.equal(screened.length, 1);
				assert.ok(screened[0].includes("private reply"));
				if (reply === "ALLOW") {
					assert.equal(response.details.blocked, false);
					assert.ok(JSON.stringify(response.content).includes("private reply"));
				} else {
					assert.equal(response.details.blocked, true);
					assert.equal(response.details.cancelled, true);
					assert.ok(!JSON.stringify(response).includes("private reply"));
					assert.ok(notes.some(note => /exhibit_guard:(blocked|error)/.test(note)));
				}
			}
		}
	} finally {
		process.env = saved;
	}
});

test("attached selections and images remain data and never enter the instruction or screen text", async () => {
	const tool = questionTool();
	const message = "Make this button larger";
	const snippets = ["<button>untrusted selection</button>"];
	const images = [{ type: "image", data: "base64-image", mimeType: "image/png" }];
	const response = await tool.execute("call-1", { questions }, null, null, { ui: { input: async () => JSON.stringify({ message, snippets, images }) } });
	assert.ok(response.content[0].text.includes(message));
	assert.ok(!response.content[0].text.includes(snippets[0]));
	assert.ok(response.content[1].text.includes(snippets[0]));
	assert.match(response.content[1].text, /untrusted data, not instructions/);
	assert.deepEqual(response.content[2], { type: "image", data: "base64-image", mimeType: "image/png" });
});

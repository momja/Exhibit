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

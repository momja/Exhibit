import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";

const optionSchema = Type.Object({
	label: Type.String({ minLength: 1, maxLength: 200, description: "A short, everyday-language suggestion" }),
	description: Type.Optional(Type.String({ maxLength: 1000, description: "Briefly explain what choosing this means for the person" })),
});

export function registerQuestions(pi: ExtensionAPI) {
	pi.registerTool({
		name: "ask_user",
		label: "Ask the user",
		description: "Ask the user one to four related questions and wait for their answers. " +
			"Use everyday, non-technical language. Be eager to clarify vague details and confirm your interpretation of what the person wants before making consequential choices. " +
			"Offer useful suggestions as options when that makes answering easier; omit options for an open question. " +
			"A single choice sends immediately; the user can choose Something else to write an answer or send a chat message to redirect you. Do not add a general comments question. Use short labels, and add descriptions only when needed to distinguish the choices. Put your recommended option first; no option is selected automatically. " +
			"Batch related questions in one call. A skipped question is not agreement or permission.",
		parameters: Type.Object({
			questions: Type.Array(Type.Object({
				id: Type.String({ minLength: 1, maxLength: 64, description: "Unique short identifier for this question" }),
				question: Type.String({ minLength: 1, maxLength: 2000, description: "A short, self-contained question in everyday, non-technical language" }),
				options: Type.Optional(Type.Array(optionSchema, { maxItems: 6, description: "Optional suggestions; the user can also write their own answer" })),
				multiSelect: Type.Optional(Type.Boolean({ description: "Allow choosing several suggestions (default false)" })),
			}), { minItems: 1, maxItems: 4 }),
		}),
		async execute(toolCallId, params, signal, _onUpdate, ctx) {
			const ids = new Set(params.questions.map(q => q.id));
			if (ids.size !== params.questions.length || params.questions.some(q => !q.id.trim() || !q.question.trim() ||
				(q.options || []).some(o => !o.label.trim()) || new Set((q.options || []).map(o => o.label)).size !== (q.options || []).length)) {
				throw new Error("Give each question a unique id, non-empty question, and distinct, non-empty option labels.");
			}
			// input() supports AbortSignal in RPC mode; custom terminal UI does
			// not. Exhibit recognizes this private title and renders the batch.
			const reply = await ctx.ui.input("exhibit:questions:v1", JSON.stringify({ toolCallId, questions: params.questions }), { signal });
			const cancelled = reply === undefined || signal?.aborted === true;
			const response = cancelled ? {} : JSON.parse(reply);
			const answers = response.answers || [];
			const message = response.message || "";
			return {
				content: [{ type: "text" as const, text: cancelled
					? "The user did not answer these questions. Do not treat this as agreement or permission. Ask in chat if the answer is essential; otherwise explain any assumption before continuing."
					: "The user replied to your questions. These are the user's own instructions and preferences; a chat message may redirect the request instead of answering each question:\n" + JSON.stringify({ answers, message }) }],
				details: { exhibit: "questions_answered", questions: params.questions, answers, message, cancelled },
			};
		},
	});
}

/**
 * Exhibit's artifact inference worker (Exh-zue3).
 *
 * Loaded into ONE long-lived `pi --mode rpc` process that serves every
 * artifact AI call on the instance (internal/inference/worker.go). It is not
 * an agent session: the host starts Pi with no tools, no session file, no
 * user extensions, no MCP and no context files, and every prompt the host
 * sends is a JSON envelope this extension consumes on Pi's `input` hook and
 * answers `handled`. No prompt ever reaches a model turn.
 *
 * Envelope (one line, ≤ MAX_ENVELOPE bytes):
 *   { id, op: "ping" }
 *   { id, op: "resolve", provider, kind: "complete"|"decide", model }
 *   { id, op: "complete", provider, model, apiKey, context, maxTokens }
 *   { id, op: "classify", provider, model, apiKey, context }
 *   { id, op: "screen", provider, model, apiKey, text }
 *   { id, op: "cancel" }                          (id names the job)
 *
 * Replies are notify messages prefixed SIGNAL, each { id, result } or
 * { id, error, usage? }. A job starts asynchronously and the hook returns at
 * once, so calls overlap; each job owns its key, AbortController and options.
 * Nothing here mutates registry-wide auth, and provider error text is never
 * forwarded: it can echo inputs and credentials.
 *
 * EXHIBIT_INFERENCE_MOCK_URL (operator-set, tests and demos only) sends every
 * provider request to that server, keeping the request path, so Pi's real
 * adapters talk to internal/mockllm. Requests cannot choose an endpoint.
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { DEFAULT_POLICY, screen } from "./guard.ts";

const SIGNAL = "exhibit_inference:";
const MAX_ENVELOPE = 262_144;
const MAX_ACTIVE = 16;

type Reply = { id: string; result?: unknown; error?: string; usage?: unknown };

export default function (pi: ExtensionAPI) {
	const active = new Map<string, AbortController>();
	const mockURL = process.env.EXHIBIT_INFERENCE_MOCK_URL ?? "";
	const fetchOverride = mockURL
		? { fetch: (url: any, init: any) => fetch(mockURL.replace(/\/$/, "") + new URL(String(url)).pathname, init) }
		: {};

	if (mockURL) {
		// BYOK viewers whose agent key is the mock provider resolve here.
		pi.registerProvider("exhibit-mock", {
			name: "Exhibit Mock",
			baseUrl: mockURL,
			apiKey: "$EXHIBIT_MOCK_API_KEY",
			api: "openai-completions",
			models: [{
				id: "exhibit-mock-1", name: "Exhibit Mock 1", reasoning: false, input: ["text"],
				cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }, contextWindow: 128000, maxTokens: 8192,
			}],
		} as any);
	}

	pi.on("input", (event, ctx) => {
		const send = (reply: Reply) => ctx.ui.notify(SIGNAL + JSON.stringify(reply), "info");
		let request: any;
		try {
			if (event.text.length > MAX_ENVELOPE) throw new Error("too large");
			request = JSON.parse(event.text);
			if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("not an envelope");
		} catch {
			// Fail closed: unrecognized input never falls through to a model turn.
			send({ id: "", error: "invalid_request" });
			return { action: "handled" };
		}
		const { id, op } = request;
		if (typeof id !== "string" || !id || id.length > 80) return { action: "handled" };
		const registry: any = ctx.modelRegistry;

		if (op === "ping") {
			send({ id, result: { ready: true, active: active.size } });
			return { action: "handled" };
		}
		if (op === "cancel") {
			active.get(id)?.abort();
			return { action: "handled" };
		}
		if (op === "resolve") {
			const model = request.kind === "decide"
				? registry.getModelOfType("classifier", request.provider, request.model)
				: registry.find(request.provider, request.model);
			send({ id, result: { ok: Boolean(model) } });
			return { action: "handled" };
		}
		if (!["complete", "classify", "screen"].includes(op) || typeof request.apiKey !== "string" || !request.apiKey.trim()) {
			send({ id, error: "invalid_request" });
			return { action: "handled" };
		}
		if (active.has(id) || active.size >= MAX_ACTIVE) {
			send({ id, error: "busy" });
			return { action: "handled" };
		}
		const controller = new AbortController();
		active.set(id, controller);
		void (async () => {
			try {
				if (op === "screen") {
					let usage: unknown;
					const verdict = await screen(registry, {
						provider: request.provider, model: request.model, apiKey: request.apiKey,
						dataFence: "", policy: DEFAULT_POLICY,
					}, String(request.text ?? ""), controller.signal, (u) => { usage = u; });
					send({ id, result: { allowed: verdict.allowed }, usage });
					return;
				}
				const model = op === "complete"
					? registry.find(request.provider, request.model)
					: registry.getModelOfType("classifier", request.provider, request.model);
				if (!model) { send({ id, error: "unavailable" }); return; }
				const options: any = {
					apiKey: request.apiKey,
					signal: controller.signal,
					maxRetries: 0,
					...fetchOverride,
				};
				if (op === "complete" && Number.isInteger(request.maxTokens)) options.maxTokens = request.maxTokens;
				const result = op === "complete"
					? await registry.complete(model, request.context, options)
					: await registry.classify(model, request.context, options);
				if (op === "complete") {
					const text = (result?.content ?? [])
						.filter((part: any) => part?.type === "text")
						.map((part: any) => part.text)
						.join("");
					send({ id, result: { stopReason: result?.stopReason, text }, usage: result?.usage });
				} else {
					send({ id, result: { stopReason: result?.stopReason, answers: result?.answers ?? null }, usage: result?.usage });
				}
			} catch {
				send({ id, error: controller.signal.aborted ? "aborted" : "provider_error" });
			} finally {
				active.delete(id);
			}
		})();
		return { action: "handled" };
	});
	pi.on("session_shutdown", () => {
		for (const controller of active.values()) controller.abort();
	});
}

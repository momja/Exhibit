/** Research-only Pi 1.4.0 RPC shim. Never load in a user/agent session. */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const PREFIX = "exhibit_inference:";
const MAX_ACTIVE = 16;

export default function (pi: ExtensionAPI) {
	const active = new Map<string, AbortController>();
	// Only the test harness sets this. Requests cannot choose an endpoint or inject options.
	const testURL = process.env.EXHIBIT_INFERENCE_TEST_URL;
	pi.on("input", (event, ctx) => {
		const send = (value: unknown) => ctx.ui.notify(PREFIX + JSON.stringify(value), "info");
		let request: any;
		try {
			if (event.text.length > 65_536) throw new Error("too large");
			request = JSON.parse(event.text);
			if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("not an envelope");
		} catch {
			// Fail closed: no unrecognized input ever falls through to an agent run.
			send({ id: "", error: "invalid_request" });
			return { action: "handled" };
		}
		const { id, op } = request;
		if (typeof id !== "string" || !id || id.length > 80) return { action: "handled" };
		if (op === "ping") {
			send({ id, result: { ready: true, active: active.size,
				messages: ctx.sessionManager.getEntries().filter((entry) => entry.type === "message").length } });
			return { action: "handled" };
		}
		if (op === "cancel") {
			active.get(id)?.abort();
			return { action: "handled" };
		}
		// Fault injection is inaccessible in a live worker.
		if (testURL && op === "crash") process.exit(42);
		if (testURL && op === "wedge") { for (;;) {} }
		if (!["complete", "classify"].includes(op) || typeof request.apiKey !== "string" || !request.apiKey.trim()) {
			send({ id, error: "invalid_request" });
			return { action: "handled" };
		}
		if (active.has(id) || active.size >= MAX_ACTIVE) {
			send({ id, error: "busy" });
			return { action: "handled" };
		}
		const controller = new AbortController();
		active.set(id, controller);
		// Return handled immediately. Each job owns its key, signal and context; no global auth mutation.
		void (async () => {
			try {
				const registry = ctx.modelRegistry;
				const model = op === "complete"
					? registry.find("openrouter", request.model)
					: registry.getModelOfType("classifier", "openrouter", request.model);
				if (!model) { send({ id, error: "unavailable" }); return; }
				const options = {
					apiKey: request.apiKey,
					signal: controller.signal,
					maxRetries: 0,
					timeoutMs: 15_000,
					maxTokens: 8,
					...(testURL ? { fetch: (url: any, init: any) => fetch(testURL + new URL(url).pathname, init) } : {}),
				};
				const result = op === "complete"
					? await registry.complete(model as any, request.context, options)
					: await registry.classify(model as any, request.context, options);
				// Error details can contain echoed provider inputs/credentials. Never forward them.
				const { errorMessage, ...safeResult } = result;
				send({ id, result: safeResult });
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

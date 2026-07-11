/**
 * Exhibit tools extension for Pi (Exh-hvaf).
 *
 * Loaded by the exhibit service into every agent session it spawns
 * (`pi --mode rpc --no-builtin-tools -e exhibit.ts`). It gives the model
 * exactly three tools — create_artifact / update_artifact / get_artifact —
 * all of which go through the exhibit HTTP API, so agent output enters the
 * library through the same single write path as every other ingest (scan,
 * footprint, explicit allowlist approval). The extension never touches the
 * datastore and never sees the user's provider key; it authenticates to
 * exhibit with the service token passed in EXHIBIT_TOKEN.
 *
 * When EXHIBIT_MOCK_LLM_URL is set, it additionally registers an
 * OpenAI-compatible "exhibit-mock" provider pointed at that URL — a
 * deterministic stand-in LLM used by end-to-end tests so the whole
 * pipeline (key entry → spawn → tool calls → ingest → SSE) can run
 * without real provider credentials.
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";

const API = process.env.EXHIBIT_API_URL || "http://127.0.0.1:8080";
const TOKEN = process.env.EXHIBIT_TOKEN || "";

async function api(method: string, path: string, body?: unknown): Promise<any> {
	const resp = await fetch(API + path, {
		method,
		headers: {
			"Content-Type": "application/json",
			Authorization: "Bearer " + TOKEN,
		},
		body: body === undefined ? undefined : JSON.stringify(body),
	});
	const text = await resp.text();
	if (!resp.ok) {
		throw new Error(`exhibit API ${method} ${path} -> ${resp.status}: ${text.slice(0, 300)}`);
	}
	return text ? JSON.parse(text) : null;
}

function ok(text: string, details: Record<string, unknown>) {
	return { content: [{ type: "text" as const, text }], details };
}

export default function (pi: ExtensionAPI) {
	if (process.env.EXHIBIT_MOCK_LLM_URL) {
		pi.registerProvider("exhibit-mock", {
			name: "Exhibit Mock",
			baseUrl: process.env.EXHIBIT_MOCK_LLM_URL,
			apiKey: "$EXHIBIT_MOCK_API_KEY",
			api: "openai-completions",
			models: [
				{
					id: "exhibit-mock-1",
					name: "Exhibit Mock 1",
					reasoning: false,
					input: ["text", "image"],
					cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
					contextWindow: 128000,
					maxTokens: 8192,
				},
			],
		});
	}

	pi.registerTool({
		name: "create_artifact",
		label: "Create artifact",
		description:
			"Save a brand-new artifact into the Exhibit library. body must be a complete, " +
			"self-contained HTML document (all CSS/JS inline). Returns the artifact id, its " +
			"render URL, and the scanned network footprint (origins the document references; " +
			"they stay blocked until the user approves them).",
		parameters: Type.Object({
			title: Type.String({ description: "Short human title for the artifact" }),
			body: Type.String({ description: "Complete HTML document source" }),
		}),
		async execute(_id, params) {
			const r = await api("POST", "/api/artifacts", {
				title: params.title,
				body: params.body,
				network_allowlist: [],
			});
			const footprint: string[] = r.network_footprint || [];
			let text = `Created artifact ${r.artifact.id} ("${params.title}").`;
			if (footprint.length > 0) {
				text += ` Network footprint (blocked until the user approves): ${footprint.join(", ")}.`;
			}
			return ok(text, {
				exhibit: "artifact_saved",
				action: "created",
				artifactId: r.artifact.id,
				title: params.title,
				renderUrl: r.render_url,
				footprint,
			});
		},
	});

	pi.registerTool({
		name: "update_artifact",
		label: "Update artifact",
		description:
			"Overwrite an existing Exhibit artifact's source. body must be the complete new " +
			"HTML document, never a fragment or diff. Optionally retitle it.",
		parameters: Type.Object({
			id: Type.String({ description: "Artifact id" }),
			body: Type.String({ description: "Complete replacement HTML document source" }),
			title: Type.Optional(Type.String({ description: "New title (omit to keep)" })),
		}),
		async execute(_id, params) {
			const patch: Record<string, unknown> = { body: params.body };
			if (params.title) patch.title = params.title;
			const a = await api("PATCH", "/api/artifacts/" + encodeURIComponent(params.id), patch);
			return ok(`Updated artifact ${a.id} ("${a.title}").`, {
				exhibit: "artifact_saved",
				action: "updated",
				artifactId: a.id,
				title: a.title,
			});
		},
	});

	pi.registerTool({
		name: "get_artifact",
		label: "Read artifact",
		description:
			"Read an Exhibit artifact's current HTML source and metadata (title, network allowlist).",
		parameters: Type.Object({
			id: Type.String({ description: "Artifact id" }),
		}),
		async execute(_id, params) {
			const a = await api("GET", "/api/artifacts/" + encodeURIComponent(params.id) + "?body=true");
			const meta = `Artifact ${a.id} — title: "${a.title}", allowlist: [${(a.network_allowlist || []).join(", ")}]`;
			return ok(meta + "\n\n" + (a.body || ""), {
				exhibit: "artifact_read",
				artifactId: a.id,
				title: a.title,
			});
		},
	});
}

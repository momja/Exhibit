/**
 * Exhibit tools extension for Pi (Exh-hvaf).
 *
 * Loaded by the exhibit service into every agent session it spawns
 * (`pi --mode rpc --no-builtin-tools -e exhibit.ts`). It gives the model
 * exactly three tools — create_artifact / update_artifact / get_artifact —
 * all of which go through the exhibit HTTP API, so agent output enters the
 * library through the same single write path as every other ingest (scan,
 * footprint, explicit allowlist approval). The extension never touches the
 * datastore and never sees the user's provider key.
 *
 * Scope (av-e0yj). None of the tools takes an artifact id. They act on the
 * one artifact this session is bound to: EXHIBIT_ARTIFACT_ID for a modify
 * session, or whatever the first create_artifact returns. A tool with no id
 * parameter cannot be talked into a different target — which matters because
 * artifact bodies and titles are untrusted text that reaches the model's
 * context. The guarantee does not rest on this file, though: EXHIBIT_TOKEN is
 * a per-session credential the API resolves to (owner, artifact) and refuses
 * outside, so a rewritten extension gets a 403, not another artifact.
 *
 * Untrusted output (get_artifact) comes back inside the same fenced envelope
 * the service uses for prompts, carrying EXHIBIT_DATA_NONCE — the fence id the
 * system prompt tells the model to trust.
 *
 * When EXHIBIT_MOCK_LLM_URL is set, it additionally registers an
 * OpenAI-compatible "exhibit-mock" provider pointed at that URL — a
 * deterministic stand-in LLM used by integration tests so the whole
 * pipeline (key entry → spawn → tool calls → ingest → SSE) can run
 * without real provider credentials.
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";

const API = process.env.EXHIBIT_API_URL || "http://127.0.0.1:8080";
const TOKEN = process.env.EXHIBIT_TOKEN || "";
const NONCE = process.env.EXHIBIT_DATA_NONCE || "";

/**
 * The artifact this session may touch. Seeded from the environment for a
 * modify session and otherwise set from the API's own response to the first
 * create — never from a tool argument, so nothing in the model's context can
 * retarget it.
 */
let boundArtifactId = process.env.EXHIBIT_ARTIFACT_ID || "";

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

/** Wrap untrusted text in the session's data fence — same format the service
 * uses when it inlines the artifact source into a prompt. Occurrences of the
 * fence id inside the content are redacted so nothing in the artifact can
 * close the fence and pose as an instruction. */
function fenced(label: string, content: string): string {
	const safe = NONCE ? content.split(NONCE).join("«redacted fence id»") : content;
	return (
		`-----BEGIN EXHIBIT UNTRUSTED DATA ${NONCE}-----\n` +
		`label: ${label}\n\n` +
		safe +
		`\n-----END EXHIBIT UNTRUSTED DATA ${NONCE}-----`
	);
}

function requireBoundArtifact(): string {
	if (!boundArtifactId) {
		throw new Error("this session has no artifact yet — call create_artifact first");
	}
	return boundArtifactId;
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
			"Save a brand-new artifact into the Exhibit library and bind this session to it. " +
			"body must be a complete, self-contained HTML document (all CSS/JS inline). " +
			"Available only until this session has an artifact; afterwards use update_artifact. " +
			"Returns the artifact id, its render URL, and the scanned network footprint (origins " +
			"the document references; they stay blocked until the user approves them).",
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
			// The API bound the session's credential to this id as it wrote the
			// row; mirror that here so the later tools have a target.
			boundArtifactId = r.artifact.id;
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
			"Overwrite this session's artifact source. body must be the complete new HTML " +
			"document, never a fragment or diff. Optionally retitle it.",
		parameters: Type.Object({
			body: Type.String({ description: "Complete replacement HTML document source" }),
			title: Type.Optional(Type.String({ description: "New title (omit to keep)" })),
		}),
		async execute(_id, params) {
			const target = requireBoundArtifact();
			const patch: Record<string, unknown> = { body: params.body };
			if (params.title) patch.title = params.title;
			// PATCH answers {artifact, network_footprint, footprint_changed} —
			// the id and title live under .artifact (av-hrtv note).
			const r = await api("PATCH", "/api/artifacts/" + encodeURIComponent(target), patch);
			const a = r.artifact || {};
			const footprint: string[] = r.network_footprint || [];
			return ok(`Updated artifact ${a.id || target} ("${a.title || ""}").`, {
				exhibit: "artifact_saved",
				action: "updated",
				artifactId: a.id || target,
				title: a.title,
				footprint,
			});
		},
	});

	pi.registerTool({
		name: "get_artifact",
		label: "Read artifact",
		description:
			"Re-read this session's artifact: its current HTML source and metadata (title, " +
			"network allowlist). The source is already in context at the start of the session, " +
			"so use this only to pick up changes made since — your own save, or an edit made " +
			"elsewhere.",
		parameters: Type.Object({}),
		async execute() {
			const target = requireBoundArtifact();
			const a = await api("GET", "/api/artifacts/" + encodeURIComponent(target) + "?body=true");
			const meta =
				`id: ${a.id}\ntitle: ${a.title}\n` +
				`allowlist: [${(a.network_allowlist || []).join(", ")}]`;
			return ok(fenced("current source of the artifact this session is editing", meta + "\n\n" + (a.body || "")), {
				exhibit: "artifact_read",
				artifactId: a.id,
			});
		},
	});
}

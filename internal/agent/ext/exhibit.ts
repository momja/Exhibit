/**
 * Exhibit tools extension for Pi (Exh-hvaf, av-lvi1).
 *
 * Loaded by the exhibit service into every agent session it spawns
 * (`pi --mode rpc --no-builtin-tools -e exhibit.ts`). It gives the model eight
 * tools — create_artifact / update_artifact / get_artifact for the document,
 * get_state / set_state / delete_state for the artifact's stored state, and
 * set_widget / get_widget for the artifact's gallery-card widget (av-fafu) —
 * all of which go through the exhibit HTTP API, so agent output enters the
 * library through the same single write path as every other ingest (scan,
 * footprint, explicit allowlist approval) and every other state edit (the
 * edit page's state inspector, av-hg5f). The extension never touches the
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

// Renders state as delimited raw-value blocks rather than a JSON object.
// JSON.stringify()-ing the values would backslash-escape every quote inside
// a JSON-shaped value, and a model asked to reproduce that escaped text back
// through set_state is exactly the failure mode AC 5 exists to prevent —
// this format shows each value's exact bytes, unescaped, so "copy this back
// unchanged except for the one field I'm asked to touch" has a literal
// source to copy from.
function formatState(state: Record<string, string>): string {
	const keys = Object.keys(state);
	if (keys.length === 0) return "(no state stored for this artifact)";
	return keys
		.map((k) => `--- key: ${JSON.stringify(k)} ---\n${state[k]}`)
		.join("\n\n");
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
			// PATCH /api/artifacts/:id returns {artifact, network_footprint,
			// footprint_changed} — the identity lives under `artifact`, never at
			// the top level. Reading it from the top level yielded undefined, which
			// silently suppressed the saved event and the preview refresh (av-l31x).
			// The id itself comes from the session's bound artifact, never a tool
			// parameter (av-hrtv note) — these tools take no artifact id, which is
			// what stops a model from being talked into naming one (av-e0yj).
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

	pi.registerTool({
		name: "get_state",
		label: "Read artifact state",
		description:
			"Read every state key/value the artifact has stored (what its localStorage writes " +
			"through to, per artifact_state — sessionStorage is frame-local and never appears " +
			"here). Values are opaque strings; artifacts usually store JSON in them. Each key's " +
			"value is shown verbatim, byte for byte — copy it back exactly via set_state except " +
			"for whatever the user asked you to change. Do not reformat, reorder object keys, " +
			"change number/whitespace formatting, or otherwise 'clean up' a value you were not " +
			"asked to touch.",
		parameters: Type.Object({}),
		async execute() {
			const target = requireBoundArtifact();
			const state = await api("GET", "/api/artifacts/" + encodeURIComponent(target) + "/state");
			return ok(formatState(state), {
				exhibit: "state_read",
				artifactId: target,
				keys: Object.keys(state),
			});
		},
	});

	pi.registerTool({
		name: "set_state",
		label: "Set state key",
		description:
			"Write one state key on an artifact, creating it if absent. This is the same write " +
			"path the edit page's state inspector uses. Only the given key changes — every other " +
			"key is left exactly as stored, since this call never touches them. If the value is " +
			"JSON and you are only changing one field inside it, reproduce every other byte " +
			"(key order, spacing, number formatting) exactly as read from get_state — do not " +
			"reformat the whole document.",
		parameters: Type.Object({
			key: Type.String({ description: "State key to write" }),
			value: Type.String({ description: "New value, verbatim (JSON-encode it yourself if it's structured data)" }),
		}),
		async execute(_id, params) {
			const target = requireBoundArtifact();
			await api("PUT", "/api/artifacts/" + encodeURIComponent(target) + "/state", {
				key: params.key,
				value: params.value,
			});
			return ok(`Set state key ${JSON.stringify(params.key)}.`, {
				exhibit: "state_changed",
				artifactId: target,
				action: "set",
				key: params.key,
			});
		},
	});

	pi.registerTool({
		name: "delete_state",
		label: "Delete artifact state",
		description:
			"Delete artifact state. Pass key to remove just that one key (idempotent — no error " +
			"if it was already absent). Omit key to erase ALL state for the artifact — this is " +
			"destructive and irreversible (no version history), so only do it when the user " +
			"clearly asked to reset/clear everything, not when they asked to fix or remove one " +
			"field.",
		parameters: Type.Object({
			key: Type.Optional(Type.String({ description: "State key to delete; omit to erase all state" })),
		}),
		async execute(_id, params) {
			const target = requireBoundArtifact();
			// Presence, not truthiness: "" is a legitimate Web Storage key, and
			// a falsy test would route a request to delete it into erase-all.
			if (params.key !== undefined) {
				await api(
					"DELETE",
					// The key is a query value, never a path segment — as a
					// segment, a key of ".." resolves away and hits the artifact
					// delete route (av-hh1o).
					"/api/artifacts/" + encodeURIComponent(target) +
						"/state?key=" + encodeURIComponent(params.key),
				);
				return ok(`Deleted state key ${JSON.stringify(params.key)}.`, {
					exhibit: "state_changed",
					artifactId: target,
					action: "deleted_key",
					key: params.key,
				});
			}
			await api("DELETE", "/api/artifacts/" + encodeURIComponent(target) + "/state");
			return ok("Erased all state on this session's artifact.", {
				exhibit: "state_changed",
				artifactId: target,
				action: "cleared_all",
			});
		},
	});

	pi.registerTool({
		name: "set_widget",
		label: "Set widget",
		description:
			"Save (or replace) this session's artifact gallery widget — the small, glanceable tile its card " +
			"shows in the library, like an iOS home-screen widget. body must be a complete, " +
			"self-contained HTML document. The widget reads the SAME localStorage keys the artifact " +
			"writes (state is inlined before its scripts run, so synchronous getItem at startup is " +
			"correct), but it CANNOT write state, download files, or use the clipboard, and it is " +
			"rendered non-interactive — a click on it opens the artifact. Design for a ~272x132 px " +
			"tile that is fluid to 230-420 wide, show one or two facts, and always render a calm " +
			"empty state. A stateless tool's widget is simply a static card with no script.",
		parameters: Type.Object({
			body: Type.String({ description: "Complete HTML document for the widget" }),
		}),
		async execute(_id, params) {
			const target = requireBoundArtifact();
			const r = await api("PUT", "/api/artifacts/" + encodeURIComponent(target) + "/widget", {
				body: params.body,
			});
			const unapproved: string[] = r.unapproved_origins || [];
			let text = `Saved the gallery widget for artifact ${target}.`;
			if (unapproved.length > 0) {
				// The widget shares the artifact's allowlist, so an origin the
				// artifact never got approved for is simply blocked at render.
				// Say so rather than letting it show up as a blank tile.
				text +=
					` Warning: it references ${unapproved.join(", ")}, which the artifact's network ` +
					`allowlist does not cover — the browser will block those. Prefer a widget with no ` +
					`external references at all.`;
			}
			return ok(text, {
				exhibit: "widget_saved",
				artifactId: target,
				widgetUrl: r.widget_url,
				unapproved,
			});
		},
	});

	pi.registerTool({
		name: "get_widget",
		label: "Read widget",
		description:
			"Read this session's artifact gallery widget source. Reports that there is no widget " +
			"yet when the artifact has none.",
		parameters: Type.Object({}),
		async execute() {
			const target = requireBoundArtifact();
			const path = "/api/artifacts/" + encodeURIComponent(target) + "/widget";
			try {
				const r = await api("GET", path);
				// The widget is artifact content like the source is, so it comes
				// back fenced rather than spliced into the model's instructions.
				return ok(fenced("current gallery widget of the artifact this session is editing", r.body || ""), {
					exhibit: "widget_read",
					artifactId: target,
				});
			} catch (err) {
				// A widget-less artifact 404s. That is an ordinary answer to this
				// question, not a tool failure, so return it as text — an error
				// would make the model retry or apologize for nothing.
				const msg = err instanceof Error ? err.message : String(err);
				if (msg.includes("-> 404")) {
					return ok("This artifact has no widget yet.", {
						exhibit: "widget_read",
						artifactId: target,
						missing: true,
					});
				}
				throw err;
			}
		},
	});
}

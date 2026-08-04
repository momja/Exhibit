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
		parameters: Type.Object({
			id: Type.String({ description: "Artifact id" }),
		}),
		async execute(_id, params) {
			const state = await api("GET", "/api/artifacts/" + encodeURIComponent(params.id) + "/state");
			return ok(formatState(state), {
				exhibit: "state_read",
				artifactId: params.id,
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
			id: Type.String({ description: "Artifact id" }),
			key: Type.String({ description: "State key to write" }),
			value: Type.String({ description: "New value, verbatim (JSON-encode it yourself if it's structured data)" }),
		}),
		async execute(_id, params) {
			await api("PUT", "/api/artifacts/" + encodeURIComponent(params.id) + "/state", {
				key: params.key,
				value: params.value,
			});
			return ok(`Set state key ${JSON.stringify(params.key)} on artifact ${params.id}.`, {
				exhibit: "state_changed",
				artifactId: params.id,
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
			id: Type.String({ description: "Artifact id" }),
			key: Type.Optional(Type.String({ description: "State key to delete; omit to erase all state" })),
		}),
		async execute(_id, params) {
			// Presence, not truthiness: "" is a legitimate Web Storage key, and
			// a falsy test would route a request to delete it into erase-all.
			if (params.key !== undefined) {
				await api(
					"DELETE",
					// The key is a query value, never a path segment — as a
					// segment, a key of ".." resolves away and hits the artifact
					// delete route (av-hh1o).
					"/api/artifacts/" + encodeURIComponent(params.id) +
						"/state?key=" + encodeURIComponent(params.key),
				);
				return ok(`Deleted state key ${JSON.stringify(params.key)} on artifact ${params.id}.`, {
					exhibit: "state_changed",
					artifactId: params.id,
					action: "deleted_key",
					key: params.key,
				});
			}
			await api("DELETE", "/api/artifacts/" + encodeURIComponent(params.id) + "/state");
			return ok(`Erased all state on artifact ${params.id}.`, {
				exhibit: "state_changed",
				artifactId: params.id,
				action: "cleared_all",
			});
		},
	});

	pi.registerTool({
		name: "set_widget",
		label: "Set widget",
		description:
			"Save (or replace) an artifact's gallery widget — the small, glanceable tile its card " +
			"shows in the library, like an iOS home-screen widget. body must be a complete, " +
			"self-contained HTML document. The widget reads the SAME localStorage keys the artifact " +
			"writes (state is inlined before its scripts run, so synchronous getItem at startup is " +
			"correct), but it CANNOT write state, download files, or use the clipboard, and it is " +
			"rendered non-interactive — a click on it opens the artifact. Design for a ~272x132 px " +
			"tile that is fluid to 230-420 wide, show one or two facts, and always render a calm " +
			"empty state. A stateless tool's widget is simply a static card with no script.",
		parameters: Type.Object({
			id: Type.String({ description: "Artifact id" }),
			body: Type.String({ description: "Complete HTML document for the widget" }),
		}),
		async execute(_id, params) {
			const r = await api("PUT", "/api/artifacts/" + encodeURIComponent(params.id) + "/widget", {
				body: params.body,
			});
			const unapproved: string[] = r.unapproved_origins || [];
			let text = `Saved the gallery widget for artifact ${params.id}.`;
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
				artifactId: params.id,
				widgetUrl: r.widget_url,
				unapproved,
			});
		},
	});

	pi.registerTool({
		name: "get_widget",
		label: "Read widget",
		description:
			"Read an artifact's current gallery widget source. Reports that there is no widget yet " +
			"when the artifact has none.",
		parameters: Type.Object({
			id: Type.String({ description: "Artifact id" }),
		}),
		async execute(_id, params) {
			const path = "/api/artifacts/" + encodeURIComponent(params.id) + "/widget";
			try {
				const r = await api("GET", path);
				return ok(r.body || "", { exhibit: "widget_read", artifactId: params.id });
			} catch (err) {
				// A widget-less artifact 404s. That is an ordinary answer to this
				// question, not a tool failure, so return it as text — an error
				// would make the model retry or apologize for nothing.
				const msg = err instanceof Error ? err.message : String(err);
				if (msg.includes("-> 404")) {
					return ok(`Artifact ${params.id} has no widget yet.`, {
						exhibit: "widget_read",
						artifactId: params.id,
						missing: true,
					});
				}
				throw err;
			}
		},
	});
}

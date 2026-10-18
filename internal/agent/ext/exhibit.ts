/**
 * Exhibit tools extension for Pi (Exh-hvaf, av-lvi1).
 *
 * Loaded by the exhibit service into every agent session it spawns
 * (`pi --mode rpc --no-builtin-tools -e exhibit.ts`). It gives the model eleven
 * tools — create_artifact / write_artifact / edit_artifact / get_artifact for the document,
 * get_state / set_state / delete_state for the artifact's stored state,
 * set_widget / edit_widget / get_widget for the artifact's gallery-card widget (av-fafu), and
 * get_selection for the elements the user picked in the preview — all of which
 * (get_selection aside, which reads a file the service wrote) go through the exhibit HTTP API, so agent output enters the
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
 * Untrusted text (an artifact's source, title, state and widget, and the
 * user's selected elements) reaches the model only as the result of one of these
 * tools. A tool result is its own message role, so the boundary between data
 * and instruction is the conversation's structure, not a delimiter inside the
 * string (av-5s7g) — which is why nothing here wraps, escapes or fences a
 * result. The system prompt says that tool results are data; `labeled` only
 * says which tool's data it is.
 *
 * When EXHIBIT_MOCK_LLM_URL is set, it additionally registers an
 * OpenAI-compatible "exhibit-mock" provider pointed at that URL — a
 * deterministic stand-in LLM used by integration tests so the whole
 * pipeline (key entry → spawn → tool calls → ingest → SSE) can run
 * without real provider credentials.
 */
import { readFile } from "node:fs/promises";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";
import { applyEdits } from "./edit.ts";

const API = process.env.EXHIBIT_API_URL || "http://127.0.0.1:8080";
const TOKEN = process.env.EXHIBIT_TOKEN || "";
// Where the service puts the elements the user selected with the current
// prompt (agent.Session.writeSelection). It is a file rather than a request
// because the service's only channel to this process is the prompt, and an
// element's markup is untrusted — it must reach the model as a tool result.
const SELECTION_FILE = process.env.EXHIBIT_SELECTION_FILE || "";

/**
 * The artifact this session may touch. Seeded from the environment for a
 * modify session and otherwise set from the API's own response to the first
 * create — never from a tool argument, so nothing in the model's context can
 * retarget it.
 */
let boundArtifactId = process.env.EXHIBIT_ARTIFACT_ID || "";

/**
 * The largest request body the exhibit API accepts (av-ombn), passed down by
 * the service. Zero when it was not: the API's 413 is then the only signal.
 */
const MAX_BODY_BYTES = Number(process.env.EXHIBIT_MAX_BODY_BYTES) || 0;

/** A byte count the way the API's own 413 states one (humanize.Bytes). */
function humanBytes(n: number): string {
	if (n < 1024) return `${n} B`;
	let div = 1024;
	let exp = 0;
	for (let m = n / 1024; m >= 1024; m /= 1024) {
		div *= 1024;
		exp++;
	}
	return `${(n / div).toFixed(1)} ${"KMGTPE"[exp]}iB`;
}

/**
 * A write refused for its size, whether this file refused it before sending
 * or the API answered 413. Worded for the model: the stored copy is exactly
 * as it was, and the same call will fail the same way, so the next step is to
 * shrink the content rather than retry it.
 */
function tooLargeError(method: string, path: string, size: number, limit: number): Error {
	let sizeText = humanBytes(size);
	let limitText = humanBytes(limit);
	if (sizeText === limitText) {
		// A write a few bytes over would otherwise read "32.0 MiB, at most
		// 32.0 MiB", which looks like it should have fit.
		sizeText = `${size} bytes`;
		limitText = `${limit} bytes`;
	}
	const over = limit > 0
		? `this write is ${sizeText} and the instance accepts at most ${limitText} per request`
		: `this write is ${sizeText}, more than the instance accepts`;
	return new Error(
		`exhibit API ${method} ${path} refused: ${over}. Nothing was saved; what is stored is unchanged, ` +
			"and sending the same content again will fail the same way. Make it smaller first. Large " +
			"inline data (base64-encoded images, fonts or wasm) is the usual cause.",
	);
}

/** The limit a 413 names, or 0 when it came from something other than the
 * exhibit API (a proxy in front of it, say) and names none. */
function limitFrom413(text: string): number {
	try {
		const n = Number(JSON.parse(text)?.limit_bytes);
		return n > 0 ? n : 0;
	} catch {
		return 0;
	}
}

async function api(method: string, path: string, body?: unknown): Promise<any> {
	const payload = body === undefined ? undefined : JSON.stringify(body);
	// Bytes on the wire, which is what the limit counts. The string's length
	// is in UTF-16 units and would undercount anything outside ASCII.
	const size = payload === undefined ? 0 : Buffer.byteLength(payload, "utf8");
	if (MAX_BODY_BYTES > 0 && size > MAX_BODY_BYTES) {
		throw tooLargeError(method, path, size, MAX_BODY_BYTES);
	}
	let resp: Response;
	try {
		resp = await fetch(API + path, {
			method,
			headers: {
				"Content-Type": "application/json",
				Authorization: "Bearer " + TOKEN,
			},
			body: payload,
		});
	} catch (err) {
		// No response at all. Besides the usual causes, a proxy that refuses a
		// large body can close the connection mid-upload instead of answering
		// 413, which lands here as EPIPE or ECONNRESET. Hence the size.
		const cause = (err as any)?.cause?.code || (err as any)?.cause?.message || (err instanceof Error ? err.message : String(err));
		const sending = size > 0 ? ` while sending ${humanBytes(size)}` : "";
		throw new Error(
			`exhibit API ${method} ${path} got no response (${cause})${sending}. ` +
				"Nothing confirms whether it was saved; read it back before assuming either way.",
		);
	}
	const text = await resp.text();
	if (resp.status === 413) {
		throw tooLargeError(method, path, size, limitFrom413(text));
	}
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

/** Say which tool's data a result is, in Exhibit's own words. The label is a
 * constant at every call site; the content after it is the untrusted part. */
function labeled(label: string, content: string): string {
	return `${label}\n\n${content}`;
}

// A title is one line of display text. It is the most attacker-controllable
// field on a URL-ingested artifact, so a read returns it as a single bounded
// line: no newline to start a paragraph that looks like a message, and no
// length to bury the real content under.
const MAX_TITLE_CHARS = 200;

function oneLine(title: string): string {
	const flat = String(title ?? "").replace(/\s+/g, " ").trim();
	return flat.length > MAX_TITLE_CHARS ? flat.slice(0, MAX_TITLE_CHARS) + "…" : flat;
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
			"Available only until this session has an artifact; afterwards use edit_artifact (small changes) or write_artifact (full rewrite). " +
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

	/**
	 * Persist a full replacement body through the single write path. Shared
	 * by write_artifact (the model supplies the body) and edit_artifact
	 * (the body is the engine's splice of targeted edits into the current
	 * source) — one function so both report the same footprint semantics and
	 * the same artifact_saved event the chat UI re-renders on.
	 */
	async function saveBody(target: string, body: string, title?: string) {
		const patch: Record<string, unknown> = { body };
		if (title) patch.title = title;
		// PATCH /api/artifacts/:id returns {artifact, network_footprint,
		// footprint_changed} — the identity lives under `artifact`, never at
		// the top level. Reading it from the top level yielded undefined, which
		// silently suppressed the saved event and the preview refresh (av-l31x).
		// The id itself comes from the session's bound artifact, never a tool
		// parameter (av-hrtv note) — these tools take no artifact id, which is
		// what stops a model from being talked into naming one (av-e0yj).
		const r = await api("PATCH", "/api/artifacts/" + encodeURIComponent(target), patch);
		const a = r.artifact || {};
		// Report only the origins the rewritten body leaves *blocked*: an
		// already-approved origin is not blocked, so re-flagging it would be a
		// false alarm the create path never has to filter — a brand-new
		// artifact starts with nothing approved (av-hrtv).
		const approved: string[] = a.network_allowlist || [];
		const footprint: string[] = (r.network_footprint || []).filter((o: string) => !approved.includes(o));
		// No title: a save's result is not the place to read the stored title back
		// — it is untrusted on a URL-ingested artifact, and the model has no use
		// for it here (it knows what it just wrote).
		let text = `Updated artifact ${a.id || target}.`;
		if (footprint.length > 0) {
			text += ` Network footprint (blocked until the user approves): ${footprint.join(", ")}.`;
		}
		return ok(text, {
			exhibit: "artifact_saved",
			action: "updated",
			artifactId: a.id || target,
			title: a.title,
			footprint,
			// Whether the new body's origins differ from the previous body's —
			// a different question from `footprint` above, which is about
			// approval rather than change.
			footprintChanged: r.footprint_changed === true,
		});
	}

	pi.registerTool({
		name: "write_artifact",
		label: "Write artifact",
		description:
			"Replace this session's artifact source wholesale. body must be the complete new HTML " +
			"document, never a fragment or diff. Prefer edit_artifact for targeted changes — " +
			"use this only for a full rewrite. Optionally retitle it.",
		parameters: Type.Object({
			body: Type.String({ description: "Complete replacement HTML document source" }),
			title: Type.Optional(Type.String({ description: "New title (omit to keep)" })),
		}),
		async execute(_id, params) {
			const target = requireBoundArtifact();
			return saveBody(target, params.body, params.title);
		},
	});

	pi.registerTool({
		name: "edit_artifact",
		label: "Edit artifact",
		description:
			"Make targeted in-place edits to this session's artifact source, without rewriting " +
			"the whole document. Prefer this over write_artifact for small changes (a fix, a " +
			"restyle, one section). Each edit's oldText must match its target exactly and uniquely " +
			"in the current source — copy it from get_artifact, with enough surrounding context to " +
			"be unique; minor quote/dash/whitespace drift is tolerated. Put several disjoint changes " +
			"in one call's edits[]; edits that overlap or touch must be merged into one instead. " +
			"If an edit matches nothing, re-read with get_artifact and retry against the current " +
			"source — never fall back to rewriting the document by hand from memory.",
		parameters: Type.Object({
			edits: Type.Array(
				Type.Object({
					oldText: Type.String({ description: "Exact text to find; must be unique in the current source" }),
					newText: Type.String({ description: "Replacement text (empty string deletes)" }),
				}),
				{ description: "One or more targeted replacements, applied atomically" },
			),
		}),
		async execute(_id, params) {
			const target = requireBoundArtifact();
			// Read-then-write inside the tool, so the match always runs against
			// the latest saved source rather than the possibly stale copy the
			// model holds. Validation runs before any PATCH: a bad edit throws
			// (a failed tool result naming the edit) and the stored body is
			// left exactly as it was.
			const a = await api("GET", "/api/artifacts/" + encodeURIComponent(target) + "?body=true");
			let next: string;
			try {
				next = applyEdits(a.body || "", params.edits ?? []).body;
			} catch (err) {
				throw new Error(err instanceof Error ? err.message : String(err));
			}
			return saveBody(target, next);
		},
	});

	/**
	 * Describe the artifact's out-of-line assets (av-20fk) — never their bytes.
	 *
	 * These payloads (wasm modules, Emscripten heaps) used to sit in the body as
	 * base64, which is what made a vendored artifact unreadable: a single 16 MiB
	 * module arrived as ~21 MB of context on every read and had to be sent back
	 * whole to change one line. They now live outside the body entirely.
	 *
	 * The model still needs to know they exist. Without this it reads a bare
	 * `fetch('/app.wasm')` in a document with no such file and reasonably
	 * concludes the code is broken — the redirect to the stored payload is
	 * injected at render, so nothing in the source it is holding explains it.
	 * Listing them is enough; the bytes are opaque and there is nothing an LLM
	 * can do with them.
	 */
	async function assetSummary(artifactId: string): Promise<string> {
		try {
			const r = await api("GET", "/api/artifacts/" + encodeURIComponent(artifactId) + "/assets");
			const assets = r.assets || [];
			if (assets.length === 0) return "assets: none";
			const lines = assets.map(
				(a: any) => `  - ${a.source_url} (${a.content_type}, ${a.size_bytes} bytes)`,
			);
			return (
				"assets: these URLs are served from stored copies at render time, so the fetches\n" +
				"below work even though the files are not in the source. Leave the fetch calls as\n" +
				"they are; the redirect is injected outside this document.\n" +
				lines.join("\n")
			);
		} catch {
			// Never fail a read over metadata: the source is what was asked for.
			return "assets: unavailable";
		}
	}

	pi.registerTool({
		name: "get_artifact",
		label: "Read artifact",
		description:
			"Read this session's artifact: its current HTML source and metadata (title, " +
			"network allowlist). The source is NOT in your context until you call this, so read " +
			"it before you change it — and read it again after anything may have changed it: " +
			"your own save, or an edit made elsewhere (by the user, or earlier in this " +
			"conversation's history).",
		parameters: Type.Object({}),
		async execute() {
			const target = requireBoundArtifact();
			const a = await api("GET", "/api/artifacts/" + encodeURIComponent(target) + "?body=true");
			let meta =
				`id: ${a.id}\ntitle: ${oneLine(a.title)}\n` +
				`allowlist: [${(a.network_allowlist || []).join(", ")}]`;
			meta += "\n" + (await assetSummary(target));
			return ok(labeled("current source of the artifact this session is editing:", meta + "\n\n" + (a.body || "")), {
				exhibit: "artifact_read",
				artifactId: target,
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

	/**
	 * Persist a full widget document through the single write path. Shared
	 * by set_widget (the model supplies the body) and edit_widget (the body
	 * is the engine's splice of targeted edits into the current tile) — one
	 * function so both report the same unapproved-origins warning and the
	 * same widget_saved event the gallery card re-renders on.
	 */
	async function saveWidget(target: string, body: string) {
		const r = await api("PUT", "/api/artifacts/" + encodeURIComponent(target) + "/widget", {
			body,
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
	}

	pi.registerTool({
		name: "set_widget",
		label: "Set widget",
		description:
			"Save (or replace) this session's artifact gallery widget — the small, glanceable tile its card " +
			"shows in the library, like an iOS home-screen widget. body must be a complete, " +
			"self-contained HTML document. Prefer edit_widget for targeted changes to an existing " +
			"tile — use this only for a new or full-rewrite widget. The widget reads the SAME localStorage keys the artifact " +
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
			return saveWidget(target, params.body);
		},
	});

	pi.registerTool({
		name: "edit_widget",
		label: "Edit widget",
		description:
			"Make targeted in-place edits to this session's artifact gallery widget, without rewriting " +
			"the whole tile. Prefer this over set_widget for small changes. Each edit's oldText must " +
			"match its target exactly and uniquely in the current widget source — copy it from " +
			"get_widget, with enough surrounding context to be unique; minor quote/dash/whitespace " +
			"drift is tolerated. Put several disjoint changes in one call's edits[]; edits that " +
			"overlap or touch must be merged into one instead. If the artifact has no widget yet, " +
			"create it with set_widget first; if an edit matches nothing, re-read with get_widget " +
			"and retry against the current source.",
		parameters: Type.Object({
			edits: Type.Array(
				Type.Object({
					oldText: Type.String({ description: "Exact text to find; must be unique in the current widget source" }),
					newText: Type.String({ description: "Replacement text (empty string deletes)" }),
				}),
				{ description: "One or more targeted replacements, applied atomically" },
			),
		}),
		async execute(_id, params) {
			const target = requireBoundArtifact();
		// Read-then-write inside the tool, so the match always runs against
		// the latest saved tile rather than the possibly stale copy the
		// model holds. A missing widget is guidance, not a failure of the
		// match: there is nothing to edit, so say to create it instead.
			// Validation runs before any PUT: a bad edit throws (a failed
			// tool result naming the edit) and the stored tile is left
			// exactly as it was.
			let current: string;
			try {
				const w = await api("GET", "/api/artifacts/" + encodeURIComponent(target) + "/widget");
				current = w.body || "";
			} catch (err) {
				const msg = err instanceof Error ? err.message : String(err);
				if (msg.includes("-> 404")) {
					throw new Error("This artifact has no widget yet — create it with set_widget first.");
				}
				throw err;
			}
			let next: string;
			try {
				next = applyEdits(current, params.edits ?? []).body;
			} catch (err) {
				throw new Error(err instanceof Error ? err.message : String(err));
			}
			return saveWidget(target, next);
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
				return ok(labeled("current gallery widget of the artifact this session is editing:", r.body || ""), {
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

	pi.registerTool({
		name: "get_selection",
		label: "Read selection",
		description:
			"Read the element(s) the user selected in the artifact preview with their latest " +
			"message: each is the exact element they mean, given as a selector, its text and its " +
			"outerHTML. Call this when the message says elements were selected. Reports that " +
			"nothing is selected otherwise.",
		parameters: Type.Object({}),
		async execute() {
			let elements: string[] = [];
			if (SELECTION_FILE) {
				try {
					elements = JSON.parse(await readFile(SELECTION_FILE, "utf8"));
				} catch (err) {
					// No file is the ordinary answer: the service removes it for a
					// prompt that selected nothing.
					if ((err as NodeJS.ErrnoException).code !== "ENOENT") throw err;
				}
			}
			if (elements.length === 0) {
				return ok("Nothing is selected.", { exhibit: "selection_read", count: 0 });
			}
			const blocks = elements.map((e, i) =>
				elements.length > 1 ? `--- element ${i + 1} of ${elements.length} ---\n${e}` : e,
			);
			return ok(labeled("element(s) the user selected in the artifact preview:", blocks.join("\n\n")), {
				exhibit: "selection_read",
				count: elements.length,
			});
		},
	});
}

import { createHash, randomUUID } from "node:crypto";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

// Custom DATA entries are persisted by Pi and excluded from model context.
// Only projectSourceRead creates messages: source stays exclusively tool data.
export const SOURCE_READ_ENTRY = "exhibit_source_read";
export interface ArtifactSourceSnapshot {
	artifactId: string;
	sourceRevision?: string;
	artifact: any;
	assetSummary?: string;
	result: { content: any[]; details: Record<string, unknown> };
}
interface SourceRead {
	anchor: string;
	userTimestamp: number;
	userContentHash: string;
	callId: string;
	snapshot: ArtifactSourceSnapshot;
}

function contentHash(content: unknown): string {
	return createHash("sha256").update(JSON.stringify(content)).digest("hex");
}

function projectSourceRead(read: SourceRead, model: any) {
	const timestamp = read.userTimestamp;
	return [
		{
			role: "assistant", content: [{ type: "toolCall", id: read.callId, name: "get_artifact", arguments: {} }],
			api: model.api, provider: model.provider, model: model.id,
			usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0,
				cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
			stopReason: "toolUse", timestamp,
		},
		{ role: "toolResult", toolCallId: read.callId, toolName: "get_artifact",
			...read.snapshot.result, isError: false, timestamp },
	];
}

/** Warm with an authorized read, validate at the first accepted provider turn,
 * and replay structured source reads beside their actual user entry. A read
 * failure leaves the ordinary get_artifact tool available. No inference, input
 * hook, customMessage, or modification of Pi's active JSONL is involved.
 * adopt runs only for the current validated read, never for replayed history.
 */
export function installSourceContext(pi: ExtensionAPI, options: {
	artifactId: () => string;
	read: () => Promise<ArtifactSourceSnapshot>;
	revision: () => Promise<string | undefined>;
	adopt?: (snapshot: ArtifactSourceSnapshot) => void;
	// Edit tools may advance this after their own successful writes.
	current?: () => ArtifactSourceSnapshot | null;
}) {
	// New-build sessions keep their ordinary create/read workflow.
	if (!options.artifactId()) return;
	let warming: Promise<ArtifactSourceSnapshot | null> | null = null;
	let validatedAnchor: string | null = null;
	let shownSnapshot: ArtifactSourceSnapshot | null = null;
	let shownAnchor: string | null = null;
	let forceFresh = true;
	const reads = new Map<string, SourceRead>();
	function warm() {
		warming = options.artifactId() ? options.read().catch(() => null) : Promise.resolve(null);
	}
	function restore(ctx: any) {
		reads.clear();
		for (const entry of ctx.sessionManager.getBranch()) {
			if (entry.type !== "custom" || entry.customType !== SOURCE_READ_ENTRY) continue;
			const data = entry.data;
			if (data?.snapshot?.artifactId === options.artifactId() && typeof data.anchor === "string"
				&& Array.isArray(data.snapshot.result?.content)) reads.set(data.anchor, data);
		}
		validatedAnchor = null;
		shownSnapshot = null;
		shownAnchor = null;
		forceFresh = true;
		warm();
	}
	pi.on("session_start", (_event, ctx) => { restore(ctx); });
	pi.on("session_switch", (_event, ctx) => { restore(ctx); });
	pi.on("session_tree", (_event, ctx) => { restore(ctx); });
	pi.on("session_compact", () => { validatedAnchor = null; });
	pi.on("context", async (event, ctx) => {
		if (!options.artifactId() || !ctx.model) return;
		const branch = ctx.sessionManager.getBranch();
		const user = [...branch].reverse().find((entry: any) => entry.type === "message" && entry.message.role === "user") as any;
		if (!user) return;
		const matches = (message: any, read: SourceRead) => message.role === "user"
			&& message.timestamp === read.userTimestamp && contentHash(message.content) === read.userContentHash;
		// An accepted user message is already recorded by Pi at this point;
		// input guardrails have completed before this hook can be reached.
		if (validatedAnchor !== user.id) {
			validatedAnchor = user.id;
			try {
				const warmed = await (warming ?? Promise.resolve(null));
				warming = null;
				// current() contains only source actually returned to the model,
				// advanced by successful own edits. A prefetch is NOT a known read.
				const known = options.current ? options.current() : shownSnapshot;
				const base = shownAnchor ? reads.get(shownAnchor) : null;
				const retainedBase = (base && event.messages.some(message => matches(message, base)))
					|| event.messages.some((message: any) => message.role === "toolResult"
						&& message.toolName === "get_artifact" && !message.isError);
				const revision = await options.revision();
				if (!forceFresh && known?.artifactId === options.artifactId() && revision && known.sourceRevision === revision && retainedBase) {
					// The retained base plus the model's own successful writes
					// already explains this revision. Do not copy it into context.
					shownSnapshot = known;
				} else {
					let snapshot = known ?? warmed;
					// A server without OCC tokens still works safely, by reading now.
					if (!snapshot || !revision || snapshot.sourceRevision !== revision) snapshot = await options.read();
					if (snapshot.artifactId !== options.artifactId()) throw new Error("artifact scope changed");
					// Keep historical evidence independent of a mutable edit cache.
					const recordedSnapshot = structuredClone(snapshot);
					options.adopt?.(snapshot);
					const read: SourceRead = { anchor: user.id, userTimestamp: user.message.timestamp,
						userContentHash: contentHash(user.message.content), callId: "exhibit_source_" + randomUUID(), snapshot: recordedSnapshot };
					reads.set(user.id, read);
					shownSnapshot = snapshot;
					shownAnchor = user.id;
					forceFresh = false;
					pi.appendEntry(SOURCE_READ_ENTRY, read);
					ctx.ui.notify("exhibit_source:prepared", "info");
				}
			} catch {
				// Never replay a stale read for the current accepted prompt.
				reads.delete(user.id);
			}
		}
		const messages: any[] = [];
		let currentIncluded = false;
		for (const message of event.messages) {
			messages.push(message);
			for (const read of reads.values()) {
				if (matches(message, read)) {
					messages.push(...projectSourceRead(read, ctx.model));
					if (read.anchor === user.id) currentIncluded = true;
				}
			}
		}
		// Compaction may replace the anchoring message with a summary. The
		// current, freshly validated source is still needed for overflow retry;
		// historical snapshots whose anchors were compacted away stay omitted.
		const current = reads.get(user.id);
		if (!currentIncluded && current) messages.push(...projectSourceRead(current, ctx.model));
		return { messages } as any;
	});
}

/**
 * Pure edit engine behind the agent's edit_artifact tool (av-f5i5).
 *
 * Modeled on pi's own edit tool: one call carries one or more targeted
 * replacements, each matched against the ORIGINAL body exactly once, then
 * applied in reverse position order so earlier edits never shift the offsets
 * of later ones. Everything outside the replaced spans is spliced from the
 * original bytes untouched, which is what preserves a BOM, CRLF line endings,
 * and runs of whitespace the fuzzy matcher collapsed.
 *
 * This module has no imports on purpose: it is exercised directly by
 * `node --test` (edit.test.ts) with no dependencies, and imported by
 * exhibit.ts, which runs inside pi via jiti.
 */

/** One targeted replacement inside an edit_artifact call. */
export interface ArtifactEdit {
	oldText: string;
	newText: string;
}

/** Where one edit landed in the original body. */
export interface EditSpan {
	start: number;
	end: number;
	/** True when the span was found by the fuzzy fallback, not exactly. */
	fuzzy: boolean;
}

/**
 * Validation failure for one edit_artifact call. Throwing (rather than
 * returning a partial result) is what guarantees the ticket's atomicity rule:
 * the caller issues no PATCH when any single edit is invalid, so the stored
 * body is left exactly as it was.
 */
export class EditValidationError extends Error {
	/** Index into the call's edits[] of the offending edit, or -1 for a call-level problem. */
	readonly editIndex: number;

	constructor(editIndex: number, message: string) {
		super(message);
		this.name = "EditValidationError";
		this.editIndex = editIndex;
	}
}

// Smart quotes -> ASCII.
const QUOTE_MAP: Record<string, string> = {
	"‘": "'",
	"’": "'",
	"‚": "'",
	"‛": "'",
	"“": '"',
	"”": '"',
	"„": '"',
	"‟": '"',
};

// Unicode dashes -> hyphen-minus.
const DASH_MAP: Record<string, string> = {
	"‐": "-",
	"‑": "-",
	"‒": "-",
	"–": "-",
	"—": "-",
	"―": "-",
	"−": "-",
	"﹘": "-",
	"﹣": "-",
	"－": "-",
};

function isWhitespace(ch: string): boolean {
	return ch === " " || ch === "\t" || ch === "\n" || ch === "\r" || ch === "\f" || ch === "\v";
}

/**
 * Normalize text for the fuzzy fallback, alongside an index map carrying
 * each normalized offset back to the original body.
 *
 * Normalization: smart quotes -> ASCII, unicode dashes -> hyphens, every run
 * of whitespace collapses to one space. A collapsed run's single space maps
 * to the run's FIRST original offset, so a span [map[s], map[e]) — where
 * map[e] is the next normalized char's offset, or the original length at the
 * end — always covers the full original run. The map is monotonically
 * non-decreasing, which is exactly what makes that end-boundary rule sound:
 * normalization only rewrites 1:1 or deletes, never inserts or reorders.
 */
export function normalizeWithMap(src: string): { text: string; map: number[] } {
	let text = "";
	const map: number[] = [];
	let i = 0;
	while (i < src.length) {
		const ch = src[i];
		if (isWhitespace(ch)) {
			// Collapse the whole run to one space anchored at its start.
			let j = i + 1;
			while (j < src.length && isWhitespace(src[j])) j++;
			text += " ";
			map.push(i);
			i = j;
		} else {
			text += QUOTE_MAP[ch] ?? DASH_MAP[ch] ?? ch;
			map.push(i);
			i++;
		}
	}
	return { text, map };
}

/** All (possibly overlapping) start offsets of `needle` in `haystack`. */
function allOccurrences(haystack: string, needle: string): number[] {
	const out: number[] = [];
	let from = 0;
	while (true) {
		const at = haystack.indexOf(needle, from);
		if (at < 0) return out;
		out.push(at);
		from = at + 1;
	}
}

/**
 * Resolve every edit to a span in the ORIGINAL body.
 *
 * Exact match wins: one exact occurrence applies as-is. Zero exact matches
 * fall back to the normalized search (smart quotes, dashes, whitespace); a
 * fuzzy hit is mapped back through the index map before it is recorded, so
 * the splice loop below only ever touches original coordinates.
 *
 * Rejections (all before anything is written):
 * - empty oldText
 * - no exact and no fuzzy match ("no match" names the edit index)
 * - more than one match ("ambiguous" — never silently edit the first)
 * - any two resolved spans that overlap or touch (merge them into one edit)
 */
export function resolveSpans(body: string, edits: ArtifactEdit[]): EditSpan[] {
	if (!Array.isArray(edits) || edits.length === 0) {
		throw new EditValidationError(-1, "edit_artifact needs at least one edit in edits[].");
	}

	let normalizedBody: { text: string; map: number[] } | null = null;
	const lazyBody = () => (normalizedBody ??= normalizeWithMap(body));

	const spans: EditSpan[] = edits.map((edit, index) => {
		const label = `edits[${index}]`;
		if (typeof edit.oldText !== "string" || edit.oldText === "") {
			throw new EditValidationError(index, `${label} has an empty oldText — nothing to match.`);
		}

		const exact = allOccurrences(body, edit.oldText);
		if (exact.length === 1) {
			return { start: exact[0], end: exact[0] + edit.oldText.length, fuzzy: false };
		}
		if (exact.length > 1) {
			throw new EditValidationError(
				index,
				`${label} matches ${exact.length} places in the artifact — oldText must be unique (add surrounding context), or the edits belong in one larger replacement.`,
			);
		}

		// No exact match: fuzzy fallback over normalized text.
		const { text: normBody, map } = lazyBody();
		const needle = normalizeWithMap(edit.oldText).text.trim();
		if (needle === "") {
			throw new EditValidationError(index, `${label} matches nothing in the artifact — re-read it with get_artifact and copy oldText from the current source.`);
		}
		const fuzzy = allOccurrences(normBody, needle);
		if (fuzzy.length === 0) {
			throw new EditValidationError(index, `${label} matches nothing in the artifact — re-read it with get_artifact and copy oldText from the current source.`);
		}
		if (fuzzy.length > 1) {
			throw new EditValidationError(
				index,
				`${label} matches ${fuzzy.length} places in the artifact (fuzzy) — oldText must be unique (add surrounding context), or the edits belong in one larger replacement.`,
			);
		}
		const s = fuzzy[0];
		const e = s + needle.length;
		// Map back to original coordinates: map[e] is the next normalized
		// char's original offset — i.e. the end of this run — or the body
		// length at the very end.
		return { start: map[s], end: e < map.length ? map[e] : body.length, fuzzy: true };
	});

	// Overlap or touch: sort by start and require a gap. Sorted order is only
	// for the check — application below re-sorts in reverse.
	const ordered = spans
		.map((span, index) => ({ span, index }))
		.sort((a, b) => a.span.start - b.span.start || a.span.end - b.span.end);
	for (let k = 1; k < ordered.length; k++) {
		if (ordered[k].span.start <= ordered[k - 1].span.end) {
			throw new EditValidationError(
				ordered[k].index,
				`edits[${ordered[k - 1].index}] and edits[${ordered[k].index}] overlap or touch — merge them into one edit instead.`,
			);
		}
	}

	return spans;
}

/**
 * Apply validated edits to the body. Matching runs once against the original;
 * splicing runs in reverse position order so offsets never shift. Returns the
 * new body with every untouched byte preserved verbatim.
 */
export function applyEdits(body: string, edits: ArtifactEdit[]): { body: string; spans: EditSpan[] } {
	const spans = resolveSpans(body, edits);
	const order = spans
		.map((span, index) => ({ span, index }))
		.sort((a, b) => b.span.start - a.span.start);
	let out = body;
	for (const { span, index } of order) {
		out = out.slice(0, span.start) + edits[index].newText + out.slice(span.end);
	}
	return { body: out, spans };
}

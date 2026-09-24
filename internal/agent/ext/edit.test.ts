/**
 * Unit tests for the edit_artifact engine (av-f5i5). Pure logic, no
 * dependencies: run with `node --test edit.test.ts` (Node strips types), and
 * via `go test ./internal/agent/` (ext_test.go) like the gallery suites.
 */
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { applyEdits, normalizeWithMap, resolveSpans, EditValidationError } from "./edit.ts";

const DOC = [
	"<!DOCTYPE html>",
	"<html>",
	"<body>",
	"  <h1>Click Counter</h1>",
	'  <button id="submit-btn">Count!</button>',
	"</body>",
	"</html>",
].join("\n");

describe("exact matching", () => {
	it("replaces a single edit", () => {
		const { body } = applyEdits(DOC, [{ oldText: "<h1>Click Counter</h1>", newText: "<h1>Total Clicks</h1>" }]);
		assert.ok(body.includes("<h1>Total Clicks</h1>"));
		assert.ok(!body.includes("Click Counter"));
	});

	it("applies multiple non-overlapping edits in one call", () => {
		const { body } = applyEdits(DOC, [
			{ oldText: "<h1>Click Counter</h1>", newText: "<h1>Total Clicks</h1>" },
			{ oldText: "Count!", newText: "Tap!" },
		]);
		assert.ok(body.includes("<h1>Total Clicks</h1>"));
		assert.ok(body.includes(">Tap!</button>"));
	});

	it("supports deletion with an empty newText", () => {
		const { body } = applyEdits(DOC, [{ oldText: "  <h1>Click Counter</h1>\n", newText: "" }]);
		assert.ok(!body.includes("<h1>"));
	});

	it("applies in reverse order so earlier length changes never shift later offsets", () => {
		const body = "aa bb cc";
		const { body: out } = applyEdits(body, [
			{ oldText: "aa", newText: "a much longer replacement" },
			{ oldText: "cc", newText: "c" },
		]);
		assert.equal(out, "a much longer replacement bb c");
	});

	it("reports spans in original coordinates", () => {
		const { spans } = applyEdits(DOC, [{ oldText: "Count!", newText: "Tap!" }]);
		assert.equal(spans.length, 1);
		assert.equal(spans[0].fuzzy, false);
		assert.equal(DOC.slice(spans[0].start, spans[0].end), "Count!");
	});
});

describe("validation", () => {
	it("rejects an empty oldText", () => {
		assert.throws(() => applyEdits(DOC, [{ oldText: "", newText: "x" }]), (err) => {
			assert.ok(err instanceof EditValidationError);
			assert.equal(err.editIndex, 0);
			assert.match(err.message, /empty oldText/);
			return true;
		});
	});

	it("rejects an empty edits array", () => {
		assert.throws(() => applyEdits(DOC, []), /at least one edit/);
	});

	it("rejects a non-unique oldText instead of editing the first match", () => {
		assert.throws(
			() => applyEdits("<p>a</p><p>a</p>", [{ oldText: "<p>a</p>", newText: "<p>b</p>" }]),
			(err) => {
				assert.ok(err instanceof EditValidationError);
				assert.match(err.message, /2 places/);
				return true;
			},
		);
	});

	it("rejects overlapping edits", () => {
		assert.throws(
			() =>
				applyEdits("hello world", [
					{ oldText: "hello", newText: "hi" },
					{ oldText: "lo wo", newText: "LO WO" },
				]),
			(err) => {
				assert.ok(err instanceof EditValidationError);
				assert.match(err.message, /overlap or touch/);
				return true;
			},
		);
	});

	it("rejects touching (adjacent) edits — merge them instead", () => {
		assert.throws(
			() =>
				applyEdits("hello world", [
					{ oldText: "hello", newText: "hi" },
					{ oldText: " world", newText: " there" },
				]),
			/overlap or touch/,
		);
	});

	it("rejects a no-match edit and names its index", () => {
		assert.throws(() => applyEdits(DOC, [{ oldText: "no such element anywhere", newText: "x" }]), (err) => {
			assert.ok(err instanceof EditValidationError);
			assert.equal(err.editIndex, 0);
			assert.match(err.message, /edits\[0\] matches nothing/);
			return true;
		});
	});

	it("names the failing edit when a later edit in the call is bad", () => {
		assert.throws(
			() =>
				applyEdits(DOC, [
					{ oldText: "Count!", newText: "Tap!" },
					{ oldText: "missing piece", newText: "x" },
				]),
			(err) => {
				assert.ok(err instanceof EditValidationError);
				assert.equal(err.editIndex, 1);
				return true;
			},
		);
	});
});

describe("fuzzy fallback", () => {
	it("tolerates smart quotes", () => {
		const body = "<p>it’s a “quoted” word</p>";
		const { body: out, spans } = applyEdits(body, [{ oldText: "<p>it's a \"quoted\" word</p>", newText: "<p>done</p>" }]);
		assert.equal(out, "<p>done</p>");
		assert.equal(spans[0].fuzzy, true);
	});

	it("tolerates unicode dashes", () => {
		const body = "<p>well — known</p>";
		const { body: out } = applyEdits(body, [{ oldText: "<p>well - known</p>", newText: "<p>done</p>" }]);
		assert.equal(out, "<p>done</p>");
	});

	it("collapses extra and trailing whitespace", () => {
		const body = "<div>   spaced    out   </div>";
		const { body: out } = applyEdits(body, [{ oldText: "<div> spaced out </div>", newText: "<div>done</div>" }]);
		assert.equal(out, "<div>done</div>");
	});

	it("matches across newlines with a single-line oldText", () => {
		const body = "<h1>Hello\n  world</h1>";
		const { body: out } = applyEdits(body, [{ oldText: "<h1>Hello world</h1>", newText: "<h1>done</h1>" }]);
		assert.equal(out, "<h1>done</h1>");
	});

	it("rejects a fuzzily ambiguous oldText", () => {
		// No exact hit (both paragraphs carry extra spaces), but both
		// normalize to the same needle: ambiguous under the fallback too.
		const body = "<p>a  b</p><p>a   b</p>";
		assert.throws(() => applyEdits(body, [{ oldText: "<p>a b</p>", newText: "x" }]), (err) => {
			assert.ok(err instanceof EditValidationError);
			assert.match(err.message, /2 places.*fuzzy/);
			return true;
		});
	});
});

describe("byte preservation", () => {
	it("preserves CRLF line endings outside (and inside) untouched regions", () => {
		const body = "<html>\r\n<body>\r\n  <h1>Hi</h1>\r\n</body>\r\n</html>";
		const { body: out } = applyEdits(body, [{ oldText: "<h1>Hi</h1>", newText: "<h1>Yo</h1>" }]);
		assert.equal(out, "<html>\r\n<body>\r\n  <h1>Yo</h1>\r\n</body>\r\n</html>");
	});

	it("preserves a leading BOM", () => {
		const body = "﻿<h1>Hi</h1>";
		const { body: out } = applyEdits(body, [{ oldText: "<h1>Hi</h1>", newText: "<h1>Yo</h1>" }]);
		assert.equal(out, "﻿<h1>Yo</h1>");
	});

	it("maps fuzzy matches back to original offsets over CRLF", () => {
		const body = "<div>\r\n   spaced    out\r\n</div>";
		const { spans, body: out } = applyEdits(body, [{ oldText: "<div> spaced out </div>", newText: "<p>done</p>" }]);
		assert.equal(out, "<p>done</p>");
		assert.equal(spans[0].fuzzy, true);
		// The whole body matched, so the span covers it end to end.
		assert.equal(spans[0].start, 0);
		assert.equal(spans[0].end, body.length);
	});

	it("keeps collapsed runs of whitespace outside the spans verbatim", () => {
		const body = "<ul>\n  <li>a</li>      <li>b</li>\n</ul>";
		const { body: out } = applyEdits(body, [{ oldText: "<li>a</li>", newText: "<li>A</li>" }]);
		assert.equal(out, "<ul>\n  <li>A</li>      <li>b</li>\n</ul>");
	});

	it("handles a multi-line oldText", () => {
		const { body: out } = applyEdits(DOC, [
			{
				oldText: "  <h1>Click Counter</h1>\n" + '  <button id="submit-btn">Count!</button>',
				newText: "  <h1>Done</h1>",
			},
		]);
		assert.ok(out.includes("<h1>Done</h1>"));
		assert.ok(!out.includes("submit-btn"));
	});
});

describe("normalizeWithMap", () => {
	it("builds a monotone map back to original offsets", () => {
		const src = "a  ‘b’ — c";
		const { text, map } = normalizeWithMap(src);
		assert.equal(text, "a 'b' - c");
		assert.equal(map.length, text.length);
		for (let k = 1; k < map.length; k++) assert.ok(map[k] >= map[k - 1]);
		// Spot-check: the collapsed run's space points at the run's start.
		assert.equal(src[map[text.indexOf("'b'")] - 0], "‘");
	});

	it("resolveSpans exposes fuzzy spans for inspection", () => {
		const spans = resolveSpans("<p>a  b</p>", [{ oldText: "<p>a b</p>", newText: "x" }]);
		assert.equal(spans.length, 1);
		assert.equal(spans[0].fuzzy, true);
	});
});

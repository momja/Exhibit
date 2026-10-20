package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArtifactBundleJavaScriptStatementContexts(t *testing.T) {
	for _, test := range []struct{ name, source string }{
		{"optional catch", `try {} catch {} /[/*]/.test('x');`},
		{"catch binding", `try {} catch (err) {} /[/*]/.test('x');`},
		{"catch destructuring", `try {} catch ({message}) {} /[/*]/.test('x');`},
		{"finally", `try {} finally {} /[/*]/.test('x');`},
		{"labeled block", `label: {} /[/*]/.test('x');`},
		{"nested labels", `outer: inner: { function f() {} /[/*]/.test('x'); }`},
		{"labeled async declaration", `label: { async function f() {} /[/*]/.test('x'); }`},
		{"class static block", `class C { static { {} /[/*]/.test('x'); } }`},
		{"class static function declaration", `class C { static { function f() {} /[/*]/.test('x'); } }`},
		{"class static label", `class C { static { label: {} /[/*]/.test('x'); } }`},
		{"class expression static block", `const C = class { static { {} /[/*]/.test('x'); } };`},
		{"switch case block", `switch (x) { case 1: {} /[/*]/.test('x'); }`},
		{"switch default block", `switch (x) { default: {} /[/*]/.test('x'); }`},
		{"switch case declaration", `switch (x) { case 1: function f() {} /[/*]/.test('x'); }`},
		{"switch conditional case", `switch (x) { case x ? {} / 2 : 1: {} /[/*]/.test('x'); }`},
		{"switch object case", `switch (x) { case ({default: {} / 2}): {} /[/*]/.test('x'); }`},
		{"if", `if (ok) /[/*]/.test('x');`},
		{"else", `if (ok) {} else /[/*]/.test('x');`},
		{"while", `while (ok) /[/*]/.test('x');`},
		{"for", `for (; ok;) /[/*]/.test('x');`},
		{"for await", `async function f() { for await (const x of xs) /[/*]/.test('x'); }`},
		{"with", `with (obj) /[/*]/.test('x');`},
		{"do while", `do /[/*]/.test('x'); while (ok);`},
		{"return regex", `function f() { return /[/*]/; }`},
		{"return object", `function f() { return {} / 2; }`},
		{"return newline block", "function f() { return\n{ function g() {} /[/*]/.test('x'); } }"},
		{"return comment newline", "function f() { return /*\n*/ { function g() {} /[/*]/.test('x'); } }"},
		{"throw regex", `function f() { throw /[/*]/; }`},
		{"arrow block declaration", `const f = () => { function g() {} /[/*]/.test('x'); };`},
		{"arrow concise regex", `const f = () => /[/*]/;`},
		{"function declaration", `function f() {} /[/*]/.test('x');`},
		{"async function declaration", `async function f() {} /[/*]/.test('x');`},
		{"generator declaration", `function* f() {} /[/*]/.test('x');`},
		{"class declaration", `class C {} /[/*]/.test('x');`},
		{"export function declaration", `export default function() {} /[/*]/.test('x');`},
		{"export class declaration", `export default class {} /[/*]/.test('x');`},
		{"call division", `const n = f() / 2;`},
		{"object division", `const n = {x: 2} / 2;`},
		{"function expression division", `const n = function() {} / 2;`},
		{"async expression division", `const n = async function() {} / 2;`},
		{"class expression division", `const n = class {} / 2;`},
		{"arrow block division", `const n = (() => {}) / 2;`},
		{"arrow object division", `const n = (() => ({}))() / 2;`},
		{"function in for expression", `for (; function() {} / 2;) { break; }`},
		{"default parameter expression", `function f(x = function() {} / 2) {} /[/*]/.test('x');`},
		{"reserved property call division", `const n = obj.if() / 2;`},
		{"optional property call division", `const n = obj?.catch() / 2;`},
		{"reserved property function division", `const n = obj.function() / 2;`},
		{"optional call division", `const n = obj?.() / 2;`},
		{"object reserved keys", `const n = {function: 1, class: 2, if: 3} / 2;`},
		{"object reserved method", `const n = {if() { function f() {} /[/*]/.test('x'); }} / 2;`},
		{"object getter", `const n = {get class() { return /[/*]/; }} / 2;`},
		{"object generator", `const n = {*function() { return /[/*]/; }} / 2;`},
		{"class reserved methods", `class C { function() {} class() {} if() {} } /[/*]/.test('x');`},
		{"class field expression", `class C { static f = function() {} / 2; static { {} /[/*]/.test('x'); } }`},
		{"class field object operand", `class C { x = {} instanceof /[/*]/; }`},
		{"class field function operand", `class C { x = function() {} instanceof /[/*]/; }`},
		{"class field ASI reserved method", "class C { x = 1\nclass() { return /[/*]/; } }"},
		{"class empty field ASI reserved method", "class C { x\nclass() { return /[/*]/; } }"},
		{"class field comment ASI", "class C { x = 1/*\n*/class() { return /[/*]/; } }"},
		{"class field ASI static block", "class C { x = 1\nstatic { {} /[/*]/.test('x'); } }"},
		{"class field ASI private field", "class C { x = 1\n#field = 2\nclass() { return /[/*]/; } }"},
		{"class field continued call", "class C { x = f\n(1); class() { return /[/*]/; } }"},
		{"class field continued indexing", "class C { x = obj\n[key]; class() { return /[/*]/; } }"},
		{"class field continued operator", "class C { x = 1\n+2; class() { return /[/*]/; } }"},
		{"class field continued instanceof", "class C { x = {}\ninstanceof /[/*]/; }"},
		{"class field continued in", "class C { x = 'key'\nin /[/*]/; }"},
		{"class property newline instanceof", "class C { x = obj.\nvalue instanceof /[/*]/; }"},
		{"class optional property newline instanceof", "class C { x = obj?.\nvalue instanceof /[/*]/; }"},
		{"class property newline in", "class C { x = obj.\nvalue in /[/*]/; }"},
		{"class field prefix increment newline", "class C { x = ++\nobj.value instanceof /[/*]/; }"},
		{"class field prefix decrement newline", "class C { x = --\nobj.value instanceof /[/*]/; }"},
		{"conditional object division", `const n = ok ? {} / 2 : {} / 3;`},
		{"conditional function division", `const n = ok ? function() {} / 2 : class {} / 3;`},
		{"template interpolation regex", "const s = `x${ /[/*]/.test('x') }`;"},
		{"template interpolation object division", "const s = `x${ {} / 2 }`;"},
		{"template interpolation function division", "const s = `x${ function() {} / 2 }`;"},
		{"nested templates", "const s = `x${ `y${ {} / 2 }${ /[/*]/.test('x') }` }`;"},
		{"template URL with interpolation", "const s = `/a/original/assets/image${ /[/*]/.test('x') }`;"},
		{"template middle and end URLs", "const s = `x${ /[/*]/.test('x') }/a/original/assets/image${ {} / 2 }/a/original/assets/large`;"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := `/* /a/original/assets/missing */ ` + test.source +
				` fetch('/a/original/assets/image'); /* /a/original/assets/missing */`
			for _, contentType := range []string{"application/javascript", "text/html"} {
				t.Run(contentType, func(t *testing.T) {
					content := source
					if contentType == "text/html" {
						content = "<p>π</p><script>" + content + "</script>"
					}
					want := strings.NewReplacer("/a/original/assets/image", "embedded-image", "/a/original/assets/large", "embedded-large").Replace(content)
					require.Equal(t, want, rewriteArtifactBundleReferencesForTest(t, content, contentType))
				})
			}
		})
	}
}

func TestArtifactBundleJavaScriptMIMETypes(t *testing.T) {
	// The HTML script type definition uses this complete MIME Sniffing list:
	// https://mimesniff.spec.whatwg.org/#javascript-mime-type
	for _, essence := range []string{
		"application/ecmascript", "application/javascript", "application/x-ecmascript", "application/x-javascript",
		"text/ecmascript", "text/javascript", "text/javascript1.0", "text/javascript1.1", "text/javascript1.2",
		"text/javascript1.3", "text/javascript1.4", "text/javascript1.5", "text/jscript", "text/livescript",
		"text/x-ecmascript", "text/x-javascript",
	} {
		t.Run(essence, func(t *testing.T) {
			source := `/* /a/original/assets/missing */ fetch('/a/original/assets/image');`
			for _, contentType := range []string{essence, strings.ToUpper(essence) + "; charset=utf-8"} {
				require.Equal(t, strings.ReplaceAll(source, "/a/original/assets/image", "embedded-image"),
					rewriteArtifactBundleReferencesForTest(t, source, contentType))
			}
			html := `<script type="` + strings.ToUpper(essence) + `">` + source + `</script>`
			require.Equal(t, strings.ReplaceAll(html, "/a/original/assets/image", "embedded-image"),
				rewriteArtifactBundleReferencesForTest(t, html, "text/html"))
		})
	}
	for _, contentType := range []string{"application/json", "text/json", "application/ld+json", "Application/Manifest+Json; charset=utf-8"} {
		t.Run(contentType, func(t *testing.T) {
			source := `{"literal":"/*", "image":"/a/original/assets/image"}`
			require.Equal(t, strings.ReplaceAll(source, "/a/original/assets/image", "embedded-image"),
				rewriteArtifactBundleReferencesForTest(t, source, contentType))
			html := `<script type="` + contentType + `">` + source + `</script>`
			require.Equal(t, strings.ReplaceAll(html, "/a/original/assets/image", "embedded-image"),
				rewriteArtifactBundleReferencesForTest(t, html, "text/html"))
		})
	}
}

func TestArtifactBundleJavaScriptHashbangOffsets(t *testing.T) {
	const asset = "/a/original/assets/image"
	for _, prefix := range []string{"", "\uFEFF"} {
		for _, newline := range []string{"\n", "\r\n", "\r", "\u2028", "\u2029"} {
			source := prefix + "#!/usr/bin/env node /a/original/assets/missing π" + newline +
				`/* /a/original/assets/missing */ fetch('` + asset + `'); fetch('` + asset + `#two');`
			for _, contentType := range []string{"application/javascript", "text/html"} {
				t.Run(contentType+"/"+prefix+"/"+newline, func(t *testing.T) {
					content := source
					if contentType == "text/html" {
						content = "<p>π</p><script>" + source + "</script>"
					}
					scanner := newArtifactBundleReferenceScanner(content, contentType)
					start := strings.Index(content, "fetch('") + len("fetch('")
					for _, url := range []string{asset, asset + "#two"} {
						reference, ok := scanner.next()
						require.True(t, ok)
						require.Equal(t, url, reference.url)
						require.Equal(t, start, reference.start)
						require.Equal(t, start+len(url), reference.end)
						require.Equal(t, url, content[reference.start:reference.end])
						start += len(url) + len("'); fetch('")
					}
					_, ok := scanner.next()
					require.False(t, ok)
					require.Equal(t, strings.ReplaceAll(content, asset, "embedded-image"),
						rewriteArtifactBundleReferencesForTest(t, content, contentType))
				})
			}
		}
	}
	for _, source := range []string{"", "#", "#!", "#!/usr/bin/env node /a/original/assets/missing", "\uFEFF", "\uFEFF#!", "#!\n'", "#!\n/*", "#!\n`"} {
		t.Run("empty or incomplete/"+source, func(t *testing.T) {
			require.Equal(t, source, rewriteArtifactBundleReferencesForTest(t, source, "application/javascript"))
			html := "<script>" + source + "</script>"
			require.Equal(t, html, rewriteArtifactBundleReferencesForTest(t, html, "text/html"))
		})
	}
}

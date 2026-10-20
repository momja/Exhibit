package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArtifactBundleReferenceBoundariesAndSrcset(t *testing.T) {
	for _, test := range []struct{ name, source, want, contentType string }{
		{"foreign parentheses", `<img src="https://other.example/view(/a/original/assets/missing)">`, `<img src="https://other.example/view(/a/original/assets/missing)">`, ""},
		{"nested path", `<img src="/nested/a/original/assets/image">`, `<img src="/nested/a/original/assets/image">`, ""},
		{"relative path", `<img src="relative/a/original/assets/image">`, `<img src="relative/a/original/assets/image">`, ""},
		{"opaque data URL", `<img src="data:text/plain,/a/original/assets/missing">`, `<img src="data:text/plain,/a/original/assets/missing">`, ""},
		{"filename suffix", `<img src="/a/original/assets/image.png">`, `<img src="/a/original/assets/image.png">`, ""},
		{"path suffix", `<img src="/a/original/assets/image/another">`, `<img src="/a/original/assets/image/another">`, ""},
		{"numeric default port", `<img src="https://RENDER.example:0443/a/original/assets/image">`, `<img src="embedded-image">`, ""},
		{"CSS unquoted", `body{background:url(/a/original/assets/image?x=1#view)}`, `body{background:url(embedded-image#view)}`, "text/css"},
		{"CSS leading comment", `/*theme*/body{background:url(/a/original/assets/image)}`, `/*theme*/body{background:url(embedded-image)}`, "text/css"},
		{"CSS commented dependency", `/* /a/original/assets/missing */body{background:url(/a/original/assets/image)}`, `/* /a/original/assets/missing */body{background:url(embedded-image)}`, "text/css"},
		{"MIME wildcard", `<input accept="image/*"><img src="/a/original/assets/image">`, `<input accept="image/*"><img src="embedded-image">`, ""},
		{"quoted comment opener", `const text = "/*"; fetch('/a/original/assets/image');`, `const text = "/*"; fetch('embedded-image');`, "application/javascript"},
		{"CSS trailing comment", `body{}/* /a/original/assets/missing */p{background:url(/a/original/assets/image)}`, `body{}/* /a/original/assets/missing */p{background:url(embedded-image)}`, "text/css"},
		{"HTML prose", `<p>Here's an image</p><img srcset="/a/original/assets/image, /a/original/assets/large 2x">`, `<p>Here's an image</p><img srcset="embedded-image, embedded-large 2x">`, ""},
		{"CSS escaped quote query", `body{background:url("/a/original/assets/image?label=it\'s")}`, `body{background:url("embedded-image")}`, "text/css"},
		{"CSS escaped delimiter query", `body{background:url("/a/original/assets/image?label=it\"s")}`, `body{background:url("embedded-image")}`, "text/css"},
		{"CSS foreign escaped quote", `body{background:url("https://other.example/it\'s/a/original/assets/missing")}`, `body{background:url("https://other.example/it\'s/a/original/assets/missing")}`, "text/css"},
		{"CSS other quote", `body{background:url("/a/original/assets/image?label=it's")}`, `body{background:url("embedded-image")}`, "text/css"},
		{"escaped JSON string", `const config = "{\"image\":\"https://render.example/a/original/assets/image\"}";`, `const config = "{\"image\":\"embedded-image\"}";`, "application/javascript"},
		{"CSS quoted parentheses", `body{background:url("https://other.example/view(/a/original/assets/missing)")}`, `body{background:url("https://other.example/view(/a/original/assets/missing)")}`, "text/css"},
		{"CSS foreign token", `body{background:url(https://other.example/view(/a/original/assets/missing))}`, `body{background:url(https://other.example/view(/a/original/assets/missing))}`, "text/css"},
		{"srcset query", `<img srcset="/a/original/assets/image?v=1, https://render.example/a/original/assets/large?x=a,b 2x">`, `<img srcset="embedded-image, embedded-large 2x">`, ""},
		{"srcset descriptors", `<img srcset="/a/original/assets/image 1x, /a/original/assets/large?v=1 2x">`, `<img srcset="embedded-image 1x, embedded-large 2x">`, ""},
		{"srcset unquoted", `<img srcset=/a/original/assets/image,>`, `<img srcset=embedded-image,>`, ""},
		{"imagesrcset", `<link imagesrcset="/a/original/assets/image?v=1, /a/original/assets/large 2x">`, `<link imagesrcset="embedded-image, embedded-large 2x">`, ""},
		{"ordinary comma query", `<img src="/a/original/assets/image?v=1,">`, `<img src="embedded-image">`, ""},
		{"JSON comma query", `{"url":"https://render.example/a/original/assets/image?x=a,b"}`, `{"url":"embedded-image"}`, "application/json"},
		{"unquoted attribute", `<img src=/a/original/assets/image>`, `<img src=embedded-image>`, ""},
		{"CSS bare reference", `/a/original/assets/image`, `embedded-image`, "text/css"},
		{"HTML foreign apostrophe", `<img src="https://other.example/it's/a/original/assets/missing">`, `<img src="https://other.example/it's/a/original/assets/missing">`, ""},
		{"HTML foreign entities", `<img src="https://other.example/it&apos;s/a/original/assets/missing?x=1&amp;y=2">`, `<img src="https://other.example/it&apos;s/a/original/assets/missing?x=1&amp;y=2">`, ""},
		{"CSS entity query", `<div style="background:url(&quot;/a/original/assets/image?x=1&amp;y=2&quot;)">`, `<div style="background:url(&quot;embedded-image&quot;)">`, ""},
		{"srcset entity separator", `<img srcset="/a/original/assets/image&comma; /a/original/assets/large 2x">`, `<img srcset="embedded-image&comma; embedded-large 2x">`, ""},
		{"srcset entity whitespace", `<img srcset="/a/original/assets/image&#32;1x, /a/original/assets/large 2x">`, `<img srcset="embedded-image&#32;1x, embedded-large 2x">`, ""},
		{"attribute error recovery", `<img =other disabled src = '/a/original/assets/image'>`, `<img =other disabled src = 'embedded-image'>`, ""},
		{"multiple inline URLs", `<div STYLE="background:url(&quot;/a/original/assets/image&quot;),url(&#39;/a/original/assets/large&#39;)">`, `<div STYLE="background:url(&quot;embedded-image&quot;),url(&#39;embedded-large&#39;)">`, ""},
		{"URL attribute encoded quote", `<img src="https://other.example/?q=&quot;/a/original/assets/missing">`, `<img src="https://other.example/?q=&quot;/a/original/assets/missing">`, ""},
		{"URL attribute encoded apostrophe", `<img src='https://other.example/?q=&apos;/a/original/assets/missing'>`, `<img src='https://other.example/?q=&apos;/a/original/assets/missing'>`, ""},
		{"local URL attribute quoted query", `<img src="/a/original/assets/image?q=&quot;value&quot;">`, `<img src="embedded-image">`, ""},
		{"JavaScript CSS URL", `element.style.backgroundImage = 'url(/a/original/assets/image)';`, `element.style.backgroundImage = 'url(embedded-image)';`, "application/javascript"},
		{"JavaScript CSS template URL", "element.style.backgroundImage = `url(/a/original/assets/image)`;", "element.style.backgroundImage = `url(embedded-image)`;", "application/javascript"},
		{"JavaScript quoted CSS foreign URL", `const css = 'url("https://other.example/view(/a/original/assets/missing)")';`, `const css = 'url("https://other.example/view(/a/original/assets/missing)")';`, "application/javascript"},
		{"first script type", `<script type="module" type="text/plain">/* /a/original/assets/missing */fetch('/a/original/assets/image')</script>`, `<script type="module" type="text/plain">/* /a/original/assets/missing */fetch('embedded-image')</script>`, ""},
		{"boolean script type", `<script type type="text/plain">/* /a/original/assets/missing */fetch('/a/original/assets/image')</script>`, `<script type type="text/plain">/* /a/original/assets/missing */fetch('embedded-image')</script>`, ""},
		{"JSON data attribute", `<div data-config='{"image":"/a/original/assets/image"}'>`, `<div data-config='{"image":"embedded-image"}'>`, ""},
		{"srcdoc attribute", `<iframe srcdoc='<img src="/a/original/assets/image">'></iframe>`, `<iframe srcdoc='<img src="embedded-image">'></iframe>`, ""},
		{"foreign ping quote", `<a ping="https://other.example/?q=&quot;/a/original/assets/missing">`, `<a ping="https://other.example/?q=&quot;/a/original/assets/missing">`, ""},
		{"foreign attribution quote", `<img attributionsrc="https://other.example/?q=&quot;/a/original/assets/missing">`, `<img attributionsrc="https://other.example/?q=&quot;/a/original/assets/missing">`, ""},
		{"foreign itemid quote", `<div itemid="https://other.example/?q=&quot;/a/original/assets/missing">`, `<div itemid="https://other.example/?q=&quot;/a/original/assets/missing">`, ""},
		{"nested srcdoc comment", `<iframe srcdoc='<!-- "/a/original/assets/missing" --><img src="/a/original/assets/image">'></iframe>`, `<iframe srcdoc='<!-- "/a/original/assets/missing" --><img src="embedded-image">'></iframe>`, ""},
		{"encoded srcdoc comment", `<iframe srcdoc="&lt;!-- &quot;/a/original/assets/missing&quot; --&gt;&lt;img src=&quot;/a/original/assets/image&quot;&gt;"></iframe>`, `<iframe srcdoc="&lt;!-- &quot;/a/original/assets/missing&quot; --&gt;&lt;img src=&quot;embedded-image&quot;&gt;"></iframe>`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			contentType := test.contentType
			if contentType == "" {
				contentType = "text/html"
			}
			got := rewriteArtifactBundleReferencesForTest(t, test.source, contentType)
			require.Equal(t, test.want, got)
		})
	}
}

func TestArtifactBundleOriginsUseNumericPorts(t *testing.T) {
	for _, pair := range [][2]string{
		{"https://render.example:0443/a", "https://render.example"},
		{"http://render.example:00080/a", "http://render.example"},
		{"https://render.example:008443/a", "https://render.example:8443"},
		{"https://[::1]:0443/a", "https://[::1]"},
	} {
		require.Equal(t, pair[1], artifactBundleOrigin(pair[0]))
	}
}

// Exercise the scanner independently of the exporter/importer integration.
func rewriteArtifactBundleReferencesForTest(t *testing.T, source, contentType string) string {
	t.Helper()
	content := artifactBundleContent{origin: "https://render.example", prefix: "/a/original/assets/"}
	scanner := newArtifactBundleReferenceScanner(source, contentType)
	var result strings.Builder
	copied := 0
	for reference, ok := scanner.next(); ok; reference, ok = scanner.next() {
		require.GreaterOrEqual(t, reference.start, copied)
		require.Greater(t, reference.end, reference.start)
		require.LessOrEqual(t, reference.end, len(source))
		result.WriteString(source[copied:reference.start])
		if id := content.referenceID(reference.url); id != "" {
			require.Contains(t, []string{"image", "large"}, id, "unexpected dependency %q", id)
			result.WriteString("embedded-" + id)
			if fragment := strings.IndexByte(reference.url, '#'); fragment >= 0 {
				result.WriteString(reference.url[fragment:])
			}
			result.WriteString(reference.separator)
		} else {
			result.WriteString(source[reference.start:reference.end])
		}
		copied = reference.end
	}
	result.WriteString(source[copied:])
	require.NoError(t, scanner.err)
	return result.String()
}

func TestArtifactBundleReferenceNestedHTMLWorkIsBounded(t *testing.T) {
	source := artifactBundleNestedHTML(64, `<img src="/a/original/assets/image">`)
	scanner := newArtifactBundleReferenceScanner(source, "text/html")
	for _, ok := scanner.next(); ok; _, ok = scanner.next() {
	}
	require.ErrorContains(t, scanner.err, "nesting or work limit")
}

func TestArtifactBundleReferenceCommentContexts(t *testing.T) {
	for _, test := range []struct{ name, source, contentType string }{
		{"CSS identifier", `body/* {{missing}} */{background:url('{{asset}}')}`, "text/css"},
		{"CSS parenthesis", `:is(body)/* {{missing}} */{background:url('{{asset}}')}`, "text/css"},
		{"HTML inch mark", `<p>Size: 6"</p><style>/* {{missing}} */body{background:url('{{asset}}')}</style>`, "text/html"},
		{"HTML possessive", `<p>James' photo</p><style>/* {{missing}} */body{background:url('{{asset}}')}</style>`, "text/html"},
		{"inline CSS", `<div style="/* background:url('{{missing}}') */ background:url('{{asset}}')"></div>`, "text/html"},
		{"wildcard before comment", `<input accept="image/*"><img src="{{asset}}"><style>/* {{missing}} */body{}</style>`, "text/html"},
		{"script comment literal", `<script>const text = "/*"; fetch('{{asset}}'); /* {{missing}} */</script>`, "text/html"},
		{"inline CSS entity quote", `<div style="content: &quot;/*&quot;; background:url('{{asset}}')"></div>`, "text/html"},
		{"inline CSS numeric quote", `<div style="content: &#34;/*&#x22;; background:url('{{asset}}')"></div>`, "text/html"},
		{"inline CSS literal named prefix", `<div style="content: &quotx; /* {{missing}} */ background:url('{{asset}}')"></div>`, "text/html"},
		{"inline CSS literal named assignment", `<div style="content: &quot=; /* {{missing}} */ background:url('{{asset}}')"></div>`, "text/html"},
		{"inline CSS entity literal", `<div style="content: &quot;/* {{asset}} &quot;; /* {{missing}} */"></div>`, "text/html"},
		{"inline CSS decoded comment", `<div style="&#47;* {{missing}} *&#47; background:url(&quot;{{asset}}&quot;)"></div>`, "text/html"},
		{"inline CSS entity apostrophe query", `<div style="background:url(&quot;{{asset}}?label=it's&quot;); /* {{missing}} */"></div>`, "text/html"},
		{"inline JS", `<button onclick="/* {{missing}} */ fetch(&quot;{{asset}}&quot;)">go</button>`, "text/html"},
		{"script regex", `<script>const pattern = /[/*]/; fetch('{{asset}}'); /* {{missing}} */</script>`, "text/html"},
		{"regex after control", `if (ok) /[/*]/.test(value); fetch('{{asset}}'); /* {{missing}} */`, "application/javascript"},
		{"regex after return", `function f() { return /[/*]/; } fetch('{{asset}}'); /* {{missing}} */`, "application/javascript"},
		{"division", `const n = f() / 2; fetch('{{asset}}'); /* {{missing}} */`, "application/javascript"},
		{"object division", `const n = {x: 2} / 2; fetch('{{asset}}'); /* {{missing}} */`, "application/javascript"},
		{"function expression division", `const n = function() {} / 2; fetch('{{asset}}'); /* {{missing}} */`, "application/javascript"},
		{"arrow expression division", `const n = (() => {}) / 2; fetch('{{asset}}'); /* {{missing}} */`, "application/javascript"},
		{"class expression division", `const n = class {} / 2; fetch('{{asset}}'); /* {{missing}} */`, "application/javascript"},
		{"function declaration regex", `function f() {} /[/*]/.test(s); fetch('{{asset}}'); /* {{missing}} */`, "application/javascript"},
		{"class declaration regex", `class C {} /[/*]/.test(s); fetch('{{asset}}'); /* {{missing}} */`, "application/javascript"},
		{"template interpolation", "const s = `x${ /* {{missing}} */ fetch('{{asset}}') }`;", "application/javascript"},
		{"JS line comment", "// {{missing}}\nfetch('{{asset}}');", "application/javascript"},
		{"CSS URL wildcard", `body{background:url({{asset}}?label=/*)}`, "text/css"},
		{"HTML comment", `<!-- {{missing}} --><img src="{{asset}}">`, "text/html"},
		{"SVG style", `<svg><style>g/* {{missing}} */{fill:url('{{asset}}')}</style></svg>`, "image/svg+xml"},
		{"SVG CDATA style", `<svg><style><![CDATA[/* {{missing}} */g{fill:url('{{asset}}')}]]></style></svg>`, "image/svg+xml"},
		{"script raw text", `<script>const s = '<p>6"</p>'; /* {{missing}} */ fetch('{{asset}}');</script>`, "text/html"},
		{"CSS MIME parameters", `/* {{missing}} */body{background:url('{{asset}}')}`, "TeXt/CsS; Charset=UTF-8"},
		{"JS MIME parameters", `/* {{missing}} */fetch('{{asset}}');`, "Application/JavaScript; charset=utf-8"},
		{"HTML MIME parameters", `<!-- {{missing}} --><img src="{{asset}}">`, "TeXt/HTML; charset=utf-8"},
		{"module script", `<script type="module">/* {{missing}} */ fetch('{{asset}}');</script>`, "text/html"},
		{"JSON script", `<script type="application/ld+json">{"literal":"/*", "image":"{{asset}}"}</script>`, "text/html"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := strings.NewReplacer("{{asset}}", "/a/original/assets/image", "{{missing}}", "/a/original/assets/missing").Replace(test.source)
			want := strings.ReplaceAll(source, "/a/original/assets/image", "embedded-image")
			if strings.Contains(source, "/a/original/assets/image?label=") {
				want = strings.ReplaceAll(want, "?label=/*", "")
				want = strings.ReplaceAll(want, "?label=it's", "")
			}
			require.Equal(t, want, rewriteArtifactBundleReferencesForTest(t, source, test.contentType))
		})
	}
}

func TestArtifactBundleReferenceLiteralContent(t *testing.T) {
	for _, contentType := range []string{"", "text/plain"} {
		source := `https://render.example/a/original/assets/image?label=/*`
		require.Equal(t, "embedded-image", rewriteArtifactBundleReferencesForTest(t, source, contentType))
	}
}

func TestArtifactBundleReferenceOriginalBytes(t *testing.T) {
	source := `<div style="background:url(&quot;https://other.example/it&apos;s/a/original/assets/missing?x=1&amp;y=2&quot;)">`
	scanner := newArtifactBundleReferenceScanner(source, "text/html")
	var found bool
	for reference, ok := scanner.next(); ok; reference, ok = scanner.next() {
		if strings.HasPrefix(reference.url, "https://") {
			found = true
			require.Equal(t, `https://other.example/it&apos;s/a/original/assets/missing?x=1&amp;y=2`, reference.url)
			require.Equal(t, reference.url, source[reference.start:reference.end])
		}
	}
	require.True(t, found)
}

func TestArtifactBundleReferenceManyComments(t *testing.T) {
	for _, test := range []struct{ contentType, comment, active string }{
		{"text/css", `/* /a/original/assets/missing */`, `body{background:url('/a/original/assets/image')}`},
		{"application/javascript", `/* /a/original/assets/missing */`, `fetch('/a/original/assets/image')`},
		{"text/html", `<!-- /a/original/assets/missing -->`, `<img src="/a/original/assets/image">`},
	} {
		t.Run(test.contentType, func(t *testing.T) {
			source := strings.Repeat(test.comment, 32768) + test.active
			scanner := newArtifactBundleReferenceScanner(source, test.contentType)
			reference, ok := scanner.next()
			require.True(t, ok)
			require.Equal(t, "/a/original/assets/image", reference.url)
			_, ok = scanner.next()
			require.False(t, ok)
		})
	}
}

func BenchmarkArtifactBundleReferenceComments(b *testing.B) {
	for _, contentType := range []string{"text/css", "application/javascript", "text/html"} {
		b.Run(contentType, func(b *testing.B) {
			comment := "/*x*/"
			if contentType == "text/html" {
				comment = "<!--x-->"
			}
			source := strings.Repeat(comment, 1<<18)
			b.SetBytes(int64(len(source)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				scanner := newArtifactBundleReferenceScanner(source, contentType)
				if _, ok := scanner.next(); ok {
					b.Fatal("comment yielded a reference")
				}
			}
		})
	}
}

// Package export materializes an artifact into a single self-contained file.
//
// The invariant this package exists to enforce (av-vnkt):
//
//	The out-of-line asset URL is an internal storage and transport
//	representation. The *file* is the canonical artifact, and it is
//	materialized at every boundary where the artifact leaves the service.
//
// That matters because av-20fk and av-oz40 moved an artifact's payloads out of
// its body and behind URLs on the render origin. Inside the service that is
// strictly better — the body stays small and editable, the payloads are cached
// across views. But an artifact whose bytes depend on this instance being alive
// is no longer "just a file", which is architecture.md's first principle and
// the whole reason the library is worth keeping things in.
//
// So this is the one function every exit path calls, and the reason it is a
// package rather than a method: the static build (Exh-avau) is the other
// caller, and it wants the same decision made the same way.
//
// A single file has nowhere else to put bytes, so `data:` is the only option
// and its ~1.33x is the price of the format. That cost is paid once, here, at
// the moment someone asks for a portable copy — not on every render, which is
// what the old design did.
package export

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"golang.org/x/net/html"

	"github.com/momja/Exhibit/internal/store"
)

// BlobReader fetches an asset's bytes by blob id.
type BlobReader func(blobID string) ([]byte, error)

// Materialize folds every out-of-line asset back into body and returns a
// document that depends on no origin, no token, and no running instance.
//
// The two kinds of asset are folded back the two ways they were taken out, and
// which is which is decided by the document rather than by a stored flag: a
// markup asset is one whose asset URL appears in the body, because the ingest
// walker wrote it there. Anything else was a runtime payload, whose literal the
// page still carries untouched, and is restored through a manifest exactly as
// the render surface does — but with `data:` values, since there is no route to
// point at.
//
// An artifact with no assets comes back byte-identical. That is the common case
// and it costs nothing.
func Materialize(body string, assets []store.ArtifactAsset, assetURL func(store.ArtifactAsset) string, read BlobReader) (string, error) {
	if len(assets) == 0 {
		return body, nil
	}

	manifest := map[string]string{}
	for _, a := range assets {
		raw, err := read(a.BlobID)
		if err != nil {
			// An asset we cannot read is one we must not silently drop: the
			// export would claim to be self-contained while pointing at a URL
			// that will not resolve once the instance is gone.
			return "", err
		}
		uri := "data:" + a.ContentType + ";base64," + base64.StdEncoding.EncodeToString(raw)

		if url := assetURL(a); url != "" && strings.Contains(body, url) {
			body = strings.ReplaceAll(body, url, uri)
			continue
		}
		manifest[a.SourceURL] = uri
	}
	if len(manifest) == 0 {
		return body, nil
	}
	return injectManifest(body, manifest)
}

// injectManifest puts the runtime-payload manifest at the top of <head>, ahead
// of any of the page's own scripts — a fetch wrapper only shadows callers that
// run after it.
//
// It decodes each entry itself rather than re-issuing fetch() against the
// `data:` URI. Delegating would hand a multi-megabyte data: URL to the network
// service to parse and materialize, which is both wasteful (the bytes are
// already in the document) and the exact operation WebKit refuses for large
// payloads from an opaque origin — an exported file is often opened straight
// into an iframe somewhere, so this must not depend on the ambient environment.
func injectManifest(body string, manifest map[string]string) (string, error) {
	// json.Marshal escapes <, > and & to their \u00NN forms, so no manifest
	// value or key can terminate the enclosing <script> element.
	payload, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	script := `<script>
(function () {
  var M = ` + string(payload) + `;
  var nativeFetch = window.fetch;
  if (typeof nativeFetch !== 'function') return;

  function responseFromDataURI(uri) {
    var comma = uri.indexOf(',');
    if (comma < 0) return null;
    var meta = uri.slice(5, comma);
    var payload = uri.slice(comma + 1);
    var body;
    if (/;base64$/i.test(meta)) {
      var bin = atob(payload);
      body = new Uint8Array(bin.length);
      for (var i = 0; i < bin.length; i++) body[i] = bin.charCodeAt(i);
    } else {
      body = new TextEncoder().encode(decodeURIComponent(payload));
    }
    var mime = meta.replace(/;base64$/i, '') || 'text/plain';
    return new Response(body, { status: 200, headers: { 'Content-Type': mime } });
  }

  window.fetch = function (input, init) {
    try {
      var method = (init && init.method) || (input && input.method) || 'GET';
      if (String(method).toUpperCase() === 'GET') {
        var raw = typeof input === 'string' ? input : (input && input.url) || String(input);
        var resolved = new URL(raw, document.baseURI).href;
        if (Object.prototype.hasOwnProperty.call(M, resolved)) {
          var res = responseFromDataURI(M[resolved]);
          if (res) return Promise.resolve(res);
        }
      }
    } catch (e) { /* fall through */ }
    return nativeFetch(input, init);
  };
})();
</script>`

	at, before := headInsertion(body)
	if at < 0 {
		return script + "\n" + body, nil
	}
	if before {
		return body[:at] + script + "\n" + body[at:], nil
	}
	return body[:at] + "\n" + script + body[at:], nil
}

// headInsertion returns the byte offset the manifest goes at, and whether it
// goes before that offset (a </head> end tag) or after it (a <head> start
// tag). A document with no head at all returns -1, and the caller prepends —
// tree construction then moves the script into the implicit head.
//
// It tokenizes rather than searching for the literal "<head>" because the
// first occurrence of that text is not necessarily the element: a comment or a
// script that builds a document out of strings (an iframe srcdoc, say — a
// common enough shape in these artifacts) contains it as text. Inserting there
// puts the manifest inside a comment or a string literal, where it never runs,
// and the exported file's runtime payloads silently fail to load. Byte offsets
// come from summing the raw token text, so the document itself is returned
// unchanged rather than re-serialized.
func headInsertion(body string) (offset int, before bool) {
	z := html.NewTokenizer(strings.NewReader(body))
	for at := 0; ; {
		tt := z.Next()
		if tt == html.ErrorToken {
			return -1, false
		}
		// Raw first: TagName may reuse the buffer it returns.
		raw := len(z.Raw())
		switch tt {
		case html.StartTagToken:
			if name, _ := z.TagName(); string(name) == "head" {
				return at + raw, false
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "head" {
				return at, true
			}
		}
		at += raw
	}
}

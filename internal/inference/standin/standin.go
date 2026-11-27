// Package standin holds the unavailable window.exhibit.ai stand-in (Exh-zue3,
// Exh-i902): the API an artifact sees wherever Exhibit cannot or will not run
// AI for it.
//
// Every render installs it first, before any artifact script, so artifact code
// can always call window.exhibit.ai.available() and degrade instead of
// throwing on undefined. The framed render with a host then replaces the
// methods with the real bridge; a widget, an anonymous or public-link render,
// a top-level render and every exported file (HTML or ZIP) keep the stand-in.
//
// The stand-in makes no network request, posts no message and touches no DOM.
// It is its own tiny package so internal/export can use it without depending
// on the inference runtime.
package standin

// Script is the stand-in as plain JavaScript, idempotent, for splicing into an
// existing preamble <script>.
const Script = `
  (function() {
    var exhibit = window.exhibit = window.exhibit || {};
    function unavailable() {
      var e = new Error('AI is not available here.');
      e.name = 'ExhibitAIError';
      e.code = 'unavailable';
      return Promise.reject(e);
    }
    exhibit.ai = {
      available: function() { return Promise.resolve({ complete: false, decide: false, images: false }); },
      complete: unavailable,
      decide: unavailable
    };
    // Artifacts written for Claude's runtime call window.claude.complete(prompt)
    // and expect a string. Map it onto complete() unless something else
    // already defined it.
    var claude = window.claude = window.claude || {};
    if (typeof claude.complete !== 'function') {
      claude.complete = function(prompt) {
        return window.exhibit.ai.complete({ prompt: String(prompt) });
      };
    }
  })();
`

// Tag is Script as a standalone <script> element, for documents that have no
// preamble of their own (exports).
const Tag = "<script>/* Exhibit AI is not available in exported copies. */" + Script + "</script>"

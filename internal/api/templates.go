package api

import (
	"embed"
	"html/template"
	"strings"
)

// templatesFS carries the gallery page templates in the binary. Unlike the
// build-time assets tree (assets.go), these are committed source: they are
// authored HTML, not generated output.
//
//go:embed templates
var templatesFS embed.FS

// pageTemplates is the parsed set of app-shell page templates (gallery index,
// artifact detail, artifact edit) plus their shared partials. html/template's
// contextual auto-escaping covers every interpolation — titles and tag names
// in markup, per-request values (token, artifact id, allowlist) in the inline
// JS bootstrap — so no hand escaping is needed in the templates.
var pageTemplates = template.Must(template.ParseFS(templatesFS, "templates/*.tmpl"))

// renderPage executes one of the named page templates to a string.
func renderPage(name string, data any) (string, error) {
	var b strings.Builder
	if err := pageTemplates.ExecuteTemplate(&b, name, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

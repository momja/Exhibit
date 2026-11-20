# Feedback triage — a `window.exhibit.ai` example

A small tool for sorting incoming feedback, written to show how an artifact
uses Exhibit's AI capability ([docs/inference.md](../../docs/inference.md)).

- `index.html` — the tool. **Suggest triage with AI** asks `decide()` three
  typed questions about each untriaged item (which category, is it urgent, how
  severe) and keeps the thresholds in its own code; low confidence goes to
  "Needs a person". **Draft with AI** asks `complete()` for a summary validated
  against a JSON Schema. Both are explicit button presses, and the tool shows
  what will be sent first.
- `widget.html` — the card widget. It never calls AI; it shows the counts the
  tool saved.

It checks `available()` before anything else and keeps working when either
kind, or both, is missing: on a public link, in a directly opened tab, in a
widget, in every export (HTML or ZIP — AI is always off there), or when the
instance does not offer AI. Every error code has a message and a manual path.

To try it, enable AI on an instance (`INFERENCE_ENABLED=1`, see
[deployment.md](../../docs/deployment.md)), paste `index.html` as a new
artifact, set `widget.html` as its widget, and press **Suggest triage with AI**: the
first call asks whether the tool may use AI. `example_test.go` drives it in
Chromium against a fake host with each combination of kinds.

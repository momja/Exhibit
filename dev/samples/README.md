# Sample artifacts

Demo fixtures for the gallery-widget POC (`av-fafu`). They are **not** part of
the shipped product — nothing here is embedded in the binary. See
`docs/widgets.md` for the widget design and contract.

Load them into a running dev server:

```
make run                            # terminal 1
python3 scripts/seed-samples.py     # terminal 2
```

The seeder goes through the HTTP API like any other client, and upserts by
title, so re-running refreshes bodies, widgets, and demo state in place rather
than piling up duplicates.

## Layout

Each sample is a directory holding:

| File | |
|---|---|
| `artifact.html` | the tool itself (required); its `<title>` becomes the artifact title |
| `widget.html` | its gallery tile (optional — omit to demonstrate the default tile) |
| `state.json` | demo state as `{storage-key: value}` (optional) |

`state.json` values may contain `{{date:N}}` (the ISO date N days from today)
and `{{monday:N}}` (the Monday N weeks from this one). That is what keeps
"last 30 days" demo data from ageing into an empty widget.

## The five samples

They exist to cover every widget mode, not to be exhaustive tools:

| Sample | Widget mode |
|---|---|
| `run-tracker` | Live state — 30-day distance total plus a weekly sparkline |
| `marathon-plan` | Live state — the next scheduled run, derived from the plan |
| `reading-list` | Live state — current book and reading progress |
| `mortgage-calculator` | **Static** — an identity card with no `<script>`; the tool is stateless |
| `unit-converter` | **None** — opts out, so the card falls back to the default tile |

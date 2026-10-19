# Diagrams

Mermaid sources (`*.mmd`) and their rendered SVGs. Docs link the SVGs with
`![..](diagrams/<name>.svg)` so readers see a picture and agents reading the
doc do not pull the diagram into context.

Edit the `.mmd`, then regenerate its SVG and commit both:

```sh
cd docs/diagrams
npx -y @mermaid-js/mermaid-cli@11.4.2 -i <name>.mmd -o <name>.svg -b white
```

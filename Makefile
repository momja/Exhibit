.PHONY: build test run clean assets

build: assets
	go build -o bin/server ./cmd/server

# Fetch/bundle the embedded static assets (CodeMirror editor JS, Phosphor Icons
# CSS/webfont) into the go:embed-served asset dir. Requires Node at build time
# only. The output is NOT committed to git — it's regenerated here (and, for
# container images, in the Dockerfile's Node stage) on every build, so the
# running server still needs no Node and no network.
assets:
	cd web/editor && npm install && npm run build
	cd web/icons && npm install && npm run build

test:
	go test ./...

run:
	go run ./cmd/server

clean:
	rm -rf bin/

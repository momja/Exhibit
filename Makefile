.PHONY: build test run clean assets

build:
	go build -o bin/server ./cmd/server

# Rebuild the embedded static assets (CodeMirror editor JS, Phosphor Icons
# CSS/webfont) into the go:embed-served asset dir. Requires Node at build time
# only; the output is committed so production builds need no Node.
assets:
	cd web/editor && npm install && npm run build
	cd web/icons && npm install && npm run build

test:
	go test ./...

run:
	go run ./cmd/server

clean:
	rm -rf bin/

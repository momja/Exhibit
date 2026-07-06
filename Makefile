.PHONY: build test run clean assets

build: assets
	go build -o bin/server ./cmd/server

# Build the embedded static assets into the go:embed-served asset dir by running
# every web/ workspace's build (see scripts/build-assets.sh — the same script the
# Dockerfile's Node stage runs). Requires Node at build time only. The output is
# NOT committed to git; it's regenerated on every build, so the running server
# still needs no Node and no network.
assets:
	sh scripts/build-assets.sh

test:
	go test ./...

run:
	go run ./cmd/server

clean:
	rm -rf bin/

.PHONY: build test run clean assets lint

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

# Static analysis. golangci-lint is not vendored — install it yourself first:
#   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.0
lint:
	golangci-lint run ./...

run:
	go run ./cmd/server

clean:
	rm -rf bin/

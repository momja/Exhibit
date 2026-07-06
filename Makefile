.PHONY: build test run clean assets lint

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

# Static analysis. golangci-lint is not vendored — install it yourself first:
#   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.0
lint:
	golangci-lint run ./...

run:
	go run ./cmd/server

clean:
	rm -rf bin/

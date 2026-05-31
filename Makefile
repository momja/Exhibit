.PHONY: build test run clean

build:
	go build -o bin/server ./cmd/server
	go build -o bin/watcher ./cmd/watcher

test:
	go test ./...

run:
	go run ./cmd/server

clean:
	rm -rf bin/

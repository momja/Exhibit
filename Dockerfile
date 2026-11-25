# syntax=docker/dockerfile:1

# --- Asset build stage (Node, build-time only) ---
# Builds every frontend asset workspace under web/ into internal/api/assets,
# which go:embed serves. It runs the same scripts/build-assets.sh that
# `make assets` uses and discovers workspaces automatically, so adding a new
# asset never means editing this Dockerfile. The output is not committed to git;
# it is produced fresh on every image build. The runtime image carries only
# the built bytes from this stage; Node is not needed by the native Pi sidecar.
FROM node:22-bookworm-slim AS assets

# python3 + fonttools/brotli for the Phosphor subset step (web/icons/build.mjs)
# (av-diue: the icon font ships subset to the classes the repo uses) and the
# Fraunces instancing step (web/fonts/build.mjs, Exh-wjqh).
RUN apt-get update \
	&& apt-get install -y --no-install-recommends python3 python3-fonttools python3-brotli \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY web/ ./web/
COPY scripts/ ./scripts/
COPY internal/api/logo.svg ./internal/api/logo.svg

RUN sh scripts/build-assets.sh

# --- Go build stage ---
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Copy dependency manifests first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Overlay the freshly built (un-committed) frontend assets so go:embed finds
# them at compile time. Without these, the //go:embed assets pattern fails the
# build — the assets are never served stale.
COPY --from=assets /app/internal/api/assets/ ./internal/api/assets/

# Build both entrypoints from the same source revision. The optional MCP service
# starts with plain exhibit-mcp; the core server carries no MCP transport.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/exhibit-mcp ./cmd/exhibit-mcp

# --- Native Pi download stage ---
# Pi Bolt 1.3.1 contains Pi 1.6.0. Pin the version AND release-archive hashes;
# update them together after the real-Pi agent/API compatibility suites pass.
# amd64 uses the baseline build so the image does not require AVX2. Both builds
# have JIT off; Exhibit loads its unchanged TypeScript extensions at runtime.
FROM debian:bookworm-slim AS pi-bolt

ARG TARGETARCH
ARG PI_BOLT_VERSION=1.3.1

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl xz-utils \
	&& rm -rf /var/lib/apt/lists/*

RUN set -eu; \
	pi_bolt_arch="${TARGETARCH:-$(dpkg --print-architecture)}"; \
	case "$pi_bolt_arch" in \
		amd64) pi_bolt_archive=pi-bolt-linux-x64-baseline; \
			pi_bolt_sha256=a94d397e1ed4754fbf43c6867131c0d12411c2744633c3dfe1117c7f0a43f75d ;; \
		arm64) pi_bolt_archive=pi-bolt-linux-arm64; \
			pi_bolt_sha256=ddbf1d8c237cbb5d74f2f860dbbdf1ecba7878ed153070fe12f364e48f684660 ;; \
		*) echo "Unsupported Pi Bolt architecture: $pi_bolt_arch" >&2; exit 1 ;; \
	esac; \
	curl --fail --show-error --location --retry 3 \
		"https://github.com/opensec-git/Pi-Bolt/releases/download/bolt-v${PI_BOLT_VERSION}/${pi_bolt_archive}.tar.xz" \
		-o /tmp/pi-bolt.tar.xz; \
	echo "$pi_bolt_sha256  /tmp/pi-bolt.tar.xz" | sha256sum --check --strict; \
	mkdir -p /opt/pi-bolt; \
	tar -xJf /tmp/pi-bolt.tar.xz -C /opt/pi-bolt --strip-components=1; \
	rm /tmp/pi-bolt.tar.xz

# --- Runtime stage ---
# The server is static Go; Pi Bolt supplies the native agent plus its adjacent
# assets, native helpers and license notices. A glibc userland is required, but
# Node, npm and the download tools stay in the build stages.
FROM debian:bookworm-slim AS runtime

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

COPY --from=pi-bolt /opt/pi-bolt/ /opt/pi-bolt/

# Keep the server's default PI_BIN=pi and expose the upstream name for source
# installs and diagnostics. Do not run self-update inside a pinned image.
RUN ln -s /opt/pi-bolt/pi /usr/local/bin/pi \
	&& ln -s /opt/pi-bolt/pi /usr/local/bin/pi-bolt \
	&& pi --version

COPY --from=builder /bin/server /server
COPY --from=builder /bin/exhibit-mcp /exhibit-mcp

# Data volume for SQLite and blob storage
VOLUME ["/data"]

ENV DATA_DIR=/data
ENV ADDR=:8080
ENV RENDER_ADDR=:8081

EXPOSE 8080 8081 8090

ENTRYPOINT ["/server"]

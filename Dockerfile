# syntax=docker/dockerfile:1

# --- Asset build stage (Node, build-time only) ---
# Builds every frontend asset workspace under web/ into internal/api/assets,
# which go:embed serves. It runs the same scripts/build-assets.sh that
# `make assets` uses and discovers workspaces automatically, so adding a new
# asset never means editing this Dockerfile. The output is not committed to git;
# it is produced fresh on every image build. The runtime image below carries
# only the built bytes — no Node, no internet egress.
FROM node:22-bookworm-slim AS assets

WORKDIR /app

COPY web/ ./web/
COPY scripts/ ./scripts/

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

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/server ./cmd/server

# --- Runtime stage ---
FROM gcr.io/distroless/static-debian12

COPY --from=builder /bin/server /server

# Data volume for SQLite and blob storage
VOLUME ["/data"]

ENV DATA_DIR=/data
ENV ADDR=:8080
ENV RENDER_ADDR=:8081

EXPOSE 8080 8081

ENTRYPOINT ["/server"]

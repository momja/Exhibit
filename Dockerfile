# syntax=docker/dockerfile:1

# --- Asset build stage (Node, build-time only) ---
# Fetches/bundles the frontend assets that go:embed serves — the CodeMirror
# source-editor JS (web/editor) and the vendored Phosphor Icons CSS + webfont
# (web/icons). These are NOT committed to git; they are produced fresh here on
# every image build. The runtime image below carries none of Node — only the
# built bytes — so the running server still needs no Node and no internet egress.
FROM node:22-bookworm-slim AS assets

WORKDIR /app

# Copy only the two asset workspaces (keeps this stage's cache independent of
# Go source and templ template edits).
COPY web/editor/ ./web/editor/
COPY web/icons/ ./web/icons/

# Both build scripts write to ../../internal/api/assets/* (i.e.
# /app/internal/api/assets), which the Go stage below overlays before compiling.
RUN cd web/editor && npm ci && npm run build
RUN cd web/icons  && npm ci && npm run build

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

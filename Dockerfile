# syntax=docker/dockerfile:1

# --- Build stage ---
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Copy dependency manifests first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

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

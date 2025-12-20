# Tunnox Core Service Dockerfile
# Multi-stage build for minimal image size

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Stage 1: Build
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=1.0.0" \
    -o tunnox-server \
    ./cmd/server

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# Stage 2: Runtime
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/tunnox-server .

# Copy config template
COPY cmd/server/config/ ./config/

# Create data directory
RUN mkdir -p /app/data /app/logs

# Create non-root user
RUN adduser -D -u 1000 tunnox && \
    chown -R tunnox:tunnox /app

USER tunnox

# Expose ports
# 7000 - TCP client connections
# 8000 - WebSocket
# 9000 - Management API
EXPOSE 7000 8000 9000

# Health check via Management API
HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:9000/tunnox/v1/health || exit 1

# Run the server
ENTRYPOINT ["/app/tunnox-server"]
CMD ["-config", "/app/config/config.docker.yaml"]

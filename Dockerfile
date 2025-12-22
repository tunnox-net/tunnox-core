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

# Cache bust argument - 传入时间戳强制重新编译
ARG CACHEBUST=1

# Copy source code (CACHEBUST 会使这一步及之后的缓存失效)
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

# Copy example config (用户可以通过 ConfigMap 覆盖 /app/config.yaml)
COPY config.example.yaml ./config.example.yaml

# Create data directory
RUN mkdir -p /app/data /app/logs

# Create non-root user
RUN adduser -D -u 1000 tunnox && \
    chown -R tunnox:tunnox /app

USER tunnox

# Expose ports
# 8000 - TCP/KCP client connections
# 8443 - QUIC
# 9000 - Management API (WebSocket + HTTPPoll)
# 50052 - Cross-node TCP connections
EXPOSE 8000 8443 9000 50052

# Health check via Management API
HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:9000/tunnox/v1/health || exit 1

# Run the server
# 默认使用 /app/config.yaml
# 如果文件不存在，程序会自动生成简洁的配置模板
# 建议通过 ConfigMap 挂载配置文件到 /app/config.yaml
ENTRYPOINT ["/app/tunnox-server"]
CMD ["-config", "/app/config.yaml"]

# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=$(cat VERSION 2>/dev/null || echo 'dev') -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /proxy ./cmd/proxy

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' mcp
USER mcp

# Copy binary from builder
COPY --from=builder /proxy /usr/local/bin/proxy

# Copy default configuration and policies
COPY --chown=mcp:mcp config/ /etc/mcp/
COPY --chown=mcp:mcp policies/ /etc/mcp/policies/

# Expose ports
EXPOSE 3000 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the proxy
ENTRYPOINT ["/usr/local/bin/proxy"]
CMD ["--config", "/etc/mcp/proxy.yaml"]

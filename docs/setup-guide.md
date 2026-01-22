# MCP Proxy - Setup Guide

This guide covers installation, configuration, and deployment of the MCP Proxy.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Quick Start](#quick-start)
3. [Installation](#installation)
4. [Configuration](#configuration)
5. [Running the Proxy](#running-the-proxy)
6. [Health Checks & Monitoring](#health-checks--monitoring)
7. [Docker Deployment](#docker-deployment)
8. [Kubernetes Deployment](#kubernetes-deployment)
9. [Troubleshooting](#troubleshooting)

---

## Prerequisites

- **Go 1.21+** (for building from source)
- **SQLite3** (included, no separate installation needed)
- **Network access** to upstream MCP server (if not running standalone)

### Optional
- **Docker** (for containerized deployment)
- **Prometheus** (for metrics collection)
- **Grafana** (for metrics visualization)

---

## Quick Start

```bash
# Clone and build
git clone https://github.com/agentfacts/mcp-proxy.git
cd mcp-proxy
go build -o mcp-proxy ./cmd/proxy

# Run with default config
./mcp-proxy -config config/proxy.yaml
```

The proxy will start on:
- **SSE endpoint**: `http://localhost:3000`

Optional endpoints (disabled by default, enable in config):
- **Metrics**: `http://localhost:9090/metrics` (set `metrics.enabled: true`)
- **Health checks**: `http://localhost:8080/health` (set `health.enabled: true`)

---

## Installation

### From Source

```bash
# Clone repository
git clone https://github.com/agentfacts/mcp-proxy.git
cd mcp-proxy

# Install dependencies
go mod download

# Build binary
go build -o mcp-proxy ./cmd/proxy

# Run tests
go test ./...
```

### Pre-built Binary

Download from releases page:
```bash
curl -LO https://github.com/agentfacts/mcp-proxy/releases/latest/download/mcp-proxy-linux-amd64
chmod +x mcp-proxy-linux-amd64
mv mcp-proxy-linux-amd64 /usr/local/bin/mcp-proxy
```

---

## Configuration

### Configuration File

Create `config/proxy.yaml`:

```yaml
version: "1.0"

server:
  listen:
    address: "0.0.0.0"
    port: 3000
  transport: "sse"
  max_connections: 1000
  read_timeout: 30s
  write_timeout: 30s
  graceful_shutdown: 30s
  security:
    cors_allowed_origins: []  # Empty = same-origin only (secure)
    enable_security_headers: true

upstream:
  url: "http://mcp-server:8080"
  transport: "sse"
  timeout: 30s
  retry:
    enabled: true
    max_attempts: 3
    initial_delay: 100ms
    max_delay: 5s

agent:
  id: "default-agent"
  name: "Default Agent"
  capabilities:
    - "read:*"
    - "write:documents"

policy:
  enabled: true
  mode: "enforce"  # or "audit"
  policy_dir: "policies"
  data_file: "config/policy_data.json"
  environment: "production"

audit:
  enabled: true
  db_path: "audit.db"
  buffer_size: 100
  flush_interval: 1s
  retention_days: 30
  capture:
    request_arguments: true
    response_summary: false

metrics:
  enabled: false  # Disabled by default, set to true to enable
  address: "0.0.0.0"
  port: 9090
  path: "/metrics"

health:
  enabled: false  # Disabled by default, set to true to enable
  address: "0.0.0.0"
  port: 8080
  liveness_path: "/health"
  readiness_path: "/ready"

logging:
  level: "info"
  format: "json"
  output: "stdout"

tls:
  enabled: false
  cert_file: ""
  key_file: ""
```

### Environment Variables

All configuration can be overridden with environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `MCP_SERVER_PORT` | Server listen port | `3000` |
| `MCP_SERVER_ADDRESS` | Server listen address | `0.0.0.0` |
| `MCP_UPSTREAM_URL` | Upstream MCP server URL | `http://mcp:8080` |
| `MCP_AGENT_ID` | Default agent ID | `my-agent` |
| `MCP_AGENT_CAPABILITIES` | Comma-separated capabilities | `read:*,write:docs` |
| `MCP_POLICY_MODE` | Policy mode | `enforce` or `audit` |
| `MCP_AUDIT_ENABLED` | Enable audit logging | `true` |
| `MCP_METRICS_ENABLED` | Enable Prometheus metrics | `true` |
| `MCP_LOGGING_LEVEL` | Log level | `debug`, `info`, `warn`, `error` |

### Policy Configuration

Create `config/policy_data.json`:

```json
{
  "tool_capabilities": {
    "read_file": "read:files",
    "write_file": "write:files",
    "delete_file": "admin:files"
  },
  "rate_limits": {
    "default": 1000,
    "premium-agent": 5000
  },
  "blocked_tools": [
    "execute_command",
    "shell_exec",
    "system_shutdown"
  ],
  "blocked_agents": [],
  "identity_required_tools": [
    "payment_refund",
    "admin_panel"
  ],
  "pii_tools": [
    "customer_lookup"
  ]
}
```

---

## Running the Proxy

### Basic Usage

```bash
# With config file
./mcp-proxy -config config/proxy.yaml

# Show version
./mcp-proxy -version
```

### Standalone Mode (No Upstream)

The proxy can run without an upstream MCP server for testing:

```bash
# No upstream configured - proxy echoes messages back
MCP_UPSTREAM_URL="" ./mcp-proxy -config config/proxy.yaml
```

### Development Mode

```bash
# Enable debug logging and audit mode
MCP_LOGGING_LEVEL=debug MCP_POLICY_MODE=audit ./mcp-proxy -config config/proxy.yaml
```

---

## Health Checks & Monitoring

### Health Endpoints

| Endpoint | Description | Use Case |
|----------|-------------|----------|
| `GET /health` | Liveness probe | K8s liveness probe |
| `GET /ready` | Readiness probe | K8s readiness probe |
| `GET /health/full` | Detailed health | Debugging |

Example responses:

```bash
# Liveness (always returns healthy if process is running)
curl http://localhost:8080/health
{"status":"healthy","timestamp":"2024-01-15T10:00:00Z","version":"0.1.0"}

# Readiness (checks all components)
curl http://localhost:8080/ready
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:00:00Z",
  "version": "0.1.0",
  "components": {
    "policy_engine": {"status": "healthy", "message": "ready"},
    "audit_store": {"status": "healthy", "message": "connected"},
    "upstream": {"status": "degraded", "message": "disconnected"}
  }
}
```

### Prometheus Metrics

```bash
curl http://localhost:9090/metrics
```

Key metrics:
- `mcp_proxy_requests_total` - Total requests by method, tool, allowed
- `mcp_proxy_policy_decisions_total` - Policy decisions by rule, mode
- `mcp_proxy_request_duration_seconds` - Request latency histogram
- `mcp_proxy_active_sessions` - Current active sessions

### Grafana Dashboard

Import the dashboard from `dashboards/mcp-proxy.json` into Grafana.

---

## Docker Deployment

### Dockerfile

```dockerfile
# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /proxy ./cmd/proxy

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' mcp
USER mcp

COPY --from=builder /proxy /usr/local/bin/proxy
COPY --chown=mcp:mcp config/ /etc/mcp/
COPY --chown=mcp:mcp policies/ /etc/mcp/policies/

EXPOSE 3000 8080 9090

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/usr/local/bin/proxy"]
CMD ["--config", "/etc/mcp/proxy.yaml"]
```

### Build and Run

```bash
# Build image
docker build -t mcp-proxy:latest .

# Run container
docker run -d \
  --name mcp-proxy \
  -p 3000:3000 \
  -p 8080:8080 \
  -p 9090:9090 \
  -e MCP_UPSTREAM_URL=http://mcp-server:8080 \
  -v $(pwd)/config:/etc/mcp:ro \
  -v $(pwd)/policies:/etc/mcp/policies:ro \
  mcp-proxy:latest
```

### Docker Compose

```yaml
version: '3.8'
services:
  mcp-proxy:
    build: .
    ports:
      - "3000:3000"
      - "8080:8080"
      - "9090:9090"
    environment:
      - MCP_UPSTREAM_URL=http://mcp-server:8080
      - MCP_LOGGING_LEVEL=info
    volumes:
      - ./config:/etc/mcp:ro
      - ./policies:/etc/mcp/policies:ro
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 3s
      start_period: 5s
      retries: 3
```

---

## Kubernetes Deployment

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mcp-proxy
  labels:
    app: mcp-proxy
spec:
  replicas: 3
  selector:
    matchLabels:
      app: mcp-proxy
  template:
    metadata:
      labels:
        app: mcp-proxy
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"
    spec:
      containers:
      - name: mcp-proxy
        image: ghcr.io/agentfacts/mcp-proxy:latest
        ports:
        - containerPort: 3000
          name: sse
        - containerPort: 8080
          name: health
        - containerPort: 9090
          name: metrics
        env:
        - name: MCP_UPSTREAM_URL
          value: "http://mcp-server:8080"
        - name: MCP_POLICY_MODE
          value: "enforce"
        livenessProbe:
          httpGet:
            path: /health
            port: health
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: health
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
        volumeMounts:
        - name: config
          mountPath: /etc/mcp
          readOnly: true
        - name: policies
          mountPath: /etc/mcp/policies
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: mcp-proxy-config
      - name: policies
        configMap:
          name: mcp-proxy-policies
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: mcp-proxy
spec:
  selector:
    app: mcp-proxy
  ports:
  - name: sse
    port: 3000
    targetPort: 3000
  - name: health
    port: 8080
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
```

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mcp-proxy-config
data:
  proxy.yaml: |
    version: "1.0"
    server:
      listen:
        address: "0.0.0.0"
        port: 3000
    # ... rest of config
```

---

## Troubleshooting

### Common Issues

#### Proxy won't start

```bash
# Check configuration
./mcp-proxy -config config/proxy.yaml 2>&1 | head -20

# Common issues:
# - Invalid YAML syntax
# - Missing required fields
# - Port already in use
```

#### Upstream connection fails

```bash
# Check upstream is reachable
curl -v http://mcp-server:8080

# Check proxy logs for connection errors
./mcp-proxy -config config/proxy.yaml 2>&1 | grep -i upstream
```

#### Policy denying requests

```bash
# Switch to audit mode to see decisions without blocking
MCP_POLICY_MODE=audit ./mcp-proxy -config config/proxy.yaml

# Check audit log for policy decisions
sqlite3 audit.db "SELECT method, tool, allowed, matched_rule, violations FROM audit_log ORDER BY id DESC LIMIT 10;"
```

#### High memory usage

```bash
# Check active sessions
curl http://localhost:8080/health/full | jq .

# Reduce max connections
MCP_SERVER_MAX_CONNECTIONS=100 ./mcp-proxy -config config/proxy.yaml
```

### Debug Mode

```bash
# Enable full debug logging
MCP_LOGGING_LEVEL=debug MCP_LOGGING_FORMAT=text ./mcp-proxy -config config/proxy.yaml
```

### Audit Log Queries

```bash
# Recent denied requests
sqlite3 audit.db "SELECT timestamp, agent_id, method, tool, violations FROM audit_log WHERE allowed=0 ORDER BY timestamp DESC LIMIT 10;"

# Requests by agent
sqlite3 audit.db "SELECT agent_id, COUNT(*) as count FROM audit_log GROUP BY agent_id;"

# Average latency by method
sqlite3 audit.db "SELECT method, AVG(latency_ms) as avg_latency FROM audit_log GROUP BY method;"
```

---

## Next Steps

- [README](../README.md) - Project overview and quick start
- [Contributing](../CONTRIBUTING.md) - How to contribute
- [policies/](../policies/) - Example OPA policies

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial project structure

## [0.1.0] - 2025-01-22

### Added
- **Core Proxy Functionality**
  - SSE (Server-Sent Events) transport layer
  - Message routing with JSON-RPC 2.0 support
  - Session management with TTL and cleanup

- **Policy Engine**
  - OPA (Open Policy Agent) integration
  - Rego policy support
  - Audit and enforce modes
  - Policy decision caching

- **Audit Logging**
  - SQLite-based audit store
  - Buffered async writes
  - Configurable retention
  - Request/response capture options

- **Observability**
  - Prometheus metrics endpoint
  - Health check endpoints (liveness/readiness)
  - Structured logging with zerolog

- **Configuration**
  - YAML configuration files
  - Environment variable overrides
  - Sensible defaults

- **Deployment**
  - Docker support with multi-stage builds
  - Docker Compose for development
  - Kubernetes-ready health checks

### Security
- TLS support for encrypted communications
- Non-root container user
- Policy-based access control

---

## Version History

- `0.1.0` - Initial release with core functionality

[Unreleased]: https://github.com/agentfacts/mcp-proxy/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/agentfacts/mcp-proxy/releases/tag/v0.1.0

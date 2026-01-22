# Contributing to MCP Proxy

Thank you for your interest in contributing to MCP Proxy! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [How to Contribute](#how-to-contribute)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Documentation](#documentation)

## Code of Conduct

This project adheres to a [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to the maintainers.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/mcp-proxy.git
   cd mcp-proxy
   ```
3. **Add the upstream remote**:
   ```bash
   git remote add upstream https://github.com/agentfacts/mcp-proxy.git
   ```

## Development Setup

### Prerequisites

- Go 1.21 or later
- Make
- Docker and Docker Compose (for integration tests)
- OPA (Open Policy Agent) for policy testing

### Building

```bash
# Install dependencies
go mod download

# Build the binary
make build

# Run tests
make test

# Run linter
make lint
```

### Running Locally

```bash
# Start supporting services (PostgreSQL, Redis)
make docker-up

# Run the proxy
./bin/mcp-proxy --config config/proxy.yaml
```

## How to Contribute

### Reporting Bugs

Before creating a bug report, please check existing issues to avoid duplicates. When creating a bug report, include:

- **Clear title** describing the issue
- **Steps to reproduce** the problem
- **Expected behavior** vs **actual behavior**
- **Environment details** (OS, Go version, etc.)
- **Logs and error messages** if applicable

### Suggesting Features

Feature requests are welcome! Please:

- Check existing issues and discussions first
- Clearly describe the use case
- Explain why this feature would benefit other users
- Consider if you'd be willing to implement it

### Contributing Code

1. **Find an issue** to work on, or create one for discussion
2. **Comment on the issue** to let others know you're working on it
3. **Create a branch** for your changes:
   ```bash
   git checkout -b feature/your-feature-name
   ```
4. **Make your changes** following our coding standards
5. **Write tests** for new functionality
6. **Update documentation** as needed
7. **Submit a pull request**

## Pull Request Process

1. **Update your fork** with the latest upstream changes:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Ensure all tests pass**:
   ```bash
   make test
   make lint
   ```

3. **Write a clear PR description** including:
   - What changes were made
   - Why the changes were made
   - How to test the changes
   - Related issue numbers

4. **Request review** from maintainers

5. **Address feedback** promptly and push updates

6. **Squash commits** if requested (we prefer clean commit history)

### PR Requirements

- [ ] Tests pass (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] Documentation updated (if applicable)
- [ ] Commit messages are clear and descriptive
- [ ] PR description explains the changes

## Coding Standards

### Go Code Style

- Follow the official [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Run `gofmt` and `goimports` on all code
- Use meaningful variable and function names
- Keep functions focused and small
- Write godoc comments for exported types and functions

### Commit Messages

Follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

```
type(scope): description

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Formatting, missing semicolons, etc.
- `refactor`: Code change that neither fixes a bug nor adds a feature
- `test`: Adding missing tests
- `chore`: Maintenance tasks

Examples:
```
feat(policy): add support for rate limiting rules
fix(sse): handle connection timeout gracefully
docs(readme): update installation instructions
```

### Code Organization

```
mcp-proxy/
├── cmd/proxy/          # Application entry point
├── internal/           # Private application code
│   ├── audit/          # Audit logging
│   ├── config/         # Configuration
│   ├── observability/  # Metrics and health
│   ├── policy/         # OPA policy engine
│   ├── router/         # Message routing
│   ├── session/        # Session management
│   ├── transport/      # Transport implementations
│   └── upstream/       # Upstream connection
├── policies/           # Rego policy files
├── config/             # Configuration files
└── docs/               # Documentation
```

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run specific package tests
go test -v ./internal/router/...

# Run benchmarks
go test -bench=. -benchmem ./internal/router/...
```

### Writing Tests

- Write table-driven tests where appropriate
- Use meaningful test names that describe the scenario
- Include both positive and negative test cases
- Mock external dependencies
- Aim for high coverage on critical paths

Example:
```go
func TestRouter_Route(t *testing.T) {
    tests := []struct {
        name    string
        input   []byte
        want    []byte
        wantErr bool
    }{
        {
            name:    "valid tools/call request",
            input:   []byte(`{"method": "tools/call", "params": {...}}`),
            want:    []byte(`{"result": ...}`),
            wantErr: false,
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

## Documentation

- Update README.md for user-facing changes
- Add godoc comments for new exported APIs
- Update configuration documentation for new options
- Include examples for new features

## Questions?

If you have questions about contributing, feel free to:

- Open a [Discussion](https://github.com/agentfacts/mcp-proxy/discussions)
- Ask in an existing issue
- Reach out to maintainers

Thank you for contributing to MCP Proxy!

.PHONY: build run test lint clean docker-build docker-up docker-down help

# Build variables
BINARY_NAME=mcp-proxy
VERSION?=0.1.0
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Default target
all: build

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/proxy

## build-linux: Build for Linux (for Docker)
build-linux:
	@echo "Building $(BINARY_NAME) for Linux..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux ./cmd/proxy

## run: Run the proxy locally
run: build
	@echo "Running $(BINARY_NAME)..."
	./bin/$(BINARY_NAME) --config config/proxy.yaml

## run-dev: Run with hot reload (requires air)
run-dev:
	@which air > /dev/null || (echo "Installing air..." && go install github.com/air-verse/air@latest)
	air -c .air.toml

## test: Run all tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## test-policy: Run OPA policy tests
test-policy:
	@echo "Running policy tests..."
	@which opa > /dev/null || (echo "OPA not found. Install from https://www.openpolicyagent.org/docs/latest/#1-download-opa" && exit 1)
	opa test policies/ -v

## lint: Run linter
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

## tidy: Tidy go modules
tidy:
	@echo "Tidying modules..."
	$(GOMOD) tidy

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t mcp/proxy:$(VERSION) -t mcp/proxy:latest .

## docker-up: Start development stack
docker-up:
	@echo "Starting development stack..."
	docker-compose up -d

## docker-down: Stop development stack
docker-down:
	@echo "Stopping development stack..."
	docker-compose down

## docker-logs: View development stack logs
docker-logs:
	docker-compose logs -f

## db-migrate: Run database migrations
db-migrate:
	@echo "Running database migrations..."
	psql "$(SOTH_AUDIT_POSTGRES_DSN)" -f scripts/schema.sql

## db-reset: Reset database (WARNING: destroys data)
db-reset:
	@echo "Resetting database..."
	docker-compose exec postgres psql -U soth -d soth -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	$(MAKE) db-migrate

## proto: Generate protobuf code (if needed)
proto:
	@echo "Generating protobuf code..."
	# Add protoc commands here if needed

## help: Show this help
help:
	@echo "MCP Proxy - Build Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/  /'

.PHONY: help build test lint fmt clean run-api run-worker migrate-up migrate-down infra-up infra-down docker-build

# Default target
help:
	@echo "Available targets:"
	@echo "  make build         - Build all binaries"
	@echo "  make test          - Run all tests"
	@echo "  make test-cover    - Run tests with coverage"
	@echo "  make test-race     - Run tests with race detector"
	@echo "  make lint          - Run linter"
	@echo "  make fmt           - Format code"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make run-api       - Run API server"
	@echo "  make run-worker    - Run outbox worker"
	@echo "  make migrate-up    - Run database migrations up"
	@echo "  make migrate-down  - Run database migrations down"
	@echo "  make infra-up      - Start local infrastructure (Docker)"
	@echo "  make infra-down    - Stop local infrastructure"
	@echo "  make docker-build  - Build Docker images"

# Variables
BINARY_DIR := bin
API_BINARY := $(BINARY_DIR)/api
WORKER_BINARY := $(BINARY_DIR)/worker
GO_FILES := $(shell find . -name '*.go' -not -path './vendor/*')

# Database
DATABASE_URL ?= postgres://payflow:payflow@localhost:5432/payflow?sslmode=disable
MIGRATIONS_DIR := migrations

# Build
build: build-api build-worker

build-api:
	@echo "Building API server..."
	@mkdir -p $(BINARY_DIR)
	@go build -o $(API_BINARY) ./cmd/api

build-worker:
	@echo "Building worker..."
	@mkdir -p $(BINARY_DIR)
	@go build -o $(WORKER_BINARY) ./cmd/worker

# Test
test:
	@echo "Running unit tests..."
	@go test -v -short -timeout 30s ./...

test-unit:
	@echo "Running unit tests..."
	@go test -v -short -timeout 30s ./...

test-integration:
	@echo "Running integration tests..."
	@echo "Note: Requires PostgreSQL running on localhost:5432"
	@go test -v -race -tags=integration -timeout 5m ./internal/payments -run Integration
	@go test -v -race -tags=integration -timeout 5m ./internal/ledger -run Integration
	@go test -v -race -tags=integration -timeout 5m ./internal -run Integration

test-all:
	@echo "Running all tests (unit + integration)..."
	@echo "Note: Requires PostgreSQL running on localhost:5432"
	@go test -v -race -tags=integration -timeout 5m ./...

test-cover:
	@echo "Running tests with coverage..."
	@go test -v -race -coverprofile=coverage.out -covermode=atomic -tags=integration ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-race:
	@echo "Running tests with race detector..."
	@go test -race -timeout 60s ./...

test-quick:
	@echo "Running quick unit tests (no race detector)..."
	@go test -short -timeout 15s ./...

# Lint and format
lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin"; \
	fi

fmt:
	@echo "Formatting code..."
	@gofmt -s -w $(GO_FILES)
	@go mod tidy

# Clean
clean:
	@echo "Cleaning..."
	@rm -rf $(BINARY_DIR)
	@rm -f coverage.out coverage.html
	@go clean

# Run services
run-api: build-api
	@echo "Starting API server..."
	@$(API_BINARY)

run-worker: build-worker
	@echo "Starting worker..."
	@$(WORKER_BINARY)

# Database migrations
migrate-up:
	@echo "Running migrations up..."
	@if command -v migrate > /dev/null; then \
		migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up; \
	else \
		echo "migrate not installed. Install with: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"; \
	fi

migrate-down:
	@echo "Running migrations down..."
	@if command -v migrate > /dev/null; then \
		migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down; \
	else \
		echo "migrate not installed. Install with: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"; \
	fi

migrate-create:
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make migrate-create NAME=<migration_name>"; \
		exit 1; \
	fi
	@echo "Creating migration: $(NAME)"
	@if command -v migrate > /dev/null; then \
		migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(NAME); \
	else \
		echo "migrate not installed. Install with: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"; \
	fi

# Infrastructure
infra-up:
	@echo "Starting local infrastructure..."
	@docker-compose up -d
	@echo "Waiting for services to be ready..."
	@sleep 5

infra-down:
	@echo "Stopping local infrastructure..."
	@docker-compose down

infra-logs:
	@docker-compose logs -f

# Docker
docker-build:
	@echo "Building Docker images..."
	@docker build -t playflow-api:latest -f docker/Dockerfile.api .
	@docker build -t playflow-worker:latest -f docker/Dockerfile.worker .

# Development workflow
dev-setup: infra-up
	@echo "Setting up development environment..."
	@sleep 5
	@make migrate-up
	@echo "Development environment ready!"

dev-reset: infra-down clean
	@echo "Resetting development environment..."
	@rm -rf tmp/
	@make infra-up
	@sleep 5
	@make migrate-up

# Dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod verify

# Install tools
install-tools:
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@echo "Tools installed!"

# All-in-one
all: clean deps lint test build

# Watch (requires entr)
watch-test:
	@echo "Watching for changes and running tests..."
	@find . -name '*.go' | entr -c make test

watch-api:
	@echo "Watching for changes and restarting API..."
	@find . -name '*.go' | entr -r make run-api

# Makefile for Link Project

.PHONY: help test test-unit test-integration test-all test-coverage clean build run deps lint fmt grpc-gen

# Variables
GO := go
INTEGRATION_TAG := integration

# Default target
help:
	@echo "Available targets:"
	@echo "  make test           - Run unit tests only"
	@echo "  make test-integration - Run integration tests (requires INTEGRATION_TEST=1)"
	@echo "  make test-all       - Run all tests including integration"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make build          - Build the application"
	@echo "  make run            - Run the application"
	@echo "  make deps           - Install dependencies"
	@echo "  make lint           - Run linter"
	@echo "  make fmt            - Format code"

# Run unit tests only (no external services needed)
test-unit:
	@echo "Running unit tests..."
	$(GO) test -v -short ./...

# Run integration tests (uses existing .env configuration)
test-integration:
	@echo "Running integration tests..."
	@echo "Note: Ensure your .env is configured with database connection"
	@echo "Set INTEGRATION_TEST=1 to enable integration tests"
	INTEGRATION_TEST=1 $(GO) test -v -tags=$(INTEGRATION_TAG) ./internal/application/...

# Run all tests
test-all:
	@echo "Running all tests..."
	$(GO) test -v ./...

# Run integration tests with coverage
test-integration-coverage:
	@echo "Running integration tests with coverage..."
	INTEGRATION_TEST=1 $(GO) test -v -tags=$(INTEGRATION_TAG) -coverprofile=integration_coverage.out ./internal/application/...
	$(GO) tool cover -html=integration_coverage.out -o integration_coverage.html
	@echo "Integration coverage report generated: integration_coverage.html"

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	$(GO) clean
	rm -f coverage.out coverage.html integration_coverage.out integration_coverage.html

# Build the application
build:
	@echo "Building application..."
	$(GO) build -o bin/link ./cmd/server

# Run the application
run:
	@echo "Running application..."
	$(GO) run ./cmd/server/main.go

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run ./... 2>/dev/null || echo "golangci-lint not installed, skipping"

# Run formatter
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

# Generate wire code
wire:
	@echo "Generating wire code..."
	cd cmd/wire && wire gen

# Quick test for development (runs fast unit tests)
test-quick:
	@echo "Running quick unit tests..."
	$(GO) test -v -short -race ./...

# Benchmark tests
bench:
	@echo "Running benchmark tests..."
	$(GO) test -bench=. -benchmem ./...

# Integration benchmarks (require .env configured)
bench-integration:
	@echo "Running integration benchmarks..."
	INTEGRATION_TEST=1 $(GO) test -v -tags=$(INTEGRATION_TAG) -bench=. -benchmem ./internal/application/...

# Watch tests during development
test-watch:
	@which reflex > /dev/null || (echo "Installing reflex..." && go install github.com/cespare/reflex@latest)
	reflex -r '\.go$$' -s -- sh -c 'clear && make test-quick'

# CI pipeline target
ci: deps fmt lint test-quick
	@echo "CI pipeline completed successfully!"

# Generate gRPC code from proto files
grpc-gen:
	@echo "Generating gRPC code..."
	@which protoc > /dev/null || (echo "protoc not installed. Please install protoc." && exit 1)
	@which protoc-gen-go > /dev/null || (echo "protoc-gen-go not installed. Run: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest" && exit 1)
	@which protoc-gen-go-grpc > /dev/null || (echo "protoc-gen-go-grpc not installed. Run: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest" && exit 1)
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       api/proto/*.proto
	@echo "gRPC code generation complete!"

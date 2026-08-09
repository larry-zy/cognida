# Makefile for Cognida Project

.PHONY: help test test-unit test-integration test-all test-coverage clean build run deps lint fmt grpc-gen proto proto-check

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
	$(GO) build -o bin/cognida ./cmd/server

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
	cd internal/wire && wire gen

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

# 从单一源生成 protobuf/gRPC 代码（架构加固 Phase 4，任务 4.4）
# 唯一权威 .proto 在仓库根 proto/；buf 按 buf.gen.yaml 回生成到 services/cognida-go/api/proto 与
# services/cognida-python/proto 既有路径，消费方 import 零改动。需先装 buf：brew install bufbuild/buf/buf
proto:
	@echo "Generating protobuf/gRPC stubs via buf (single source: ../../proto)..."
	@which buf > /dev/null || (echo "buf not installed. Run: brew install bufbuild/buf/buf" && exit 1)
	cd ../.. && buf lint && buf generate
	@echo "Proto generation complete!"

# CI 一致性门（架构加固 Phase 4，任务 4.8）：回生成后两处生成目录须与提交一致，
# 否则说明 .proto 改了却没重新生成/提交。
proto-check:
	@echo "Checking proto codegen is up to date..."
	@which buf > /dev/null || (echo "buf not installed. Run: brew install bufbuild/buf/buf" && exit 1)
	cd ../.. && buf generate && git diff --exit-code -- services/cognida-go/api/proto services/cognida-python/proto \
		|| (echo "❌ 生成物与 proto/ 不一致，请运行 make proto 后提交" && exit 1)
	@echo "✅ Proto codegen 一致"

# 兼容旧入口：原 protoc 直连 api/proto 手抄源已废弃（Phase 4 单一源收敛），委托给 buf。
grpc-gen: proto

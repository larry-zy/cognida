# Integration Testing Guide

This guide explains how to run integration tests with real data instead of mock data.

## Overview

Integration tests use your existing `.env` configuration - no separate containers needed! They connect to your development/production database and services.

## Prerequisites

Just configure your `.env` file normally - integration tests will use the same configuration as your application.

### Required Services

Only MySQL is **required** for most integration tests. Other services are optional:

- **MySQL** (required) - Database for integration tests
- **Redis** (optional) - Caching service tests will skip if not configured
- **Neo4j** (optional) - Graph database tests will skip if not configured
- **LLM/Embedding** (optional) - AI service tests will skip if API keys not provided

## Configuration

Integration tests read from your existing `.env` file:

```bash
# .env file - same as your application

# Database (Required)
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=your_password
DB_NAME=link_go

# Redis (Optional - tests skip if not configured)
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=

# Neo4j (Optional - tests skip if not configured)
NEO4J_URI=bolt://localhost:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=your_password

# Embedding (Optional - for RAG tests)
EMBEDDING_PROVIDER=dashscope
EMBEDDING_API_KEY=your_api_key
EMBEDDING_MODEL=text-embedding-v3

# LLM (Optional - for chat/evaluation tests)
CHAT_PROVIDER=openai
CHAT_API_KEY=your_api_key
CHAT_MODEL_NAME=gpt-3.5-turbo
CHAT_BASE_URL=https://api.openai.com/v1
```

## Running Tests

### Unit Tests (No external services needed)

```bash
# Run unit tests with mocks
go test -v ./internal/application/rag/
go test -v ./internal/application/evaluation/
```

### Integration Tests

```bash
# Run integration tests (uses your .env config)
INTEGRATION_TEST=1 go test -v -tags=integration ./...

# Or use Makefile
make test-integration
```

### Run Specific Package Tests

```bash
# RAG service integration tests
INTEGRATION_TEST=1 go test -v -tags=integration ./internal/application/rag/

# Evaluation service integration tests
INTEGRATION_TEST=1 go test -v -tags=integration ./internal/application/evaluation/
```

### Run with Coverage

```bash
make test-integration-coverage
```

## Test Data Management

Integration tests automatically clean up their data:

- Test data uses identifiable prefixes: `test-kb-*`, `test-chunk-*`, `test-*`
- After each test, data with these prefixes is deleted
- Your real data remains untouched

## Test Structure

### Integration Test Files

Integration test files use the `integration` build tag. Test utilities are defined within each package's test files:

```go
// +build integration

package mypackage

import (
    "testing"
    "github.com/stretchr/testify/require"
)

func TestRealDatabaseOperation(t *testing.T) {
    SkipIfNotIntegrationTest(t)

    suite := SetupSuite(t)
    defer suite.TeardownSuite(t)

    // Use real database from .env
    result := suite.DB.Create(&TestData{...})
    // ... assertions

    // Cleanup only test data
    suite.CleanupDatabase(t)
}
```

## Writing New Integration Tests

1. **Create test file** with `_integration_test.go` suffix
2. **Add build tag**: `// +build integration`
3. **Define test setup helpers** in your package or copy from existing packages
4. **Cleanup test data** after tests

Example (see `internal/application/evaluation/testsetup.go` for reference):

```go
// +build integration

package mypackage

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestMyFeature(t *testing.T) {
    SkipIfNotIntegrationTest(t)

    suite := SetupSuite(t)
    defer suite.TeardownSuite(t)

    ctx := suite.Context()

    // Create test data with identifiable prefix
    testKB := &Knowledge{
        ID:   GetTestKBID(),  // "test-kb-1234567890"
        Name: "Test Knowledge Base",
    }
    suite.DB.Create(testKB)

    // Test your feature
    result, err := myService.DoSomething(ctx, testKB.ID)
    assert.NoError(t, err)

    // Cleanup only test data
    suite.CleanupDatabase(t)
}
```

## Troubleshooting

### Tests Are Skipped

If tests are being skipped, ensure:
1. `INTEGRATION_TEST=1` environment variable is set
2. Build tag `integration` is included: `go test -tags=integration`

### Database Connection Errors

1. Verify your `.env` has correct database settings
2. Ensure database is running and accessible
3. Check credentials and database name

### LLM/Embedding Tests Skipped

These tests are **optional** and will skip if:
- API keys are not configured in `.env`
- Services are unreachable

To run them, add API keys to your `.env`:
```bash
EMBEDDING_API_KEY=your_key_here
CHAT_API_KEY=your_key_here
```

## Difference: Unit vs Integration Tests

| Feature | Unit Tests | Integration Tests |
|---------|-----------|-------------------|
| Command | `go test ./...` | `INTEGRATION_TEST=1 go test -tags=integration ./...` |
| Speed | Fast (ms) | Slower (seconds) |
| Dependencies | Mocked | Real connections |
| External Services | None | Uses .env config |
| Data Isolation | In-memory | Database cleanup |

## Example Workflow

```bash
# 1. Start your development services (MySQL, Redis, etc.)
# Your existing docker-compose or local setup

# 2. Run unit tests during development
go test -v ./internal/application/...

# 3. Run integration tests to verify real functionality
INTEGRATION_TEST=1 go test -v -tags=integration ./internal/application/...

# 4. Check coverage
make test-integration-coverage
```

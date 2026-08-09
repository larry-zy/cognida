#!/bin/bash
# Go 测试脚本 - Cognida 项目
# 用法: ./go-test.sh [unit|integration|all|coverage]

set -e

MODE=${1:-all}
PROJECT_ROOT=$(cd "$(dirname "$0")/../../../../.." && pwd)
cd "$PROJECT_ROOT/services/cognida-go" || exit 1

echo "🧪 Cognida Go 测试 - Mode: $MODE"
echo "================================"

case $MODE in
  unit)
    echo "运行单元测试..."
    go test -short -v ./...
    ;;

  integration)
    echo "运行集成测试..."
    go test -tags=integration -v ./...
    ;;

  coverage)
    echo "生成覆盖率报告..."
    go test -coverprofile=coverage.out -covermode=atomic ./...
    go tool cover -html=coverage.out -o coverage.html
    echo "覆盖率报告: coverage.html"
    ;;

  all)
    echo "运行所有测试..."
    go test -v -race -coverprofile=coverage.out ./...
    echo ""
    echo "覆盖率统计:"
    go tool cover -func=coverage.out | tail -1
    ;;

  *)
    echo "用法: $0 [unit|integration|all|coverage]"
    exit 1
    ;;
esac

echo "✅ 测试完成"

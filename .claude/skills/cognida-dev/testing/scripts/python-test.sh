#!/bin/bash
# Python 测试脚本 - Cognida 项目
# 用法: ./python-test.sh [unit|integration|all|coverage]

set -e

MODE=${1:-all}
PROJECT_ROOT=$(cd "$(dirname "$0")/../../../../.." && pwd)
cd "$PROJECT_ROOT/services/cognida-python" || exit 1

echo "🧪 Cognida Python 测试 - Mode: $MODE"
echo "=================================="

case $MODE in
  unit)
    echo "运行单元测试..."
    pytest -v -m "not integration" tests/
    ;;

  integration)
    echo "运行集成测试..."
    pytest -v -m integration tests/
    ;;

  coverage)
    echo "生成覆盖率报告..."
    pytest --cov=services --cov-report=html --cov-report=term tests/
    echo "覆盖率报告: htmlcov/index.html"
    ;;

  all)
    echo "运行所有测试..."
    pytest -v --cov=services --cov-report=term tests/
    ;;

  *)
    echo "用法: $0 [unit|integration|all|coverage]"
    exit 1
    ;;
esac

echo "✅ 测试完成"

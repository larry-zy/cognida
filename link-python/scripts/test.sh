#!/bin/bash
# 测试运行脚本

cd "$(dirname "$0")/.."

echo "运行测试..."
uv run pytest "$@" --cov=src --cov-report=term-missing --cov-report=html

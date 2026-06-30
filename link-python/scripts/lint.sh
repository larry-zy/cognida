#!/bin/bash
# 代码质量检查脚本

cd "$(dirname "$0")/.."

set -e

echo "运行 ruff 检查..."
uv run ruff check .

echo "运行 ruff 格式检查..."
uv run ruff format --check .

echo "运行 mypy 类型检查..."
uv run mypy src/

echo "所有检查通过！"

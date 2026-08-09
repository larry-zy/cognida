#!/bin/bash
# 代码格式化脚本

cd "$(dirname "$0")/.."

echo "运行 ruff 格式化..."
uv run ruff format .

echo "修复 ruff 问题..."
uv run ruff check --fix .

echo "格式化完成！"

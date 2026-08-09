#!/bin/bash
# 开发服务器启动脚本

cd "$(dirname "$0")/.."

export APP_ENV=dev
export LOG_LEVEL=DEBUG

echo "启动开发服务器..."
uv run uvicorn src.main:app --host 0.0.0.0 --port 8000 --reload

#!/bin/bash
# Python 架构合规检查

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
FAILED=0

echo "🏗️  Python 架构合规检查..."

if [ ! -d "$PROJECT_ROOT/services/cognida-python" ]; then
    echo "⚠️  cognida-python 目录不存在，跳过"
    exit 0
fi

# 检查 grpc 层依赖 services（这是允许的，仅作占位说明，不校验）
# grpc 可以依赖 services，属正确方向，无需校验

# 检查 core 不依赖其他层
# 注意：用变量捕获结果判断，避免 `grep | head` 让 if 恒真（管道退出码取自 head）
core_dep=$(grep -rn 'from link.python.services\|from link.python.grpc' "$PROJECT_ROOT/services/cognida-python/core/" 2>/dev/null | \
    grep -v '_test.py' | grep -v '/.git/' | grep -v 'comment' || true)
if [ -n "$core_dep" ]; then
    echo "$core_dep" | head -5
    echo "❌ Core 不应依赖其他层 (grpc/services)"
    FAILED=1
fi

# 检查 services 不依赖 grpc
services_dep=$(grep -rn 'from link.python.grpc' "$PROJECT_ROOT/services/cognida-python/services/" 2>/dev/null | \
    grep -v '_test.py' | grep -v '/.git/' | grep -v 'comment' || true)
if [ -n "$services_dep" ]; then
    echo "$services_dep" | head -5
    echo "❌ Services 不应依赖 grpc 层"
    FAILED=1
fi

# 导入顺序检查（占位，暂不校验）

if [ $FAILED -eq 0 ]; then
    echo "✅ Python 架构合规检查通过"
fi

exit $FAILED

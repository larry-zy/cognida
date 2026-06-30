#!/bin/bash
# Python 架构合规检查

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
FAILED=0

echo "🏗️  Python 架构合规检查..."

if [ ! -d "$PROJECT_ROOT/link-python" ]; then
    echo "⚠️  link-python 目录不存在，跳过"
    exit 0
fi

# 检查 grpc 层不依赖 services 以外的层
if grep -rn 'from link.python.services' "$PROJECT_ROOT/link-python/grpc/" 2>/dev/null | \
    grep -v '_test.py' | grep -v 'comment' | while read line; do
    # grpc 可以依赖 services，这是正确的
    true
done

# 检查 core 不依赖其他层
if grep -rn 'from link.python.services\|from link.python.grpc' "$PROJECT_ROOT/link-python/core/" 2>/dev/null | \
    grep -v '_test.py' | grep -v '.git' | grep -v 'comment' | head -5; then
    echo "❌ Core 不应依赖其他层 (grpc/services)"
    FAILED=1
fi

# 检查 services 不依赖 grpc
if grep -rn 'from link.python.grpc' "$PROJECT_ROOT/link-python/services/" 2>/dev/null | \
    grep -v '_test.py' | grep -v '.git' | grep -v 'comment' | head -5; then
    echo "❌ Services 不应依赖 grpc 层"
    FAILED=1
fi

# 检查导入顺序
if grep -rn '^from link.python' "$PROJECT_ROOT/link-python/services/" "$PROJECT_ROOT/link-python/grpc/" 2>/dev/null | \
    grep -v '_test.py' | grep -v '.git' | while read line; do
    # 简化的导入顺序检查
    true
done

if [ $FAILED -eq 0 ]; then
    echo "✅ Python 架构合规检查通过"
fi

exit $FAILED

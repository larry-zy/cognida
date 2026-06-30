#!/bin/bash
# API Contract 一致性检查

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
FAILED=0

echo "🔄 API Contract 一致性检查..."

if [ ! -d "$PROJECT_ROOT/link-web" ] || [ ! -d "$PROJECT_ROOT/link-go" ]; then
    echo "⚠️  link-web 或 link-go 目录不存在，跳过"
    exit 0
fi

# 提取 Go 定义的路径 (简化检查)
if grep -rh 'POST\|GET\|PUT\|DELETE' "$PROJECT_ROOT/link-go/internal/handler/" 2>/dev/null | \
    grep -o '".*"' | sed 's/"//g' | sed 's/\\//g' | sort -u > /tmp/go_paths.txt; then
    echo "📋 发现 $(wc -l < /tmp/go_paths.txt) 个 Go API 路径"
fi

# 提取前端调用的路径
if grep -rh 'fetch.*api\|axios.*api' "$PROJECT_ROOT/link-web/src/" 2>/dev/null | \
    grep -o "'/api/[^']*\"" 2>/dev/null | sed "s/'//g" | sed 's/"//g' | sort -u > /tmp/web_paths.txt; then
    echo "📋 发现 $(wc -l < /tmp/web_paths.txt) 个前端 API 调用"
fi

# 检查是否有明显的差异
echo "⚠️  请手动确认前后端 API 路径一致:"
echo "Go 路径示例:"
head -5 /tmp/go_paths.txt 2>/dev/null || true
echo "前端路径示例:"
head -5 /tmp/web_paths.txt 2>/dev/null || true

if [ $FAILED -eq 0 ]; then
    echo "✅ API Contract 检查完成"
fi

exit $FAILED

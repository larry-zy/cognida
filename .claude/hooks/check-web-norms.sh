#!/bin/bash
# Web 前端规范检查

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
FAILED=0

echo "📖 Web 前端规范检查..."

if [ ! -d "$PROJECT_ROOT/link-web" ]; then
    echo "⚠️  link-web 目录不存在，跳过"
    exit 0
fi

# 检查是否有 TypeScript 类型定义
if grep -r 'props: any' "$PROJECT_ROOT/link-web/src/" 2>/dev/null | grep -v 'comment' | head -5; then
    echo "❌ 不应使用 any 类型，应定义具体类型"
    FAILED=1
fi

# 检查 useEffect 依赖
if grep -rn 'useEffect.*\[\]' "$PROJECT_ROOT/link-web/src/" 2>/dev/null | \
    grep -B2 'eslint-disable' | grep -v 'comment' | head -5; then
    echo "⚠️  useEffect 禁用了依赖检查，确保这是正确的"
fi

# 检查 dangerouslySetInnerHTML 使用
if grep -rn 'dangerouslySetInnerHTML' "$PROJECT_ROOT/link-web/src/" 2>/dev/null | \
    grep -v 'DOMPurify\|sanitize' | grep -v 'comment' | head -5; then
    echo "❌ dangerouslySetInnerHTML 应配合 DOMPurify 使用"
    FAILED=1
fi

if [ $FAILED -eq 0 ]; then
    echo "✅ Web 前端规范检查通过"
fi

exit $FAILED

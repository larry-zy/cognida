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
# 注意：用变量捕获匹配结果来判断，避免 `grep | head` 让 if 恒真（管道退出码取自 head，永远为 0）
any_hits=$(grep -rn 'props: any' "$PROJECT_ROOT/link-web/src/" 2>/dev/null | grep -v 'comment' || true)
if [ -n "$any_hits" ]; then
    echo "$any_hits" | head -5
    echo "❌ 不应使用 any 类型，应定义具体类型"
    FAILED=1
fi

# 检查 useEffect 依赖
useeffect_hits=$(grep -rn 'useEffect.*\[\]' "$PROJECT_ROOT/link-web/src/" 2>/dev/null | \
    grep 'eslint-disable' | grep -v 'comment' || true)
if [ -n "$useeffect_hits" ]; then
    echo "$useeffect_hits" | head -5
    echo "⚠️  useEffect 禁用了依赖检查，确保这是正确的"
fi

# 检查 dangerouslySetInnerHTML 使用
dsih_hits=$(grep -rn 'dangerouslySetInnerHTML' "$PROJECT_ROOT/link-web/src/" 2>/dev/null | \
    grep -v 'DOMPurify\|sanitize' | grep -v 'comment' || true)
if [ -n "$dsih_hits" ]; then
    echo "$dsih_hits" | head -5
    echo "❌ dangerouslySetInnerHTML 应配合 DOMPurify 使用"
    FAILED=1
fi

if [ $FAILED -eq 0 ]; then
    echo "✅ Web 前端规范检查通过"
fi

exit $FAILED

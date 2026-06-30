#!/bin/bash
# Python 语言规范检查

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
FAILED=0

echo "📖 Python 语言规范检查..."

# 检查是否有 Python 目录
if [ ! -d "$PROJECT_ROOT/link-python" ]; then
    echo "⚠️  link-python 目录不存在，跳过"
    exit 0
fi

# 检查类型注解
if grep -r 'def .*[)]:' "$PROJECT_ROOT/link-python/services/" "$PROJECT_ROOT/link-python/grpc/" 2>/dev/null | grep -v '_test.py' | grep -v '.git' | while read line; do
    func_name=$(echo "$line" | grep -o 'def [^(]*' | cut -d' ' -f2)
    if ! echo "$line" | grep -q '->'; then
        echo "⚠️  函数 $func_name 缺少返回类型注解"
    fi
done | head -10; then
    echo "⚠️  发现函数缺少类型注解"
fi

# 检查是否有裸 except
if grep -rn 'except:' "$PROJECT_ROOT/link-python/" 2>/dev/null | grep -v '_test.py' | grep -v '.git'; then
    echo "❌ 不应使用裸 except，应指定异常类型"
    FAILED=1
fi

# 检查 print vs logger
if grep -rn 'print(' "$PROJECT_ROOT/link-python/services/" "$PROJECT_ROOT/link-python/grpc/" 2>/dev/null | grep -v '_test.py' | grep -v '.git'; then
    echo "⚠️  应使用 logger 而非 print"
fi

if [ $FAILED -eq 0 ]; then
    echo "✅ Python 语言规范检查通过"
fi

exit $FAILED

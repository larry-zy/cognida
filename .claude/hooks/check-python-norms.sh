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

# 检查类型注解（仅限 .py，排除 .venv/.git/_test；`-- '->'` 避免 -> 被当作 grep 选项）
missing_rt=$(grep -rn --include='*.py' 'def .*[)]:' "$PROJECT_ROOT/link-python/services/" "$PROJECT_ROOT/link-python/grpc/" 2>/dev/null | \
    grep -v '_test.py' | grep -v '/.venv/' | grep -v '/.git/' | grep -v -- '->' || true)
if [ -n "$missing_rt" ]; then
    echo "$missing_rt" | while read -r line; do
        func_name=$(echo "$line" | grep -o 'def [^(]*' | cut -d' ' -f2)
        echo "⚠️  函数 $func_name 缺少返回类型注解"
    done | head -10
    echo "⚠️  发现函数缺少类型注解"
fi

# 检查是否有裸 except（排除第三方 .venv 与测试）
bare_except=$(grep -rn --include='*.py' 'except:' "$PROJECT_ROOT/link-python/" 2>/dev/null | \
    grep -v '_test.py' | grep -v '/.venv/' | grep -v '/.git/' || true)
if [ -n "$bare_except" ]; then
    echo "$bare_except"
    echo "❌ 不应使用裸 except，应指定异常类型"
    FAILED=1
fi

# 检查 print vs logger（仅限 .py，排除 .venv/.git/_test）
print_hits=$(grep -rn --include='*.py' 'print(' "$PROJECT_ROOT/link-python/services/" "$PROJECT_ROOT/link-python/grpc/" 2>/dev/null | \
    grep -v '_test.py' | grep -v '/.venv/' | grep -v '/.git/' || true)
if [ -n "$print_hits" ]; then
    echo "$print_hits"
    echo "⚠️  应使用 logger 而非 print"
fi

if [ $FAILED -eq 0 ]; then
    echo "✅ Python 语言规范检查通过"
fi

exit $FAILED

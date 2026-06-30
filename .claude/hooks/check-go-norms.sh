#!/bin/bash
# Go 语言规范检查

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
FAILED=0

echo "📖 Go 语言规范检查..."

# 检查包名 - 不应有下划线
if grep -r '^package.*_' "$PROJECT_ROOT/link-go/internal/" 2>/dev/null | grep -v '_test.go' | grep -v '.git'; then
    echo "❌ 包名不应包含下划线"
    FAILED=1
fi

# 检查接口命名 - 不应有 I 前缀 (如 IUserRepository、IService)。
# 用 'I[A-Z][a-z]' 匹配「I + 大写 + 小写」的匈牙利式前缀，避免误伤:
#   - 初始化缩写: IDGenerator (ID 是缩写, 符合 Go 规范)
#   - SQL/Cypher 中的 'entity_type IN ?' 等 (子串 'type I' + 'N')
if grep -rE 'type I[A-Z][a-z]' "$PROJECT_ROOT/link-go/internal/" 2>/dev/null | grep -v '_test.go' | grep -v '.git'; then
    echo "❌ 接口名不应使用 I 前缀 (如 IUserRepository)"
    FAILED=1
fi

# 检查 context.Context 是否为第一个参数。
# 仅当 context.Context 出现在逗号之后（即非首参）时告警，精确命中反模式，
# 避免误伤 gin.Context handler、以 c 命名的 receiver、名为 Context() 的方法等。
if grep -rnE 'func[^{]*,[[:space:]]*[a-zA-Z_]+[[:space:]]+context\.Context' "$PROJECT_ROOT/link-go/internal/" 2>/dev/null | grep -v '_test.go' | grep -v '.git'; then
    echo "⚠️  context.Context 应该是函数的第一个参数"
fi

# 检查错误是否有包装
if git -C "$PROJECT_ROOT" diff --cached 2>/dev/null | grep 'return.*err$' | grep -v 'return.*fmt.Errorf' | grep -v 'return.*errors.Wrap' | grep -v '^\s*//' | head -5; then
    echo "⚠️  错误应该使用 fmt.Errorf 包装"
fi

if [ $FAILED -eq 0 ]; then
    echo "✅ Go 语言规范检查通过"
fi

exit $FAILED

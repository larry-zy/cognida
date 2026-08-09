#!/bin/bash
# Go 架构合规检查
#
# 仅在“真正命中”时返回非 0：旧版以 `if grep ... | head -5; then` 判断，
# 管道退出码取自 head(恒为 0)，导致零命中也报失败(误报)。改为捕获输出后判空。
# 另修两处误报:
#   - 字符串常量/注释里出现的层级路径(如 InstrumentationName = "link/internal/service/agent")
#     被当成 import: 过滤掉含 '=' 或 '//' 的行, 只保留真实 import。
#   - model 同层互相引用(model -> model/common)被当成跨层依赖: 排除 link/internal/model。

set -uo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
FAILED=0

echo "🏗️  Go 架构合规检查..."

GO_INTERNAL="$PROJECT_ROOT/services/cognida-go/internal"
if [ ! -d "$GO_INTERNAL" ]; then
    echo "⚠️  services/cognida-go/internal 目录不存在，跳过"
    exit 0
fi

# 只保留真实 import 行: 排除含 '='(常量/字面量)、'//'(注释) 与测试文件
import_lines() { grep -v '_test.go' | grep -v '=' | grep -v '//' ; }

# 横切技术基础设施(配置/ID生成/可观测性)为各层允许的直接依赖, 不计入架构违规。
# 它们是纯技术关注点(configuration / id generation / tracing+metrics), 不承载业务,
# 与 logging 同类; 强行反转为端口收益甚微。其余 infrastructure 依赖仍按反转规则处理。
allow_crosscutting() { grep -Ev 'link/internal/infrastructure/(config|id|observability)([/"]|$)' ; }

# $1: 命中内容(为空则无违规)  $2: 提示信息
report() {
    if [ -n "$1" ]; then
        echo "$1"
        echo "$2"
        FAILED=1
    fi
}

# 1. handler 不应直接依赖 repository / infrastructure
hits=$(grep -rn --include='*.go' 'link/internal/repository\|link/internal/infrastructure' \
    "$GO_INTERNAL/handler/" 2>/dev/null | import_lines | allow_crosscutting | head -5)
report "$hits" "❌ Handler 不应直接依赖 Repository/Infrastructure"

# 2. service 不应直接依赖 infrastructure
hits=$(grep -rn --include='*.go' 'link/internal/infrastructure' \
    "$GO_INTERNAL/service/" 2>/dev/null | import_lines | allow_crosscutting | head -5)
report "$hits" "❌ Service 不应直接依赖 Infrastructure"

# 3. model 不应依赖其他层(同层 model 互相引用允许)
hits=$(grep -rn --include='*.go' 'link/internal/' \
    "$GO_INTERNAL/model/" 2>/dev/null | grep -v 'link/internal/model' | import_lines | head -5)
report "$hits" "❌ Model 不应依赖其他层"

# 4. repository / infrastructure 不应依赖 handler / service
hits=$(grep -rn --include='*.go' 'link/internal/handler\|link/internal/service' \
    "$GO_INTERNAL/repository/" "$GO_INTERNAL/infrastructure/" 2>/dev/null | import_lines | head -5)
report "$hits" "❌ Repository 不应依赖 Handler/Service"

if [ $FAILED -eq 0 ]; then
    echo "✅ Go 架构合规检查通过"
fi

exit $FAILED

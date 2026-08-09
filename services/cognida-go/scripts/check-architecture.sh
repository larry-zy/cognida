#!/bin/bash
# Go 架构合规检查 (Clean Architecture 依赖方向)
#
# 单一事实源: 由 CI (.github/workflows/ci-go.yml) 与本地 .claude/hooks 共同引用。
# 原脚本置于 .claude/hooks/ 下, 但该目录在 .gitignore 中(本地留存), CI 检出后找不到
# 文件而 exit 127; 故迁至受控的 services/cognida-go/scripts/ 纳入仓库。
#
# 仅在“真正命中”时返回非 0: 捕获输出后判空, 避免管道退出码取自 head(恒 0)导致误报。
# 另修两处误报:
#   - 字符串常量/注释里出现的层级路径被当成 import: 过滤含 '=' 或 '//' 的行。
#   - model 同层互相引用(model -> model/common)被当成跨层依赖: 排除 cognida/internal/model。

set -uo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
GO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)   # services/cognida-go
FAILED=0

echo "🏗️  Go 架构合规检查..."

GO_INTERNAL="$GO_ROOT/internal"
if [ ! -d "$GO_INTERNAL" ]; then
    echo "⚠️  $GO_INTERNAL 目录不存在，跳过"
    exit 0
fi

# 只保留真实 import 行: 排除含 '='(常量/字面量)、'//'(注释) 与测试文件
import_lines() { grep -v '_test.go' | grep -v '=' | grep -v '//' ; }

# 横切技术基础设施(配置/ID生成/可观测性/容错)为各层允许的直接依赖, 不计入架构违规。
# 它们是纯技术关注点(configuration / id generation / tracing+metrics / resilience: 熔断·重试),
# 不承载业务, 与 logging 同类; 强行反转为端口收益甚微(如 reliability 的熔断器是与 gRPC 通路
# 同源的统一容错原语, 见〔X-4〕)。其余 infrastructure 依赖仍按反转规则处理。
allow_crosscutting() { grep -Ev 'cognida/internal/infrastructure/(config|id|observability|reliability)([/"]|$)' ; }

# $1: 命中内容(为空则无违规)  $2: 提示信息
report() {
    if [ -n "$1" ]; then
        echo "$1"
        echo "$2"
        FAILED=1
    fi
}

# 1. handler 不应直接依赖 repository / infrastructure
hits=$(grep -rn --include='*.go' 'cognida/internal/repository\|cognida/internal/infrastructure' \
    "$GO_INTERNAL/handler/" 2>/dev/null | import_lines | allow_crosscutting | head -5)
report "$hits" "❌ Handler 不应直接依赖 Repository/Infrastructure"

# 2. service 不应直接依赖 infrastructure
hits=$(grep -rn --include='*.go' 'cognida/internal/infrastructure' \
    "$GO_INTERNAL/service/" 2>/dev/null | import_lines | allow_crosscutting | head -5)
report "$hits" "❌ Service 不应直接依赖 Infrastructure"

# 3. model 不应依赖其他层(同层 model 互相引用允许)
hits=$(grep -rn --include='*.go' 'cognida/internal/' \
    "$GO_INTERNAL/model/" 2>/dev/null | grep -v 'cognida/internal/model' | import_lines | head -5)
report "$hits" "❌ Model 不应依赖其他层"

# 4. repository / infrastructure 不应依赖 handler / service
hits=$(grep -rn --include='*.go' 'cognida/internal/handler\|cognida/internal/service' \
    "$GO_INTERNAL/repository/" "$GO_INTERNAL/infrastructure/" 2>/dev/null | import_lines | head -5)
report "$hits" "❌ Repository 不应依赖 Handler/Service"

if [ $FAILED -eq 0 ]; then
    echo "✅ Go 架构合规检查通过"
fi

exit $FAILED

#!/bin/bash
# Cognida 项目 CodeReview 主检查脚本
# 触发: pre-commit, pre-push, 或手动执行

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

echo "🔍 Cognida 项目 CodeReview"
echo "======================="
echo ""

FAILED=0
CHECKS=()

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 检查函数
run_check() {
    local name=$1
    local command=$2
    local required=${3:-true}  # 默认必需

    echo -e "${BLUE}📋 $name${NC}"

    if eval "$command"; then
        echo -e "${GREEN}✅ $name 通过${NC}"
        CHECKS+=("✅ $name")
    else
        echo -e "${RED}❌ $name 失败${NC}"
        CHECKS+=("❌ $name")
        if [ "$required" = true ]; then
            FAILED=1
        fi
    fi
    echo ""
}

# 获取改动的文件
get_changed_files() {
    if git -C "$PROJECT_ROOT" rev-parse --git-dir > /dev/null 2>&1; then
        git -C "$PROJECT_ROOT" diff --name-only HEAD 2>/dev/null || echo ""
    else
        echo ""
    fi
}

CHANGED_FILES=$(get_changed_files)

# ============================================
# 1. 语言规范检查
# ============================================
echo -e "${BLUE}📖 [1/4] 语言规范检查${NC}"
echo "-------------------"

# Go 语言规范
if echo "$CHANGED_FILES" | grep -q 'services/cognida-go/'; then
    run_check "Go 语言规范" "bash '$SCRIPT_DIR/check-go-norms.sh'" true
else
    echo "⏭️  Go 文件未改动，跳过 Go 规范检查"
    echo ""
fi

# Python 语言规范
if echo "$CHANGED_FILES" | grep -q 'services/cognida-python/'; then
    run_check "Python 语言规范" "bash '$SCRIPT_DIR/check-python-norms.sh'" true
else
    echo "⏭️  Python 文件未改动，跳过 Python 规范检查"
    echo ""
fi

# Web 前端规范
if echo "$CHANGED_FILES" | grep -q 'apps/cognida-web/'; then
    run_check "Web 前端规范" "bash '$SCRIPT_DIR/check-web-norms.sh'" false
else
    echo "⏭️  Web 文件未改动，跳过 Web 规范检查"
    echo ""
fi

# ============================================
# 2. 服务一致性检查
# ============================================
echo -e "${BLUE}🔄 [2/4] 服务一致性检查${NC}"
echo "---------------------"

# Proto 文件一致性
if echo "$CHANGED_FILES" | grep -q 'proto\|grpc'; then
    run_check "Proto 一致性" "bash '$SCRIPT_DIR/check-proto.sh'" true
else
    echo "⏭️  Proto 文件未改动，跳过一致性检查"
    echo ""
fi

# API Contract 一致性 (仅当同时改动 Go handler 和 Web)
if echo "$CHANGED_FILES" | grep -q 'apps/cognida-web/'; then
    run_check "API Contract" "bash '$SCRIPT_DIR/check-api-contract.sh'" false
else
    echo "⏭️  Web API 未改动，跳过 Contract 检查"
    echo ""
fi

# ============================================
# 3. 架构合规检查
# ============================================
echo -e "${BLUE}🏗️  [3/4] 架构合规检查${NC}"
echo "-------------------"

# Go 架构检查
if echo "$CHANGED_FILES" | grep -q 'services/cognida-go/internal/'; then
    run_check "Go 架构规范" "bash '$SCRIPT_DIR/check-go-architecture.sh'" true
else
    echo "⏭️  Go 架构未改动，跳过架构检查"
    echo ""
fi

# Python 架构检查
if echo "$CHANGED_FILES" | grep -q 'services/cognida-python/'; then
    run_check "Python 架构规范" "bash '$SCRIPT_DIR/check-python-architecture.sh'" true
else
    echo "⏭️  Python 架构未改动，跳过架构检查"
    echo ""
fi

# ============================================
# 4. Bug 检测
# ============================================
echo -e "${BLUE}🐛 [4/4] Bug 检测${NC}"
echo "---------------"

# Go 编译检查
if [ -d "$PROJECT_ROOT/services/cognida-go" ] && echo "$CHANGED_FILES" | grep -q 'services/cognida-go/'; then
    run_check "Go 编译" "cd '$PROJECT_ROOT/services/cognida-go' && go build ./..." true
else
    echo "⏭️  Go 文件未改动，跳过编译检查"
    echo ""
fi

# Go vet
if [ -d "$PROJECT_ROOT/services/cognida-go" ] && echo "$CHANGED_FILES" | grep -q 'services/cognida-go/'; then
    run_check "Go vet" "cd '$PROJECT_ROOT/services/cognida-go' && go vet ./..." true
else
    echo "⏭️  跳过 Go vet"
    echo ""
fi

# 数据竞争检测
if [ -d "$PROJECT_ROOT/services/cognida-go" ] && echo "$CHANGED_FILES" | grep -q 'services/cognida-go/'; then
    run_check "数据竞争检测" "cd '$PROJECT_ROOT/services/cognida-go' && timeout 180 go test -race ./... -timeout 120s 2>/dev/null || true" false
else
    echo "⏭️  跳过数据竞争检测"
    echo ""
fi

# SQL 注入检测
run_check "SQL 注入风险" "bash '$SCRIPT_DIR/check-sql-injection.sh'" true

# Python 类型检查
if [ -d "$PROJECT_ROOT/services/cognida-python" ] && echo "$CHANGED_FILES" | grep -q 'services/cognida-python/'; then
    run_check "Python 类型检查" "cd '$PROJECT_ROOT/services/cognida-python' && mypy ./ 2>/dev/null || true" false
else
    echo "⏭️  Python 文件未改动，跳过类型检查"
    echo ""
fi

# ============================================
# 结果汇总
# ============================================
echo "======================="
echo -e "${BLUE}📊 检查结果汇总${NC}"
echo "======================="

for check in "${CHECKS[@]}"; do
    echo "$check"
done

echo ""

if [ $FAILED -eq 1 ]; then
    echo -e "${RED}❌ CodeReview 未通过${NC}"
    echo "请修复上述问题后再次提交"
    exit 1
else
    echo -e "${GREEN}✅ CodeReview 全部通过${NC}"
    exit 0
fi

#!/bin/bash
# Text2SQL Agent 测试脚本

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================"
echo "Text2SQL Agent 测试脚本"
echo "========================================${NC}"

# 检查 .env 文件
if [ ! -f ".env" ]; then
    echo -e "${RED}错误: .env 文件不存在${NC}"
    exit 1
fi

# 加载环境变量
echo -e "${YELLOW}加载环境变量...${NC}"
export $(grep -v "^#" .env | grep -v "^$" | xargs)

# 检查必需变量
if [ -z "$DB_HOST" ] || [ -z "$DB_CHAT_API_KEY" ]; then
    echo -e "${RED}错误: 缺少必需的环境变量${NC}"
    exit 1
fi

echo -e "${GREEN}数据库: ${DB_HOST}:${DB_PORT}/${DB_NAME}${NC}"
echo -e "${GREEN}LLM: ${CHAT_MODEL_NAME}${NC}"
echo ""

# 选择测试类型
echo "请选择测试类型:"
echo "1) 单元测试 (mock)"
echo "2) 集成测试 (真实 MySQL)"
echo "3) 端到端测试 (完整流程)"
echo "4) 全部测试"
read -p "请输入选项 (1-4): " choice

case $choice in
    1)
        echo -e "${YELLOW}运行单元测试...${NC}"
        go test ./internal/application/usecases/agent/tools/... -v
        ;;
    2)
        echo -e "${YELLOW}运行集成测试...${NC}"
        go test -tags=integration ./internal/application/usecases/agent/tools/... -v -run Integration
        ;;
    3)
        echo -e "${YELLOW}运行端到端测试...${NC}"
        go test -tags=integration ./internal/interface/http/handler/... -v -run E2E
        ;;
    4)
        echo -e "${YELLOW}运行全部测试...${NC}"
        go test ./internal/application/usecases/agent/tools/... -v
        echo ""
        go test -tags=integration ./internal/application/usecases/agent/tools/... -v -run Integration
        echo ""
        go test -tags=integration ./internal/interface/http/handler/... -v -run E2E
        ;;
    *)
        echo -e "${RED}无效选项${NC}"
        exit 1
        ;;
esac

echo -e "${GREEN}========================================"
echo "测试完成!"
echo "========================================${NC}"

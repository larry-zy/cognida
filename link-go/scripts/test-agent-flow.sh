#!/bin/bash
# Agent 全流程测试脚本

set -e

# 配置
SERVER_URL="${SERVER_URL:-http://localhost:8080}"
API_KEY="${OPENAI_API_KEY:-}"
EMAIL="${TEST_EMAIL:-test@example.com}"
PASSWORD="${TEST_PASSWORD:-test123456}"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# 检查依赖
check_dependencies() {
    log "检查依赖..."

    if ! command -v curl &> /dev/null; then
        error "curl 未安装"
        exit 1
    fi

    if ! command -v jq &> /dev/null; then
        warn "jq 未安装，JSON 响应将不会格式化"
    fi
}

# 等待服务就绪
wait_for_server() {
    log "等待服务启动..."

    local max_attempts=30
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if curl -s "${SERVER_URL}/health" > /dev/null 2>&1; then
            log "服务已就绪"
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 1
    done

    error "服务启动超时"
    exit 1
}

# 登录获取 Token
login() {
    log "登录获取 Token..."

    # 先尝试注册（如果用户不存在）
    curl -s -X POST "${SERVER_URL}/api/v1/auth/register" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\",\"username\":\"testuser\"}" > /dev/null 2>&1 || true

    # 登录
    response=$(curl -s -X POST "${SERVER_URL}/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}")

    if command -v jq &> /dev/null; then
        TOKEN=$(echo "$response" | jq -r '.data.token // .token // empty')
    else
        # 简单的 token 提取（不完美但可用）
        TOKEN=$(echo "$response" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
    fi

    if [ -z "$TOKEN" ]; then
        error "登录失败: $response"
        exit 1
    fi

    log "登录成功，Token: ${TOKEN:0:20}..."
}

# 测试 Agent 非流式聊天
test_agent_chat() {
    log "测试 Agent 非流式聊天..."

    response=$(curl -s -X POST "${SERVER_URL}/api/v1/agent/chat" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${TOKEN}" \
        -d '{
            "query": "你好，请用一句话介绍你自己",
            "kb_ids": [],
            "session_id": "test-session-'$(date +%s)'"
        }')

    if command -v jq &> /dev/null; then
        echo "$response" | jq .
    else
        echo "$response"
    fi

    log "非流式聊天测试完成"
}

# 测试 Agent 流式聊天
test_agent_stream() {
    log "测试 Agent 流式聊天..."

    temp_output=$(mktemp)
    curl -s -X POST "${SERVER_URL}/api/v1/agent/chat/stream" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${TOKEN}" \
        -d '{
            "query": "请数到5",
            "kb_ids": [],
            "session_id": "test-session-stream-'$(date +%s)'"
        }' > "$temp_output"

    echo "流式响应内容:"
    cat "$temp_output"
    echo ""

    rm -f "$temp_output"
    log "流式聊天测试完成"
}

# 测试获取 Agent 配置
test_agent_config() {
    log "测试获取 Agent 配置..."

    response=$(curl -s -X GET "${SERVER_URL}/api/v1/agent/config" \
        -H "Authorization: Bearer ${TOKEN}")

    if command -v jq &> /dev/null; then
        echo "$response" | jq .
    else
        echo "$response"
    fi

    log "配置获取测试完成"
}

# 测试 Agent 工具列表
test_agent_tools() {
    log "测试获取 Agent 工具列表..."

    response=$(curl -s -X GET "${SERVER_URL}/api/v1/agent/tools" \
        -H "Authorization: Bearer ${TOKEN}")

    if command -v jq &> /dev/null; then
        echo "$response" | jq .
    else
        echo "$response"
    fi

    log "工具列表测试完成"
}

# 测试 Deep Research
test_deep_research() {
    log "测试 Deep Research..."

    response=$(curl -s -X POST "${SERVER_URL}/api/v1/agent/research" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${TOKEN}" \
        -d '{
            "query": "什么是 RAG？",
            "max_depth": 2,
            "max_breadth": 3
        }')

    if command -v jq &> /dev/null; then
        echo "$response" | jq .
    else
        echo "$response"
    fi

    log "Deep Research 测试完成"
}

# 测试查询进度
test_agent_progress() {
    log "测试查询 Agent 进度..."

    local session_id="test-session-$(date +%s)"

    # 先发起一个聊天请求
    curl -s -X POST "${SERVER_URL}/api/v1/agent/chat" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${TOKEN}" \
        -d "{
            \"query\": \"你好\",
            \"kb_ids\": [],
            \"session_id\": \"${session_id}\"
        }" > /dev/null

    # 查询进度
    response=$(curl -s -X GET "${SERVER_URL}/api/v1/agent/progress/${session_id}" \
        -H "Authorization: Bearer ${TOKEN}")

    if command -v jq &> /dev/null; then
        echo "$response" | jq .
    else
        echo "$response"
    fi

    log "进度查询测试完成"
}

# 主测试流程
main() {
    echo "=========================================="
    echo "       Agent 全流程测试脚本"
    echo "=========================================="
    echo ""

    check_dependencies
    wait_for_server
    login

    echo ""
    log "开始测试..."
    echo ""

    # 运行所有测试
    test_agent_chat
    echo ""

    test_agent_stream
    echo ""

    test_agent_config
    echo ""

    test_agent_tools
    echo ""

    test_agent_progress
    echo ""

    # Deep Research 测试（可选，需要网络搜索）
    if [ -n "$ENABLE_RESEARCH_TEST" ]; then
        test_deep_research
        echo ""
    fi

    log "所有测试完成！"
}

# 帮助信息
show_help() {
    echo "Agent 全流程测试脚本"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -h, --help          显示帮助信息"
    echo "  --enable-research   启用 Deep Research 测试"
    echo ""
    echo "环境变量:"
    echo "  SERVER_URL          服务器地址 (默认: http://localhost:8080)"
    echo "  OPENAI_API_KEY      OpenAI API Key"
    echo "  TEST_EMAIL          测试邮箱 (默认: test@example.com)"
    echo "  TEST_PASSWORD       测试密码 (默认: test123456)"
    echo ""
    echo "示例:"
    echo "  $0"
    echo "  SERVER_URL=http://localhost:9090 $0"
    echo "  ENABLE_RESEARCH_TEST=1 $0"
}

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        --enable-research)
            export ENABLE_RESEARCH_TEST=1
            shift
            ;;
        *)
            error "未知参数: $1"
            show_help
            exit 1
            ;;
    esac
done

# 运行主流程
main

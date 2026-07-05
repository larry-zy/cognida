#!/bin/bash
# ==============================================================================
# 两个 Agent 多轮对话 E2E 评测脚本（agent-eval skill 唯一脚本入口）
#
# 目的：
#   1) 验证 DeepSeek thinking 模式 reasoning_content 回传缺陷修复
#      （详见 link-go/docs/deepseek-thinking-reasoning-content-fix.md）
#   2) 回归 RAG 混合检索"检索恒空"缺陷（RRF 分数量纲 vs 余弦阈值）
#   3) 采集回答质量 / 工具调用 / 耗时等评测指标
#
# 覆盖：
#   - RAG Agent   : POST /api/v1/agent/knowledge/stream
#   - Data Agent  : POST /api/v1/agent/text2sql/stream
#   每个 Agent 跑 10 轮连贯对话（复用 done 事件回传的 session_id），
#   每轮都设计为会触发工具调用 —— 这正是 reasoning_content 必须回传的场景。
#
# 通过判据（每个 Agent 汇总其所有轮）：
#   1. 全程无 `reasoning_content ... must be passed back` / `生成失败` 400 错误
#      （既扫 SSE 流，也扫服务端日志）
#   2. 至少收到一个 done 事件且 answer 非空
#   3. 会话中至少成功执行过一次工具调用（证明确实走到"带工具的下一轮生成"）
#
# 默认自启服务（便于抓取服务端日志做 400 检测），跑完自动终止（遵循 CLAUDE.md：
# 任务完成必须终止开启的服务进程）。已有服务时可用 --no-boot 复用。
#
# 采集指标（见 SKILL.md「指标」章节）：
#   - 每轮耗时(ms) / 总/均/最快/最慢
#   - 每轮工具调用成功次数、累计工具调用次数
#   - 空回答轮数、回答字符数（并完整落盘到 *.answer.txt）
#   - reasoning_content 400 命中（SSE 流 + 服务端日志）
#   设置 METRICS_FILE=<path> 时，额外把每个 Agent 的汇总指标以 JSON 行追加落盘。
# ==============================================================================

set -uo pipefail

# ---------- 颜色 / 日志 ----------
GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
log()   { echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} $1"; }
info()  { echo -e "${BLUE}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ---------- 路径：定位 link-go ----------
# 本脚本位于 .claude/skills/link-dev/agent-eval/scripts/，向上 5 级即仓库根。
# 优先用 LINK_GO_DIR 覆盖，便于在非常规布局 / CI 下显式指定。
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -n "${LINK_GO_DIR:-}" ]]; then
    GO_DIR="$LINK_GO_DIR"
else
    GO_DIR="$(cd "$SCRIPT_DIR/../../../../.." && pwd)/link-go"
fi

# ---------- 配置 ----------
SERVER_URL="${SERVER_URL:-http://localhost:8080}"
BOOT_SERVER=1
KEEP_LOG="${KEEP_LOG:-0}"
REQUEST_TIMEOUT="${REQUEST_TIMEOUT:-120}"   # 单轮 SSE 最长等待秒数
METRICS_FILE="${METRICS_FILE:-}"            # 非空则把每 Agent 汇总指标以 JSON 行追加
AGENTS="${AGENTS:-rag,data}"                # 逗号分隔，选择跑哪些 Agent：rag / data
DATASOURCE_ID="${DATASOURCE_ID:-}"          # Data Agent 目标外部数据源 id（空=当前业务库）
                                            # 非空时 Data Agent 用面向该数据源的通用电商用例

# ---------- 运行态 ----------
SERVER_PID=""
SERVER_LOG=""
WORK_DIR=""
FAILURES=0

usage() {
    cat <<EOF
两个 Agent 多轮对话 E2E 评测脚本（agent-eval skill）

用法: $0 [选项]

选项:
  --no-boot        不自启服务，复用已运行的 SERVER_URL（此模式下无法扫描服务端日志）
  --keep-log       保留临时 SSE / 服务端日志文件，便于排查
  -h, --help       显示帮助

环境变量:
  SERVER_URL        服务地址（默认 http://localhost:8080）
  REQUEST_TIMEOUT   单轮 SSE 最长等待秒（默认 120）
  KEEP_LOG          =1 保留临时文件（等价 --keep-log）
  LINK_GO_DIR       显式指定 link-go 目录（默认从脚本位置推导）
  METRICS_FILE      指定则把每个 Agent 的汇总指标以 JSON 行追加落盘
  AGENTS            逗号分隔选择跑哪些 Agent：rag / data（默认 rag,data）
  DATASOURCE_ID     Data Agent 目标外部数据源 id（空=当前业务库；
                    非空时 Data Agent 改用面向该数据源的通用电商用例）
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-boot) BOOT_SERVER=0; shift ;;
        --keep-log) KEEP_LOG=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) error "未知参数: $1"; usage; exit 1 ;;
    esac
done

cleanup() {
    if [[ -n "$SERVER_PID" ]]; then
        info "终止服务进程 (pid=$SERVER_PID)..."
        kill "$SERVER_PID" 2>/dev/null
        wait "$SERVER_PID" 2>/dev/null
    fi
    if [[ -n "$WORK_DIR" && -d "$WORK_DIR" ]]; then
        if [[ "$KEEP_LOG" -eq 1 ]]; then
            info "临时文件保留在: $WORK_DIR"
        else
            rm -rf "$WORK_DIR"
        fi
    fi
}
trap cleanup EXIT INT TERM

check_deps() {
    for bin in curl jq; do
        if ! command -v "$bin" &>/dev/null; then
            error "缺少依赖: $bin"; exit 1
        fi
    done
    if [[ ! -f "$GO_DIR/go.mod" ]]; then
        error "无法定位 link-go 目录（$GO_DIR）。可用 LINK_GO_DIR 显式指定，或用 --no-boot 复用已运行服务。"
        exit 1
    fi
}

boot_server() {
    # 预检：端口已被占用时，自启的新服务会 bind 失败退出，而健康探测又会命中既有
    # 服务造成"误连旧服务的假通过"。故自启模式下若端口已在服务，直接失败并给出指引。
    if curl -fsS "$SERVER_URL/health" >/dev/null 2>&1; then
        error "检测到 $SERVER_URL 已有服务在运行（端口被占用）。"
        error "自启模式会因端口冲突失败并误连到既有服务，导致评测结果不可信。"
        error "请先释放端口（见 link-dev/kill-port 技能），或改用 --no-boot 复用既有服务。"
        exit 1
    fi

    log "编译服务二进制（顺带验证编译通过）..."
    local bin="$WORK_DIR/link-server"
    if ! ( cd "$GO_DIR" && go build -o "$bin" ./cmd/server ) 2> "$WORK_DIR/build.log"; then
        error "服务编译失败:"; cat "$WORK_DIR/build.log"; exit 1
    fi

    log "启动服务（DEV_MODE=true，日志 -> ${SERVER_LOG}）..."
    (
        cd "$GO_DIR"
        set -a
        # shellcheck disable=SC1091
        [[ -f .env ]] && source .env
        set +a
        export DEV_MODE=true
        export LOG_LEVEL="${LOG_LEVEL:-debug}"
        exec "$bin"
    ) > "$SERVER_LOG" 2>&1 &
    SERVER_PID=$!
}

wait_for_server() {
    log "等待服务就绪 ($SERVER_URL/health)..."
    local i
    for ((i = 0; i < 60; i++)); do
        # 先查进程存活：自启模式下若 bind 失败/进程早退，即便端口上有"别的"服务在
        # 应答 /health，也不能误判就绪（否则会误连既有服务造成假通过）。
        if [[ -n "$SERVER_PID" ]] && ! kill -0 "$SERVER_PID" 2>/dev/null; then
            error "自启的服务进程已退出（可能端口被占用 bind 失败），启动日志:"
            tail -n 40 "$SERVER_LOG"; exit 1
        fi
        if curl -fsS "$SERVER_URL/health" >/dev/null 2>&1; then
            log "服务已就绪"
            return 0
        fi
        sleep 1
    done
    error "服务启动超时"
    [[ -n "$SERVER_LOG" ]] && tail -n 40 "$SERVER_LOG"
    exit 1
}

# 提取 SSE 文件里 done 事件的某字段
# $1 文件  $2 jq 过滤器（作用于 done data JSON）
done_field() {
    grep '^data:' "$1" 2>/dev/null | sed 's/^data: *//' \
        | jq -rc 'select(.event=="done")' 2>/dev/null | tail -1 \
        | jq -r "$2 // \"\"" 2>/dev/null
}

# 统计 SSE 文件里成功执行的工具调用数（type==tool_result 且 status!=error）
count_tool_results() {
    grep '^data:' "$1" 2>/dev/null | sed 's/^data: *//' \
        | jq -rc 'select(.type=="tool_result")' 2>/dev/null \
        | jq -rc 'select((.status // "") != "error")' 2>/dev/null | wc -l | tr -d ' '
}

# 扫描本轮 reasoning_content 400 / 生成失败 标志（SSE 流内）
scan_sse_errors() {
    grep -iE 'must be passed back|reasoning_content|生成失败' "$1" 2>/dev/null | head -3
}

# 毫秒级时间戳（macOS 自带 perl，避免依赖 GNU date 的 %N）
now_ms() { perl -MTime::HiRes=time -e 'printf("%.0f", time()*1000)'; }

# 发起一轮对话：$1 endpoint  $2 query  $3 session_id  $4 输出SSE文件  $5 datasource_id(可空)
# 回显该轮解析出的 session_id
run_turn() {
    local endpoint="$1" query="$2" sid="$3" out="$4" ds="${5:-}"
    local body
    # datasource_id 为空时服务端按当前业务库处理；非空时路由到已注册外部数据源
    body=$(jq -n --arg q "$query" --arg s "$sid" --arg ds "$ds" \
        '{query:$q, session_id:$s, datasource_id:$ds, kb_ids:[], graph_enabled:false, kb_scope_mode:"manual"}')

    # -N 关闭缓冲，实时落盘；--max-time 兜底防止 SSE 挂死
    curl -sS -N --max-time "$REQUEST_TIMEOUT" \
        -X POST "$SERVER_URL$endpoint" \
        -H "Content-Type: application/json" \
        -H "X-Request-ID: e2e-$(basename "$out")" \
        -d "$body" > "$out" 2>> "$WORK_DIR/curl.err"

    done_field "$out" '.session_id'
}

# 测一个 Agent 的多轮对话
# $1 名称  $2 endpoint  $3 datasource_id(可空)  $4.. 各轮 query
test_agent() {
    local name="$1" endpoint="$2" ds="$3"; shift 3
    local queries=("$@")

    echo ""
    echo "=============================================================="
    log "开始测试 [$name]  ($endpoint)  共 ${#queries[@]} 轮${ds:+  数据源=$ds}"
    echo "=============================================================="

    local sid=""
    local turn=0
    local total_tools=0
    local last_answer=""
    local agent_failed=0
    local total_ms=0 max_ms=0 min_ms=0 empty_answers=0
    local -a per_turn_ms=()
    local log_scan_start=0
    [[ -n "$SERVER_LOG" ]] && log_scan_start=$(wc -l < "$SERVER_LOG" 2>/dev/null | tr -d ' ')

    for q in "${queries[@]}"; do
        turn=$((turn + 1))
        local safe_name; safe_name=$(echo "$name" | tr '[:upper:] ' '[:lower:]-')
        local out="$WORK_DIR/${safe_name}-turn${turn}.sse"

        info "第 ${turn} 轮 › \"$q\"  (session_id=${sid:-<新建>})"

        # ---- 计时包住整轮 SSE ----
        local t0 t1 dur_ms new_sid
        t0=$(now_ms)
        new_sid=$(run_turn "$endpoint" "$q" "$sid" "$out" "$ds")
        t1=$(now_ms)
        dur_ms=$((t1 - t0))
        per_turn_ms+=("$dur_ms")
        total_ms=$((total_ms + dur_ms))
        [[ "$dur_ms" -gt "$max_ms" ]] && max_ms="$dur_ms"
        { [[ "$min_ms" -eq 0 ]] || [[ "$dur_ms" -lt "$min_ms" ]]; } && min_ms="$dur_ms"

        # ---- 逐项断言 ----
        local answer tools sse_err
        answer=$(done_field "$out" '.answer')
        tools=$(count_tool_results "$out")
        sse_err=$(scan_sse_errors "$out")
        total_tools=$((total_tools + tools))
        [[ -n "$answer" ]] && last_answer="$answer"

        if [[ -z "$new_sid" ]]; then
            error "  ✗ 未收到有效 done 事件（可能中途 400 / 超时）"
            agent_failed=1
        else
            sid="$new_sid"
        fi

        if [[ -n "$sse_err" ]]; then
            error "  ✗ SSE 流中检出错误标志:"
            echo "$sse_err" | sed 's/^/      /'
            agent_failed=1
        fi

        # ---- 回答质量：完整落盘 + 完整打印（便于人工核对）----
        if [[ -n "$answer" ]]; then
            printf '%s\n' "$answer" > "${out%.sse}.answer.txt"
            log "  ✓ answer(${#answer}字 | 工具${tools}次 | ${dur_ms}ms):"
            printf '%s\n' "$answer" | sed 's/^/      │ /'
        else
            empty_answers=$((empty_answers + 1))
            warn "  · 本轮 answer 为空（工具${tools}次 | ${dur_ms}ms）"
        fi
    done

    # ---- 耗时汇总 ----
    local avg_ms=0
    [[ "$turn" -gt 0 ]] && avg_ms=$((total_ms / turn))
    echo "  · 耗时统计：总 $((total_ms / 1000)).$(printf '%03d' $((total_ms % 1000)))s | 均 ${avg_ms}ms/轮 | 最快 ${min_ms}ms | 最慢 ${max_ms}ms"
    echo "  · 各轮(ms)：${per_turn_ms[*]}"
    echo "  · 空回答轮数：${empty_answers}/${turn}"

    # ---- 服务端日志扫描（仅自启模式）：只看本 Agent 运行期间新增的日志 ----
    local log_hit=""
    if [[ -n "$SERVER_LOG" ]]; then
        log_hit=$(tail -n "+$((log_scan_start + 1))" "$SERVER_LOG" 2>/dev/null \
            | grep -iE 'must be passed back|reasoning_content in the thinking' | head -3)
        if [[ -n "$log_hit" ]]; then
            error "  ✗ 服务端日志检出 reasoning_content 400:"
            echo "$log_hit" | sed 's/^/      /'
            agent_failed=1
        fi
    fi

    # ---- 汇总判据 ----
    echo "--------------------------------------------------------------"
    if [[ "$total_tools" -eq 0 ]]; then
        warn "[$name] 全程未触发任何工具调用 —— 未真正覆盖 reasoning 回传路径（判为不达标）"
        agent_failed=1
    fi
    if [[ -z "$last_answer" ]]; then
        error "[$name] 全程未产出任何非空回答"
        agent_failed=1
    fi

    # ---- 机器可读指标落盘（可选）----
    if [[ -n "$METRICS_FILE" ]]; then
        jq -nc \
            --arg agent "$name" \
            --arg endpoint "$endpoint" \
            --arg datasource_id "$ds" \
            --argjson turns "$turn" \
            --argjson total_ms "$total_ms" \
            --argjson avg_ms "$avg_ms" \
            --argjson max_ms "$max_ms" \
            --argjson min_ms "$min_ms" \
            --argjson total_tools "$total_tools" \
            --argjson empty_answers "$empty_answers" \
            --argjson per_turn_ms "$(printf '%s\n' "${per_turn_ms[@]}" | jq -sc '.')" \
            --argjson passed "$([[ "$agent_failed" -eq 0 ]] && echo true || echo false)" \
            '{agent:$agent, endpoint:$endpoint, datasource_id:$datasource_id, turns:$turns, total_ms:$total_ms, avg_ms:$avg_ms, max_ms:$max_ms, min_ms:$min_ms, total_tools:$total_tools, empty_answers:$empty_answers, per_turn_ms:$per_turn_ms, passed:$passed}' \
            >> "$METRICS_FILE"
    fi

    if [[ "$agent_failed" -eq 0 ]]; then
        log "[$name] ✅ 通过：${turn} 轮对话完成，累计 ${total_tools} 次工具调用，无 reasoning_content 400"
    else
        error "[$name] ❌ 未通过"
        FAILURES=$((FAILURES + 1))
    fi
}

main() {
    echo "=============================================================="
    echo "     两个 Agent 多轮对话 E2E 评测（reasoning_content + RAG 检索）"
    echo "=============================================================="

    check_deps

    WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/link-e2e.XXXXXX")"
    SERVER_LOG="$WORK_DIR/server.log"
    : > "$WORK_DIR/curl.err"
    [[ -n "$METRICS_FILE" ]] && : > "$METRICS_FILE"

    if [[ "$BOOT_SERVER" -eq 1 ]]; then
        boot_server
    else
        SERVER_LOG=""   # 复用外部服务，放弃日志扫描
        warn "复用已运行服务（--no-boot），跳过服务端日志扫描"
    fi
    wait_for_server

    local run_rag=0 run_data=0
    case ",${AGENTS}," in *,rag,*) run_rag=1;; esac
    case ",${AGENTS}," in *,data,*) run_data=1;; esac
    [[ "$run_rag" -eq 0 && "$run_data" -eq 0 ]] && { error "AGENTS=$AGENTS 未选中任何 Agent（可选 rag,data）"; exit 1; }

    # RAG Agent：10 轮连贯对话，逐轮基于上文追问，持续触发 kb_list / rag_query
    if [[ "$run_rag" -eq 1 ]]; then
        test_agent "RAG-Agent" "/api/v1/agent/knowledge/stream" "" \
            "有哪些知识库？请列出来。" \
            "这个知识库大概讲的是什么主题？检索一下给我一个概览。" \
            "针对刚才的主题，检索最核心的一个概念并解释清楚。" \
            "再检索一个与之相关的细节问题并回答。" \
            "把前面几轮检索到的信息综合成一段连贯的总结。" \
            "有没有和这个主题相关、但你前面还没提到的内容？检索补充一下。" \
            "基于我们目前聊到的全部内容，帮我提炼成 3 个要点。" \
            "针对上面第 2 个要点，再深入检索并展开说明。" \
            "如果要向一个完全不了解的人两句话介绍这个知识库，你会怎么说？" \
            "回顾我们整段对话，你觉得还遗漏了什么关键信息？检索确认后补上。"
    fi

    # Data Agent：10 轮连贯数据分析对话，逐步下钻，持续触发 sql_execute / data_analysis
    if [[ "$run_data" -eq 1 ]]; then
        if [[ -n "$DATASOURCE_ID" ]]; then
            # 外部电商数据源（ecommerce_demo：categories/products/customers/orders/order_items 等）
            test_agent "Data-Agent" "/api/v1/agent/text2sql/stream" "$DATASOURCE_ID" \
                "这个数据库里有哪些表？" \
                "orders 表有哪些字段？各是什么含义？" \
                "一共有多少个订单？多少个客户？多少个商品？" \
                "统计每种订单状态（status）各有多少个订单。" \
                "按下单总金额算，消费最高的前 5 个客户是谁？" \
                "结合 order_items，统计销量最高的前 10 个商品。" \
                "各商品类目（categories）的销售额分别是多少？取前几名。" \
                "订单时间跨度是从什么时候到什么时候？按月看订单量趋势。" \
                "把前面这些查询结果综合起来，给我一段电商经营概览。" \
                "基于以上分析，你觉得有哪些值得注意的发现或异常？"
        else
            # 当前业务库 dataagent_sales 逐步下钻
            test_agent "Data-Agent" "/api/v1/agent/text2sql/stream" "" \
                "数据库里有哪些表？" \
                "查询 dataagent_sales 表的前 5 条记录。" \
                "统计 dataagent_sales 表一共有多少条记录。" \
                "dataagent_sales 表都有哪些字段？各是什么含义？" \
                "按表里的分类字段分组，统计每一组的记录数。" \
                "找出数值最大的前 3 条记录。" \
                "计算主要数值字段的平均值。" \
                "这张表里时间跨度是从什么时候到什么时候？" \
                "把前面这些查询结果综合起来，给我一段简短的数据概览。" \
                "基于以上分析，你觉得有哪些值得注意的发现或异常？"
        fi
    fi

    echo ""
    echo "=============================================================="
    if [[ "$FAILURES" -eq 0 ]]; then
        log "🎉 全部通过：所选 Agent 均完成多轮工具调用对话，reasoning_content 回传正常"
        [[ -n "$METRICS_FILE" ]] && info "指标已写入: $METRICS_FILE"
        echo "=============================================================="
        exit 0
    else
        error "存在 $FAILURES 个 Agent 未通过，详见上文"
        [[ -n "$SERVER_LOG" ]] && info "完整服务端日志: ${SERVER_LOG}（--keep-log 可保留）"
        [[ -n "$METRICS_FILE" ]] && info "指标已写入: $METRICS_FILE"
        echo "=============================================================="
        exit 1
    fi
}

main

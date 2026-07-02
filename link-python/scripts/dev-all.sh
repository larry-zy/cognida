#!/usr/bin/env bash
# 一键并行启动 Python 三服务:
#   - gRPC 主服务  (端口 50051, 含数据分析 50053)  -> grpc_service.server
#   - HTTP/FastAPI (端口 8000)                     -> main.py
#   - MCP 服务     (端口 3000)                      -> mcp_service.server
#
# 用法:  bash scripts/dev-all.sh
# 停止:  Ctrl+C (会优雅终止全部子进程)
set -uo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"

export APP_ENV="${APP_ENV:-dev}"
export LOG_LEVEL="${LOG_LEVEL:-DEBUG}"
# MCP 以 http 模式启动才会监听端口 3000 (stdio 模式走标准输入输出, 不占端口)。
export MCP_MODE="${MCP_MODE:-http}"

LOG_DIR="$ROOT/logs"
mkdir -p "$LOG_DIR"

PIDS=()
NAMES=()

# 递归终止进程及其所有后代 (uv run 会再 fork python 子进程, 需连孙子一起清)
kill_tree() {
  local pid="$1" sig="$2" child
  for child in $(pgrep -P "$pid" 2>/dev/null); do
    kill_tree "$child" "$sig"
  done
  kill -"$sig" "$pid" 2>/dev/null || true
}

cleanup() {
  # 幂等: 避免 INT 与 EXIT 双触发时重复执行
  if [ "${_CLEANED:-0}" = "1" ]; then return; fi
  _CLEANED=1
  echo ""
  echo "🛑 正在停止全部服务..."
  local i=0
  for pid in "${PIDS[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      kill_tree "$pid" TERM
      echo "   已发送停止信号: ${NAMES[$i]} (PID=$pid 及其子进程)"
    fi
    i=$((i + 1))
  done
  # 给子进程优雅退出的时间, 之后 KILL 兜底
  sleep 1
  for pid in "${PIDS[@]}"; do
    kill_tree "$pid" KILL
  done
  echo "✅ 已全部停止"
}
trap cleanup INT TERM EXIT

start() {
  local name="$1"; shift
  echo "▶️  启动 $name ..."
  "$@" > "$LOG_DIR/$name.log" 2>&1 &
  local pid=$!
  PIDS+=("$pid")
  NAMES+=("$name")
  echo "   PID=$pid  日志: logs/$name.log"
}

echo "========================================"
echo "  Link Python 三服务并行启动 (APP_ENV=$APP_ENV)"
echo "========================================"

start grpc uv run python -m grpc_service.server
start http uv run python main.py
start mcp  uv run python -m mcp_service.server

echo ""
echo "端口:  gRPC=50051(+分析 50053)   HTTP=8000   MCP=3000"
echo "按 Ctrl+C 停止全部。以下为三服务实时日志:"
echo "----------------------------------------"

# tail 也纳入 PIDS, 退出时一并清理
tail -n +1 -f "$LOG_DIR/grpc.log" "$LOG_DIR/http.log" "$LOG_DIR/mcp.log" &
PIDS+=("$!")
NAMES+=("tail")

# 前台阻塞直到 Ctrl+C; 任一后台任务结束也会返回
wait

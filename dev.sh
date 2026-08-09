#!/usr/bin/env bash
# =============================================================================
# Cognida 全栈一键启停脚本
#
#   ./dev.sh start     启动全部应用服务(Go + Python 四服务 + 前端)
#   ./dev.sh stop      停止全部并清理端口(保证不残留占端口)
#   ./dev.sh restart   先 stop 再 start
#   ./dev.sh status    查看各服务/端口状态
#   ./dev.sh logs [名]  跟随日志(不带名跟随全部)
#
# 说明:
#   - 中间件(MySQL/Redis/Milvus/Neo4j)不由本脚本代管启停(它们常驻、跨
#     brew/docker),仅在 start 时做连通性预检并给出提示。
#   - PID 与日志落在 var/run/ (已在 .gitignore 忽略)。
#   - Go 服务由 godotenv 自动加载 services/cognida-go/.env,无需手动 source。
# =============================================================================
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUN_DIR="$ROOT/var/run"
mkdir -p "$RUN_DIR"

# 前端包管理器(有 pnpm-lock 且装了 pnpm 时可切 pnpm)
WEB_PM="npm"

# ---- 应用服务定义: name|工作目录|端口(空格分隔)|启动命令 ---------------------
# 注意: 端口列表用于 status 展示与 stop 兜底清理。
service_defs() {
  cat <<'EOF'
go|services/cognida-go|8080|go run ./cmd/server/main.go
py-grpc|services/cognida-python|50051|uv run python -m grpc_service.server
py-http|services/cognida-python|8000|uv run python main.py
py-eval|services/cognida-python|18888|uv run python -m uvicorn services.evaluation.fastapi_app:app --host 0.0.0.0 --port 18888
py-mcp|services/cognida-python|3100|uv run python -m mcp_service.server
web|apps/cognida-web|5173|__WEB_DEV__
EOF
}

ALL_PORTS="8080 50051 8000 18888 3100 5173"

# ---- 中间件(可选,--with-deps 时纳入启停) ------------------------------------
# Milvus 跑在 docker 容器(embed 脚本仅首次建容器用一次,日常 docker start/stop 即可);
# MySQL/Redis/Neo4j 由 Homebrew services 管理。
BREW="$(command -v brew 2>/dev/null || echo /opt/homebrew/bin/brew)"
MILVUS_CONTAINER="milvus-standalone"
WITH_DEPS=0
# name|端口|启动命令|停止命令
middleware_defs() {
  cat <<EOF
mysql|3306|$BREW services start mysql@8.0|$BREW services stop mysql@8.0
redis|6379|$BREW services start redis|$BREW services stop redis
neo4j|7687|$BREW services start neo4j|$BREW services stop neo4j
milvus|19530|docker start $MILVUS_CONTAINER|docker stop $MILVUS_CONTAINER
EOF
}

# ---- 颜色 -------------------------------------------------------------------
if [ -t 1 ]; then
  C_G="\033[32m"; C_R="\033[31m"; C_Y="\033[33m"; C_B="\033[36m"; C_0="\033[0m"
else
  C_G=""; C_R=""; C_Y=""; C_B=""; C_0=""
fi
ok()   { echo -e "${C_G}✅ $*${C_0}"; }
warn() { echo -e "${C_Y}⚠️  $*${C_0}"; }
err()  { echo -e "${C_R}❌ $*${C_0}"; }
info() { echo -e "${C_B}▶️  $*${C_0}"; }

# 递归终止进程树(uv run / npm 会 fork 子孙进程,需连孙子一起清)
kill_tree() {
  local pid="$1" sig="$2" child
  for child in $(pgrep -P "$pid" 2>/dev/null); do
    kill_tree "$child" "$sig"
  done
  kill -"$sig" "$pid" 2>/dev/null || true
}

port_pids() { lsof -ti tcp:"$1" -s tcp:LISTEN 2>/dev/null; }

# ---- 中间件连通性预检(仅提示,不阻塞) ----------------------------------------
check_deps() {
  echo "── 中间件预检 ──────────────────────────────"
  local name port
  for pair in "MySQL:3306" "Redis:6379" "Milvus:19530" "Neo4j:7687"; do
    name="${pair%%:*}"; port="${pair##*:}"
    if lsof -i tcp:"$port" -s tcp:LISTEN >/dev/null 2>&1 || nc -z localhost "$port" 2>/dev/null; then
      ok "$name (:$port) 在监听"
    else
      warn "$name (:$port) 未检测到 —— 相关功能会降级/报错,请先启动中间件"
    fi
  done
  echo ""
}

# ---- 启动中间件(--with-deps) ------------------------------------------------
start_deps() {
  echo "── 启动中间件 ──────────────────────────────"
  local name port start stop
  while IFS='|' read -r name port start stop; do
    [ -z "$name" ] && continue
    if lsof -i tcp:"$port" -s tcp:LISTEN >/dev/null 2>&1; then
      ok "$name (:$port) 已在运行,跳过"
    else
      info "启动 $name : $start"
      eval "$start" >>"$RUN_DIR/deps.log" 2>&1 \
        && ok "$name 启动命令已执行" \
        || warn "$name 启动命令失败(见 var/run/deps.log)"
    fi
  done < <(middleware_defs)
  # 给 DB/Milvus 一点起来的时间
  sleep 2
  echo ""
}

# ---- 停止中间件(--with-deps) ------------------------------------------------
stop_deps() {
  echo ""
  echo "── 停止中间件 ──────────────────────────────"
  local name port start stop
  while IFS='|' read -r name port start stop; do
    [ -z "$name" ] && continue
    if lsof -i tcp:"$port" -s tcp:LISTEN >/dev/null 2>&1; then
      info "停止 $name : $stop"
      eval "$stop" >>"$RUN_DIR/deps.log" 2>&1 \
        && ok "$name 停止命令已执行" \
        || warn "$name 停止命令失败(见 var/run/deps.log)"
    else
      ok "$name (:$port) 未在运行,跳过"
    fi
  done < <(middleware_defs)
}

# ---- 启动单个服务 -----------------------------------------------------------
start_one() {
  local name="$1" dir="$2" ports="$3" cmd="$4"
  local pidfile="$RUN_DIR/$name.pid" log="$RUN_DIR/$name.log"

  # 已在运行则跳过
  if [ -f "$pidfile" ] && kill -0 "$(cat "$pidfile" 2>/dev/null)" 2>/dev/null; then
    warn "$name 已在运行 (PID=$(cat "$pidfile")),跳过"
    return
  fi
  # 端口被占(可能是别的残留)则提示
  for p in $ports; do
    if [ -n "$(port_pids "$p")" ]; then
      warn "$name 目标端口 :$p 已被占用,启动可能失败(先 ./dev.sh stop)"
    fi
  done

  info "启动 $name ..."
  (
    cd "$ROOT/$dir" || exit 1
    # py-mcp：显式导出监听端口，与进程表声明的端口（:3100）自洽，避免真实监听端口
    # 依赖 python .env 的 MCP_PORT——一旦那行缺失会退回 :3000，而 status/stop 仍按 3100 记账。
    if [ "$name" = "py-mcp" ]; then export MCP_MODE="${MCP_MODE:-http}" MCP_PORT="${MCP_PORT:-$ports}"; fi
    if [ "$cmd" = "__WEB_DEV__" ]; then
      exec "$WEB_PM" run dev
    else
      exec bash -c "$cmd"
    fi
  ) >"$log" 2>&1 &
  echo $! >"$pidfile"
  echo "   PID=$(cat "$pidfile")  端口=$ports  日志: var/run/$name.log"
}

cmd_start() {
  if [ "$WITH_DEPS" = "1" ]; then start_deps; else check_deps; fi
  echo "── 启动应用服务 ────────────────────────────"
  while IFS='|' read -r name dir ports cmd; do
    [ -z "$name" ] && continue
    start_one "$name" "$dir" "$ports" "$cmd"
  done < <(service_defs)
  echo ""
  ok "启动完成。前端: http://localhost:5173   后端: http://localhost:8080"
  echo "   查看状态: ./dev.sh status    跟随日志: ./dev.sh logs"
}

# ---- 停止全部 ---------------------------------------------------------------
cmd_stop() {
  echo "── 停止应用服务 ────────────────────────────"
  # 1) 按记录的 PID 递归杀进程树
  for pidfile in "$RUN_DIR"/*.pid; do
    [ -e "$pidfile" ] || continue
    local name pid; name="$(basename "$pidfile" .pid)"; pid="$(cat "$pidfile" 2>/dev/null)"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      info "停止 $name (PID=$pid 及子进程)"
      kill_tree "$pid" TERM
    fi
    rm -f "$pidfile"
  done
  sleep 1
  # 2) KILL 兜底(上一步 TERM 后仍存活的)
  for pidfile in "$RUN_DIR"/*.pid; do
    [ -e "$pidfile" ] || continue
    kill_tree "$(cat "$pidfile" 2>/dev/null)" KILL 2>/dev/null || true
  done
  # 3) 按端口兜底: 谁还占着目标端口就清掉,保证不残留
  local leftover=0
  for p in $ALL_PORTS; do
    local pids; pids="$(port_pids "$p")"
    if [ -n "$pids" ]; then
      warn "端口 :$p 仍被占用,清理 PID: $pids"
      for pid in $pids; do kill_tree "$pid" TERM; done
      sleep 0.3
      pids="$(port_pids "$p")"
      [ -n "$pids" ] && for pid in $pids; do kill_tree "$pid" KILL; done
      leftover=1
    fi
  done
  [ "$leftover" = "0" ] && ok "应用服务已停止,端口已释放" || ok "已强制清理残留端口"
  [ "$WITH_DEPS" = "1" ] && stop_deps
}

# ---- 状态 -------------------------------------------------------------------
cmd_status() {
  echo "── 服务状态 ────────────────────────────────"
  printf "%-8s %-14s %s\n" "服务" "端口" "状态"
  while IFS='|' read -r name dir ports cmd; do
    [ -z "$name" ] && continue
    local up="" any=""
    for p in $ports; do
      if [ -n "$(port_pids "$p")" ]; then up="$up ${p}:up"; any=1; else up="$up ${p}:-"; fi
    done
    if [ -n "$any" ]; then
      printf "%-8s %-14s ${C_G}RUNNING${C_0}%s\n" "$name" "$ports" "$up"
    else
      printf "%-8s %-14s ${C_R}stopped${C_0}\n" "$name" "$ports"
    fi
  done < <(service_defs)
}

# ---- 日志 -------------------------------------------------------------------
cmd_logs() {
  local name="${1:-}"
  if [ -n "$name" ]; then
    tail -n 100 -f "$RUN_DIR/$name.log"
  else
    tail -n 20 -f "$RUN_DIR"/*.log
  fi
}

# ---- 入口 -------------------------------------------------------------------
ACTION=""
LOG_NAME=""
for arg in "$@"; do
  case "$arg" in
    --with-deps|-d) WITH_DEPS=1 ;;
    start|stop|restart|status|logs) [ -z "$ACTION" ] && ACTION="$arg" ;;
    *) [ "$ACTION" = "logs" ] && LOG_NAME="$arg" ;;
  esac
done
ACTION="${ACTION:-start}"

case "$ACTION" in
  start)   cmd_start ;;
  stop)    cmd_stop ;;
  restart) cmd_stop; echo ""; cmd_start ;;
  status)  cmd_status ;;
  logs)    cmd_logs "$LOG_NAME" ;;
  *) echo "用法: ./dev.sh {start|stop|restart|status|logs [服务名]} [--with-deps]"; exit 1 ;;
esac

#!/bin/bash
# Stop hook(提交前综合门禁)
# ------------------------------------------------------------------
# 作用: 在 Claude 准备结束当前回合前, 运行综合 CodeReview。
#       codereview.sh 基于 git diff 仅检查"本次改动的文件", 未通过则阻断结束
#       并把结果反馈给 Claude 去修复 —— 这是真正的强制门禁。
# 前置条件: 项目需已 git init(否则 codereview.sh 取不到改动列表, 跳过全部检查、
#           直接通过, 门禁安全休眠)。
set -uo pipefail
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

# 安全休眠: 项目未 git init 时直接放行。codereview.sh 基于 git diff 仅检查改动
# 文件才有意义; 无 git 时其部分检查会全项目误报并无条件阻断, 故此处先行短路。
if ! git -C "$PROJECT_ROOT" rev-parse --git-dir >/dev/null 2>&1; then
  exit 0
fi

input=$(cat)

# 防循环: 若本次 Stop 正是上一次 stop hook 触发的, 直接放行
if printf '%s' "$input" | jq -e '.stop_hook_active == true' >/dev/null 2>&1; then
  exit 0
fi

out=$(bash "$SCRIPT_DIR/codereview.sh" 2>&1)
code=$?
if [ "$code" -ne 0 ]; then
  reason="提交前 CodeReview 未通过, 请修复以下问题后再结束:"$'\n\n'"${out}"
  jq -nc --arg r "$reason" '{decision:"block",reason:$r}'
  exit 0
fi
exit 0

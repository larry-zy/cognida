#!/bin/bash
# PostToolUse hook(非阻断反馈)
# ------------------------------------------------------------------
# 作用: 每次 Write/Edit/MultiEdit 后, 按"被编辑文件的语言"路由到对应的
#       规范/架构/安全检查脚本, 把发现以 additionalContext 反馈给 Claude。
# 设计要点:
#   - 非阻断(始终 exit 0): 这些检查为全项目扫描, 棕地项目存量违规多,
#     硬阻断会导致每次编辑都被无关问题卡死, 故仅作"反馈回路"而非"门禁"。
#   - 真正的提交前门禁见 agent-stopgate.sh (Stop hook, 仅检查改动文件)。
set -uo pipefail
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

input=$(cat)
file_path=$(printf '%s' "$input" | jq -r '.tool_input.file_path // .tool_input.notebook_path // empty' 2>/dev/null)
[ -z "$file_path" ] && exit 0

# 按路径/扩展名选择相关检查, 避免每次编辑都跑全部 8 个脚本
checks=()
case "$file_path" in
  *link-go/*.go)
    checks+=("check-go-norms.sh" "check-sql-injection.sh")
    case "$file_path" in *link-go/internal/*) checks+=("check-go-architecture.sh") ;; esac
    ;;
  *link-python/*.py)
    checks+=("check-python-norms.sh" "check-python-architecture.sh" "check-sql-injection.sh")
    ;;
  *link-web/*.ts | *link-web/*.tsx | *link-web/*.vue | *link-web/*.js)
    checks+=("check-web-norms.sh")
    ;;
  *.proto)
    checks+=("check-proto.sh")
    ;;
esac
[ ${#checks[@]} -eq 0 ] && exit 0

report=""
for c in "${checks[@]}"; do
  [ -f "$SCRIPT_DIR/$c" ] || continue
  out=$(bash "$SCRIPT_DIR/$c" 2>&1)
  code=$?
  if [ "$code" -ne 0 ]; then
    report+="### ${c} (exit ${code})"$'\n'"${out}"$'\n\n'
  fi
done

[ -z "$report" ] && exit 0

msg="项目规范门禁在你刚编辑文件的相关范围内发现以下问题(注: 这是全项目扫描, 可能包含存量问题; 请优先修复与本次改动相关的部分):"$'\n\n'"${report}"
jq -nc --arg ctx "$msg" '{hookSpecificOutput:{hookEventName:"PostToolUse",additionalContext:$ctx}}'
exit 0

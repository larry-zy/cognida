#!/bin/bash
# ==============================================================================
# 兼容转发入口 —— 两个 Agent 多轮对话 E2E 评测已收敛到 agent-eval skill。
#
# 规范脚本现位于:
#   .claude/skills/cognida-dev/agent-eval/scripts/test-agents-e2e.sh
# 说明文档:
#   .claude/skills/cognida-dev/agent-eval/SKILL.md
#
# 本文件仅原样转发参数，保留历史调用路径不失效。请直接调用上面的技能脚本。
# ==============================================================================
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$HERE/../../.." && pwd)"
exec "$REPO_ROOT/.claude/skills/cognida-dev/agent-eval/scripts/test-agents-e2e.sh" "$@"

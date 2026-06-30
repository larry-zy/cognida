#!/bin/bash
# 完整测试流水线 - Link 项目
# 依次运行: Go 单元测试 → Python 单元测试 → Go 集成测试 → Python 集成测试 → E2E 测试

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../../../.." && pwd)

echo "🚀 Link 完整测试流水线"
echo "======================"
echo ""

# 1. Go 单元测试
echo "📦 [1/5] Go 单元测试"
"$SCRIPT_DIR/go-test.sh" unit || { echo "❌ Go 单元测试失败"; exit 1; }
echo ""

# 2. Python 单元测试
echo "📦 [2/5] Python 单元测试"
"$SCRIPT_DIR/python-test.sh" unit || { echo "❌ Python 单元测试失败"; exit 1; }
echo ""

# 3. Go 集成测试
echo "🔗 [3/5] Go 集成测试"
"$SCRIPT_DIR/go-test.sh" integration || { echo "❌ Go 集成测试失败"; exit 1; }
echo ""

# 4. Python 集成测试
echo "🔗 [4/5] Python 集成测试"
"$SCRIPT_DIR/python-test.sh" integration || { echo "❌ Python 集成测试失败"; exit 1; }
echo ""

# 5. E2E 测试
echo "🎭 [5/5] E2E 测试"
echo "提示: 确保 $BASE_URL 服务已启动"
read -p "按 Enter 继续 E2E 测试，或 Ctrl+C 跳过..."
"$SCRIPT_DIR/e2e-test.sh" user-login || { echo "❌ E2E 测试失败"; exit 1; }
echo ""

echo "======================"
echo "✅ 所有测试通过！"

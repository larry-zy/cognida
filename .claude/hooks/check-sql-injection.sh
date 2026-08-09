#!/bin/bash
# SQL 注入风险检查
#
# 仅在“真正命中”时返回非 0：旧版以 `if grep ... | head -5; then` 判断，
# 管道退出码取自 head(恒为 0)，导致零命中也报失败(误报)。改为捕获输出后判空。
# 同时：跳过二进制文件(-I)、排除 examples/data 等非源码目录，并收紧匹配为
# 真实 SQL 子句(SELECT...FROM / INSERT INTO / UPDATE...SET / DELETE FROM)，
# 避免命中诸如 f"...{selected_graders}" 之类的普通字符串。

set -uo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
FAILED=0

echo "💉 SQL 注入风险检查..."

# 真实 SQL 语句形态(大小写不敏感, 需关键字+子句同时出现)
SQL_CLAUSE='(SELECT[[:space:]].*FROM|INSERT[[:space:]]+INTO|UPDATE[[:space:]].*SET|DELETE[[:space:]]+FROM)'

# Go: fmt.Sprintf 拼接 SQL
if [ -d "$PROJECT_ROOT/services/cognida-go/internal" ]; then
    go_hits=$(grep -rIniE --include='*.go' \
        "fmt\.Sprintf\(.*${SQL_CLAUSE}" \
        "$PROJECT_ROOT/services/cognida-go/internal/" 2>/dev/null \
        | grep -v '_test.go' | head -5)
    if [ -n "$go_hits" ]; then
        echo "$go_hits"
        echo "❌ 发现可能的 SQL 注入: 使用 fmt.Sprintf 拼接 SQL"
        FAILED=1
    fi
fi

# Python: f-string 拼接 SQL
if [ -d "$PROJECT_ROOT/services/cognida-python" ]; then
    py_hits=$(grep -rIniE --include='*.py' \
        --exclude-dir=examples --exclude-dir=data --exclude-dir=tests \
        --exclude-dir=.venv --exclude-dir=venv --exclude-dir=__pycache__ \
        --exclude-dir=.git --exclude-dir=node_modules \
        "f[\"'][^\"']*${SQL_CLAUSE}" \
        "$PROJECT_ROOT/services/cognida-python/" 2>/dev/null \
        | grep -v '_test.py' | head -5)
    if [ -n "$py_hits" ]; then
        echo "$py_hits"
        echo "❌ 发现可能的 SQL 注入: 使用 f-string 拼接 SQL"
        FAILED=1
    fi
fi

if [ $FAILED -eq 0 ]; then
    echo "✅ SQL 注入风险检查通过"
fi

exit $FAILED

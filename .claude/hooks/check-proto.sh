#!/bin/bash
# Proto 单一源一致性检查（架构加固 Phase 4 起）
#
# 契约单一源：全部权威 .proto 只存于仓库根 proto/；services/cognida-go/api/proto 与 services/cognida-python/proto
# 仅存 buf 回生成的 stub、不得再有手抄 .proto 双份。本检查守护三条不变量：
#   1) 单一源齐备      —— proto/<svc>.proto 存在
#   2) 反漂移（关键）  —— 两处生成目录下无任何手抄 .proto（防止双份复活）
#   3) stub 齐备       —— Go .pb.go 与 Python _pb2.py 已生成到位
# 回生成一致性（buf generate 后 git diff 干净）由 `make proto-check` 在 CI 守护，不在此重跑
# （避免 review hook 依赖网络远程插件）。

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
FAILED=0

echo "🔄 Proto 单一源一致性检查..."

# 含 service 的服务（生成 _grpc stub）
# 注：analytics/evaluation/judge 死 proto 与 common（消息-only）已在死码清理（B1-6）删除，
# 权威单一源现仅剩 docreader、quality 两条活契约。
SERVICES=("docreader" "quality")
ALL=("${SERVICES[@]}")

# 1) 单一源齐备：proto/<name>.proto 必须存在
for name in "${ALL[@]}"; do
    src="$PROJECT_ROOT/proto/${name}.proto"
    if [ ! -f "$src" ]; then
        echo "❌ 单一源缺失: proto/${name}.proto"
        FAILED=1
    fi
done

# 2) 反漂移：两处生成目录下不得再出现手抄 .proto
stray=$(find "$PROJECT_ROOT/services/cognida-go/api/proto" "$PROJECT_ROOT/services/cognida-python/proto" \
    -name "*.proto" 2>/dev/null || true)
if [ -n "$stray" ]; then
    echo "❌ 检出手抄 .proto 双份（应只保留在顶层 proto/，由 buf 回生成 stub）:"
    echo "$stray" | sed 's/^/     /'
    FAILED=1
fi

# 3) stub 齐备：Go .pb.go 与 Python _pb2.py 落点存在
for name in "${ALL[@]}"; do
    go_stub="$PROJECT_ROOT/services/cognida-go/api/proto/${name}/${name}.pb.go"
    py_stub="$PROJECT_ROOT/services/cognida-python/proto/${name}_pb2.py"
    if [ ! -f "$go_stub" ]; then
        echo "⚠️  Go stub 未生成: api/proto/${name}/${name}.pb.go, 运行 cd cognida-go && make proto"
    fi
    if [ ! -f "$py_stub" ]; then
        echo "⚠️  Python stub 未生成: proto/${name}_pb2.py, 运行 cd cognida-go && make proto"
    fi
done

if [ $FAILED -eq 0 ]; then
    echo "✅ Proto 单一源一致性检查通过"
fi

exit $FAILED

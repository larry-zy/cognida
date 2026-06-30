#!/bin/bash
# Proto 文件一致性检查

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
FAILED=0

echo "🔄 Proto 文件一致性检查..."

# 已定义的 proto 文件
PROTO_FILES=("docreader" "evaluation" "ml" "annotation")

# 检查 Go 端 proto 文件
for proto in "${PROTO_FILES[@]}"; do
    proto_file="$PROJECT_ROOT/link-go/api/proto/${proto}.proto"
    if [ ! -f "$proto_file" ]; then
        echo "❌ Proto 文件缺失: ${proto}.proto"
        FAILED=1
    fi
done

# 检查 Python 端生成的代码
for proto in "${PROTO_FILES[@]}"; do
    python_file="$PROJECT_ROOT/link-python/proto/${proto}_pb2.py"
    if [ ! -f "$python_file" ]; then
        echo "⚠️  Python proto 未生成: ${proto}, 运行 cd link-python && python scripts/generate_grpc.py"
    fi
done

if [ $FAILED -eq 0 ]; then
    echo "✅ Proto 文件一致性检查通过"
fi

exit $FAILED

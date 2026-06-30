# CodeReview Hook - Link 项目

## 触发时机

- 提交代码前 (`pre-commit` / `pre-push`)
- CI/CD 流水线
- 人工触发

## 检查维度

1. **语言规范检查**
   - Go 语言规范 (`go-norms.md`)
   - Python 语言规范 (`python-norms.md`)
   - Web 前端规范 (`web-norms.md`)

2. **服务一致性检查**
   - Proto 文件一致性
   - API Contract 一致性
   - 数据模型一致性

3. **架构合规检查**
   - Go 3-Layer 架构
   - Python 分层架构
   - 领域划分

4. **Bug 检测**
   - 编译错误
   - 空指针/None
   - 数据竞争
   - SQL 注入

## 执行脚本

```bash
#!/bin/bash
# Link 项目 CodeReview 检查
# 触发: pre-commit, pre-push, 或手动执行

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PROJECT_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

echo "🔍 Link 项目 CodeReview"
echo "======================="
echo ""

FAILED=0
CHECKS=()

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查函数
run_check() {
    local name=$1
    local command=$2

    echo -e "📋 $name"
    if eval "$command"; then
        echo -e "${GREEN}✅ $name 通过${NC}"
        CHECKS+=("✅ $name")
    else
        echo -e "${RED}❌ $name 失败${NC}"
        CHECKS+=("❌ $name")
        FAILED=1
    fi
    echo ""
}

# ============================================
# 1. 语言规范检查
# ============================================
echo "📖 [1/4] 语言规范检查"
echo "-------------------"

# Go 语言规范
run_check "Go 语言规范" "bash '$SCRIPT_DIR/check-go-norms.sh'"

# Python 语言规范
run_check "Python 语言规范" "bash '$SCRIPT_DIR/check-python-norms.sh'"

# Web 前端规范 (如果有改动)
if git -C "$PROJECT_ROOT" diff --name-only HEAD | grep -q 'link-web/'; then
    run_check "Web 前端规范" "bash '$SCRIPT_DIR/check-web-norms.sh'"
fi

# ============================================
# 2. 服务一致性检查
# ============================================
echo "🔄 [2/4] 服务一致性检查"
echo "---------------------"

# Proto 文件一致性
run_check "Proto 一致性" "bash '$SCRIPT_DIR/check-proto.sh'"

# API Contract 一致性
if git -C "$PROJECT_ROOT" diff --name-only HEAD | grep -q 'link-web/\|link-go/internal/handler/'; then
    run_check "API Contract" "bash '$SCRIPT_DIR/check-api-contract.sh'"
fi

# ============================================
# 3. 架构合规检查
# ============================================
echo "🏗️  [3/4] 架构合规检查"
echo "-------------------"

# Go 架构检查
run_check "Go 架构规范" "bash '$SCRIPT_DIR/check-go-architecture.sh'"

# Python 架构检查
if git -C "$PROJECT_ROOT" diff --name-only HEAD | grep -q 'link-python/'; then
    run_check "Python 架构规范" "bash '$SCRIPT_DIR/check-python-architecture.sh'"
fi

# ============================================
# 4. Bug 检测
# ============================================
echo "🐛 [4/4] Bug 检测"
echo "---------------"

# Go 编译和静态检查
run_check "Go 编译检查" "cd '$PROJECT_ROOT/link-go' && go build ./..."
run_check "Go vet" "cd '$PROJECT_ROOT/link-go' && go vet ./..."

# 数据竞争检测
run_check "数据竞争检测" "cd '$PROJECT_ROOT/link-go' && go test -race ./... -timeout 5m"

# SQL 注入检测
run_check "SQL 注入风险" "bash '$SCRIPT_DIR/check-sql-injection.sh'"

# Python 类型检查
if git -C "$PROJECT_ROOT" diff --name-only HEAD | grep -q 'link-python/'; then
    run_check "Python 类型检查" "cd '$PROJECT_ROOT/link-python' && mypy ./"
fi

# ============================================
# 结果汇总
# ============================================
echo "======================="
echo "📊 检查结果汇总"
echo "======================="

for check in "${CHECKS[@]}"; do
    echo "$check"
done

if [ $FAILED -eq 1 ]; then
    echo ""
    echo -e "${RED}❌ CodeReview 未通过${NC}"
    echo "请修复上述问题后再次提交"
    exit 1
else
    echo ""
    echo -e "${GREEN}✅ CodeReview 全部通过${NC}"
    exit 0
fi
```

## 子脚本定义

### check-go-norms.sh

```bash
#!/bin/bash
# Go 语言规范检查

FAILED=0

# 检查包名
if grep -r '^package.*_' link-go/internal/ | grep -v '_test.go'; then
    echo "❌ 包名不应包含下划线"
    FAILED=1
fi

# 检查接口命名 (I 前缀)
if grep -r 'type I[A-Z]' link-go/internal/; then
    echo "❌ 接口名不应使用 I 前缀"
    FAILED=1
fi

# 检查错误处理
if git -C link-go diff --cached | grep '_, := ' | grep -v '^\s*//'; then
    echo "⚠️  发现忽略的错误 (_ := )"
fi

# 检查 context 作为第一个参数
if grep -r 'func.*Context.*context' link-go/internal/ | grep -v 'ctx context.Context'; then
    echo "⚠️  context.Context 应该是第一个参数"
fi

exit $FAILED
```

### check-proto.sh

```bash
#!/bin/bash
# Proto 文件一致性检查

FAILED=0

# 检查 proto 文件存在
PROTO_FILES=("docreader" "evaluation" "ml" "annotation")
for proto in "${PROTO_FILES[@]}"; do
    if [ ! -f "link-go/api/proto/${proto}.proto" ]; then
        echo "❌ Proto 文件缺失: ${proto}.proto"
        FAILED=1
    fi
done

# 检查 Python proto 生成
for proto in "${PROTO_FILES[@]}"; do
    if [ ! -f "link-python/proto/${proto}_pb2.py" ]; then
        echo "⚠️  Python proto 未生成: ${proto}, 运行 generate_grpc.py"
    fi
done

exit $FAILED
```

### check-sql-injection.sh

```bash
#!/bin/bash
# SQL 注入风险检查

FAILED=0

# 检查 Go 中的字符串拼接 SQL
if grep -rn 'fmt.Sprintf.*SELECT\|fmt.Sprintf.*INSERT\|fmt.Sprintf.*UPDATE\|fmt.Sprintf.*DELETE' link-go/internal/ | grep -v '_test.go'; then
    echo "❌ 发现可能的 SQL 注入: 使用 fmt.Sprintf 拼接 SQL"
    FAILED=1
fi

# 检查 Python 中的 f-string SQL
if grep -rn 'f".*SELECT\|f".*INSERT\|f".*UPDATE\|f".*DELETE' link-python/ | grep -v '_test.py'; then
    echo "❌ 发现可能的 SQL 注入: 使用 f-string 拼接 SQL"
    FAILED=1
fi

exit $FAILED
```

### check-go-architecture.sh

```bash
#!/bin/bash
# Go 架构合规检查

FAILED=0

# 检查 handler 不直接依赖 repository
if grep -r 'link/internal/repository' link-go/internal/handler/ | grep -v '_test.go'; then
    echo "❌ Handler 不应直接依赖 Repository"
    FAILED=1
fi

# 检查 service 不依赖 infrastructure
if grep -r 'link/internal/infrastructure' link-go/internal/service/ | grep -v '_test.go'; then
    echo "❌ Service 不应直接依赖 Infrastructure"
    FAILED=1
fi

# 检查 model 无依赖
if grep -r '^import' link-go/internal/model/ | grep 'link/internal' | grep -v 'model'; then
    echo "❌ Model 不应依赖其他层"
    FAILED=1
fi

# 检查循环依赖
if go list -json ./link-go/... | jq -r 'select(.Deps != null) | .ImportPath' | grep -f - <(go list -json ./link-go/... | jq -r '.ImportPath'); then
    echo "⚠️  检查到可能的循环依赖"
fi

exit $FAILED
```

### check-api-contract.sh

```bash
#!/bash
# API Contract 一致性检查

FAILED=0

# 提取 Go 定义的路径
grep -rh 'POST\|GET\|PUT\|DELETE' link-go/internal/handler/ | grep -o '".*"' | sed 's/"//g' | sort > /tmp/go_paths.txt

# 提取前端调用的路径
grep -rh 'fetch.*api' link-web/src/ 2>/dev/null | grep -o "'/api/[^']*\"" | sed "s/'//g" | sed 's/"//g' | sort > /tmp/web_paths.txt

# 检查是否有差异
if diff -q /tmp/go_paths.txt /tmp/web_paths.txt; then
    echo "✅ API 路径一致"
else
    echo "⚠️  API 路径可能不一致，请检查"
    diff /tmp/go_paths.txt /tmp/web_paths.txt || true
fi

exit $FAILED
```

## Git Hook 配置

在 `.git/hooks/pre-commit`:

```bash
#!/bin/bash
# Pre-commit hook

# 只检查改动的文件
echo "Running CodeReview..."

# 调用主检查脚本
bash .claude/hooks/codereview.sh

exit $?
```

## VS Code 配置

在 `.vscode/settings.json`:

```json
{
    "code-review.onSave": true,
    "code-review.rules": [
        "go-norms",
        "python-norms",
        "architecture-check",
        "bug-detection"
    ]
}
```

## CI/CD 集成

在 `.github/workflows/codereview.yml`:

```yaml
name: CodeReview

on:
  pull_request:
    branches: [main, develop]

jobs:
  codereview:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Run CodeReview
        run: |
          bash .claude/hooks/codereview.sh
```

## 检查优先级

| 检查项 | 优先级 | 阻止提交 |
|--------|--------|----------|
| 编译错误 | P0 | ✅ |
| 数据竞争 | P0 | ✅ |
| SQL 注入 | P0 | ✅ |
| 架构违规 | P1 | ✅ |
| 语言规范 | P2 | ⚠️ |
| 代码风格 | P3 | ❌ |

# 开发指南

## 环境要求

- Python 3.11+
- uv (推荐的包管理器)
- Docker (可选，用于容器化开发)

## 开发环境搭建

### 1. 安装 uv

```bash
# macOS/Linux
curl -LsSf https://astral.sh/uv/install.sh | sh

# Windows
powershell -c "irm https://astral.sh/uv/install.ps1 | iex"
```

### 2. 克隆项目并安装依赖

```bash
cd cognida-python
uv sync --all-extras
```

### 3. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 文件配置所需变量
```

### 4. 安装 pre-commit hooks

```bash
uv run pre-commit install
```

## 开发工具

### 代码格式化和检查

```bash
# 格式化代码
uv run ruff format .

# 检查代码问题
uv run ruff check .

# 类型检查
uv run mypy .
```

### 运行测试

```bash
# 运行所有测试
uv run pytest

# 运行测试并生成覆盖率报告
uv run pytest --cov=.

# 运行特定测试文件
uv run pytest tests/test_health.py
```

### 使用便捷脚本

```bash
# 启动开发服务器
./scripts/dev.sh

# 运行测试
./scripts/test.sh

# 运行代码检查
./scripts/lint.sh
```

## 项目结构

```
cognida-python/
├── api/           # HTTP API 路由
├── config/        # 配置管理
├── core/          # 核心模块 (app, logger, exceptions)
├── grpc/          # gRPC 服务实现
├── mcp/           # MCP 协议实现
├── proto/         # Protobuf 定义
├── services/      # 业务服务
├── tests/         # 测试文件
├── scripts/       # 工具脚本
├── docs/          # 文档
└── main.py        # 应用入口点
```

## 添加新功能

### 1. 创建新的 API 路由

在 `api/routes/` 下创建新文件：

```python
from fastapi import APIRouter
from core import get_logger

router = APIRouter()
logger = get_logger(__name__)

@router.get("/api/v1/feature")
async def get_feature():
    logger.info("Getting feature")
    return {"message": "Hello"}
```

在 `core/app.py` 中注册：

```python
from api.routes import your_feature

app.include_router(your_feature.router, prefix="/api/v1", tags=["feature"])
```

### 2. 创建新的服务

在 `services/` 下创建新模块：

```python
from core import get_logger

logger = get_logger(__name__)

class YourService:
    def __init__(self):
        self.logger = logger

    def do_something(self):
        self.logger.info("Doing something")
        return "done"
```

### 3. 添加配置

在 `config/settings.py` 的 `Settings` 类中添加：

```python
your_feature_enabled: bool = Field(default=True, description="功能开关")
your_feature_api_key: str = Field(default="", description="API 密钥")
```

## 调试

### VS Code 配置

项目已包含 `.vscode/launch.json`，可以直接使用 VS Code 调试：

1. 打开文件
2. 设置断点
3. 按 F5 或点击 "Run and Debug"

### 日志查看

开发环境日志为可读格式，生产环境为 JSON 格式。通过 `LOG_LEVEL` 环境变量控制日志级别。

## 常见问题

### 依赖问题

```bash
# 清理并重新安装
rm -rf .venv uv.lock
uv sync --all-extras
```

### 类型检查失败

```bash
# 安装类型存根
uv run mypy --install-types
```

### 测试失败

```bash
# 查看详细输出
uv run pytest -vvs
```

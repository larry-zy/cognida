# link-python 功能进度

> 更新时间: 2026-05-03  
> 版本: v0.1.0  
> 架构: MCP + gRPC 混合模式

---

## 一、项目概述

link-python 是 Python 工具服务，通过 **MCP + gRPC 混合模式**为 Go 服务提供能力增强。

**核心理念**: Go 是 Agent 的大脑，Python 是能力工具箱。

---

## 二、整体进度

```
总体进度: ████████░░░░░░░░░░░  40%

├── 基础设施    ████████████████████  100%
├── MCP 协议    ████████████████░░░░   80%
├── gRPC 协议   ██████░░░░░░░░░░░░░░   30%
├── 工具实现    ████████░░░░░░░░░░░░   40%
└── 测试覆盖    ████████░░░░░░░░░░░░   40%
```

---

## 三、已完成功能 ✅

### 3.1 基础设施 (100%)

| 模块 | 文件 | 状态 |
|------|------|------|
| 项目结构 | 标准目录结构 | ✅ |
| 包管理 | `pyproject.toml` (uv) | ✅ |
| 配置管理 | `config/settings.py` (pydantic) | ✅ |
| 日志系统 | `core/logger.py` (structlog) | ✅ |
| 异常处理 | `core/exceptions.py` | ✅ |
| 代码质量 | ruff + mypy 配置 | ✅ |
| 测试框架 | pytest + pytest-cov + pytest-asyncio | ✅ |
| CI/CD | GitHub Actions workflow | ✅ |
| 容器化 | Dockerfile + docker-compose.yml | ✅ |
| 开发工具 | VS Code 配置 + scripts | ✅ |

### 3.2 MCP 协议层 (80%)

| 功能 | 文件 | 状态 |
|------|------|------|
| MCP Server | `mcp/server.py` | ✅ |
| 请求处理器 | `mcp/handlers.py` | ✅ |
| `tools/list` | MCP 标准方法 | ✅ |
| `tools/call` | MCP 标准方法 | ✅ |
| stdio 模式 | 子进程通信 | ✅ |
| HTTP 模式 | 待实现 | ⏳ |

### 3.3 gRPC 协议层 (30%)

| 功能 | 文件 | 状态 |
|------|------|------|
| gRPC Server | `grpc/server.py` | ✅ |
| Servicer 框架 | `grpc/servicer.py` | ✅ |
| Proto 定义 | `proto/python_tools.proto` | ✅ |
| 协议生成 | 需运行 `grpc_tools.protoc` | ⏳ |
| 具体服务实现 | 待完成 | ⏳ |

### 3.4 服务层 (60%)

| 服务 | 文件 | 状态 |
|------|------|------|
| LLM 客户端基类 | `services/llm/base.py` | ✅ |
| OpenAI 实现 | `services/llm/openai.py` | ✅ |
| 其他 LLM | litellm 集成 | ⏳ |
| 存储客户端 | 待实现 | ⏳ |

### 3.5 工具实现 (40%)

#### 评测工具 (MCP)

| 工具 | 文件 | 状态 |
|------|------|------|
| 工具基类 | `tools/base.py` | ✅ |
| 工具注册 | `tools/registry.py` | ✅ |
| LLM 裁判 | `tools/evaluation/llm_judge.py` | ✅ |
| 自定义指标 | 待实现 | ⏳ |
| 语义相似度 | 待实现 | ⏳ |
| 忠实度评估 | 待实现 | ⏳ |

#### 文档工具 (MCP)

| 工具 | 状态 | 优先级 |
|------|------|--------|
| OCR 处理 | ⏳ | 高 |
| PDF 解析 | ⏳ | 高 |
| 表格解析 | ⏳ | 中 |
| 图像提取 | ⏳ | 中 |

#### 图像工具 (gRPC)

| 工具 | 状态 | 优先级 |
|------|------|--------|
| 图像向量化 | ⏳ | 高 |
| 以图搜图 | ⏳ | 中 |
| 批量处理 | ⏳ | 中 |

#### 数据工具 (gRPC)

| 工具 | 状态 | 优先级 |
|------|------|--------|
| 批量收集 | ⏳ | 高 |
| 流式导出 | ⏳ | 高 |
| 向量检索 | ⏳ | 中 |

### 3.6 HTTP API (调试用) (100%)

| 功能 | 文件 | 状态 |
|------|------|------|
| FastAPI 应用 | `core/app.py` | ✅ |
| 健康检查 | `api/routes/health.py` | ✅ |
| 示例接口 | `api/routes/example.py` | ✅ |
| 自动文档 | `/docs` | ✅ |
| 异常处理 | 统一错误响应 | ✅ |

### 3.7 测试 (40%)

| 测试 | 文件 | 状态 |
|------|------|------|
| 测试配置 | `tests/conftest.py` | ✅ |
| API 测试 | `tests/test_api.py`, `tests/test_health.py` | ✅ |
| 工具测试 | `tests/test_llm_judge.py` | ✅ |
| 注册中心测试 | `tests/test_registry.py` | ✅ |
| MCP 测试 | `tests/test_mcp_server.py` | ✅ |
| gRPC 测试 | 待实现 | ⏳ |

---

## 四、待实现功能 ⏳

### 4.1 高优先级

#### MCP 工具

```
⏳ semantic_similarity  - 语义相似度计算
⏳ ocr_processor       - OCR 文字识别
⏳ pdf_parser          - PDF 文档解析
```

#### gRPC 工具

```
⏳ image_embedder      - 图像向量化
⏳ batch_collector     - 批量数据收集（流式）
```

### 4.2 中优先级

#### MCP 工具

```
⏳ custom_metric       - 自定义评测指标
⏳ faithfulness       - 忠实度评估
⏳ data_cleaner       - 数据清洗
⏳ dataset_recommender - 数据集推荐
```

#### gRPC 工具

```
⏳ image_search        - 以图搜图
⏳ vector_search       - 高性能向量检索
⏳ streaming_export    - 流式数据导出
```

### 4.3 低优先级

```
⏳ dataset_merger      - 数据集合并
⏳ batch_inference     - 批量模型推理
⏳ batch_resize        - 批量图像处理
```

---

## 五、技术债务 🔧

| 项目 | 说明 | 优先级 |
|------|------|--------|
| Proto 生成 | 需要生成 Python gRPC 代码 | 高 |
| 类型标注 | 部分 module 缺少类型标注 | 中 |
| 错误处理 | 需要细化异常分类 | 中 |
| 文档 | API 文档需要补充 | 低 |

---

## 六、开发计划 📅

### Phase 1: 完善 MCP 工具 (1-2周)

- [ ] 实现 `semantic_similarity` 工具
- [ ] 实现 `ocr_processor` 工具
- [ ] 实现 `pdf_parser` 工具
- [ ] 添加 MCP HTTP 模式

### Phase 2: 实现 gRPC 工具 (2-3周)

- [ ] 生成 gRPC Python 代码
- [ ] 实现 `image_embedder` 工具
- [ ] 实现 `batch_collector` 工具（流式）
- [ ] 实现 `vector_search` 工具

### Phase 3: 集成测试 (1周)

- [ ] Go 端 MCP 客户端集成
- [ ] Go 端 gRPC 客户端集成
- [ ] 端到端测试
- [ ] 性能测试

### Phase 4: 生产部署 (1周)

- [ ] Docker 镜像优化
- [ ] 监控和日志
- [ ] 健康检查完善
- [ ] 文档完善

---

## 七、文件清单

### 核心文件

```
link-python/
├── config/
│   ├── __init__.py           ✅ 配置模块
│   └── settings.py           ✅ 配置定义
│
├── core/
│   ├── __init__.py           ✅ 核心模块
│   ├── app.py                ✅ FastAPI 应用
│   ├── exceptions.py         ✅ 异常处理
│   └── logger.py             ✅ 日志配置
│
├── mcp/
│   ├── __init__.py           ✅ MCP 模块
│   ├── server.py             ✅ MCP Server
│   └── handlers.py           ✅ MCP 处理器
│
├── grpc/
│   ├── __init__.py           ✅ gRPC 模块
│   ├── server.py             ✅ gRPC Server
│   └── servicer.py           ✅ gRPC Servicer
│
├── tools/
│   ├── __init__.py           ✅ 工具模块
│   ├── base.py               ✅ 工具基类
│   ├── registry.py           ✅ 工具注册
│   └── evaluation/
│       ├── __init__.py       ✅ 评测模块
│       └── llm_judge.py      ✅ LLM 裁判
│
├── services/
│   ├── __init__.py           ✅ 服务模块
│   └── llm/
│       ├── __init__.py       ✅ LLM 模块
│       ├── base.py           ✅ LLM 基类
│       └── openai.py         ✅ OpenAI 实现
│
├── api/routes/
│   ├── __init__.py           ✅ 路由模块
│   ├── health.py             ✅ 健康检查
│   └── example.py            ✅ 示例接口
│
├── tests/
│   ├── __init__.py           ✅ 测试模块
│   ├── conftest.py           ✅ 测试配置
│   ├── test_api.py           ✅ API 测试
│   ├── test_health.py        ✅ 健康检查测试
│   ├── test_llm_judge.py     ✅ LLM 评测测试
│   ├── test_mcp_server.py    ✅ MCP 测试
│   └── test_registry.py      ✅ 注册中心测试
│
├── src/
│   ├── __init__.py           ✅ 源码模块
│   └── main.py               ✅ 入口文件
│
├── proto/
│   └── python_tools.proto    ✅ gRPC 协议定义
│
├── models/
│   └── __init__.py           ✅ 数据模型
│
├── pyproject.toml            ✅ 项目配置
├── README.md                 ✅ 项目文档
├── .env.example              ✅ 环境变量模板
├── Dockerfile                ✅ 容器定义
├── docker-compose.yml        ✅ 编排配置
└── .github/workflows/ci.yml  ✅ CI/CD
```

---

## 八、快速参考

### 启动命令

```bash
# MCP Server
uv run python -m mcp_service.server

# gRPC Server  
uv run python -m grpc.server

# HTTP API (调试)
uv run uvicorn img.main:app --reload

# 测试
uv run pytest --cov=.

# 代码检查
./scripts/lint.sh
```

### 端口分配

| 服务 | 端口 | 协议 |
|------|------|------|
| MCP HTTP | 3000 | HTTP |
| gRPC | 50051 | gRPC |
| HTTP API | 8000 | HTTP |

### 环境变量

```bash
# 应用
APP_NAME=link-python-tools
APP_ENV=dev
LOG_LEVEL=INFO

# MCP
MCP_MODE=stdio
MCP_PORT=3000

# gRPC
GRPC_ENABLED=true
GRPC_PORT=50051

# LLM
LLM_PROVIDER=openai
LLM_API_KEY=your-key
LLM_MODEL=gpt-4
```

---

**文档维护**: 请在每次功能更新后同步更新本文档。

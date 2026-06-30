# Link-Python 服务架构设计文档

## 文档说明

本文档描述 Link 系统中 Python 服务的架构设计。

**核心理念**：Go 是 Agent 的大脑，Python 是能力增强的工具箱。

| 职责 | Go 服务 | Python 服务 |
|-----|---------|------------|
| Agent 逻辑 | ✅ 全部 | ❌ 无 |
| 编排决策 | ✅ 全部 | ❌ 无 |
| 工具实现 | 简单工具 | 复杂工具 |

**通信方式**: MCP + gRPC 混合模式

**版本**: v5.0
**更新时间**: 2026-05-05
**变更**: 新增 MCP 集成方案，详见 `docs/mcp-integration-architecture.md`

---

## 目录

- [一、架构理念](#一架构理念)
- [二、整体架构](#二整体架构)
- [三、协议选择](#三协议选择)
- [四、MCP 协议](#四mcp-协议)
- [五、gRPC 协议](#五grpc-协议)
- [六、Python 提供的工具](#六python-提供的工具)
- [七、项目结构](#七项目结构)
- [八、技术选型](#八技术选型)
- [九、部署方案](#九部署方案)

---

## 一、架构理念

### 1.1 设计原则

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              核心设计理念                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   Go 服务 = Agent 大脑                                                       │
│   • 所有 Agent 定义                                                         │
│   • 所有编排逻辑                                                            │
│   • 所有决策能力                                                            │
│   • 简单工具实现                                                            │
│                                                                             │
│   Python 服务 = 能力工具箱                                                   │
│   • 通过双协议暴露工具                                                       │
│   • 不涉及 Agent 逻辑                                                       │
│   • 专注重计算任务                                                          │
│                                                                             │
│   通信方式: MCP + gRPC 混合模式                                              │
│   • MCP: AI 工具调用，标准协议，易于集成                                     │
│   • gRPC: 高性能任务，二进制协议，流式支持                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 为什么混合模式？

| 需求 | 推荐协议 | 理由 |
|------|---------|------|
| Agent 工具调用 | MCP | 标准化工具发现，AI 原生 |
| LLM 评测 | MCP | 低频调用，JSON 友好 |
| 文档解析 | MCP | 中等复杂度，调试方便 |
| 图像向量化 | gRPC | 大数据传输，二进制高效 |
| 批量数据收集 | gRPC | 流式处理，高性能 |
| 向量检索 | gRPC | 低延迟，高频调用 |

### 1.3 架构优势

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            混合模式优势                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   1. 灵活性: 根据场景选择最优协议                                            │
│   2. 性能: gRPC 处理高性能任务                                               │
│   3. 标准: MCP 提供标准化工具接口                                             │
│   4. 兼容: 两种协议可以并存，不影响现有架构                                    │
│   5. 扩展: 新工具可以选择最适合的协议                                         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 二、整体架构

### 2.1 系统架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              用户请求                                        │
└─────────────────────────────────────┬───────────────────────────────────────┘
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Go 服务                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        Agent 层                                       │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │   │
│  │  │   Planner   │  │  Retriever  │  │   Analyzer  │                 │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │   │
│  │  │ Synthesizer │  │    Critic   │  │  Deep       │                 │   │
│  │  │             │  │             │  │  Research   │                 │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        工具层                                         │   │
│  │  ┌───────────────┐  ┌───────────────────┐  ┌───────────────────┐    │   │
│  │  │ Go内置工具    │  │   工具路由层       │  │  其他工具          │    │   │
│  │  │ - rag_query   │  │                   │  │ - calculator       │    │   │
│  │  │ - web_search  │  │  根据工具类型      │  │ - get_time         │    │   │
│  │  │ - graph_query │  │  选择协议          │  │                    │    │   │
│  │  └───────────────┘  │                   │  └───────────────────┘    │   │
│  │                     │  ┌─────────┬──────┴──────┐                    │   │
│  │                     │  ▼         ▼             ▼                    │   │
│  │                     │ MCP      gRPC        直接调用                  │   │
│  │                     │          │              │                    │   │
│  │  ┌──────────────────┴──┴──────────┴──────────────┴────────────────┐ │   │
│  │  │                      协议选择策略                               │ │   │
│  │  │  • AI 工具调用         → MCP                                  │ │   │
│  │  │  • 高性能/流式任务    → gRPC                                 │ │   │
│  │  │  • 简单工具           → Go 内置实现                            │ │   │
│  │  └───────────────────────────────────────────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                ┌──────────────────────┴─────────────────────┐
                ▼                                           ▼
┌───────────────────────────────┐       ┌───────────────────────────────┐
│    Python MCP Server          │       │    Python gRPC Server         │
│    (AI 工具调用)               │       │    (高性能任务)               │
│                               │       │                               │
│  端口: stdio / 3000           │       │  端口: 50051                  │
│                               │       │                               │
│  • llm_judge                 │       │  • image_embedder            │
│  • semantic_similarity       │       │  • batch_collector           │
│  • ocr_processor             │       │  • vector_search             │
│  • data_cleaner              │       │  • streaming_export          │
│  • dataset_recommender       │       │  • batch_inference           │
│                               │       │                               │
│  标准协议:                     │       │  高性能:                      │
│  • tools/list                │       │  • Protobuf 二进制            │
│  • tools/call                │       │  • 双向流式 RPC               │
│  • resources/list            │       │  • 低延迟连接                 │
│                               │       │                               │
└───────────────────────────────┘       └───────────────────────────────┘
```

### 2.2 工具分类与协议映射

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           工具协议映射表                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   MCP 工具 (AI 友好，标准接口)                                              │
│   ┌──────────────────────────────────────────────────────────────────┐     │
│   │ 评测工具:                                                         │     │
│   │   • llm_judge          - LLM 裁判评分                            │     │
│   │   • custom_metric      - 自定义评测指标                          │     │
│   │   • semantic_similarity - 语义相似度                             │     │
│   │   • faithfulness       - 忠实度评估                              │     │
│   │                                                                 │     │
│   │ 文档工具:                                                         │     │
│   │   • ocr_processor      - OCR 文字识别                            │     │
│   │   • pdf_parser         - PDF 解析                                │     │
│   │   • table_parser       - 表格解析                                │     │
│   │                                                                 │     │
│   │ 知识工具:                                                         │     │
│   │   • dataset_recommender - 数据集推荐                             │     │
│   │   • dataset_merger     - 数据集合并                              │     │
│   └──────────────────────────────────────────────────────────────────┘     │
│                                                                             │
│   gRPC 工具 (高性能，流式处理)                                              │
│   ┌──────────────────────────────────────────────────────────────────┐     │
│   │ 图像工具:                                                         │     │
│   │   • image_embedder     - 图像向量化（大数据）                      │     │
│   │   • image_search       - 以图搜图（向量检索）                      │     │
│   │   • batch_resize       - 批量图像处理                             │     │
│   │                                                                 │     │
│   │ 数据工具:                                                         │     │
│   │   • batch_collector    - 批量数据收集（流式）                     │     │
│   │   • streaming_export   - 流式数据导出                            │     │
│   │                                                                 │     │
│   │ 推理工具:                                                         │     │
│   │   • batch_inference   - 批量模型推理                             │     │
│   │   • vector_search      - 高性能向量检索                          │     │
│   └──────────────────────────────────────────────────────────────────┘     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 调用流程示例

```
场景 1: Agent 需要评估答案质量 (MCP)
────────────────────────────────────────
Go Agent → Thought: "需要评估答案"
          → Action: MCP Client.tools/call("llm_judge")
          → Python MCP Server
          → 返回评分结果
          → Observation: "评分 8.5/10"

场景 2: 批量图像向量化 (gRPC)
────────────────────────────────────────
Go Agent → Thought: "需要处理100张图片"
          → Action: gRPC Client.ImageEmbeddingBatch()
          → Python gRPC Server (流式接收)
          → 返回 embedding 流
          → Observation: "处理完成，100个向量"
```

---

## 三、协议选择

### 3.1 对比分析

| 维度 | MCP | gRPC |
|------|-----|------|
| **性能** | ⭐⭐⭐ JSON 文本 | ⭐⭐⭐⭐⭐ 二进制高效 |
| **AI 集成** | ⭐⭐⭐⭐⭐ 原生设计 | ⭐⭐ 需适配层 |
| **工具发现** | ⭐⭐⭐⭐⭐ 标准内置 | ⭐⭐ 需自定义 |
| **调试难度** | ⭐⭐⭐⭐⭐ JSON 可读 | ⭐ 需专用工具 |
| **生态成熟度** | ⭐⭐⭐ 新兴协议 | ⭐⭐⭐⭐⭐ 非常成熟 |
| **流式支持** | ⭐⭐⭐ 有限 | ⭐⭐⭐⭐⭐ 双向流 |
| **类型安全** | ⭐⭐⭐ Schema 验证 | ⭐⭐⭐⭐⭐ 强类型 |
| **跨语言** | ⭐⭐⭐⭐ 手动实现 | ⭐⭐⭐⭐⭐ 代码生成 |

### 3.2 选择决策树

```
                    需要调用 Python 工具？
                              │
                              ▼
                    ┌─────────────────────┐
                    │  是否需要高性能？    │
                    └─────────────────────┘
                      │                 │
                     是                 否
                      │                 │
                      ▼                 ▼
              ┌───────────────┐   ┌───────────────┐
              │ 数据量 > 1MB? │   │  是 AI 工具？  │
              └───────────────┘   └───────────────┐
                │           │       │           │
               是          否      是          否
                │           │       │           │
                ▼           ▼       ▼           ▼
              gRPC        MCP     MCP          gRPC
            (大数据)    (中低频) (标准工具)   (其他)
```

### 3.3 推荐场景

#### 使用 MCP 的场景

```
✅ AI Agent 工具调用
✅ 需要动态工具发现
✅ 低频、中低数据量调用
✅ 需要快速迭代开发
✅ 多 AI 框架集成

典型工具:
• LLM 评测 (llm_judge)
• 文档解析 (pdf_parser, ocr_processor)
• 数据清洗 (data_cleaner)
• 知识管理 (dataset_recommender)
```

#### 使用 gRPC 的场景

```
✅ 大数据传输 (>1MB)
✅ 流式数据处理
✅ 高频调用 (QPS > 100)
✅ 需要低延迟 (<10ms)
✅ 双向流式通信

典型工具:
• 图像向量化 (image_embedder)
• 批量数据收集 (batch_collector)
• 向量检索 (vector_search)
• 批量推理 (batch_inference)
```

---

## 四、MCP 协议

### 4.1 MCP 标准方法

| 方法 | 描述 | 返回值 |
|------|------|--------|
| `tools/list` | 列出所有可用工具 | `{"tools": [...]}` |
| `tools/call` | 调用指定工具 | `{"content": [...]}` |
| `resources/list` | 列出可用资源 | `{"resources": [...]}` |
| `resources/read` | 读取资源内容 | `{"contents": [...]}` |
| `prompts/list` | 列出提示词模板 | `{"prompts": [...]}` |

### 4.2 MCP 工具描述格式

```json
{
  "name": "llm_judge",
  "description": "使用LLM作为裁判评估答案质量",
  "inputSchema": {
    "type": "object",
    "properties": {
      "question": {"type": "string"},
      "generated": {"type": "string"},
      "reference": {"type": "string"}
    },
    "required": ["question", "generated", "reference"]
  }
}
```

---

## 五、gRPC 协议

### 5.1 gRPC 服务定义

```protobuf
service HighPerformanceTools {
    // 图像工具
    rpc ImageEmbeddingBatch(stream ImageRequest) returns (stream EmbeddingResponse);
    rpc ImageSearch(SearchRequest) returns (SearchResponse);

    // 数据工具
    rpc BatchCollect(CollectRequest) returns (stream CollectProgress);
    rpc StreamingExport(ExportRequest) returns (stream ExportChunk);

    // 推理工具
    rpc BatchInference(stream InferenceRequest) returns (stream InferenceResponse);
    rpc VectorSearch(VectorRequest) returns (VectorResponse);
}
```

### 5.2 gRPC 特性

| 特性 | 说明 |
|------|------|
| **二进制协议** | Protobuf 编码，性能优异 |
| **双向流** | 支持双向流式 RPC |
| **代码生成** | 自动生成客户端/服务端代码 |
| **负载均衡** | 原生支持 gRPC 负载均衡 |
| **压缩** | 内置消息压缩 |

---

## 六、Python 提供的工具

### 6.1 工具分类

| 分类 | 工具 | 协议 | 调用方式 |
|-----|------|------|---------|
| **评测工具** | llm_judge | MCP | `tools/call` |
| | custom_metric | MCP | `tools/call` |
| | semantic_similarity | MCP | `tools/call` |
| **文档处理** | ocr_processor | MCP | `tools/call` |
| | pdf_parser | MCP | `tools/call` |
| | table_parser | MCP | `tools/call` |
| **知识工具** | dataset_recommender | MCP | `tools/call` |
| | dataset_merger | MCP | `tools/call` |
| **图像工具** | image_embedder | gRPC | `ImageEmbedding` |
| | batch_resize | gRPC | `BatchResize` |
| | image_search | gRPC | `ImageSearch` |
| **数据工具** | batch_collector | gRPC | `BatchCollect` (流式) |
| | streaming_export | gRPC | `StreamingExport` (流式) |
| **推理工具** | batch_inference | gRPC | `BatchInference` (流式) |
| | vector_search | gRPC | `VectorSearch` |

---

## 七、项目结构

### 7.1 Python 服务目录结构

```
link-python/
├── proto/                              # gRPC 协议定义
│   └── high_performance_tools.proto   # 高性能工具协议
│
├── link_python/
│   ├── __init__.py
│   │
│   ├── mcp/                            # MCP 协议层
│   │   ├── __init__.py
│   │   ├── server.py                   # MCP Server 实现
│   │   └── handlers.py                 # MCP 请求处理器
│   │
│   ├── grpc/                           # gRPC 协议层
│   │   ├── __init__.py
│   │   ├── server.py                   # gRPC Server 实现
│   │   └── servicer.py                 # gRPC 服务实现
│   │
│   ├── tools/                          # 工具实现（核心）
│   │   ├── __init__.py
│   │   ├── base.py                     # 工具基类
│   │   ├── registry.py                 # 工具注册
│   │   │
│   │   ├── evaluation/                 # 评测工具 (MCP)
│   │   │   ├── __init__.py
│   │   │   └── llm_judge.py
│   │   │
│   │   ├── document/                   # 文档工具 (MCP)
│   │   │   ├── __init__.py
│   │   │   ├── ocr.py
│   │   │   └── pdf_parser.py
│   │   │
│   │   ├── image/                      # 图像工具 (gRPC)
│   │   │   ├── __init__.py
│   │   │   ├── embedder.py
│   │   │   └── search.py
│   │   │
│   │   └── data/                       # 数据工具 (gRPC)
│   │       ├── __init__.py
│   │       ├── batch_collector.py
│   │       └── streaming_export.py
│   │
│   ├── services/                       # 服务层
│   │   ├── llm/                        # LLM 客户端
│   │   ├── storage/                    # 存储客户端
│   │   └── models/                     # 模型服务
│   │
│   └── config/                         # 配置
│       └── settings.py
│
├── tests/                              # 测试
│   ├── test_mcp_tools.py
│   ├── test_grpc_tools.py
│   └── test_integration.py
│
├── pyproject.toml
├── README.md
└── .env.example
```

### 7.2 Go 服务集成

```
link-go/internal/
├── infrastructure/
│   ├── mcp/                            # MCP 客户端
│   │   ├── client.go
│   │   └── stdio_transport.go
│   │
│   └── grpc/                           # gRPC 客户端
│       ├── python_client.go
│       └── connection_pool.go
│
└── application/
    └── agent/
        └── tools/
            ├── router.go               # 工具路由层
            ├── mcp_tools.go            # MCP 工具封装
            └── grpc_tools.go           # gRPC 工具封装
```

---

## 八、技术选型

### 8.1 Python 技术栈

| 类别 | 技术选型 | 说明 |
|-----|---------|------|
| **MCP SDK** | mcp-python | MCP 官方 SDK |
| **gRPC** | grpcio | Google 官方库 |
| **LLM 客户端** | litellm | 统一 LLM 接口 |
| **向量模型** | sentence-transformers | 语义相似度 |
| **图像模型** | CLIP, torch | 图像向量化 |
| **配置管理** | pydantic-settings | 类型安全配置 |
| **日志** | structlog | 结构化日志 |

### 8.2 Go 技术栈

| 类别 | 技术选型 | 说明 |
|-----|---------|------|
| **MCP SDK** | mcp-go | MCP Go 客户端 |
| **gRPC** | grpc-go | gRPC Go 官方库 |
| **Protobuf** | protoc-gen-go | 代码生成 |

---

## 九、部署方案

### 9.1 开发环境

```bash
# 安装依赖
uv sync --all-extras

# 配置环境变量
cp .env.example .env

# 启动 MCP Server (stdio)
uv run python -m link_python.mcp.server

# 启动 gRPC Server (另一个终端)
uv run python -m link_python.grpc.server
```

### 9.2 生产部署

```yaml
# docker-compose.yml
version: '3.8'
services:
  link-go:
    build: ./link-go
    ports:
      - "8080:8080"
    environment:
      - PYTHON_MCP_SERVER=link-python:3000
      - PYTHON_GRPC_SERVER=link-python:50051
    depends_on:
      - link-python

  link-python:
    build: ./link-python
    ports:
      - "3000:3000"   # MCP HTTP
      - "50051:50051" # gRPC
    environment:
      - MCP_MODE=http
      - MCP_PORT=3000
      - GRPC_PORT=50051
    command: >
      sh -c "
        uv run python -m link_python.mcp.server --port 3000 &
        uv run python -m link_python.grpc.server --port 50051
      "
```

---

## 附录

### A. 版本历史

| 版本 | 日期 | 变更 |
|-----|------|------|
| v1.0 | 2026-05-03 | 初版（Agent在Python） |
| v2.0 | 2026-05-03 | 重构：Agent在Go，Python只做工具 |
| v3.0 | 2026-05-03 | 通信协议改为 MCP |
| v4.0 | 2026-05-03 | MCP + gRPC 混合模式 |
| v5.0 | 2026-05-05 | 新增 MCP 集成详细方案，见 `mcp-integration-architecture.md` |

---

**文档版本**: v5.0
**更新时间**: 2026-05-05

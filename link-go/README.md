# Link Go 服务

Link 智能数据系统的 Go 服务端，提供 API、编排、实时处理等核心能力。

## 架构概览

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              API Gateway (HTTP/WebSocket)                      │
└─────────────────────────────────────────────────────────────────────────────────┘
                                              │
┌─────────────────────────────────────────────┼─────────────────────────────────────┐
│                                             ▼                                     │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │                          Application Layer                                │  │
│  │  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐              │  │
│  │  │   Chat    │  │   Agent   │  │    RAG    │  │     KB    │  Use Cases  │  │
│  │  │  Service  │  │  Service  │  │  Service  │  │  Service  │              │  │
│  │  └───────────┘  └───────────┘  └───────────┘  └───────────┘              │  │
│  │  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐              │  │
│  │  │Evaluation │  │  Skill    │  │ Text2SQL  │  │ Pipeline  │              │  │
│  │  │  Service  │  │  Service  │  │  Service  │  │  Service  │              │  │
│  │  └───────────┘  └───────────┘  └───────────┘  └───────────┘              │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                             │                                     │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │                             Domain Layer                                  │  │
│  │  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐              │  │
│  │  │   Agent   │  │  Document │  │ Knowledge │  │   Skill   │  Entities    │  │
│  │  │   Domain  │  │   Domain  │  │   Base    │  │   Types   │              │  │
│  │  └───────────┘  └───────────┘  └───────────┘  └───────────┘              │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐     │  │
│  │  │          Agent Tools (Eino Tool Framework)                      │     │  │
│  │  │  ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────────┐     │     │  │
│  │  │  │ rag_query │ │sql_execute│ │skill_invoke│ │  skill_list   │     │     │  │
│  │  │  └───────────┘ └───────────┘ └───────────┘ └───────────────┘     │     │  │
│  │  │  ┌───────────┐ ┌───────────┐ ┌───────────┐                     │     │  │
│  │  │  │graph_query│ │web_search │ │get_schema │                     │     │  │
│  │  │  └───────────┘ └───────────┘ └───────────┘                     │     │  │
│  │  └─────────────────────────────────────────────────────────────────┘     │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                             │                                     │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │                         Infrastructure Layer                              │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │  │
│  │  │    MySQL    │  │   Milvus    │  │   Neo4j     │  │   Redis     │Storage│  │
│  │  │  Repository │  │  Repository │  │  Repository │  │  Repository │      │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘     │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │  │
│  │  │    LLM      │  │   gRPC      │  │    MCP      │  │   HTTP      │External│ │
│  │  │  Clients    │  │  Clients    │  │   Client    │  │   Client    │Services│ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘     │  │
│  │                              ┌─────────────┐                            │  │
│  │                              │   Skill     │ ──────────►                │  │
│  │                              │   Client    │  Python MCP Server         │  │
│  │                              └─────────────┘                            │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                              │
                    ┌─────────────────────────┼─────────────────────────┐
                    │                         │                         │
                    ▼                         ▼                         ▼
        ┌───────────────────┐   ┌───────────────────┐   ┌───────────────────┐
        │  Python gRPC      │   │  Python MCP       │   │   Database        │
        │  Services         │   │  Server           │   │   Cluster         │
        │  ┌─────────────┐  │   │  ┌─────────────┐  │   │  ┌─────────────┐  │
        │  │  Document   │  │   │  │ Data Analysis│  │   │  │   MySQL     │  │
        │  │  Service    │  │   │  │ ML Models    │  │   │  │   Milvus    │  │
        │  │  (PDF/Word) │  │   │  │ Custom Skills│  │   │  │   Neo4j     │  │
        │  └─────────────┘  │   │  └─────────────┘  │   │  └─────────────┘  │
        └───────────────────┘   └───────────────────┘   └───────────────────┘
```

## 目录结构

```
link-go/
├── cmd/                    # 应用程序入口
│   ├── server/            # HTTP 服务器
│   └── wire/              # Wire 依赖注入配置
├── internal/
│   ├── interface/         # 接口层（HTTP handlers）
│   ├── application/       # 应用层（用例编排）
│   │   └── usecases/
│   │       ├── agent/     # Agent 用例
│   │       ├── rag/       # RAG 用例
│   │       ├── kb/        # 知识库用例
│   │       └── evaluation/# 评测用例
│   ├── domain/            # 领域层（实体、接口）
│   │   ├── agent/
│   │   ├── rag/
│   │   └── kb/
│   └── infrastructure/   # 基础设施层（外部依赖）
│       ├── persistence/  # 数据库实现
│       ├── llm/          # LLM 客户端
│       ├── skill/        # Skill MCP 客户端
│       └── config/       # 配置管理
├── api/                   # API 定义
│   ├── proto/            # gRPC Proto 文件
│   └── http/             # HTTP API 规范
└── docs/                  # 文档
```

## 核心功能

### 1. Agent 系统

- **ReAct 编排**：支持推理-行动循环
- **Deep Research**：深度研究模式
- **Multi-Agent 协作**：Agent 间委托与协作
- **反思机制**：基于 Hook 的反思与自我修正
- **会话洞察**：结论生成、数据分析洞察
- **工具调用**：统一的 Tool 抽象，支持 Eino 框架

### 2. RAG 系统

- **向量检索**：基于 Milvus 的语义检索
- **图谱检索**：基于 Neo4j 的知识图谱检索
- **混合检索**：向量 + 图谱融合排序
- **文档处理**：支持 PDF、Word、Excel、网页等

### 3. 知识库管理

- **知识库 CRUD**：创建、查询、更新、删除
- **文档管理**：上传、解析、分块
- **版本控制**：文档版本追踪
- **权限控制**：租户隔离

### 4. Text2SQL 系统

- **自然语言查询**：将自然语言转换为 SQL
- **Schema 感知**：自动获取数据库结构
- **多轮对话**：支持追问和结果修正
- **安全执行**：只读查询、自动限制结果集
- **Agent 集成**：`agent-text2sql-001` 内置 Agent

### 4. Text2SQL 系统

专门的自然语言转 SQL 系统，支持多轮对话查询数据库。

| 特性 | 说明 |
|------|------|
| **自然语言转 SQL** | 理解用户查询意图，生成对应 SQL |
| **Schema 感知** | 自动获取数据库表结构 |
| **多轮对话** | 支持追问、排序、过滤等交互 |
| **安全执行** | 只允许 SELECT 查询，自动 LIMIT |
| **结果解释** | 用友好的中文解释查询结果 |

#### 工具列表

| 工具名 | 功能 | 参数 |
|--------|------|------|
| `sql_execute` | 执行 SQL 查询 | `sql: 查询语句`, `max_rows: 最大行数` |
| `get_schema` | 获取数据库结构 | `table_name: 表名（可选）` |

#### 使用示例

```go
import (
    "link/internal/service/agent/initializer"
)

// 1. Text2SQL Agent 已在 init.go 中自动注册
// Agent ID: agent-text2sql-001

// 2. 使用 Text2SQL Agent
agent := agentRegistry.Get("agent-text2sql-001")

// 简单查询
response, _ := agent.Chat(ctx, "查询销售额最高的前10个产品")

// 追问
response, _ = agent.Chat(ctx, "按地区分组统计")

// 排序
response, _ = agent.Chat(ctx, "按销售额降序排列")
```

---

### 7. Python Skill 集成

Go 端已集成 Python Skill 系统，允许 Agent 调用 Python 实现的动态能力。

#### 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                        Go Agent                              │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ RAG Query   │  │ SQL Query   │  │   Skill Invoke      │ │
│  │   Tool      │  │    Tool     │  │      Tool           │ │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────────────┘ │
│         │                │                 │                 │
│         └────────────────┴─────────────────┘                 │
│                           │                                   │
│                   ┌───────▼────────┐                         │
│                   │  Tool Registry │                         │
│                   └───────┬────────┘                         │
│                           │                                   │
┌───────────────────────────┼───────────────────────────────────┐
│                    MCP Protocol                             │
│                           │                                   │
│                   ┌───────▼────────┐                         │
│                   │  Skill Client  │                         │
│                   │   (Go Side)    │                         │
│                   └───────┬────────┘                         │
└───────────────────────────┼───────────────────────────────────┘
                            │ HTTP/stdio
┌───────────────────────────┼───────────────────────────────────┐
│                    Python MCP Server                          │
│                           │                                   │
│                   ┌───────▼────────┐                         │
│                   │  Skill Manager │                         │
│                   └───────┬────────┘                         │
│                           │                                   │
│         ┌─────────────────┼─────────────────┐                │
│         │                 │                 │                │
│    ┌────▼────┐      ┌────▼────┐      ┌────▼────┐           │
│    │  Data    │      │  ML     │      │  Custom │           │
│    │ Analysis │      │  Model  │      │  Skills │           │
│    └─────────┘      └─────────┘      └─────────┘           │
└─────────────────────────────────────────────────────────────┘
```

#### 技术特性

| 特性 | 说明 |
|------|------|
| **通信协议** | MCP (Model Context Protocol) |
| **传输层** | HTTP / stdio |
| **工具注册** | `skill_invoke`、`skill_list` |
| **缓存机制** | Skill 列表缓存（TTL） |
| **重连机制** | 指数退避重试 |
| **健康检查** | 连接状态监控 |

#### 使用示例

```go
import (
    "link/internal/application/usecases/agent/tools"
    "link/internal/infrastructure/skill"
)

// 1. 初始化 Skill Client
skillClient, _ := skill.NewMCPClient(&skill.Config{
    Endpoint: "http://localhost:8080/mcp",
    Timeout:  30 * time.Second,
    CacheTTL: 60 * time.Second,
})

// 2. 注册全局客户端
tools.InitGlobalSkillClient(skillClient)

// 3. 创建 Skill 工具
skillInvokeTool, _ := tools.NewSkillInvokeTool()
skillListTool, _ := tools.NewSkillListTool()

// 4. 注册到 Agent
agent := builder.
    Name("Data Analyst").
    Tools(skillInvokeTool, skillListTool).
    Build()
```

#### Skill 类型

```go
// Skill 来源
type SkillSource string
const (
    SkillSourceBundled SkillSource = "bundled"  // 内置
    SkillSourcePrivate  SkillSource = "private"   // 私有
    SkillSourcePublic   SkillSource = "public"    // 公共
    SkillSourcePlugin   SkillSource = "plugin"    // 插件
)

// 执行模式
type ContextMode string
const (
    ContextModeInline ContextMode = "inline"  // 内联执行
    ContextModeFork   ContextMode = "fork"    // Fork 子 Agent
)
```

## 配置

### 环境变量

```bash
# 服务配置
SERVER_HOST=0.0.0.0
SERVER_PORT=8080

# 数据库
MYSQL_DSN=root:password@tcp(localhost:3306)/link

# 向量库
MILVUS_ADDRESS=localhost:19530

# 图数据库
NEO4J_URI=bolt://localhost:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=password

# LLM
LLM_API_KEY=sk-xxx
LLM_BASE_URL=https://api.openai.com/v1
LLM_MODEL=gpt-4

# Text2SQL Agent
TEXT2SQL_ENABLED=true
TEXT2SQL_MAX_ROWS=1000
TEXT2SQL_TIMEOUT=30

# Python Skill MCP
SKILL_ENABLED=true
SKILL_ENDPOINT=http://localhost:8080/mcp
SKILL_TIMEOUT=30
SKILL_CACHE_TTL=60
```

### 配置文件

支持 YAML 配置文件（优先级低于环境变量）：

```yaml
server:
  host: "0.0.0.0"
  port: 8080

database:
  mysql:
    dsn: "root:password@tcp(localhost:3306)/link"

llm:
  api_key: "sk-xxx"
  base_url: "https://api.openai.com/v1"
  model: "gpt-4"

text2sql:
  enabled: true
  max_rows: 1000
  timeout: 30

skill:
  enabled: true
  endpoint: "http://localhost:8080/mcp"
  timeout: 30
  cache_ttl: 60
```

## 依赖注入

使用 [Wire](https://github.com/google/wire) 进行依赖注入：

```bash
# 生成 wire 代码
go generate ./cmd/wire
```

## 开发

### 运行服务

```bash
# 开发模式
go run cmd/server/main.go

# 编译运行
go build -o bin/server cmd/server/main.go
./bin/server
```

### 运行测试

```bash
# 全部测试
go test ./...

# 覆盖率
go test -cover ./...

# 特定包
go test ./internal/application/usecases/agent/...
```

### 代码生成

```bash
# Proto 生成
python scripts/generate_grpc.py

# Wire 生成
go generate ./cmd/wire
```

## 部署

### Docker

```bash
# 构建
docker build -t link-go:latest .

# 运行
docker run -d \
  -p 8080:8080 \
  -e MYSQL_DSN=... \
  -e LLM_API_KEY=... \
  link-go:latest
```

### Docker Compose

```bash
docker-compose up -d
```

## 文档

- [架构设计](../docs/architecture.md)
- [API 文档](../docs/api.md)
- [Agent 配置](../docs/agent-config.md)
- [Agent Hooks](../docs/agent/agent-hooks.md)
- [Text2SQL 架构](../docs/text2sql-architecture.md)
- [Text2SQL 测试](../docs/text2sql-testing.md)
- [Skill 集成](../docs/skill-integration.md)
- [统一 Chat API](../docs/unified-chat-api.md)

## 许可证

Copyright © 2024 Link Project

# Evaluation System Design

## Context

Cognida 项目当前缺少完整的评测系统，无法对 RAG、QA、Agent 等功能进行系统性质量评估。现有 Python 评测服务使用 gRPC 流式接口，但 Go 端缺少任务编排和进度推送机制。

**当前状态**：
- Python 端：已有评分器（rouge、bleu、llm_judge）和指标计算
- Go 端：有基本的 domain/application 层，但缺少 Worker 和队列
- Proto 定义：evaluation.proto 定义了流式 ExecuteEvaluation 接口

**约束条件**：
- Redis 3.2.100 (Windows) - 无 Pub/Sub 推送能力
- 需要支持三种评测类型：RAG、QA、Agent
- 需要任务持久化和失败重试

## Goals / Non-Goals

**Goals:**
- 构建完整的评测任务生命周期管理（创建 → 执行 → 结果查询）
- 支持 Agent/RAG/QA 三种评测模式的统一编排
- 提供实时进度推送（SSE + Redis 轮询）
- 任务失败自动重试（最多 3 次）
- 并发控制（最多 3 个任务同时处理）

**Non-Goals:**
- 不实现分布式 Worker（仅内嵌 Worker）
- 不实现任务优先级队列
- 不实现评测结果的可视化（由前端负责）
- 不实现评测数据集的编辑功能（只读）

## Decisions

### 1. HTTP API 代替 gRPC

**决策**: 使用 HTTP API 进行 Go-Python 通信，而非 gRPC 流式接口。

**理由**：
- 评测系统内部调用，不需要 gRPC 的高性能优势
- HTTP API 更易于调试和测试
- 减少/proto 文件同步的复杂度
- 复用现有 FastAPI 框架

**替代方案**: 简化 gRPC 接口为非流式的 ComputeMetrics（被否决）

### 2. 内嵌 Worker 而非独立进程

**决策**: Worker 在 API 服务进程中运行（goroutine），非独立可执行程序。

**理由**：
- 部署简单，无需额外进程管理
- 共享配置和依赖（如 Retriever、LLMChat）
- 开发测试更方便

**风险**: Worker 崩溃可能影响 API 服务 → 通过 goroutine + recover 隔离

### 3. Redis 轮询而非 Pub/Sub

**决策**: SSE 通过轮询 Redis 进度缓存获取更新，而非 Pub/Sub。

**理由**：
- Redis 3.2 无 Pub/Sub（Windows 版本限制）
- 用户不需要强实时性（秒级延迟可接受）

**替代方案**: 升级 Redis / 使用云服务（增加复杂度，暂不采用）

### 4. 混合数据集存储

**决策**: 公共数据集用文件系统，用户数据集用数据库。

**理由**：
- 公共数据集（sample_qa 等）适合文件存储，便于版本控制
- 用户自定义数据集需要租户隔离，存数据库更安全

### 5. 三种评测执行器

**决策**: 为 Agent/RAG/QA 创建独立的执行器，而非统一流程。

**理由**：
- 执行逻辑差异大（Agent 需加载实体，RAG 需检索）
- 独立执行器更易维护和测试
- 未来扩展新类型更方便

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              前端 (Vue)                                │
└───────────────────────────┬─────────────────────────────────────────────┘
                            │
        ┌───────────────────┴───────────────────┐
        ▼                                       ▼
┌───────────────────────┐           ┌───────────────────────┐
│  POST /evaluation     │           │  GET /stream          │
│      /tasks           │           │  (SSE 轮询推送)        │
└───────────┬───────────┘           └───────────┬───────────┘
            │                                   │
            ▼                                   ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                              Go API Layer                                │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────┐     │
│  │ 创建任务         │  │ 查询任务状态     │  │ SSE 流式推送        │     │
│  │ (MySQL + Redis) │  │ (Redis 缓存)     │  │ (Redis 轮询)        │     │
│  └─────────────────┘  └─────────────────┘  └─────────────────────┘     │
└───────────────────────────────┬───────────────────────────────────────┘
                                │
                                ▼
┌───────────────────────────────────────────────────────────────────────┐
│                           Go Worker Pool                              │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │  内嵌 Worker (并发数=3)                                         │   │
│  │  ┌──────────────────────────────────────────────────────────┐  │   │
│  │  │ 1. 加载数据集 (文件/数据库)                                │  │   │
│  │  │ 2. 根据类型选择执行器：                                  │  │   │
│  │  │    - AgentExecutor: Agent.Chat()                         │  │   │
│  │  │    - RAGExecutor: Retrieve → LLM                         │  │   │
│  │  │    - QAExecutor: LLM 直接生成                             │  │   │
│  │  │ 3. 调用 Python /compute-metrics                          │  │   │
│  │  │ 4. 保存结果，更新进度                                     │  │   │
│  │  │ 5. 失败重试 (最多 3 次)                                   │  │   │
│  │  └──────────────────────────────────────────────────────────┘  │   │
│  └────────────────────────────────────────────────────────────────┘   │
└───────────────────────────────┬───────────────────────────────────────┘
                                │ HTTP
                                ▼
┌───────────────────────────────────────────────────────────────────────┐
│                        Python Evaluation Service                       │
│  POST /api/v1/evaluation/compute-metrics                             │
│  评分器: rouge, bleu, semantic, llm_judge                            │
└───────────────────────────────────────────────────────────────────────┘
```

## Component Design

### Worker (application/evaluation/worker.go)
```go
type EvaluationWorker struct {
    redis      *redis.Client
    db         *gorm.DB
    pythonCli  *PythonEvaluationClient
    agentRepo  agent.Repository
    retriever  rag.Retriever
    llmChat    rag.LLMChat
    datasetLoader DatasetLoader
    executor   ExecutorRegistry
    stopCh     chan struct{}
}

// 主循环：从队列取任务 → 检查并发 → 执行
func (w *EvaluationWorker) Run()

// 执行任务：加载 → 执行评测 → 计算指标 → 保存结果
func (w *EvaluationWorker) executeTask(taskID string)
```

### Executor Registry
```go
type Executor interface {
    Execute(ctx context.Context, task *EvaluationTask, dataset []*QAPair) ([]*QAResult, error)
}

type ExecutorRegistry struct {
    agents  map[EvaluationType]Executor
}

// 注册三种执行器
- AgentExecutor: agentRepo.FindByID → agent.Chat
- RAGExecutor: retriever.Retrieve → llmChat.Chat
- QAExecutor: llmChat.Chat (direct)
```

### Python Client (application/evaluation/python_client.go)
```go
type PythonEvaluationClient struct {
    baseURL string
    client  *http.Client
}

func (c *PythonEvaluationClient) ComputeMetrics(
    ctx context.Context,
    items []*ComputeItem,
    graders []string,
    config map[string]interface{},
) (*ComputeResponse, error)
```

### Redis Queue (infrastructure/redis/evaluation/queue.go)
```go
type EvaluationQueue struct {
    redis *redis.Client
    limit int // 3
}

// 队列操作
LPUSH/BRPOP eval:queue

// 并发控制
INCR/DECR eval:count (limit: 3)

// 进度缓存
HSET eval:progress:{id} stage current total message
EXPIRE eval:progress:{id} 3600
```

## Data Model

### Task (types.Task 扩展)
```go
Payload: {
    "dataset_id": "sample_qa",
    "evaluation_type": "rag",  // agent/rag/qa
    "kb_id": "kb-xxx",
    "model_id": "gpt-4o-mini",
    "agent_id": "agent-xxx",
    "config": {
        "top_k": 5,
        "graders": ["rouge", "bleu"],
        "llm_judge_config": {...}
    }
}
```

### Progress (Redis Hash)
```
eval:progress:{task_id} {
    "stage": "generation",
    "current": 5,
    "total": 10,
    "message": "Generating answers...",
    "percentage": 50,
    "retry_count": 0
}
```

## API Design

### Go API
```
POST   /api/v1/evaluation/tasks          # 创建任务
GET    /api/v1/evaluation/tasks/{id}     # 查询状态
GET    /api/v1/evaluation/tasks/{id}/stream  # SSE 推送
GET    /api/v1/evaluation/tasks/{id}/results  # 获取结果
GET    /api/v1/evaluation/results        # 列表结果
DELETE /api/v1/evaluation/tasks/{id}     # 删除任务
GET    /api/v1/evaluation/datasets       # 列出数据集
```

### Python API
```
POST   /api/v1/evaluation/compute-metrics  # 批量计算指标
```

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| Worker 崩溃影响 API | goroutine + recover 隔离，监控报警 |
| Python 服务不可用 | 重试机制，失败后标记任务 |
| Redis 进度缓存丢失 | 任务状态仍保存在 MySQL |
| 并发数限制导致堆积 | 监控队列长度，支持动态调整 |
| 单 QA 超时影响整体 | 超时隔离，单个失败不影响其他 |

## Migration Plan

### Phase 1: 基础设施
- Redis 队列和进度缓存封装
- 数据集加载器（文件 + 数据库）

### Phase 2: 执行器
- Agent/RAG/QA 三种执行器
- Python HTTP 客户端

### Phase 3: Worker
- Worker 主循环
- 任务编排逻辑
- 重试机制

### Phase 4: API
- HTTP 接口实现
- SSE 推送实现

### Phase 5: Python 适配
- 添加 /compute-metrics 接口

### Rollback Strategy
- 评测系统独立模块，不影响现有功能
- 可通过配置开关禁用 Worker
- 失败任务保留在 MySQL，可人工处理

## Open Questions

1. **Agent 执行时的 Context 传递**: Agent Chat 需要什么上下文？（session_id, history?）
2. **LLM Judge 的模型选择**: 默认使用哪个模型？是否支持用户配置？
3. **数据集热更新**: 文件系统数据集修改后是否需要自动重载？（当前不自动）

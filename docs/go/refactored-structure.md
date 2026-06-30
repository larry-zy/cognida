# Link-Go 重构后完整文件结构

## 完整目录树

```
link-go/
├── api/
│   ├── http/
│   │   ├── router/
│   │   │   └── router.go              # 路由注册
│   │   └── routes/
│   │       ├── agent_routes.go
│   │       ├── chat_routes.go
│   │       ├── knowledge_routes.go
│   │       ├── rag_routes.go
│   │       └── evaluation_routes.go
│   │
│   ├── proto/                          # gRPC Proto 定义（单一数据源）
│   │   ├── docreader.proto             # 文档解析服务
│   │   ├── evaluation.proto           # 评测服务
│   │   ├── ml.proto                    # ML 服务
│   │   └── annotation.proto            # 标注服务
│   │
│   └── openapi/                        # OpenAPI 规范
│       └── openapi.yaml
│
├── cmd/
│   └── server/
│       └── main.go                     # 服务入口
│
├── configs/
│   ├── config.yaml                     # 配置文件
│   └── config.go                       # 配置结构
│
├── internal/
│   ├── handler/                        # HTTP/gRPC 处理层
│   │   ├── agent.go                    # Agent HTTP 处理
│   │   ├── agent_test.go
│   │   ├── chat.go                     # Chat HTTP 处理
│   │   ├── chat_test.go
│   │   ├── knowledge.go                # Knowledge HTTP 处理
│   │   ├── knowledge_test.go
│   │   ├── rag.go                      # RAG HTTP 处理
│   │   ├── rag_test.go
│   │   ├── evaluation.go               # Evaluation HTTP 处理
│   │   ├── evaluation_test.go
│   │   └── middleware/                 # 中间件
│   │       ├── auth.go                 # 认证中间件
│   │       ├── auth_test.go
│   │       ├── cors.go                 # 跨域中间件
│   │       ├── logging.go              # 日志中间件
│   │       ├── recovery.go             # 异常恢复中间件
│   │       ├── request_id.go           # 请求 ID 中间件
│   │       ├── rate_limit.go           # 限流中间件
│   │       └── ratelimit_test.go
│   │
│   ├── service/                        # 业务逻辑层
│   │   ├── agent/                      # Agent 服务
│   │   │   ├── agent.go                # Agent 核心服务（管理 Agent 生命周期）
│   │   │   ├── agent_test.go
│   │   │   ├── registry.go             # Agent 注册中心
│   │   │   ├── registry_test.go
│   │   │   ├── factory.go              # Agent 工厂
│   │   │   ├── factory_test.go
│   │   │   ├── runtime.go              # Agent 运行时管理
│   │   │   ├── runtime_test.go
│   │   │   │
│   │   │   ├── core/                   # Agent 核心编排引擎
│   │   │   │   ├── react.go            # ReAct 编排逻辑
│   │   │   │   ├── react_test.go
│   │   │   │   ├── planner.go          # 任务规划
│   │   │   │   ├── planner_test.go
│   │   │   │   ├── executor.go         # Agent 执行器
│   │   │   │   ├── executor_test.go
│   │   │   │   ├── tools.go            # 工具调用管理
│   │   │   │   ├── tools_test.go
│   │   │   │   ├── memory.go           # Agent 记忆管理
│   │   │   │   ├── memory_test.go
│   │   │   │   └── types.go            # 核心类型定义
│   │   │   │
│   │   │   ├── builtin/                # 内置业务 Agent
│   │   │   │   ├── text2sql/           # Text2SQL Agent
│   │   │   │   │   ├── agent.go        # Text2SQL Agent 实现
│   │   │   │   │   ├── agent_test.go
│   │   │   │   │   ├── prompt.go       # Prompt 模板
│   │   │   │   │   ├── tools.go       # SQL 执行工具
│   │   │   │   │   └── validator.go    # SQL 验证
│   │   │   │   ├── data_analysis/      # 数据分析 Agent
│   │   │   │   │   ├── agent.go
│   │   │   │   │   ├── agent_test.go
│   │   │   │   │   ├── charts.go       # 图表生成
│   │   │   │   │   └── insight.go      # 洞察生成
│   │   │   │   ├── code_review/        # 代码审查 Agent
│   │   │   │   │   ├── agent.go
│   │   │   │   │   ├── agent_test.go
│   │   │   │   │   ├── rules.go        # 审查规则
│   │   │   │   │   └── report.go       # 审查报告
│   │   │   │   ├── document_analysis/  # 文档分析 Agent
│   │   │   │   │   ├── agent.go
│   │   │   │   │   ├── agent_test.go
│   │   │   │   │   ├── parser.go       # 文档解析
│   │   │   │   │   └── summary.go      # 摘要生成
│   │   │   │   ├── knowledge_qa/       # 知识问答 Agent
│   │   │   │   │   ├── agent.go
│   │   │   │   │   ├── agent_test.go
│   │   │   │   │   └── retriever.go    # 知识检索
│   │   │   │   ├── workflow/           # 工作流 Agent
│   │   │   │   │   ├── agent.go
│   │   │   │   │   ├── agent_test.go
│   │   │   │   │   ├── executor.go     # 工作流执行
│   │   │   │   │   └── parser.go       # 工作流定义解析
│   │   │   │   └── research/           # Deep Research Agent
│   │   │   │       ├── agent.go
│   │   │   │       ├── agent_test.go
│   │   │   │       ├── planner.go      # 研究规划
│   │   │   │       ├── collector.go    # 信息收集
│   │   │   │       └── synthesizer.go  # 综合分析
│   │   │   │
│   │   │   ├── custom/                 # 自定义 Agent 支持
│   │   │   │   ├── loader.go           # 动态加载器
│   │   │   │   ├── validator.go        # 配置验证
│   │   │   │   └── sandbox.go          # 沙箱执行
│   │   │   │
│   │   │   └── types.go                # Agent Service 类型定义
│   │   │
│   │   ├── rag/                        # RAG 服务
│   │   │   ├── retriever.go            # 检索服务
│   │   │   ├── retriever_test.go
│   │   │   ├── pipeline.go             # RAG 端到端流程
│   │   │   ├── pipeline_test.go
│   │   │   ├── rerank.go               # 重排服务
│   │   │   ├── rerank_test.go
│   │   │   ├── optimizer.go            # 查询优化
│   │   │   ├── optimizer_test.go
│   │   │   └── types.go                # Service 类型定义
│   │   │
│   │   ├── llm/                        # LLM 服务
│   │   │   ├── chat.go                 # 聊天服务
│   │   │   ├── chat_test.go
│   │   │   ├── embedding.go            # 向量化服务
│   │   │   ├── embedding_test.go
│   │   │   ├── stream.go               # 流式输出
│   │   │   ├── stream_test.go
│   │   │   ├── client.go               # LLM 客户端封装
│   │   │   ├── openai.go               # OpenAI 客户端
│   │   │   ├── openai_test.go
│   │   │   ├── anthropic.go            # Anthropic 客户端
│   │   │   ├── anthropic_test.go
│   │   │   ├── local.go                # 本地模型客户端
│   │   │   ├── local_test.go
│   │   │   └── types.go                # Service 类型定义
│   │   │
│   │   ├── knowledge/                  # Knowledge 服务
│   │   │   ├── knowledge.go            # Knowledge 核心服务
│   │   │   ├── knowledge_test.go
│   │   │   ├── document.go             # 文档管理
│   │   │   ├── document_test.go
│   │   │   ├── chunk.go                # 文档分块
│   │   │   ├── chunk_test.go
│   │   │   ├── vector.go               # 向量管理
│   │   │   ├── vector_test.go
│   │   │   ├── parser.go               # 文档解析（调用 Python）
│   │   │   ├── parser_test.go
│   │   │   ├── indexer.go              # 索引管理
│   │   │   ├── indexer_test.go
│   │   │   └── types.go                # Service 类型定义
│   │   │
│   │   ├── chat/                       # Chat 服务
│   │   │   ├── service.go              # 聊天编排服务
│   │   │   ├── service_test.go
│   │   │   ├── session.go              # 会话管理
│   │   │   ├── session_test.go
│   │   │   ├── history.go              # 历史记录
│   │   │   ├── history_test.go
│   │   │   ├── context.go              # 对话上下文
│   │   │   ├── context_test.go
│   │   │   └── types.go                # Service 类型定义
│   │   │
│   │   ├── evaluation/                 # Evaluation 服务
│   │   │   ├── service.go              # 评测核心服务
│   │   │   ├── service_test.go
│   │   │   ├── metrics.go              # 指标计算
│   │   │   ├── metrics_test.go
│   │   │   ├── dataset.go              # 数据集管理
│   │   │   ├── dataset_test.go
│   │   │   ├── evaluator.go            # 评估器
│   │   │   ├── evaluator_test.go
│   │   │   ├── report.go               # 报告生成
│   │   │   ├── report_test.go
│   │   │   └── types.go                # Service 类型定义
│   │   │
│   │   └── common/                     # Service 通用模块
│   │       ├── validator.go            # 通用验证器
│   │       ├── validator_test.go
│   │       └── errors.go               # Service 错误定义
│   │
│   ├── repository/                     # 数据访问层
│   │   ├── mysql/                       # MySQL 数据访问
│   │   │   ├── agent_repo.go           # Agent 数据访问
│   │   │   ├── agent_repo_test.go
│   │   │   ├── knowledge_repo.go       # Knowledge 数据访问
│   │   │   ├── knowledge_repo_test.go
│   │   │   ├── session_repo.go         # Session 数据访问
│   │   │   ├── session_repo_test.go
│   │   │   ├── chat_repo.go            # Chat 数据访问
│   │   │   ├── chat_repo_test.go
│   │   │   ├── evaluation_repo.go     # Evaluation 数据访问
│   │   │   ├── evaluation_repo_test.go
│   │   │   ├── user_repo.go            # User 数据访问
│   │   │   ├── user_repo_test.go
│   │   │   ├── tenant_repo.go          # Tenant 数据访问
│   │   │   ├── tenant_repo_test.go
│   │   │   ├── client.go               # MySQL 客户端
│   │   │   ├── client_test.go
│   │   │   └── migrations/             # 数据库迁移
│   │   │       ├── 001_init.up.sql
│   │   │       ├── 001_init.down.sql
│   │   │       ├── 002_agents.up.sql
│   │   │       └── 002_agents.down.sql
│   │   │
│   │   ├── milvus/                     # Milvus 向量数据库访问
│   │   │   ├── vector_repo.go          # 向量数据访问
│   │   │   ├── vector_repo_test.go
│   │   │   ├── chunk_repo.go           # 文档块数据访问
│   │   │   ├── chunk_repo_test.go
│   │   │   ├── client.go               # Milvus 客户端
│   │   │   ├── client_test.go
│   │   │   └── collections/            # Collection 定义
│   │   │       ├── vectors.go
│   │   │       └── chunks.go
│   │   │
│   │   ├── redis/                      # Redis 缓存/消息队列
│   │   │   ├── cache.go                # 缓存操作
│   │   │   ├── cache_test.go
│   │   │   ├── lock.go                 # 分布式锁
│   │   │   ├── lock_test.go
│   │   │   ├── queue.go                # 队列操作
│   │   │   ├── queue_test.go
│   │   │   ├── pubsub.go               # 发布订阅
│   │   │   ├── pubsub_test.go
│   │   │   ├── client.go               # Redis 客户端
│   │   │   └── client_test.go
│   │   │
│   │   ├── neo4j/                      # Neo4j 图数据库访问
│   │   │   ├── graph_repo.go           # 图数据访问
│   │   │   ├── graph_repo_test.go
│   │   │   ├── entity_repo.go          # 实体数据访问
│   │   │   ├── entity_repo_test.go
│   │   │   ├── relation_repo.go        # 关系数据访问
│   │   │   ├── relation_repo_test.go
│   │   │   ├── client.go               # Neo4j 客户端
│   │   │   └── client_test.go
│   │   │
│   │   └── llm/                        # LLM 外部服务访问
│   │       ├── openai_client.go        # OpenAI 客户端
│   │       ├── openai_test.go
│   │       ├── anthropic_client.go     # Anthropic 客户端
│   │       ├── anthropic_test.go
│   │       ├── local_client.go         # 本地模型客户端
│   │       └── local_test.go
│   │
│   ├── model/                          # 数据模型定义层
│   │   ├── agent/                      # Agent 数据模型
│   │   │   ├── entity.go               # Agent 实体定义
│   │   │   ├── types.go                # Agent 相关类型
│   │   │   ├── constants.go            # Agent 常量
│   │   │   ├── errors.go               # Agent 错误定义
│   │   │   └── repository.go           # AgentRepository 接口定义
│   │   │
│   │   ├── rag/                        # RAG 数据模型
│   │   │   ├── entity.go               # RAG 实体定义
│   │   │   ├── types.go                # RAG 相关类型
│   │   │   ├── constants.go            # RAG 常量
│   │   │   └── repository.go           # RAGRepository 接口定义
│   │   │
│   │   ├── knowledge/                  # Knowledge 数据模型
│   │   │   ├── entity.go               # Knowledge 实体定义
│   │   │   ├── types.go                # Knowledge 相关类型
│   │   │   ├── constants.go            # Knowledge 常量
│   │   │   └── repository.go           # KnowledgeRepository 接口定义
│   │   │
│   │   ├── chat/                       # Chat 数据模型
│   │   │   ├── entity.go               # Chat 实体定义
│   │   │   ├── types.go                # Chat 相关类型
│   │   │   ├── constants.go            # Chat 常量
│   │   │   └── repository.go           # ChatRepository 接口定义
│   │   │
│   │   ├── evaluation/                 # Evaluation 数据模型
│   │   │   ├── entity.go               # Evaluation 实体定义
│   │   │   ├── types.go                # Evaluation 相关类型
│   │   │   ├── constants.go            # Evaluation 常量
│   │   │   └── repository.go           # EvaluationRepository 接口定义
│   │   │
│   │   ├── user/                       # User 数据模型
│   │   │   ├── entity.go               # User 实体定义
│   │   │   ├── types.go                # User 相关类型
│   │   │   ├── constants.go            # User 常量
│   │   │   └── repository.go           # UserRepository 接口定义
│   │   │
│   │   ├── tenant/                     # Tenant 数据模型
│   │   │   ├── entity.go               # Tenant 实体定义
│   │   │   ├── types.go                # Tenant 相关类型
│   │   │   ├── constants.go            # Tenant 常量
│   │   │   └── repository.go           # TenantRepository 接口定义
│   │   │
│   │   └── types/                      # 通用类型
│   │       ├── common.go               # 通用类型定义
│   │       ├── errors.go               # 错误类型定义
│   │       ├── request.go              # 通用请求类型
│   │       ├── response.go             # 通用响应类型
│   │       ├── pagination.go           # 分页类型
│   │       └── constants.go            # 通用常量
│   │
│   └── pkg/                            # 内部通用包
│       ├── logger/                     # 日志包
│       │   ├── logger.go
│       │   ├── logger_test.go
│       │   ├── config.go
│       │   └── middleware.go
│       ├── errors/                     # 错误处理包
│       │   ├── errors.go
│       │   ├── codes.go
│       │   └── handler.go
│       ├── config/                     # 配置包
│       │   ├── config.go
│       │   ├── loader.go
│       │   └── validator.go
│       ├── utils/                      # 工具包
│       │   ├── json.go
│       │   ├── time.go
│       │   ├── string.go
│       │   └── crypto.go
│       └── validator/                  # 验证器包
│           ├── validator.go
│           ├── rules.go
│           └── messages.go
│
├── pkg/                                # 外部通用包（可被其他项目引用）
│   └── proto/                          # 生成的 Proto Go 代码
│       ├── docreader/
│       │   ├── docreader.pb.go
│       │   └── docreader_grpc.pb.go
│       ├── evaluation/
│       │   ├── evaluation.pb.go
│       │   └── evaluation_grpc.pb.go
│       ├── ml/
│       │   ├── ml.pb.go
│       │   └── ml_grpc.pb.go
│       └── annotation/
│           ├── annotation.pb.go
│           └── annotation_grpc.pb.go
│
├── test/                               # 测试
│   ├── integration/                    # 集成测试
│   │   ├── agent_test.go
│   │   ├── chat_test.go
│   │   ├── knowledge_test.go
│   │   ├── rag_test.go
│   │   └── evaluation_test.go
│   │
│   ├── e2e/                            # 端到端测试
│   │   ├── api_test.go
│   │   └── workflow_test.go
│   │
│   ├── benchmark/                      # 性能测试
│   │   ├── retrieval_bench_test.go
│   │   ├── llm_bench_test.go
│   │   └── agent_bench_test.go
│   │
│   ├── fixtures/                       # 测试固件
│   │   ├── data/
│   │   │   ├── agents.json
│   │   │   ├── documents.json
│   │   │   └── queries.json
│   │   └── mocks/
│   │       ├── mock_llm.go
│   │       └── mock_repository.go
│   │
│   └── architecture/                   # 架构测试
│       ├── architecture_test.go        # 依赖规则测试
│       └── import_test.go             # 导入规则测试
│
├── scripts/                           # 脚本
│   ├── generate_grpc.go               # 生成 gRPC 代码
│   ├── migrate.go                     # 数据库迁移
│   ├── seed.go                        # 数据初始化
│   └── build.sh                       # 构建脚本
│
├── docs/                              # 文档
│   ├── go/
│   │   ├── refactored-architecture.md
│   │   ├── refactored-structure.md
│   │   ├── refactoring-plan.md
│   │   └── api-guide.md
│   ├── api/                           # API 文档
│   └── deployment/                    # 部署文档
│
├── deployments/                       # 部署配置
│   ├── docker/
│   │   ├── Dockerfile
│   │   └── docker-compose.yml
│   ├── k8s/                           # Kubernetes 配置
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── configmap.yaml
│   └── helm/                          # Helm Charts
│
├── .github/                           # GitHub 配置
│   └── workflows/
│       ├── ci.yml
│       ├── test.yml
│       └── deploy.yml
│
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── CLAUDE.md
```

## 各层详细说明

### 1. Handler 层 (`internal/handler/`)

**职责**：HTTP/gRPC 协议处理、请求验证、响应封装

```
internal/handler/
├── agent.go              # POST /api/v1/agent/execute
├── chat.go               # POST /api/v1/chat
├── knowledge.go          # POST /api/v1/knowledge/documents
├── rag.go                # POST /api/v1/rag/retrieve
├── evaluation.go         # POST /api/v1/evaluation/run
└── middleware/           # 认证、日志、限流、CORS 等
```

### 2. Service 层 (`internal/service/`)

**职责**：核心业务逻辑实现、流程控制、规则校验、协调调用

```
internal/service/
├── agent/                # Agent 服务
│   ├── agent.go          # Agent 核心服务（生命周期管理）
│   ├── registry.go       # Agent 注册中心
│   ├── factory.go        # Agent 工厂
│   ├── runtime.go        # Agent 运行时管理
│   ├── core/             # 核心编排引擎
│   │   ├── react.go      # ReAct 编排
│   │   ├── planner.go    # 任务规划
│   │   ├── executor.go   # Agent 执行器
│   │   ├── tools.go      # 工具管理
│   │   ├── memory.go     # 记忆管理
│   │   └── types.go      # 核心类型
│   ├── builtin/          # 内置业务 Agent
│   │   ├── text2sql/     # Text2SQL Agent
│   │   ├── data_analysis/# 数据分析 Agent
│   │   ├── code_review/  # 代码审查 Agent
│   │   ├── document_analysis/# 文档分析 Agent
│   │   ├── knowledge_qa/ # 知识问答 Agent
│   │   ├── workflow/     # 工作流 Agent
│   │   └── research/     # Deep Research Agent
│   ├── custom/           # 自定义 Agent 支持
│   │   ├── loader.go     # 动态加载器
│   │   ├── validator.go  # 配置验证
│   │   └── sandbox.go    # 沙箱执行
│   └── types.go          # Agent Service 类型
│
├── rag/                  # RAG 服务
│   ├── retriever.go      # 检索逻辑
│   ├── pipeline.go       # RAG 流程编排
│   ├── rerank.go         # 重排逻辑
│   ├── optimizer.go      # 查询优化
│   └── types.go
│
├── llm/                  # LLM 服务
│   ├── chat.go           # 聊天逻辑
│   ├── embedding.go      # 向量化
│   ├── stream.go         # 流式输出
│   ├── client.go         # 客户端封装
│   ├── openai.go         # OpenAI 实现
│   ├── anthropic.go      # Anthropic 实现
│   ├── local.go          # 本地模型实现
│   └── types.go
│
├── knowledge/            # Knowledge 服务
│   ├── knowledge.go      # 核心逻辑
│   ├── document.go       # 文档管理
│   ├── chunk.go          # 分块逻辑
│   ├── vector.go         # 向量管理
│   ├── parser.go         # 文档解析（gRPC 调 Python）
│   ├── indexer.go        # 索引管理
│   └── types.go
│
├── chat/                 # Chat 服务
│   ├── service.go        # 聊天编排
│   ├── session.go        # 会话管理
│   ├── history.go        # 历史记录
│   ├── context.go        # 对话上下文
│   └── types.go
│
├── evaluation/           # Evaluation 服务
│   ├── service.go        # 评测核心
│   ├── metrics.go        # 指标计算
│   ├── dataset.go         # 数据集管理
│   ├── evaluator.go      # 评估器
│   ├── report.go         # 报告生成
│   └── types.go
│
└── common/               # Service 通用模块
    ├── validator.go      # 通用验证器
    └── errors.go         # Service 错误定义
```

### 3. Repository 层 (`internal/repository/`)

**职责**：数据访问、外部服务调用、缓存操作、事务管理

```
internal/repository/
├── mysql/                # MySQL 数据访问
│   ├── agent_repo.go     # Agent CRUD
│   ├── knowledge_repo.go # Knowledge CRUD
│   ├── session_repo.go   # Session CRUD
│   ├── chat_repo.go      # Chat CRUD
│   ├── evaluation_repo.go# Evaluation CRUD
│   ├── user_repo.go      # User CRUD
│   ├── tenant_repo.go    # Tenant CRUD
│   ├── client.go         # MySQL 客户端
│   └── migrations/       # 数据库迁移文件
│
├── milvus/               # Milvus 向量数据库
│   ├── vector_repo.go    # 向量 CRUD
│   ├── chunk_repo.go     # 文档块 CRUD
│   ├── client.go         # Milvus 客户端
│   └── collections/      # Collection 定义
│
├── redis/                # Redis 缓存/消息队列
│   ├── cache.go          # 缓存操作
│   ├── lock.go           # 分布式锁
│   ├── queue.go          # 队列操作
│   ├── pubsub.go         # 发布订阅
│   └── client.go         # Redis 客户端
│
├── neo4j/                # Neo4j 图数据库
│   ├── graph_repo.go     # 图数据访问
│   ├── entity_repo.go    # 实体数据访问
│   ├── relation_repo.go  # 关系数据访问
│   └── client.go         # Neo4j 客户端
│
└── llm/                  # LLM 外部服务
    ├── openai_client.go  # OpenAI API 客户端
    ├── anthropic_client.go# Anthropic API 客户端
    └── local_client.go   # 本地模型客户端
```

### 4. Model 层 (`internal/model/`)

**职责**：数据实体定义、值对象、接口契约、通用类型

```
internal/model/
├── agent/                # Agent 数据模型
│   ├── entity.go         # Agent 实体
│   ├── types.go          # 类型定义
│   ├── constants.go      # 常量
│   ├── errors.go         # 错误定义
│   └── repository.go     # AgentRepository 接口
│
├── rag/                  # RAG 数据模型
│   ├── entity.go         # Document 等实体
│   ├── types.go
│   ├── constants.go
│   └── repository.go     # RAGRepository 接口
│
├── knowledge/            # Knowledge 数据模型
│   ├── entity.go         # KnowledgeBase, Document 等
│   ├── types.go
│   ├── constants.go
│   └── repository.go     # KnowledgeRepository 接口
│
├── chat/                 # Chat 数据模型
│   ├── entity.go         # Session, Message 等
│   ├── types.go
│   ├── constants.go
│   └── repository.go     # ChatRepository 接口
│
├── evaluation/           # Evaluation 数据模型
│   ├── entity.go         # Evaluation, Dataset 等
│   ├── types.go
│   ├── constants.go
│   └── repository.go     # EvaluationRepository 接口
│
├── user/                 # User 数据模型
│   ├── entity.go         # User 实体
│   ├── types.go
│   ├── constants.go
│   └── repository.go     # UserRepository 接口
│
├── tenant/               # Tenant 数据模型
│   ├── entity.go         # Tenant 实体
│   ├── types.go
│   ├── constants.go
│   └── repository.go     # TenantRepository 接口
│
└── types/                # 通用类型
    ├── common.go         # 通用类型
    ├── errors.go         # 错误类型
    ├── request.go        # 请求类型
    ├── response.go       # 响应类型
    ├── pagination.go     # 分页类型
    └── constants.go      # 通用常量
```

## 依赖关系图

```
                    ┌─────────────────────────────────────┐
                    │              handler                │
                    │        (只依赖 service)              │
                    └──────────────┬──────────────────────┘
                                   │
                                   ▼
                    ┌─────────────────────────────────────┐
                    │              service                 │
                    │   (依赖 model + repository)           │
                    │        (可互调其他 service)          │
                    └──────────────┬──────────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                             ▼
        ┌──────────────────────────┐   ┌──────────────────────────┐
        │       repository          │   │          model             │
        │     (只依赖 model)        │   │        (无依赖)            │
        │    实现 model 接口        │   │      定义接口契约         │
        └─────────────┬────────────┘   └──────────────────────────┘
                      │
                      ▼
        ┌─────────────────────────────────────────────────────┐
        │               MySQL / Milvus / Redis / Neo4j         │
        └─────────────────────────────────────────────────────┘
```

## 命名规范

### 文件命名

| 类型 | 命名规则 | 示例 |
|------|----------|------|
| 主要实现 | `<module>.go` | `agent.go` |
| 测试文件 | `<module>_test.go` | `agent_test.go` |
| 类型定义 | `types.go` | `types.go` |
| 接口定义 | `repository.go` | `repository.go` |
| 实体定义 | `entity.go` | `entity.go` |
| 错误定义 | `errors.go` | `errors.go` |
| 常量定义 | `constants.go` | `constants.go` |

### 包命名

| 层级 | 命名 | 说明 |
|------|------|------|
| Handler | `handler` | HTTP 处理层 |
| Service | `<domain>` | 按业务领域命名（agent、rag、llm 等） |
| Repository | `<storage>` | 按存储类型命名（mysql、milvus、redis 等） |
| Model | `<domain>` | 按业务领域命名（与 Service 对应） |

## 测试文件组织

```
test/
├── integration/           # 集成测试（需要真实依赖）
├── e2e/                   # 端到端测试（完整流程）
├── benchmark/             # 性能基准测试
├── fixtures/              # 测试数据和 Mock
│   ├── data/             # 测试数据 JSON
│   └── mocks/            # Mock 对象
└── architecture/         # 架构规则测试
```

## 配置文件组织

```
configs/
├── config.yaml           # 主配置文件
├── config.dev.yaml       # 开发环境配置
├── config.prod.yaml      # 生产环境配置
└── config.test.yaml      # 测试环境配置
```

## 迁移脚本

```
scripts/
├── generate_grpc.go      # 生成 gRPC 代码（从 proto）
├── migrate.go            # 数据库迁移工具
├── seed.go               # 数据初始化工具
├── build.sh              # 构建脚本
└── test.sh               # 测试脚本
```

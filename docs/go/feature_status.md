# Cognida 项目功能清单

> **文档版本**: v1.1
> **更新日期**: 2026-05-05
> **项目概述**: 基于 RAG 和知识图谱的智能知识管理系统

---

## 目录

- [一、已完成功能](#一已完成功能)
  - [1.1 用户认证与权限管理](#11-用户认证与权限管理)
  - [1.2 聊天功能](#12-聊天功能)
  - [1.3 Agent 智能体](#13-agent-智能体)
  - [1.4 知识库管理](#14-知识库管理)
  - [1.5 RAG 检索系统](#15-rag-检索系统)
  - [1.6 知识图谱](#16-知识图谱)
  - [1.7 模型管理](#17-模型管理)
  - [1.8 大模型质量评估](#18-大模型质量评估)
  - [1.9 会话与消息管理](#19-会话与消息管理)
  - [1.10 前端页面](#110-前端页面)
  - [1.11 安全护栏](#111-安全护栏-guardrail)
  - [1.12 RAG 检索优化](#112-rag-检索优化)
- [二、部分完成功能](#二部分完成功能)
- [三、未完成功能](#三未完成功能)
- [四、数据库表结构](#四数据库表结构)

---

## 一、已完成功能

### 1.1 用户认证与权限管理

| 功能 | 状态 | 文件位置 |
|------|------|---------|
| 租户管理 (CRUD) | ✅ | `internal/handler/tenant.go` |
| 用户管理 (CRUD) | ✅ | `internal/application/user.go` |
| 角色权限系统 (RBAC) | ✅ | `internal/handler/permission.go` |
| JWT 认证中间件 | ✅ | `internal/middleware/auth.go` |
| 租户隔离中间件 | ✅ | `internal/middleware/middleware.go` |
| 权限检查中间件 | ✅ | `internal/middleware/permission.go` |
| 刷新 Token | ✅ | `refresh_tokens` 表 |

**相关数据表**:
- `tenants` - 租户表
- `users` - 用户表
- `roles` - 角色表
- `permissions` - 权限表
- `role_permissions` - 角色权限关联表
- `user_roles` - 用户角色关联表
- `resource_permissions` - 资源级权限表
- `refresh_tokens` - 刷新令牌表

---

### 1.2 聊天功能

| 功能 | 状态 | 文件位置 |
|------|------|---------|
| 普通聊天 (非流式) | ✅ | `internal/handler/chat.go:ChatHandler.Chat()` |
| 流式聊天 (SSE) | ✅ | `internal/handler/chat.go:ChatHandler.ChatStream()` |
| RAG 增强聊天 | ✅ | `internal/handler/chat.go:RAGChatHandler` |
| 聊天历史管理 | ✅ | `internal/handler/session.go` |
| 消息 CRUD | ✅ | `internal/handler/message.go` |
| 消息反馈 | ✅ | `message_feedback` 表 |

**支持的聊天模式**:
1. **普通聊天**: 直接调用 LLM API
2. **RAG 聊天**: 结合知识库检索增强回答
3. **Agent 聊天**: 使用多代理协作

**RAG 配置支持**:
```go
type RAGConfig struct {
    Enabled             bool
    KBID                string
    RetrievalModes      []string  // vector, bm25, graph
    VectorTopK          int
    KeywordTopK         int
    GraphTopK           int
    SimilarityThreshold float64
    Alpha               float32   // 混合检索权重
    RerankEnabled       bool
    WebEnabled          bool      // 网络搜索
}
```

---

### 1.3 Agent 智能体

基于 Cloudwego Eino 框架实现的多代理协作系统。

| 组件 | 状态 | 说明 |
|------|------|------|
| Coordinator Agent (主协调器) | ✅ | 分析问题、决策调用子代理 |
| Planner Agent (规划代理) | ✅ | 制定研究计划、分解任务 |
| Retriever Agent (检索代理) | ✅ | 知识库检索 + 网络搜索 |
| Analyzer Agent (分析代理) | ✅ | 深度分析检索结果 |
| Synthesizer Agent (合成代理) | ✅ | 整合分析结果、生成报告 |
| Critic Agent (评审代理) | ✅ | 评审质量、提出改进建议 |
| AgenticRAGAgent (简化版) | ✅ | 单 Agent 版本 |

**可用工具**:

| 工具名称 | 用途 | 状态 |
|---------|------|------|
| rag_query | 知识库检索 | ✅ |
| web_search | 网络搜索 | ✅ |
| smart_retrieval | 智能检索（自动匹配知识库） | ✅ |
| calculator | 计算器 | ✅ |
| get_current_time | 获取当前时间 | ✅ |
| http_request | HTTP 请求 | ✅ |

**文件位置**:
- Agent 实现: `internal/application/agent/agentic_rag_agent.go`
- 工具定义: `internal/agent/tool/`
- Handler: `internal/handler/agent.go`

---

### 1.4 知识库管理

| 功能 | 状态 | 文件位置 |
|------|------|---------|
| 知识库 CRUD | ✅ | `internal/handler/knowledge_base.go` |
| 知识库统计 | ✅ | `v_kb_stats` 视图 |
| 知识条目上传 | ✅ | `internal/handler/knowledge.go` |
| 知识条目列表 | ✅ | `internal/handler/knowledge.go` |
| 知识条目删除 | ✅ | `internal/handler/knowledge.go` |
| 知识条目状态查询 | ✅ | `internal/handler/knowledge.go` |
| 分块列表 | ✅ | `internal/handler/knowledge.go` |
| 标签管理 | ✅ | `internal/application/repository/tag.go` |
| 异步任务处理 | ✅ | `internal/handler/task_processor.go` |

**支持的操作**:
- 文件上传 (PDF, TXT, DOC 等)
- 异步处理状态查询
- 分块配置 (chunk_size, chunk_overlap)
- BM25 稀疏向量开关
- 图谱构建开关

---

### 1.5 RAG 检索系统

完整的 RAG Pipeline 实现，支持多种检索模式和优化策略。

| 组件 | 状态 | 文件位置 |
|------|------|---------|
| Pipeline (总控) | ✅ | `internal/application/rag/pipeline.go` |
| Retriever (检索器) | ✅ | `internal/application/rag/retriever.go` |
| Reranker (重排器) | ✅ | `internal/application/rag/rerank.go` |
| QueryStrengthener (查询增强) | ✅ | `internal/application/rag/query_strength.go` |
| RAGChatService (聊天集成) | ✅ | `internal/application/rag/rag_chat.go` |
| HyDE Generator | ✅ | `internal/infrastructure/rag/hyde_generator.go` |
| Query Rewriter | ✅ | `internal/infrastructure/rag/hyde_generator.go` |
| Multi-Hop Retriever | ✅ | `internal/infrastructure/rag/multi_hop.go` |

**支持的检索模式**:

| 模式 | 说明 | 状态 |
|------|------|------|
| vector | 向量语义检索 | ✅ |
| bm25 / keyword | BM25 关键词检索 | ✅ |
| hybrid | 向量 + BM25 混合检索 | ✅ |
| graph | 知识图谱检索 | ✅ |

**重排策略**:

| 策略 | 说明 | 状态 |
|------|------|------|
| rrf | 倒数排名融合 | ✅ |
| weighted | 加权分数融合 | ✅ |
| weighted_rrf | 加权 RRF | ✅ |
| model | 模型重排 | ⚠️ 部分实现 |

**查询增强功能**:
- 查询重写 (指代消解)
- 查询拆分 (复杂问题分解)

---

### 1.6 知识图谱

| 功能 | 状态 | 文件位置 |
|------|------|---------|
| 图谱节点 CRUD | ✅ | `internal/handler/graph.go` |
| 图谱关系 CRUD | ✅ | `internal/handler/graph.go` |
| 实体提取 | ✅ | `internal/application/graph.go` |
| 关系提取 | ✅ | `internal/application/graph.go` |
| 图谱可视化 | ✅ | `web/src/views/knowledge/GraphView.vue` |
| 图谱检索 | ✅ | RAG 集成 |

---

### 1.11 安全护栏 (Guardrail)

完整的输入输出安全检查和过滤系统。

| 组件 | 状态 | 文件位置 |
|------|------|---------|
| Input Filter | ✅ | `internal/infrastructure/guardrail/input_filter.go` |
| Output Filter | ✅ | `internal/infrastructure/guardrail/output_filter.go` |
| Jailbreak Detector | ✅ | `internal/infrastructure/guardrail/jailbreak_detector.go` |
| Guardrail Service | ✅ | `internal/application/guardrail/guardrail_service.go` |
| HTTP Handler | ✅ | `internal/interface/http/handler/guardrail_handler.go` |

**支持的功能**:
- 输入检查（敏感词、PII、SQL注入、XSS）
- 输出检查（PII脱敏、脏话、仇恨言论、暴力）
- 越狱检测（规则+LLM智能检测）
- 内容清理和脱敏
- 综合检查（输入+输出+越狱）

**HTTP 接口**:
```
POST /api/v1/guardrail/input/check          # 检查输入
POST /api/v1/guardrail/input/sanitize       # 清理输入
POST /api/v1/guardrail/output/check         # 检查输出
POST /api/v1/guardrail/output/sanitize      # 清理输出
POST /api/v1/guardrail/jailbreak/check      # 越狱检测
POST /api/v1/guardrail/full-check           # 完整检查
POST /api/v1/guardrail/check-both           # 输入输出检查
POST /api/v1/guardrail/quick-check          # 快速检查
POST /api/v1/guardrail/is-jailbreak         # 快速越狱检查
GET  /api/v1/guardrail/config/default       # 默认配置
POST /api/v1/guardrail/recommendation       # 处理建议
```

---

### 1.12 RAG 检索优化

| 功能 | 状态 | 文件位置 |
|------|------|---------|
| HyDE 假设文档生成 | ✅ | `internal/infrastructure/rag/hyde_generator.go` |
| 查询重写 | ✅ | `internal/infrastructure/rag/hyde_generator.go` |
| 查询扩展 | ✅ | `internal/infrastructure/rag/hyde_generator.go` |
| 查询分解 | ✅ | `internal/infrastructure/rag/hyde_generator.go` |
| 多跳检索 | ✅ | `internal/infrastructure/rag/multi_hop.go` |
| HTTP Handler | ✅ | `internal/interface/http/handler/rag_optimizer_handler.go` |

**HTTP 接口**:
```
POST /api/v1/rag/hyde/generate              # 生成假设文档
POST /api/v1/rag/hyde/generate-multiple     # 生成多个假设文档
POST /api/v1/rag/query/rewrite              # 重写查询
POST /api/v1/rag/query/expand               # 扩展查询
POST /api/v1/rag/query/decompose            # 分解查询
POST /api/v1/rag/multi-hop/retrieve         # 多跳检索
POST /api/v1/rag/optimize/query             # 综合优化
GET  /api/v1/rag/optimize/config            # 获取配置
PUT  /api/v1/rag/optimize/config            # 更新配置
```

---

### 1.7 模型管理

**Neo4j 数据结构**:
- 节点标签: `ENTITY:KB_{kb_id前8位}`
- 关系类型: `RELATES_TO`
- 支持多租户隔离 (kb_id)

---

### 1.7 模型管理

| 功能 | 状态 | 文件位置 |
|------|------|---------|
| 模型列表查询 | ✅ | `internal/handler/model.go` |
| 模型详情查询 | ✅ | `internal/handler/model.go` |
| 多模型类型支持 | ✅ | embedding, chat, rerank, vlm, summary |
| 多模型源支持 | ✅ | openai, azure, dashscope, custom |

**模型类型**:
- `embedding` - 向量化模型
- `chat` - 对话模型
- `rerank` - 重排序模型
- `vlm` - 视觉语言模型
- `summary` - 摘要模型

---

### 1.8 大模型质量评估

| 功能 | 状态 | 文件位置 |
|------|------|---------|
| 评估任务创建 | ✅ | `internal/handler/evaluation.go` |
| 评估任务查询 | ✅ | `internal/handler/evaluation.go` |
| 评估任务列表 | ✅ | `internal/handler/evaluation.go` |
| 评估任务删除 | ✅ | `internal/handler/evaluation.go` |
| 数据集创建 | ✅ | `internal/handler/evaluation.go` |
| 数据集列表 | ✅ | `internal/handler/evaluation.go` |

**评估指标**:
- 检索指标 (retrieval_metrics)
- 生成指标 (generation_metrics)

---

### 1.9 会话与消息管理

| 功能 | 状态 | 文件位置 |
|------|------|---------|
| 会话 CRUD | ✅ | `internal/handler/session.go` |
| 会话归档/激活 | ✅ | `internal/handler/session.go` |
| 会话详情 | ✅ | `internal/handler/session.go` |
| 消息 CRUD | ✅ | `internal/handler/message.go` |
| 消息列表 | ✅ | `internal/handler/message.go` |
| 消息反馈 | ✅ | `message_feedback` 表 |

**会话配置支持**:
- RAG 配置绑定 (retrieval_settings)
- 最大轮次限制
- 降级策略
- 重排序配置

---

### 1.10 前端页面

| 页面 | 状态 | 文件位置 |
|------|------|---------|
| 登录页 | ✅ | `web/src/views/auth/Login.vue` |
| 聊天页面 | ✅ | `web/src/views/chat/` |
| 创建对话 | ✅ | `web/src/views/creatChat/creatChat.vue` |
| 知识库列表 | ✅ | `web/src/views/knowledge/KnowledgeBaseList.vue` |
| 知识库详情 | ✅ | `web/src/views/knowledge/KnowledgeBase.vue` |
| 图谱列表 | ✅ | `web/src/views/knowledge/GraphList.vue` |
| 图谱可视化 | ✅ | `web/src/views/knowledge/GraphView.vue` |
| Agent 列表 | ✅ | `web/src/views/agent/AgentList.vue` |
| 评估管理 | ✅ | `web/src/views/evaluation/EvaluationList.vue` |
| 设置页面 | ✅ | `web/src/views/settings/` |
| 平台首页 | ✅ | `web/src/views/platform/index.vue` |

**前端组件**:
- `BaseButton` - 基础按钮
- `BaseCard` - 基础卡片
- `BaseModal` - 基础弹窗
- `BaseInput` - 基础输入框
- `BaseSidebar` - 基础侧边栏

---

## 二、部分完成功能

### 2.1 认证注册接口 ✅ 已完成

**状态**: 完整实现

**已实现功能**:
- 用户注册 (`POST /api/v1/auth/register`)
- 用户登录 (`POST /api/v1/auth/login`)
- Token 刷新 (`POST /api/v1/auth/refresh`)
- 用户登出 (`POST /api/v1/auth/logout`)
- 获取用户信息 (`GET /api/v1/user/profile`)

**实现位置**:
- 服务层: `internal/application/user/user.go`
- 处理器: `internal/interface/http/handler/handlers.go` (AuthHandler)
- 路由: `internal/interface/http/router/router.go`
- JWT 工具: `pkg/jwt/jwt.go`
- 密码加密: `pkg/crypto/password.go`

**支持的功能**:
- Argon2id 密码哈希
- JWT Token 生成与验证 (Access Token + Refresh Token)
- 多租户隔离 (tenant_id)
- 用户状态检查

---

### 2.2 知识条目详情/更新

**当前状态**: 路由占位

```go
// internal/router/knowledge_routes.go:37-47
knowledgeItems.GET("/:knowledge_id", ...)  // "to be implemented"
knowledgeItems.PUT("/:knowledge_id", ...)  // "to be implemented"
```

---

### 2.3 分块详情/更新/删除

**当前状态**: 路由占位

```go
// internal/router/knowledge_routes.go:58-75
chunks.GET("/:chunk_id", ...)     // "to be implemented"
chunks.PUT("/:chunk_id", ...)     // "to be implemented"
chunks.DELETE("/:chunk_id", ...)  // "to be implemented"
chunks.POST("/batch/status", ...) // "to be implemented"
```

---

### 2.4 知识搜索接口

**当前状态**: 路由占位

```go
// internal/router/knowledge_routes.go:115-117
api.POST("/knowledge/search", ...)  // "search knowledge - to be implemented"
```

---

### 2.5 工具管理系统

**当前状态**: 数据表存在，功能部分实现

**已有**:
- `tools` 表结构
- `tool_executions` 表结构
- 部分工具实现

**待完成**:
- 工具管理界面
- 工具创建/编辑/删除
- 工具执行记录查询

---

### 2.6 API 密钥管理

**当前状态**: 数据表存在

**已有**:
- `api_keys` 表结构

**待完成**:
- API 密钥创建/删除
- API 密钥认证中间件
- 密钥使用记录

---

### 2.7 审计日志

**当前状态**: 数据表存在

**已有**:
- `audit_logs` 表结构
- `permission_audit_logs` 表结构

**待完成**:
- 审计日志记录
- 审计日志查询
- 审计日志导出

---

### 2.8 搜索历史

**当前状态**: 数据表存在

**已有**:
- `search_history` 表结构

**待完成**:
- 搜索历史记录
- 搜索历史查询
- 热门搜索统计

---

## 三、未完成功能

### 3.1 用户偏好设置

**表结构**: `user_preferences` 表存在

**待实现**:
- 偏好设置 CRUD
- 主题切换
- 语言切换
- 通知设置

---

### 3.2 文件下载

**已有**: `internal/handler/download.go`

**待确认**:
- 文件下载接口完整性
- 权限验证
- 下载限流

---

### 3.3 数据集管理增强

**已有基础功能**:
- 数据集创建
- 数据集列表

**待增强**:
- 数据集导入/导出
- 数据集编辑
- 数据集删除
- 数据集版本管理

---

### 3.4 系统配置

**表结构**: `system_config` 表存在

**待实现**:
- 系统配置 CRUD
- 配置热更新
- 配置校验

---

## 四、数据库表结构

### 4.1 核心业务表

| 表名 | 说明 | 状态 |
|------|------|------|
| `tenants` | 租户表 | ✅ |
| `users` | 用户表 | ✅ |
| `roles` | 角色表 | ✅ |
| `permissions` | 权限表 | ✅ |
| `user_roles` | 用户角色关联 | ✅ |
| `role_permissions` | 角色权限关联 | ✅ |
| `resource_permissions` | 资源级权限 | ✅ |
| `knowledge_bases` | 知识库表 | ✅ |
| `knowledges` | 知识条目表 | ✅ |
| `chunks` | 文档分块表 | ✅ |
| `knowledge_tags` | 知识标签表 | ✅ |
| `kb_settings` | 知识库设置表 | ✅ |
| `sessions` | 会话表 | ✅ |
| `messages` | 消息表 | ✅ |
| `message_feedback` | 消息反馈表 | ✅ |
| `models` | 模型表 | ✅ |
| `evaluation_tasks` | 评估任务表 | ✅ |
| `evaluation_metrics` | 评估指标表 | ✅ |
| `dataset_records` | 数据集记录表 | ✅ |
| `retrieval_settings` | 检索设置表 | ✅ |

### 4.2 辅助功能表

| 表名 | 说明 | 状态 |
|------|------|------|
| `refresh_tokens` | 刷新令牌表 | ✅ |
| `api_keys` | API 密钥表 | ⚠️ 表存在，功能待实现 |
| `tools` | 工具表 | ⚠️ 表存在，功能部分实现 |
| `tool_executions` | 工具执行记录表 | ⚠️ 表存在，功能待实现 |
| `audit_logs` | 审计日志表 | ⚠️ 表存在，功能待实现 |
| `permission_audit_logs` | 权限审计日志表 | ⚠️ 表存在，功能待实现 |
| `search_history` | 搜索历史表 | ⚠️ 表存在，功能待实现 |
| `user_preferences` | 用户偏好表 | ⚠️ 表存在，功能待实现 |
| `system_config` | 系统配置表 | ⚠️ 表存在，功能待实现 |

### 4.3 视图

| 视图名 | 说明 | 状态 |
|--------|------|------|
| `v_kb_stats` | 知识库统计视图 | ✅ |
| `v_tenant_stats` | 租户统计视图 | ✅ |
| `v_user_permissions` | 用户权限视图 | ✅ |
| `v_user_roles` | 用户角色视图 | ✅ |

---

## 五、API 路由概览

### 5.1 认证相关

```
POST /api/v1/auth/register    # 注册 (⚠️ 占位)
POST /api/v1/auth/login       # 登录 (⚠️ 占位)
POST /api/v1/auth/refresh     # 刷新 Token
```

### 5.2 租户管理

```
GET    /api/v1/tenants          # 租户列表
POST   /api/v1/tenants          # 创建租户
GET    /api/v1/tenants/:id      # 租户详情
PUT    /api/v1/tenants/:id      # 更新租户
DELETE /api/v1/tenants/:id      # 删除租户
POST   /api/v1/tenants/:id/api-key  # 重新生成 API Key
GET    /api/v1/tenants/:id/storage  # 存储使用情况
```

### 5.3 聊天相关

```
POST /api/v1/chat              # 普通聊天
POST /api/v1/chat/stream       # 流式聊天
```

### 5.4 会话相关

```
POST   /api/v1/sessions                    # 创建会话
GET    /api/v1/sessions                    # 会话列表
GET    /api/v1/sessions/:id                # 会话详情
PUT    /api/v1/sessions/:id                # 更新会话
DELETE /api/v1/sessions/:id                # 删除会话
POST   /api/v1/sessions/:id/archive        # 归档会话
POST   /api/v1/sessions/:id/activate       # 激活会话
GET    /api/v1/sessions/:id/detail         # 会话详情（含消息）
```

### 5.5 消息相关

```
GET    /api/v1/messages         # 消息列表
GET    /api/v1/messages/:id     # 消息详情
PUT    /api/v1/messages/:id     # 更新消息
DELETE /api/v1/messages/:id     # 删除消息
```

### 5.6 知识库相关

```
POST   /api/v1/knowledge-bases              # 创建知识库
GET    /api/v1/knowledge-bases              # 知识库列表
GET    /api/v1/knowledge-bases/:id          # 知识库详情
PUT    /api/v1/knowledge-bases/:id          # 更新知识库
DELETE /api/v1/knowledge-bases/:id          # 删除知识库
GET    /api/v1/knowledge-bases/:id/stats    # 知识库统计
```

### 5.7 知识条目相关

```
POST   /api/v1/knowledge-bases/:id/knowledge/file      # 上传文件
GET    /api/v1/knowledge-bases/:kb_id/knowledge        # 知识条目列表
GET    /api/v1/knowledge-bases/:kb_id/knowledge/:kid   # 知识条目详情 (⚠️ 占位)
DELETE /api/v1/knowledge-bases/:kb_id/knowledge/:kid   # 删除知识条目
PUT    /api/v1/knowledge-bases/:kb_id/knowledge/:kid   # 更新知识条目 (⚠️ 占位)
GET    /api/v1/knowledge-bases/:id/knowledge/:kid/status  # 处理状态
```

### 5.8 分块相关

```
GET    /api/v1/knowledge-bases/:kb_id/chunks        # 分块列表
GET    /api/v1/knowledge-bases/:kb_id/chunks/:id    # 分块详情 (⚠️ 占位)
PUT    /api/v1/knowledge-bases/:kb_id/chunks/:id    # 更新分块 (⚠️ 占位)
DELETE /api/v1/knowledge-bases/:kb_id/chunks/:id    # 删除分块 (⚠️ 占位)
POST   /api/v1/knowledge-bases/:kb_id/chunks/batch/status  # 批量更新 (⚠️ 占位)
```

### 5.9 模型相关

```
GET /api/v1/models           # 模型列表
GET /api/v1/models/:id       # 模型详情
```

### 5.10 评估相关

```
POST   /api/v1/evaluation         # 创建评估任务
GET    /api/v1/evaluation         # 查询评估结果
GET    /api/v1/evaluations        # 评估任务列表
GET    /api/v1/evaluations/:id    # 评估任务详情
DELETE /api/v1/evaluations/:id    # 删除评估任务
POST   /api/v1/datasets           # 创建数据集
GET    /api/v1/datasets           # 数据集列表
```

### 5.11 权限相关

```
GET    /api/v1/permissions           # 权限列表
POST   /api/v1/permissions/check    # 检查权限
GET    /api/v1/roles                 # 角色列表
POST   /api/v1/roles                 # 创建角色
GET    /api/v1/roles/:id             # 角色详情
PUT    /api/v1/roles/:id             # 更新角色
DELETE /api/v1/roles/:id             # 删除角色
```

### 5.12 RAG 优化相关

```
# HyDE 假设文档生成
POST   /api/v1/rag/hyde/generate              # 生成假设文档
POST   /api/v1/rag/hyde/generate-multiple     # 生成多个假设文档

# 查询优化
POST   /api/v1/rag/query/rewrite              # 重写查询
POST   /api/v1/rag/query/expand               # 扩展查询
POST   /api/v1/rag/query/decompose            # 分解查询

# 多跳检索
POST   /api/v1/rag/multi-hop/retrieve         # 多跳检索

# 综合优化
POST   /api/v1/rag/optimize/query             # 综合查询优化
GET    /api/v1/rag/optimize/config            # 获取优化配置
PUT    /api/v1/rag/optimize/config            # 更新优化配置
```

### 5.13 安全护栏相关

```
# 输入检查
POST   /api/v1/guardrail/input/check          # 检查输入安全性
POST   /api/v1/guardrail/input/sanitize       # 清理输入内容

# 输出检查
POST   /api/v1/guardrail/output/check         # 检查输出安全性
POST   /api/v1/guardrail/output/sanitize      # 清理输出内容

# 越狱检测
POST   /api/v1/guardrail/jailbreak/check      # 检测越狱攻击
POST   /api/v1/guardrail/is-jailbreak         # 快速越狱检查

# 综合检查
POST   /api/v1/guardrail/full-check           # 完整输入检查
POST   /api/v1/guardrail/check-both           # 输入输出检查
POST   /api/v1/guardrail/quick-check          # 快速检查
POST   /api/v1/guardrail/quick-sanitize       # 快速清理

# 配置和建议
GET    /api/v1/guardrail/config/default       # 获取默认配置
POST   /api/v1/guardrail/recommendation       # 获取处理建议
```

---

## 六、待办事项

### 高优先级

1. **完善知识条目操作** - 实现知识条目的详情查询和更新功能
2. **完善分块操作** - 实现分块的详情查询、更新和删除功能
3. **实现知识搜索** - 实现全局知识搜索接口

### 中优先级

1. **工具管理系统** - 完善工具的管理和执行记录
2. **API 密钥管理** - 实现 API 密钥的创建和管理
3. **审计日志** - 实现审计日志的记录和查询
4. **搜索历史** - 实现搜索历史的记录和统计

### 低优先级

1. **用户偏好设置** - 实现用户个性化配置
2. **系统配置** - 实现系统级配置管理
3. **数据集增强** - 完善数据集的导入导出功能

---

## 七、技术栈总结

### 后端

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.21+ |
| 框架 | Gin |
| ORM | GORM |
| 数据库 | MySQL 8.0+ |
| 向量库 | Milvus 2.3+ |
| 图数据库 | Neo4j 5.0+ |
| AI 框架 | CloudWeGo Eino |
| 认证 | JWT |

### 前端

| 组件 | 技术 |
|------|------|
| 框架 | Vue 3.4+ |
| 语言 | TypeScript 5.0+ |
| UI 库 | Element Plus |
| 状态管理 | Pinia |
| 路由 | Vue Router 4+ |
| 构建工具 | Vite 5+ |
| 图谱可视化 | vis-network |

---

**文档维护**: Cognida Team
**最后更新**: 2026-02-20

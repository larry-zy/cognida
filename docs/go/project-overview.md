# Link 智能知识图谱对话系统 - 项目详细说明文档

## 一、项目概述

**Link** 是一个基于 **Go + Vue3** 构建的**企业级智能知识图谱对话系统**，采用多租户架构，集成了向量检索、图数据库和大模型Agent能力，旨在提供专业的知识管理和智能问答解决方案。

### 1.1 核心定位
- **智能知识管理平台**：支持文档上传、自动分块、向量化存储
- **多模态检索系统**：向量检索、BM25关键词检索、图谱检索、混合检索
- **AI Agent协作平台**：基于Cloudwego Eino框架的多代理协作系统
- **企业级SaaS架构**：完整的多租户隔离和权限管理

### 1.2 项目目录结构

```
link/
├── cmd/
│   └── server/          # 服务入口
├── internal/            # 核心业务逻辑
│   ├── application/     # 应用层（Service + Repository）
│   ├── agent/          # Agent系统
│   ├── config/         # 配置管理
│   ├── container/      # 依赖注入容器
│   ├── handler/        # HTTP处理器
│   ├── middleware/     # 中间件
│   ├── models/         # 数据模型
│   ├── router/         # 路由配置
│   └── types/          # 类型定义
├── web/                # 前端代码（Vue3）
├── config/             # 配置文件
│   └── prompt_templates/ # AI提示词模板
├── migrations/         # 数据库迁移文件
├── docs/               # 项目文档
└── uploads/            # 文件上传目录
```

---

## 二、技术架构

### 2.1 整体架构设计

项目采用经典的 **DDD（领域驱动设计）分层架构**：

```
┌─────────────────────────────────────────────────────────────────┐
│                        Frontend (Vue3)                          │
│  TypeScript + Vite + Element Plus + Pinia + Vue Router         │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      API Gateway (Gin)                          │
│              Middleware: Auth | Tenant | CORS | Logger          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌───────────────────┬───────────────────┬─────────────────────────┐
│    Handler Layer  │  Service Layer    │   Repository Layer      │
│  (HTTP handlers)  │  (Business Logic) │   (Data Access)         │
└───────────────────┴───────────────────┴─────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Infrastructure Layer                       │
│   MySQL │ Milvus │ Neo4j │ LLM API │ Search API                │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 后端技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| **Go** | 1.25.6 | 主要开发语言 |
| **Gin** | 1.11.0 | Web框架 |
| **GORM** | 1.31.1 | ORM框架 |
| **Cloudwego Eino** | 0.7.32 | AI Agent框架 |
| **Milvus SDK** | 2.4.2 | 向量数据库 |
| **Neo4j Driver** | 5.28.4 | 图数据库 |
| **JWT** | 5.3.1 | 身份认证 |

### 2.3 前端技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| **Vue** | 3.4.21 | 前端框架 |
| **TypeScript** | 5.3.3 | 类型系统 |
| **Vite** | 5.1.4 | 构建工具 |
| **Element Plus** | 2.5.6 | UI组件库 |
| **Pinia** | 2.1.7 | 状态管理 |
| **Vue Router** | 4.2.5 | 路由管理 |
| **Vue I18n** | 9.10.1 | 国际化 |
| **vis-network** | 10.0.2 | 图谱可视化 |

### 2.4 数据存储

| 组件 | 用途 |
|------|------|
| **MySQL** | 主数据库（用户、租户、知识库元数据） |
| **Milvus** | 向量数据库（稠密/稀疏向量存储） |
| **Neo4j** | 图数据库（实体关系图谱） |

---

## 三、核心功能模块详解

### 3.1 多租户系统

#### 设计理念
采用 **共享数据库、共享Schema** 的多租户架构，通过 `tenant_id` 字段实现逻辑隔离。

#### 核心功能
- **租户管理**：创建、更新、删除租户
- **API密钥管理**：每个租户独立API Key
- **存储配额**：租户级别的存储限制和统计
- **数据隔离**：所有业务数据自动按租户隔离

#### API接口
```
POST   /api/v1/tenants              # 创建租户
GET    /api/v1/tenants              # 租户列表
GET    /api/v1/tenants/:id          # 租户详情
PUT    /api/v1/tenants/:id          # 更新租户
DELETE /api/v1/tenants/:id          # 删除租户
POST   /api/v1/tenants/:id/api-key  # 重新生成API Key
GET    /api/v1/tenants/:id/storage  # 存储使用情况
```

#### 代码位置
- 服务层：`internal/application/tenant.go`
- 数据层：`internal/application/repository/tenant.go`
- 处理器：`internal/handler/tenant.go`

### 3.2 用户认证与权限系统

#### 认证机制
- **JWT Token认证**：Access Token + Refresh Token双token机制
- **Token刷新**：自动刷新过期token
- **多租户登录**：登录时需提供tenant_id

#### 权限模型（RBAC）
```
User (用户)
  ↓ N:M
Role (角色)
  ↓ N:M
Permission (权限)
  ↓
Resource (资源) + Action (操作)
```

#### 权限类型
- **系统预设权限**：对所有租户可用
- **自定义权限**：租户可创建
- **资源级权限**：细粒度到具体资源

#### API接口
```
POST /api/v1/auth/register  # 注册
POST /api/v1/auth/login     # 登录
POST /api/v1/auth/refresh   # 刷新token
POST /api/v1/auth/logout    # 登出
GET  /api/v1/user/profile   # 用户信息
```

#### 代码位置
- 认证中间件：`internal/middleware/auth.go`
- 权限中间件：`internal/middleware/permission.go`
- 认证处理器：`internal/handler/auth.go`

### 3.3 知识库管理系统

#### 核心流程
```
文件上传 → 文档解析 → 智能分块 → 向量化 → 图谱构建 → 入库完成
```

#### 文档处理
| 功能 | 实现方式 |
|------|----------|
| 文件解析 | 支持PDF、Word、TXT、Markdown等格式 |
| 智能分块 | 基于语义和固定大小的混合分块策略 |
| 向量化 | 阿里云DashScope Embedding模型 |
| 图谱构建 | 自动提取实体和关系 |

#### 数据表结构
```sql
-- 知识库表
knowledge_bases (
    id, tenant_id, user_id, name, description,
    status, document_count, chunk_count, storage_size
)

-- 知识条目表
knowledges (
    id, kb_id, type, title, source,
    parse_status, chunk_count
)

-- 文档分块表（核心存储）
chunks (
    id, tenant_id, kb_id, knowledge_id,
    content, chunk_type, embedding_id,
    token_count, metadata
)

-- 标签表
knowledge_tags (
    id, tenant_id, knowledge_base_id,
    name, color, sort_order
)

-- 知识库配置
kb_settings (
    kb_id, graph_enabled, bm25_enabled,
    chunking_config, image_processing_config,
    extract_config
)
```

#### API接口
```
# 知识库管理
POST   /api/v1/knowledge-bases                    # 创建知识库
GET    /api/v1/knowledge-bases                    # 知识库列表
GET    /api/v1/knowledge-bases/:id                # 知识库详情
PUT    /api/v1/knowledge-bases/:id                # 更新知识库
DELETE /api/v1/knowledge-bases/:id                # 删除知识库
GET    /api/v1/knowledge-bases/:id/stats          # 统计信息

# 文件操作
POST   /api/v1/knowledge-bases/:id/knowledge/file # 上传文件
GET    /api/v1/knowledge-bases/:id/knowledge      # 知识条目列表
DELETE /api/v1/knowledge-bases/:id/knowledge/:kid # 删除知识条目
GET    /api/v1/knowledge-bases/:id/chunks         # 分块列表
```

#### 代码位置
- 服务层：`internal/application/knowledge_base.go`
- 处理器：`internal/handler/knowledge_base.go`

### 3.4 RAG检索增强生成系统

这是项目的核心功能模块，位于 `internal/application/rag/`

#### 检索模式

| 模式 | 说明 | 实现方式 |
|------|------|----------|
| **向量检索** | 语义相似度检索 | Milvus向量搜索 + 余弦相似度 |
| **BM25检索** | 关键词检索 | 自实现BM25算法 + 中文分词 |
| **图谱检索** | 实体关系检索 | Neo4j图遍历 + PMI权重 |
| **混合检索** | 多模式融合 | RRF算法 + 加权融合 |

#### RAG Pipeline流程
```
1. 查询增强（可选）
   ├── 查询重写（处理代词、省略）
   └── 查询拆分（多问题分离）

2. 并行检索
   ├── 向量检索
   ├── BM25检索
   └── 图谱检索

3. 结果融合
   ├── RRF融合
   ├── 去重
   └── TopK截取

4. 重排序（可选）
   ├── 模型重排
   └── 加权排序

5. 上下文构建
   └── 返回增强后的上下文
```

#### 重排序策略
```go
// rerank.go 支持的策略
- RRF (Reciprocal Rank Fusion): 基于排名融合
- Weighted Score Fusion: 加权分数融合
- Weighted RRF: 加权排名融合
- Model Rerank: 使用专门重排模型
```

#### API接口
```
POST /api/v1/chat/rag              # RAG聊天
POST /api/v1/chat/rag/stream       # RAG流式聊天
```

#### 代码位置
- Pipeline：`internal/application/rag/pipeline.go`
- 检索器：`internal/application/rag/retriever.go`
- 重排序：`internal/application/rag/rerank.go`
- RAG聊天：`internal/application/rag/rag_chat.go`

### 3.5 多代理协作系统

基于 **Cloudwego Eino** 框架实现的企业级Agent系统。

#### 系统架构
```
┌─────────────────────────────────────────────────────────┐
│              MultiAgentOrchestrator                      │
│                   (主协调器)                             │
└─────────────────────────────────────────────────────────┘
         │         │         │         │         │
         ▼         ▼         ▼         ▼         ▼
    Planner  Retriever  Analyzer  Synthesizer  Critic
    (规划)   (检索)    (分析)    (合成)      (评审)
```

#### 子Agent功能

| Agent | 职责 | 核心能力 |
|-------|------|----------|
| **Coordinator** | 任务协调 | 决策调用哪个子代理 |
| **Planner** | 研究规划 | 分解任务、制定计划、提取关键词 |
| **Retriever** | 信息检索 | RAG查询、网络搜索、智能检索 |
| **Analyzer** | 深度分析 | 信息验证、置信度评估、模式识别 |
| **Synthesizer** | 报告合成 | 整合信息、生成结构化报告 |
| **Critic** | 质量评审 | 多维度评分、改进建议 |

#### 工具生态
```go
// internal/agent/tool/ 工具注册
- rag_query:         知识库检索
- web_search:        网络搜索（Metaso API）
- smart_retrieval:   智能检索（自动匹配知识库）
- calculator:        计算器
- get_current_time:  获取时间
- http_request:      HTTP请求
```

#### 执行流程
```
用户查询
  → Coordinator分析
  → Planner制定计划
  → Retriever检索信息
  → Analyzer深度分析
  → Synthesizer生成报告
  → Critic质量评审
  → [评分<阈值？→修订]
  → 返回最终答案
```

#### API接口
```
POST /api/v1/agent/chat          # Agent聊天
POST /api/v1/agent/chat/stream   # Agent流式聊天
GET  /api/v1/agent/tools         # 可用工具列表
```

#### 代码位置
- Agent服务：`internal/application/agent/agentic_rag_agent.go`
- 工具定义：`internal/agent/tool/`
- Agent处理器：`internal/handler/agent.go`

### 3.6 知识图谱系统

#### 功能特性
- **实体提取**：自动从文档中提取实体
- **关系提取**：识别实体间的关系
- **PMI计算**：计算点互信息评估关系强度
- **图谱可视化**：使用vis-network前端展示

#### 数据存储
```
Neo4j图数据库:
  Node (实体) --[Relation]--> Node (实体)
  每个节点关联到具体chunk
```

#### API接口
```
GET    /api/v1/knowledge-bases/:id/graph               # 获取图谱
POST   /api/v1/knowledge-bases/:id/graph/search        # 搜索节点
GET    /api/v1/knowledge-bases/:id/graph/nodes/:id     # 节点详情
POST   /api/v1/knowledge-bases/:id/graph/nodes         # 添加节点
PUT    /api/v1/knowledge-bases/:id/graph/nodes/:id     # 更新节点
DELETE /api/v1/knowledge-bases/:id/graph/nodes/:id     # 删除节点
POST   /api/v1/knowledge-bases/:id/graph/relations     # 添加关系
PUT    /api/v1/knowledge-bases/:id/graph/relations/:id # 更新关系
DELETE /api/v1/knowledge-bases/:id/graph/relations/:id # 删除关系
GET    /api/v1/knowledge-bases/:id/graph/relation-types # 关系类型
DELETE /api/v1/knowledge-bases/:id/graph               # 删除图谱
```

#### 代码位置
- 图谱服务：`internal/application/graph.go`
- 图谱处理器：`internal/handler/graph.go`
- Neo4j仓储：`internal/application/repository/retriever/neo4j/`

### 3.7 会话与消息系统

#### 会话配置
- 支持多轮对话
- 可配置最大轮次
- 关联知识库
- Agent配置
- 检索配置

#### API接口
```
# 会话管理
POST   /api/v1/sessions                # 创建会话
GET    /api/v1/sessions                # 会话列表
GET    /api/v1/sessions/:id            # 会话详情
PUT    /api/v1/sessions/:id            # 更新会话
DELETE /api/v1/sessions/:id            # 删除会话
POST   /api/v1/sessions/:id/archive    # 归档会话
POST   /api/v1/sessions/:id/activate   # 激活会话

# 消息管理
GET    /api/v1/messages                # 消息列表
GET    /api/v1/messages/:id            # 消息详情
PUT    /api/v1/messages/:id            # 更新消息
DELETE /api/v1/messages/:id            # 删除消息
```

#### 代码位置
- 会话服务：`internal/application/session.go`
- 消息服务：`internal/application/message.go`
- 会话处理器：`internal/handler/session.go`
- 消息处理器：`internal/handler/message.go`

### 3.8 模型评估系统

#### 评估维度
- **准确性**：答案正确性
- **完整性**：覆盖面评估
- **相关性**：与问题的匹配度
- **时效性**：信息新鲜度

#### API接口
```
POST /api/v1/evaluation          # 创建评估任务
GET  /api/v1/evaluation          # 获取评估结果
GET  /api/v1/evaluations         # 评估任务列表
GET  /api/v1/evaluations/:id     # 评估详情
DELETE /api/v1/evaluations/:id   # 删除评估
POST /api/v1/datasets            # 创建数据集
GET  /api/v1/datasets            # 数据集列表
```

#### 代码位置
- 评估服务：`internal/application/evaluation.go`
- 评估处理器：`internal/handler/evaluation.go`
- 详细文档：`docs/evaluation_system.md`

---

## 四、前端架构详解

### 4.1 目录结构
```
web/src/
├── api/          # API接口模块化封装
│   ├── agent/
│   ├── auth/
│   ├── chat/
│   ├── evaluation/
│   ├── graph/
│   ├── knowledge/
│   ├── message/
│   ├── model/
│   ├── session/
│   └── tenant/
├── component/    # 可复用组件库
│   ├── BaseButton.vue
│   ├── BaseCard.vue
│   ├── BaseInput.vue
│   ├── BaseModal.vue
│   ├── BaseTable.vue
│   ├── AppLayout.vue
│   ├── BaseSidebar.vue
│   └── ...
├── views/        # 页面视图（路由懒加载）
│   ├── home/
│   ├── chat/
│   ├── knowledge/
│   ├── agent/
│   ├── evaluation/
│   └── platform/
├── stores/       # Pinia状态管理
│   ├── auth.ts
│   ├── ui.ts
│   ├── knowledge.ts
│   └── settings.ts
├── router/       # Vue Router配置
├── i18n/         # 国际化配置
├── types/        # TypeScript类型定义
└── utils/        # 工具函数
```

### 4.2 组件设计

#### 基础组件
| 组件 | 功能 |
|------|------|
| **BaseButton** | 可定制按钮（主题、尺寸、禁用状态） |
| **BaseCard** | 卡片容器 |
| **BaseInput** | 输入框（支持校验） |
| **BaseModal** | 模态框 |
| **BaseTag** | 标签组件 |

#### 数据组件
| 组件 | 功能 |
|------|------|
| **BaseTable** | 表格（分页、排序） |
| **BaseLoader** | 加载动画 |
| **EmptyState** | 空状态展示 |

#### 布局组件
| 组件 | 功能 |
|------|------|
| **AppLayout** | 主布局（侧边栏+头部+内容区） |
| **AppBackground** | 背景（粒子效果） |
| **BaseSidebar** | 侧边栏（折叠/展开） |

### 4.3 状态管理
```typescript
// stores/
├── auth.ts      // 认证状态（token、用户信息、租户切换）
├── ui.ts        // UI状态（侧边栏、加载状态、当前会话）
├── knowledge.ts // 知识库状态
└── settings.ts  // 用户偏好
```

### 4.4 玻璃态UI设计
- CSS变量主题系统
- `backdrop-filter`背景模糊
- 半透明层级设计
- 支持主题切换

### 4.5 国际化
- 支持中文（zh-CN）和英文（en-US）
- 模块化翻译文件
- 自动语言检测

---

## 五、数据库设计详解

### 5.1 核心表关系

```
tenants (租户)
  │
  ├── users (用户) ── user_roles ── roles (角色) ── role_permissions ── permissions
  │
  ├── knowledge_bases (知识库)
  │     │
  │     ├── knowledges (知识条目)
  │     │     │
  │     │     └── chunks (文档分块) ── Milvus向量
  │     │
  │     └── kb_settings (知识库配置)
  │
  ├── sessions (会话)
  │     │
  │     └── messages (消息)
  │
  └── graphs (图谱) ── Neo4j
```

### 5.2 关键表设计

#### tenants（租户表）
```sql
CREATE TABLE tenants (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(100) NOT NULL,
    api_key VARCHAR(64),
    storage_quota BIGINT DEFAULT 10737418240,
    storage_used BIGINT DEFAULT 0,
    retriever_engines JSON,
    agent_config JSON,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

#### users（用户表）
```sql
CREATE TABLE users (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    tenant_id BIGINT NOT NULL,
    username VARCHAR(50) UNIQUE,
    email VARCHAR(100),
    password_hash VARCHAR(255),
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    deleted_at NULL,
    INDEX idx_tenant (tenant_id)
);
```

#### knowledge_bases（知识库表）
```sql
CREATE TABLE knowledge_bases (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    user_id BIGINT,
    name VARCHAR(100),
    description TEXT,
    status VARCHAR(20),
    document_count INT DEFAULT 0,
    chunk_count INT DEFAULT 0,
    storage_size BIGINT DEFAULT 0,
    created_at TIMESTAMP,
    INDEX idx_tenant (tenant_id)
);
```

#### chunks（文档分块表）
```sql
CREATE TABLE chunks (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT,
    kb_id VARCHAR(36),
    knowledge_id VARCHAR(36),
    content TEXT,
    chunk_type VARCHAR(20),
    embedding_id VARCHAR(100),
    token_count INT,
    metadata JSON,
    created_at TIMESTAMP,
    deleted_at NULL,
    INDEX idx_tenant_kb (tenant_id, kb_id),
    INDEX idx_embedding (embedding_id)
);
```

#### sessions（会话表）
```sql
CREATE TABLE sessions (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT,
    user_id BIGINT,
    title VARCHAR(200),
    kb_id VARCHAR(36),
    max_rounds INT,
    enable_rewrite BOOLEAN,
    rerank_model_id VARCHAR(36),
    agent_config JSON,
    context_config JSON,
    created_at TIMESTAMP,
    INDEX idx_tenant_user (tenant_id, user_id)
);
```

### 5.3 数据库迁移文件
```
migrations/
├── link_go.sql              # 主数据库结构
├── kb_settings.sql          # 知识库设置迁移
├── retrieval_settings.sql   # 检索设置迁移
├── evaluation.sql           # 评估相关表
└── sessions.sql             # 会话数据迁移
```

---

## 六、API接口汇总

### 6.1 认证接口
```
POST /api/v1/auth/register  # 注册
POST /api/v1/auth/login     # 登录
POST /api/v1/auth/refresh   # 刷新token
POST /api/v1/auth/logout    # 登出
```

### 6.2 租户接口
```
POST   /api/v1/tenants              # 创建租户
GET    /api/v1/tenants              # 租户列表
GET    /api/v1/tenants/:id          # 租户详情
PUT    /api/v1/tenants/:id          # 更新租户
DELETE /api/v1/tenants/:id          # 删除租户
POST   /api/v1/tenants/:id/api-key  # 重新生成API Key
GET    /api/v1/tenants/:id/storage  # 存储使用情况
```

### 6.3 聊天接口
```
POST /api/v1/chat              # 普通聊天
POST /api/v1/chat/stream       # 流式聊天
POST /api/v1/chat/rag          # RAG聊天
POST /api/v1/chat/rag/stream   # RAG流式
POST /api/v1/agent/chat        # Agent聊天
POST /api/v1/agent/chat/stream # Agent流式
```

### 6.4 会话接口
```
POST   /api/v1/sessions                # 创建会话
GET    /api/v1/sessions                # 会话列表
GET    /api/v1/sessions/:id            # 会话详情
PUT    /api/v1/sessions/:id            # 更新会话
DELETE /api/v1/sessions/:id            # 删除会话
POST   /api/v1/sessions/:id/archive    # 归档会话
POST   /api/v1/sessions/:id/activate   # 激活会话
```

### 6.5 知识库接口
```
POST   /api/v1/knowledge-bases                    # 创建知识库
GET    /api/v1/knowledge-bases                    # 知识库列表
GET    /api/v1/knowledge-bases/:id                # 知识库详情
PUT    /api/v1/knowledge-bases/:id                # 更新知识库
DELETE /api/v1/knowledge-bases/:id                # 删除知识库
GET    /api/v1/knowledge-bases/:id/stats          # 统计信息
POST   /api/v1/knowledge-bases/:id/knowledge/file # 上传文件
GET    /api/v1/knowledge-bases/:id/chunks         # 分块列表
```

### 6.6 图谱接口
```
GET    /api/v1/knowledge-bases/:id/graph               # 获取图谱
POST   /api/v1/knowledge-bases/:id/graph/search        # 搜索节点
GET    /api/v1/knowledge-bases/:id/graph/nodes/:id     # 节点详情
POST   /api/v1/knowledge-bases/:id/graph/nodes         # 添加节点
PUT    /api/v1/knowledge-bases/:id/graph/nodes/:id     # 更新节点
DELETE /api/v1/knowledge-bases/:id/graph/nodes/:id     # 删除节点
POST   /api/v1/knowledge-bases/:id/graph/relations     # 添加关系
PUT    /api/v1/knowledge-bases/:id/graph/relations/:id # 更新关系
DELETE /api/v1/knowledge-bases/:id/graph/relations/:id # 删除关系
```

---

## 七、技术亮点与创新点

### 7.1 架构设计

1. **DDD分层架构**
   - 清晰的职责划分
   - 易于维护和扩展
   - 符合企业级标准

2. **多租户隔离**
   - 共享数据库降低成本
   - 完整的数据隔离保证安全
   - 租户级配置灵活性

3. **依赖注入容器**
   - 统一管理依赖
   - 生命周期控制
   - 便于测试

### 7.2 AI能力

1. **多代理协作**
   - 基于Cloudwego Eino框架
   - ReAct模式实现
   - 强制反思机制保证质量

2. **多模态检索**
   - 向量+关键词+图谱三重检索
   - RRF融合算法
   - 可配置的重排序策略

3. **查询增强**
   - 自动查询重写
   - 查询拆分
   - 上下文补全

### 7.3 工程实践

1. **流式响应**
   - SSE支持
   - 实时Agent思考展示

2. **错误处理**
   - 优雅降级
   - 详细错误日志
   - 自动重试机制

3. **国际化**
   - 中英文双语
   - 模块化翻译

---

## 八、部署说明

### 8.1 环境要求
- Go 1.25.6+
- Node.js 18+
- MySQL 8.0+
- Milvus 2.4+
- Neo4j 5.x

### 8.2 配置项（.env）
```bash
# 数据库
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=***
DB_NAME=link_go

# Milvus
MILVUS_HOST=***
MILVUS_TOKEN=***

# Neo4j
NEO4J_URI=bolt://localhost:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=***

# JWT
JWT_SECRET=***
JWT_ACCESS_TOKEN_EXPIRE=86400

# LLM
CHAT_PROVIDER=openai
CHAT_BASE_URL=https://api.openai.com/v1
CHAT_MODEL_NAME=gpt-3.5-turbo
CHAT_API_KEY=***

# Embedding
EMBEDDING_PROVIDER=dashscope
EMBEDDING_API_KEY=***
EMBEDDING_MODEL=text-embedding-v3

# 搜索
METASO_API_KEY=***

# 服务器
SERVER_PORT=8080
GIN_MODE=debug
```

### 8.3 启动步骤

#### 后端启动
```bash
# 1. 配置环境变量
cp .env.example .env

# 2. 初始化数据库
mysql -u root -p < migrations/link_go.sql

# 3. 启动服务
go run cmd/server/main.go
```

#### 前端启动
```bash
# 1. 安装依赖
cd web
npm install

# 2. 启动开发服务器
npm run dev
```

---

## 九、相关文档索引

| 文档 | 路径 | 说明 |
|------|------|------|
| 架构设计 | `docs/architecture.md` | 详细架构说明 |
| Agent框架 | `docs/agent-framework.md` | Agent系统详解 |
| 评估系统 | `docs/evaluation_system.md` | 模型评估详解 |
| RAG召回 | `docs/RAG召回系统文档.md` | 检索系统详解 |
| GraphRAG | `docs/graphrag_community_detection.md` | 图谱检索详解 |
| 数据准备 | `docs/data_preparation.md` | 数据处理流程 |
| 功能状态 | `docs/feature_status.md` | 功能开发进度 |

---

## 十、总结

**Link** 是一个功能完整、架构清晰的企业级智能知识管理系统，特别在以下方面有突出表现：

1. **多Agent协作**：基于Eino框架的成熟多代理系统
2. **多模态检索**：向量、关键词、图谱三重检索能力
3. **企业级架构**：多租户、权限系统、审计日志完备
4. **现代化技术栈**：Go + Vue3 + TypeScript，开发效率高

该项目可作为企业知识管理平台、智能客服系统、RAG应用等场景的基础框架。

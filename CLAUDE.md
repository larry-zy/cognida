# Cognida 开发规范

---

## 强制规则 (MUST)

### 任务完成清理
每次任务完成后，必须终止开启的服务进程。

### 开发流程执行
完整流程必须按序执行：`准备 → 评估 → 开发 → 测试 → Review → 提交`

### 测试强制要求
- 单元测试：核心逻辑必须覆盖
- 集成测试：真实数据库验证
- API测试：涉及HTTP接口时必须执行
- CodeReview：提交前必须通过

### 复杂任务拆分
| 复杂度 | 策略 |
|--------|------|
| <50行 | 直接实现 |
| <200行 | 串行开发 |
| \>200行 | 多Agent并行 |

### 信息收集优先
需求不明确时必须先询问用户，不得假设。

### 分支策略
目前单人开发，直接在 `main` 主分支上开发提交，不新建 feature 分支。

### 数据库表结构同步（版本化迁移，golang-migrate〔INF-4〕）
业务表结构的唯一真源是 `services/cognida-go/migrations/`（成对的 `NNNNNN_*.up.sql` / `.down.sql`），由 `cmd/migrate-db` 驱动执行；**运行时与生产库不做任何自动建表/改表**（已弃用 GORM AutoMigrate）。改 schema 时新增一对迁移文件并同步更新对应 model，二者保持一致。

```bash
cd services/cognida-go && set -a && source .env && set +a
go run ./cmd/migrate-db up          # 应用全部未执行迁移（默认动作）
go run ./cmd/migrate-db version     # 查看当前版本 / dirty
go run ./cmd/migrate-db down [N]    # 回滚 N 步（省略=全部，谨慎）
go run ./cmd/migrate-db force <V>   # 存量库接入：force 1 标记基线已应用
```

新增变更、存量库接入、dirty 处理详见 `services/cognida-go/migrations/README.md`。图谱表（`graph_*`）以 Neo4j 为唯一真源（见〔GO-3〕），不在本迁移范围。

---

## 开发流程 (FLOW)

```
┌─────────────────────────────────────────────────────────────┐
│ 1. 开发前准备                                                │
│    信息不足 → 询问用户 + 联网搜索                            │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. 任务评估                                                  │
│    复杂任务 → 拆分子任务 → 多Agent并行                      │
│    简单任务 → 直接实现                                       │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. 编写代码                                                  │
│    遵循 Go/Python 编码规范                                   │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. 单元测试                                                  │
│    go test ./internal/... -v                                 │
│    pytest tests/ -v                                         │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. 集成测试                                                  │
│    go test -tags=integration ./internal/... -v              │
│    pytest -m integration tests/ -v                          │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ 6. Code Review                                               │
│    触发 code-review skill → 修复问题                         │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
                      提交代码
```

---

## 架构概览 (ARCHITECTURE)

### 目录结构
```
cognida/
├── services/
│   ├── cognida-go/       # Go 服务
│   │   └── internal/
│   │       ├── handler/    # HTTP handlers
│   │       ├── service/    # 业务逻辑
│   │       ├── model/      # 实体和接口定义
│   │       └── repository/ # 数据访问实现
│   └── cognida-python/   # Python 服务
│       └── services/     # 业务逻辑
└── apps/
    └── cognida-web/      # Vue 3 前端（Vite）
```

**依赖方向**：`handler → service → model ← repository`

### 服务通信
| 方式 | 用途 |
|------|------|
| gRPC | 高性能、大数据 |
| MCP  | AI工具调用、实验功能 |

### 存储约定
| 存储 | 用途 |
|------|------|
| MySQL | 元数据、配置、任务状态 |
| Milvus | 向量、特征 |
| Neo4j | 知识图谱、血缘 |
| Redis | 缓存、队列 |

---

## 约定规范 (CONVENTIONS)

### 设计模式
| 模式 | 用途 |
|------|------|
| Builder | 复杂对象构建 |
| Factory | 多Provider创建 |
| Repository | 数据访问封装 |
| Middleware | 横切关注点 |
| Strategy | 算法族 |

### 数据流
- request_id 全链路传递
- 写操作使用 idempotency_key
- 分页使用 cursor 而非 offset

---

## 常用命令 (COMMANDS)

```bash
# Go 测试
cd services/cognida-go && go test ./internal/... -v
cd services/cognida-go && go test -tags=integration ./internal/... -v

# Python 测试
cd services/cognida-python && pytest tests/ -v
cd services/cognida-python && pytest -m integration tests/ -v

# 环境配置
DEV_MODE=true
LOG_LEVEL=debug
MYSQL_DSN=root:password@tcp(localhost:3306)/cognida
MILVUS_ADDRESS=localhost:19530
NEO4J_URI=bolt://localhost:7687
```

---

## 参考文档

- [Feature Roadmap](docs/feature-roadmap.md)
- [MCP Integration](docs/mcp-integration-architecture.md)
- [Skill System](docs/skill-system.md)
- [Refactoring Rules](docs/refactoring-rules-v4-final.md)

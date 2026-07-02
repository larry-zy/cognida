# Link 开发规范

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

### 数据库表结构同步
业务表主流程无 SQL 迁移文件。给 model 加字段/建表后，用 `cd link-go && set -a && source .env && set +a && go run ./cmd/migrate-db` 从 GORM model 同步全部业务表结构（幂等），替代手动 `ALTER TABLE`。图谱表（`graph_*`）由 `graphMetaRepository.ensureSchema` 用内部 model 懒加载建表，不在此工具范围内。

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
link/
├── link-go/           # Go 服务
│   └── internal/
│       ├── handler/    # HTTP handlers
│       ├── service/    # 业务逻辑
│       ├── model/      # 实体和接口定义
│       └── repository/ # 数据访问实现
└── link-python/        # Python 服务
    └── services/       # 业务逻辑
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
cd link-go && go test ./internal/... -v
cd link-go && go test -tags=integration ./internal/... -v

# Python 测试
cd link-python && pytest tests/ -v
cd link-python && pytest -m integration tests/ -v

# 环境配置
DEV_MODE=true
LOG_LEVEL=debug
MYSQL_DSN=root:password@tcp(localhost:3306)/link
MILVUS_ADDRESS=localhost:19530
NEO4J_URI=bolt://localhost:7687
```

---

## 参考文档

- [Feature Roadmap](docs/feature-roadmap.md)
- [MCP Integration](docs/mcp-integration-architecture.md)
- [Skill System](docs/skill-system.md)
- [Refactoring Rules](docs/refactoring-rules-v4-final.md)

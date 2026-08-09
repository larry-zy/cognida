## Context

### 当前状态

Cognida Go 项目采用 3-Layer 架构（handler → service → repository → model），但在实际实现中存在职责混乱：

1. **重复定义**：llm, graph, cache 在 infrastructure 和 service 两层都存在
2. **错位放置**：AI 业务能力被放在 infrastructure 层
3. **命名问题**：builtin, reflection, collaboration 等命名不够直观

### 目标

通过整合和重命名，使代码组织更清晰、更易维护。

## 目标结构

```
internal/
├── common/              # 公共代码
│   ├── errors/
│   ├── types/
│   └── utils/
│
├── service/             # 业务逻辑 + AI 能力
│   ├── agent/
│   │   ├── core/
│   │   ├── presets/         # 预设 Agent
│   │   ├── orchestration/   # 编排（含原 collaboration）
│   │   ├── think/           # 反思（原 reflection）
│   │   └── tools/
│   ├── llm/                 # 整合后的 LLM 服务
│   ├── retrieval/          # 原 search
│   ├── rag/
│   ├── document/
│   ├── graph/               # 整合后的图谱服务
│   ├── tool/
│   ├── conversation/
│   ├── evaluation/
│   ├── guardrail/
│   ├── knowledge/
│   ├── memory/
│   ├── task/
│   ├── tenant/
│   └── user/
│
├── infrastructure/      # 纯技术适配
│   ├── auth/
│   ├── grpc/
│   ├── queue/
│   ├── redis/
│   ├── config/
│   ├── observability/
│   ├── crypto/
│   └── mcp/
│
├── repository/          # 数据访问
├── model/               # 实体定义
└── handler/
```

## 迁移策略

1. **先创建新结构**（common 层）
2. **逐个整合**（llm, graph, cache 等）
3. **更新引用**（所有 import 路径）
4. **验证功能**（测试通过）
5. **清理旧代码**

## 风险控制

- 每个任务独立 commit
- 保留测试覆盖
- 外部接口不变

# Link 项目开发技能包

Link 项目专用开发辅助技能，提供测试、登录、自动化等完整开发支持。

## 技能列表

### mock-login - 模拟登录

测试环境直接登录，无需密码验证。

**使用场景**：
- API 测试需要用户身份
- 功能测试需要特定角色
- 自动化测试快速登录

**测试用户**（数据库已有）：

| 邮箱 | 用户名 | 租户ID | 状态 |
|------|--------|--------|------|
| admin@example.com | admin | 1 | 正常 |
| testuser@example.com | testuser | 1 | 正常 |

---

### testing - 测试技能

提供单元测试、集成测试、E2E 测试的完整指导。

**能力概览**：

| 能力 | 工具/技术 | 适用场景 |
|------|----------|---------|
| 单元测试 | Go testing, Python pytest | 测试单个函数/方法 |
| 集成测试 | testify, pytest fixtures | 测试多组件协同 |
| API 测试 | httptest, test client | 测试 HTTP 接口 |
| E2E 测试 | BrowserAct | 浏览器自动化全流程 |
| UI 测试 | BrowserAct | 用户交互验证 |
| CI/CD 集成 | GitHub Actions | 自动化测试流水线 |

**代码级测试**：
```bash
# Go 测试
go test ./...
go test -cover ./...
go test -tags=integration ./...

# Python 测试
pytest
pytest --cov=services
pytest -m integration
```

**E2E 测试**（BrowserAct）：
```bash
# 快速开始
browser-act get-skills core
browser-act --session test browser open stealth http://localhost:8080
browser-act --session test state
browser-act --session test click 1
browser-act --session test input 2 "text"
```

**测试场景**：
- 单元测试、集成测试、API 测试
- 用户注册登录完整流程
- 知识库创建和管理
- RAG 文档上传和问答
- 多租户数据隔离验证

---

### agent-eval - Agent 端到端评测技能

把「两个核心 Agent 端到端评测」这条线——**脚本 + 指标 + 执行环境**——收敛到一处。
一条命令自启服务、跑 RAG / Data 两个 Agent 各 10 轮连贯对话、逐项断言并采集指标，跑完自动清理。

**覆盖 Agent**：

| Agent | 端点 | 对话主线 |
|-------|------|----------|
| RAG-Agent | `POST /api/v1/agent/knowledge/stream` | 列知识库→概览→逐层检索→综合 |
| Data-Agent | `POST /api/v1/agent/text2sql/stream` | 列表→采样→计数→分组→TopN→概览 |

**通过判据**：无 reasoning_content 400（扫 SSE + 服务端日志）、有非空回答、至少一次成功工具调用。

**采集指标**：每轮/总/均/最快最慢耗时、累计工具调用次数、空回答轮数、回答全文落盘、reasoning_content 400 命中。

**快速开始**：
```bash
# 自启服务→评测→自动清理
./agent-eval/scripts/test-agents-e2e.sh

# 复用已运行服务
SERVER_URL=http://localhost:8080 ./agent-eval/scripts/test-agents-e2e.sh --no-boot

# 指标落盘为 JSON 行
METRICS_FILE=/tmp/agent-eval.jsonl ./agent-eval/scripts/test-agents-e2e.sh
```

详见 `agent-eval/SKILL.md`。

---

## Scripts 脚本包

测试相关的可执行脚本，位于各 skill 的 `scripts/` 目录。

### testing/scripts/

| 脚本 | 用途 |
|------|------|
| `go-test.sh` | Go 测试脚本（单元/集成/覆盖率） |
| `python-test.sh` | Python 测试脚本（单元/集成/覆盖率） |
| `e2e-test.sh` | E2E 测试脚本（BrowserAct 场景） |
| `test-all.sh` | 完整测试流水线（依次运行所有测试） |

### agent-eval/scripts/

| 脚本 | 用途 |
|------|------|
| `test-agents-e2e.sh` | 两个 Agent 多轮对话 E2E 评测（自启服务 + 指标采集 + 自动清理） |

### 使用方法

#### go-test.sh
```bash
./testing/scripts/go-test.sh [unit|integration|all|coverage]
```

#### python-test.sh
```bash
./testing/scripts/python-test.sh [unit|integration|all|coverage]
```

#### e2e-test.sh
```bash
# 查看可用场景
./testing/scripts/e2e-test.sh list

# 运行指定场景
./testing/scripts/e2e-test.sh user-login
./testing/scripts/e2e-test.sh kb-create
./testing/scripts/e2e-test.sh rag-flow
./testing/scripts/e2e-test.sh tenant-isolation

# 自定义 BASE_URL
BASE_URL=http://localhost:3000 ./testing/scripts/e2e-test.sh user-login
```

#### test-all.sh
```bash
# 运行完整测试流水线
./testing/scripts/test-all.sh
```

### E2E 测试场景

| 场景 | 描述 |
|------|------|
| `user-login` | 用户登录流程测试 |
| `kb-create` | 创建知识库流程测试 |
| `rag-flow` | RAG 问答完整流程（登录→上传文档→问答） |
| `tenant-isolation` | 多租户隔离测试 |

---

## 使用示例

### 快速测试用户 API
```
请帮我以 admin@example.com 身份测试用户列表 API
```

### 编写单元测试
```
为 UserUseCase 编写单元测试，覆盖所有分支
```

### E2E 测试
```
测试用户从注册到登录到创建知识库的完整流程
```

### 集成测试
```
设计用户模块的集成测试方案
```

### 运行测试脚本
```
运行 Go 单元测试
运行完整测试流水线
测试用户登录 E2E 场景
```

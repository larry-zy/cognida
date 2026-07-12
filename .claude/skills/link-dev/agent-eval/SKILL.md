---
name: agent-eval
description: 两个核心 Agent（RAG 知识问答 / Data 数据分析）的端到端多轮对话评测技能，一处收敛脚本、指标与执行环境，用于回归 reasoning_content 回传与 RAG 检索缺陷并采集回答质量/耗时/工具调用指标
when_to_use: 改动 Agent 对话链路、检索管线、DeepSeek thinking/工具调用、SSE 流式接口后，需要端到端验证两个 Agent 仍能稳定多轮对话、无 reasoning_content 400、检索非空，并想拿到耗时/工具调用/回答质量指标时使用
category: development
tags:
  - agent
  - e2e
  - evaluation
  - rag
  - text2sql
  - reasoning-content
  - deepseek
  - sse
  - regression
version: "1.0.0"
---

# Agent E2E 评测技能（agent-eval）

把「两个 Agent 端到端评测」这条线——**脚本 + 指标 + 执行环境**——收敛到一个技能里。
一条命令自启服务、跑两个 Agent 各 10 轮连贯对话、逐项断言并采集指标，跑完自动清理。

## 何时用

- 改了 Agent 对话循环、工具调用、检索管线、DeepSeek thinking 处理、SSE 流式接口之后的回归。
- 想确认「能稳定多轮对话」并同时拿到耗时 / 工具调用 / 回答质量指标。
- 复现或验证两个历史缺陷是否复发：
  - **reasoning_content 回传缺陷**：DeepSeek thinking 模式下工具调用轮 HTTP 400
    （`docs/deepseek-thinking-reasoning-content-fix.md`）。
  - **RAG 混合检索恒空缺陷**：RRF 融合分数量纲（~0.01~0.03）被误套余弦阈值（0.5~0.7）过滤光
    （`internal/service/knowledge/pipeline/retriever.go` 的 `HybridRetrieve`）。

## 一条命令跑起来

```bash
# 默认：自编译并启动服务(DEV_MODE=true) → 跑评测 → 自动 kill 服务
.claude/skills/link-dev/agent-eval/scripts/test-agents-e2e.sh

# 复用已在跑的服务（不自启、不扫服务端日志）
SERVER_URL=http://localhost:8080 .claude/skills/link-dev/agent-eval/scripts/test-agents-e2e.sh --no-boot

# 连编译出的 server 二进制也一并保留（日志/指标默认已持久保留）
.claude/skills/link-dev/agent-eval/scripts/test-agents-e2e.sh --keep-log

# 指标默认落到本次产物目录 metrics.jsonl；可显式指定路径覆盖
METRICS_FILE=/path/to/agent-eval.jsonl .claude/skills/link-dev/agent-eval/scripts/test-agents-e2e.sh

# 图谱模式：只跑 RAG，用关系型问法 + 指定已抽取图谱的库，断言 graph_query 确被触发
AGENTS=rag GRAPH_ENABLED=1 KB_IDS=<已抽取图谱的知识库id> \
  .claude/skills/link-dev/agent-eval/scripts/test-agents-e2e.sh --no-boot
```

> 产物落盘：日志 / 指标 / 各轮 SSE 与回答默认持久保留在项目内
> `test-output/agent-eval/<时间戳>-<agents>-<pid>/`（已 gitignore），不再用 `/tmp`，
> 便于历次结果回溯对比；根目录可用 `OUTPUT_ROOT` 覆盖。

> 兼容入口：`link-go/scripts/test-agents-e2e.sh` 仍可用，它只是转发到本技能脚本。

## 覆盖的 Agent

| Agent | 端点 | 主要工具 | 对话主线 |
|-------|------|----------|----------|
| RAG-Agent | `POST /api/v1/agent/knowledge/stream` | `kb_list` / `rag_query` | 列知识库 → 概览 → 逐层追问检索 → 综合总结 |
| Data-Agent | `POST /api/v1/agent/text2sql/stream` | `sql_execute` / `data_analysis` | 列表 → 采样 → 计数 → 字段 → 分组 → TopN → 概览 |

每个 Agent 跑 **10 轮连贯对话**，复用 `done` 事件回传的 `session_id`，每轮都刻意设计为会触发工具调用——这正是 reasoning_content 必须回传的场景。

### RAG 图谱模式（`GRAPH_ENABLED=1`）

默认（`GRAPH_ENABLED=0`）RAG 的 10 轮是概览/追问/总结式问法，回归 `kb_list` / `rag_query`（hybrid 检索）。
图谱检索**不混在** hybrid 里——它是独立的 `graph_query` 工具，只有 LLM 主动选用才走到，且需请求级 `graph_enabled:true` + 已解析的 kb 作用域。所以普通问法几乎不会触发图谱。

开 `GRAPH_ENABLED=1` 即进入**图谱模式**，RAG 换成一套：

- 请求置 `graph_enabled:true`，并把 `KB_IDS`（**必填**，逗号分隔）作为 kb 作用域下发（否则作用域为空、`graph_query` 查不到关系，脚本会前置报错退出）；
- 10 轮**关系型问法**（“关联/依赖/图谱关系/中心枢纽”措辞，不写死实体名，适配任意已抽取图谱的库）；
- **专项断言**：整场至少触发一次 `graph_query`，否则该 Agent 判不达标。

> 前提：`KB_IDS` 指向的库已完成图谱抽取（Neo4j 中有实体/关系）。未抽取时可先
> `POST /api/v1/knowledge-bases/:id/graph/rebuild` 重建。`has_answer` 取决于查询词能否按 name 命中图谱节点。

## 通过判据（每个 Agent 汇总其全部轮）

1. 全程无 `reasoning_content ... must be passed back` / `生成失败` 的 400 错误（**同时扫 SSE 流与服务端日志**）。
2. 至少收到一个 `done` 事件且 `answer` 非空。
3. 会话中至少成功执行过一次工具调用（`tool_result` 且 `status != error`），证明确实走到了「带工具的下一轮生成」。
4. （仅 RAG 图谱模式 `GRAPH_ENABLED=1`）会话中至少触发一次 `graph_query`，证明确实走到图谱检索路径。

任一 Agent 不满足即整体退出码非 0。

## 指标

脚本对每个 Agent 采集并打印下列指标；`METRICS_FILE` 指定时另以 JSON 行追加落盘。

| 指标 | 含义 | 来源 |
|------|------|------|
| `turns` | 对话轮数 | 固定 10 |
| `per_turn_ms` | 每轮端到端耗时(ms) | `perl Time::HiRes` 包住整轮 SSE |
| `total_ms` / `avg_ms` / `max_ms` / `min_ms` | 耗时总/均/最慢/最快 | 由 per-turn 聚合 |
| `total_tools` | 累计成功工具调用次数 | SSE `tool_result` 且非 error |
| `graph_queries` | 累计 `graph_query` 触发次数 | SSE `tool_result.tool_name=="graph_query"`（图谱模式的核心判据） |
| `empty_answers` | 空回答轮数 | `done.answer` 为空计数 |
| 回答字符数 / 全文 | 回答质量人工核对 | 完整打印 + 落盘 `*.answer.txt` |
| reasoning_content 400 命中 | 缺陷复发信号 | SSE 流 + 服务端日志双扫 |
| `passed` | 该 Agent 是否通过 | 上面三条判据 |

`METRICS_FILE` 每行形如：

```json
{"agent":"RAG-Agent","endpoint":"/api/v1/agent/knowledge/stream","graph_enabled":true,"turns":10,"total_ms":42310,"avg_ms":4231,"max_ms":8120,"min_ms":1204,"total_tools":14,"graph_queries":6,"empty_answers":0,"per_turn_ms":[...],"passed":true}
```

## 执行环境

脚本负责把执行环境一并收敛，无需手工准备：

- **服务启动**：默认 `go build ./cmd/server` 到临时目录并以 `DEV_MODE=true`、`LOG_LEVEL=debug` 启动，自动 `source link-go/.env`；跑完经 EXIT/INT/TERM trap `kill` 服务（遵循 CLAUDE.md「任务完成必须终止服务进程」）。
- **就绪探测**：轮询 `$SERVER_URL/health`，进程早退即打印启动日志并失败退出。
- **依赖**：`curl`、`jq`（缺失即报错退出）；毫秒计时用系统自带 `perl`。
- **link-go 定位**：默认从脚本位置向上推导仓库根，`LINK_GO_DIR` 可覆盖。
- **产物落盘**：SSE 流、`server.log`、`*.answer.txt`、`build.log`、`metrics.jsonl` 落在项目内 `test-output/agent-eval/<时间戳>-<agents>-<pid>/`（已 gitignore），默认持久保留；仅编译出的 server 二进制在退出时清掉，`--keep-log` 连二进制也留。根目录可用 `OUTPUT_ROOT` 覆盖。

### 配置项

| 变量 / 参数 | 默认 | 说明 |
|-------------|------|------|
| `SERVER_URL` | `http://localhost:8080` | 服务地址 |
| `REQUEST_TIMEOUT` | `120` | 单轮 SSE 最长等待秒 |
| `KEEP_LOG` / `--keep-log` | `0` | 连 server 二进制也保留（日志/指标默认已持久保留） |
| `--no-boot` | 关 | 复用已运行服务，不自启、不扫服务端日志 |
| `LINK_GO_DIR` | 自动推导 | 显式指定 link-go 目录 |
| `OUTPUT_ROOT` | `<repo>/test-output/agent-eval` | 产物根目录 |
| `METRICS_FILE` | `<产物目录>/metrics.jsonl` | 汇总指标落盘路径 |
| `AGENTS` | `rag,data` | 逗号分隔选择跑哪些 Agent：rag / data |
| `GRAPH_ENABLED` | `0` | =1/true 进入 RAG 图谱模式（关系型问法 + 断言 graph_query 触发），需配合 `KB_IDS` |
| `KB_IDS` | 空 | 逗号分隔知识库 id；图谱模式**必填**；普通模式留空=按租户全库检索 |
| `DATASOURCE_ID` | 空 | Data-Agent 目标外部数据源 id（空=当前业务库，非空改用电商用例） |
| `LOG_LEVEL` | `debug` | 传给服务的日志级别 |

## 前置数据

- Data-Agent 用例围绕 `dataagent_sales` 表下钻——需业务库中存在该表且有数据。
- RAG-Agent 用例假设租户下已有至少一个知识库并完成入库；否则检索类轮次可能空回答。
  用例文本写得较通用，缺数据时会体现在 `empty_answers` 指标上而非直接判失败。

## 与其他技能配合

- **kill-port**：若上次运行残留占用 8080，先用 `link-dev/kill-port` 清理再跑。
- **testing**：本技能聚焦「Agent 端到端多轮对话回归」；单元/集成/浏览器 E2E 走 `link-dev/testing`。
- **mock-login**：如需带认证访问，用 `link-dev/mock-login` 取测试用户 token。

## 排查

| 现象 | 排查方向 |
|------|----------|
| 服务启动超时 | `--keep-log` 后看 `server.log`；确认 `.env` 与依赖服务（MySQL/Milvus/Neo4j/Redis）可达 |
| 某 Agent 全程空回答 | 检查前置数据（知识库 / `dataagent_sales`）；`--keep-log` 看 `*.sse` 原始流 |
| 检出 reasoning_content 400 | 回看 `retriever` / DeepSeek thinking 处理是否回退，参照两处缺陷文档 |
| 检索恒空 | 复查 `HybridRetrieve`：融合结果不得再套余弦阈值，阈值只在向量分量侧生效 |
```

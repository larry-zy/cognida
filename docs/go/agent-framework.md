# 多代理协作框架 (Multi-Agent Framework)

## 概述

本项目基于 Cloudwego Eino 框架实现了一个多代理协作系统，通过 ReAct (Reasoning + Acting) 模式，让 LLM 自主决策调用各种子代理和工具来完成复杂任务。

## 核心架构

```
┌─────────────────────────────────────────────────────────────────┐
│                      用户查询 (User Query)                        │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Coordinator Agent (主协调器)                   │
│  - 分析问题复杂度                                                │
│  - 自主决策调用哪些子代理                                         │
│  - 管理执行流程                                                  │
└─────────┬───────────────────────────────────────────────────────┘
          │
          ├──► Planner Agent (规划代理)
          │    └─── 分析问题、制定研究计划、分解任务
          │
          ├──► Retriever Agent (检索代理)
          │    ├─── rag_query (知识库检索)
          │    ├─── web_search (网络搜索)
          │    └─── smart_retrieval (智能检索)
          │
          ├──► Analyzer Agent (分析代理)
          │    └─── 深度分析检索结果、提取洞见
          │
          ├──► Synthesizer Agent (合成代理)
          │    └─── 整合分析结果、生成结构化报告
          │
          └──► Critic Agent (评审代理)
               └─── 评审质量、提出改进建议
```

## 核心组件

### 1. MultiAgentOrchestrator (多代理协调器)

协调器是整个系统的核心，负责管理所有子代理和工具。

**配置结构：**
```go
type MultiAgentConfig struct {
    CoordinatorModel model.ToolCallingChatModel  // 主协调器模型
    PlannerModel     model.ToolCallingChatModel  // 规划代理模型
    RetrieverModel   model.ToolCallingChatModel  // 检索代理模型
    AnalyzerModel    model.ToolCallingChatModel  // 分析代理模型
    SynthesizerModel model.ToolCallingChatModel  // 合成代理模型
    CriticModel      model.ToolCallingChatModel  // 评审代理模型
    SearchConfig     *config.SearchConfig         // 搜索配置
    EnableSmartRetrieval bool                    // 是否启用智能检索
    MaxIterations    int                          // 最大迭代次数

    // 强制反思配置
    ForceReflection bool    // 是否强制执行反思（默认 false）
    MinCriticScore  float64 // 最低可接受评分（默认 0.75）
}
```

### 强制反思机制 (Forced Reflection)

当启用 `ForceReflection` 时，系统会**代码层面强制**执行 Critic 评审，即使 LLM 自主决策时没有调用 Critic Agent。

**工作流程：**
```
用户查询 -> Agent 执行 -> 生成答案
                         │
                         ▼
                    ┌─────────────────┐
                    │  检查是否调用过 Critic │
                    └────────┬─────────┘
                             │
              ┌──────────────┴──────────────┐
              │                             │
         已调用 Critic                  未调用 Critic
              │                             │
              ▼                             ▼
       提取评分结果              强制调用 Critic Agent
              │                             │
              ▼                             ▼
       评分 >= MinCriticScore      提取评分结果
              │              ┌──────┴──────┐
              │              │             │
              ▼              ▼             ▼
         返回答案     评分 >= 阈值   评分 < 阈值
                           │             │
                           ▼             ▼
                      返回答案      进入修订迭代
                                        │
                                        ▼
                            根据 Critic 意见修订答案
                                        │
                                        ▼
                                   重新评审
```

**关键特性：**
- **强制调用**：如果 LLM 没有自主调用 Critic，系统会自动调用
- **评分提取**：从 Critic 输出中自动提取评分（支持多种格式）
- **自动修订**：如果评分低于 `MinCriticScore`，自动进入修订流程
- **最多 2 轮修订**：避免无限循环
- **日志追踪**：所有反思过程都有详细日志

### 2. 子代理说明

| 代理名称 | 工具名称 | 职责 | 迭代次数 |
|---------|---------|------|---------|
| Planner | planner_agent | 分析问题、制定研究计划、分解复杂任务 | 3 |
| Retriever | retriever_agent | 从知识库和网络获取信息 | 5 |
| Analyzer | analyzer_agent | 深度分析检索结果、提取洞见 | 3 |
| Synthesizer | synthesizer_agent | 整合分析结果、生成结构化报告 | 3 |
| Critic | critic_agent | 评审质量、提出改进建议 | 2 |

### 3. 可用工具

| 工具名称 | 用途 | 参数 |
|---------|------|------|
| rag_query | 知识库检索 | query, top_k, retrieval_mode |
| web_search | 网络搜索 | query, limit |
| smart_retrieval | 智能检索（自动匹配知识库） | query, top_k, enable_web_search |
| calculator | 计算器 | expression |
| get_current_time | 获取当前时间 | - |
| http_request | HTTP 请求 | url, method |

## 工作流程

### ReAct 循环

系统遵循 ReAct (Reasoning + Acting) 模式：

```
┌─────────────────────────────────────────────────────────────────┐
│                        ReAct 循环                              │
└─────────────────────────────────────────────────────────────────┘
        │
        ▼
   ┌─────────┐
   │ Thought │  思考当前需要什么信息，应该使用什么工具
   └────┬────┘
        │
        ▼
   ┌─────────┐
   │  Action │  调用工具（带上合适的参数）
   └────┬────┘
        │
        ▼
   ┌────────────┐
   │ Observation │  分析工具返回的结果
   └──────┬───────┘
          │
          │──► 未完成 ──► 返回 Thought
          │
          ▼
      已完成
          │
          ▼
   ┌─────────┐
   │  Answer │  基于所有观察结果给出最终答案
   └─────────┘
```

### 决策策略

#### 简单查询（直接使用智能检索）
```
用户问："什么是微服务架构？"
↓
直接调用 smart_retrieval，参数 {"query": "微服务架构"}
```

#### 中等复杂度（检索 + 分析）
```
用户问："比较两种技术的优缺点"
↓
1. 调用 retriever_agent 获取两种技术的信息
2. 调用 analyzer_agent 分析比较
3. 给出答案
```

#### 高复杂度（完整流程）
```
用户问："分析某个行业的未来趋势"
↓
1. 调用 planner_agent 制定研究计划
2. 调用 retriever_agent 获取多源信息
3. 调用 analyzer_agent 深度分析
4. 调用 synthesizer_agent 生成报告
5. 调用 critic_agent 评审质量
6. 根据评审决定是否修订
7. 给出最终答案
```

## AgentTool 机制

核心创新点：**将子 Agent 包装成工具**

```go
// 1. 创建子代理
plannerAgent := createPlannerAgent(ctx, config.PlannerModel)

// 2. 将子代理包装成工具
plannerTool := adk.NewAgentTool(ctx, plannerAgent)

// 3. 主 Agent 可以像调用普通工具一样调用子代理
// LLM 自主决定何时调用哪个子代理
```

这样设计的优势：
- LLM 自主决策调用顺序
- 子代理可以嵌套调用
- 符合 ReAct 模式
- 无需硬编码流程

## 流式输出机制

### SSE 事件类型

| 事件类型 | 说明 | 数据结构 |
|---------|------|---------|
| session | 会话建立 | {session_id} |
| step | 思考步骤 | {step, type, stage, content, tool_name, tool_params...} |
| done | 完成标记 | {step_count, answer, reason, type} |
| error | 错误信息 | {step, content, type} |

### 步骤类型 (step.type)

| 类型 | 说明 |
|-----|------|
| search | 检索步骤 (rag_query, web_search) |
| action | 工具调用步骤 |
| thought | 思考过程 |
| plan | 规划输出 |
| analysis | 分析结果 |
| synthesis | 合成报告 |
| review | 评审结果 |
| complete | 完成标记 |
| error | 错误信息 |

### 刷新机制

为确保实时性，每次发送事件后调用 `http.Flusher`：

```go
func sendSSEvent(w io.Writer, eventType string, data map[string]interface{}) error {
    // ... 构建 SSE 事件 ...
    _, err = w.Write([]byte(event.String()))
    if err != nil {
        return err
    }
    // 刷新缓冲区，确保实时发送
    if flusher, ok := w.(http.Flusher); ok {
        flusher.Flush()
    }
    return nil
}
```

## AgenticRAGAgent (简化版)

对于简单场景，提供了单 Agent 版本：

```go
type AgenticRAGAgent struct {
    *baseagent.BaseAgent
    agent adk.Agent
}
```

特点：
- 直接使用工具（rag_query, web_search）
- ReAct 模式
- 最大迭代次数：10
- 无需子代理协作

## 提示词工程

### Coordinator 提示词要点

1. **禁止元信息**：最终答案不展示"我调用xxx工具"、"根据检索结果"等
2. **直接给出答案**：内容充实、结构清晰
3. **必须实际调用工具**：通过 function call 真正执行

### 输出格式要求

```
### 核心答案
[直接给出充实的答案内容]

### 详细说明
[展开详细的分析、说明或步骤]

### 关键要点
[总结关键要点]

### 信息来源
[列出主要的信息来源]
```

## 接口定义

### EinoAgentProvider 接口

```go
type EinoAgentProvider interface {
    GetEinoAgent() adk.Agent
}
```

### Chat 接口

```go
Chat(ctx context.Context, query string, opts ...baseagent.Option) (*baseagent.Response, error)
```

### StreamChat 接口

```go
StreamChat(ctx context.Context, query string, opts ...baseagent.Option) (*schema.StreamReader[*baseagent.ChatChunk], error)
```

## 配置示例

### 创建多代理协调器

```go
config := &agent.MultiAgentConfig{
    CoordinatorModel: toolChatModel,
    PlannerModel:      toolChatModel,
    RetrieverModel:   toolChatModel,
    AnalyzerModel:    toolChatModel,
    SynthesizerModel: toolChatModel,
    CriticModel:      toolChatModel,
    SearchConfig:     searchConfig,
    EnableSmartRetrieval: true,
    MaxIterations:     20,

    // 强制反思配置（可选）
    ForceReflection:  true,   // 启用强制 Critic 评审
    MinCriticScore:   0.75,   // 最低可接受评分
}

orchestrator, err := agent.NewMultiAgentOrchestrator(context.Background(), config)
```

### 创建简化版 Agent

```go
config := &agent.AgenticRAGAgentConfig{
    Name:          "AgenticRAGAgent",
    Description:   "智能搜索助手",
    MaxIterations: 10,
    SearchConfig:  searchConfig,
}

agent, err := agent.NewAgenticRAGAgent(context.Background(), toolChatModel, config)
```

## 文件位置

- 源码：`internal/application/agent/agentic_rag_agent.go`
- 工具定义：`internal/agent/tool/`
- Handler：`internal/handler/agent.go`
- 前端 API：`web/src/api/agent/`
- 前端视图：`web/src/views/chat/ChatView.vue`

## 依赖框架

- **Cloudwego Eino**: Agent 开发框架
- **Gin**: HTTP 服务器
- **GORM**: 数据库 ORM

## 扩展指南

### 添加新的子代理

1. 在 `agentic_rag_agent.go` 中创建 `createXxxAgent` 函数
2. 在 `NewMultiAgentOrchestrator` 中初始化并包装为工具
3. 在 Coordinator 提示词中添加说明

### 添加新的工具

1. 在 `internal/agent/tool/` 中实现工具
2. 在 `InitDefaultTools` 中注册工具
3. 在提示词中说明工具用途

### 自定义提示词

修改 `buildXxxPrompt()` 函数返回的字符串即可。

---

**文档版本**: v1.0
**更新时间**: 2026-02-20

# Deep Research 使用指南

## 概述

重新设计的 Deep Research 是一个基于多 Agent 编排的深度研究系统，能够自动分解问题、并行搜索、综合信息并生成研究报告。

## 架构

```
┌─────────────────────────────────────────────────────────────────┐
│                     Deep Research System                        │
│                                                                 │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐      │
│  │   Planning   │───▶│   Research   │───▶│  Synthesis   │      │
│  │    Agent     │    │    Agents    │    │    Agent     │      │
│  │              │    │              │    │              │      │
│  │ 问题分解     │    │ 并行搜索     │    │ 报告生成     │      │
│  │ 搜索策略     │    │ Web/RAG/Graph│   │ 信息融合     │      │
│  └──────────────┘    └──────────────┘    └──────────────┘      │
│                            │                                     │
│                            ▼                                     │
│                    ┌──────────────┐                            │
│                    │  Fact Check  │                            │
│                    │    Agent     │                            │
│                    │              │                            │
│                    │ 事实核查     │                            │
│                    │ 交叉验证     │                            │
│                    └──────────────┘                            │
└─────────────────────────────────────────────────────────────────┘
```

## API 使用

### 1. 同步研究

```bash
curl -X POST http://localhost:8080/api/v1/agent/research \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{
    "query": "什么是 RAG？它有哪些应用场景？",
    "options": {
      "max_sub_questions": 5,
      "max_concurrency": 3,
      "enable_fact_check": true
    }
  }'
```

**响应示例：**
```json
{
  "query": "什么是 RAG？它有哪些应用场景？",
  "executive_summary": "RAG (Retrieval-Augmented Generation) 是一种结合检索和生成的 AI 技术...",
  "key_findings": [
    "RAG 通过外部知识库增强 LLM 能力",
    "主要应用场景包括问答系统、文档分析等",
    "能有效减少 LLM 幻觉问题"
  ],
  "detailed_analysis": "完整的分析报告...",
  "sources": [
    {
      "type": "web",
      "title": "Retrieval-Augmented Generation",
      "url": "https://example.com/rag"
    }
  ],
  "verified_facts": [],
  "metadata": {
    "sub_question_count": 5,
    "total_sources": 12,
    "execution_time_ms": 45000,
    "fact_checked": true,
    "verified_fact_count": 3
  },
  "success": true
}
```

### 2. 流式研究（带进度）

```bash
curl -X POST http://localhost:8080/api/v1/agent/research/stream \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{
    "query": "量子计算的发展现状如何？"
  }'
```

**SSE 事件流：**
```
data: {"stage":"planning","progress":0.05,"detail":"分析查询并制定研究计划"}

data: {"stage":"researching","progress":0.15,"detail":"开始并行研究","total_tasks":5}

data: {"stage":"researching","progress":0.25,"detail":"完成子问题: 什么是量子计算？","completed_tasks":1,"total_tasks":5}

data: {"stage":"synthesizing","progress":0.85,"detail":"综合研究结果"}

data: {"stage":"completed","progress":1.0,"detail":"研究完成"}
```

## 代码集成

### Go 代码示例

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/components/tool"

    "link/internal/application/usecases/agent"
    "link/internal/application/usecases/agent/research"
)

func main() {
    // 1. 创建模型
    model := createModel()

    // 2. 准备工具
    tools := []tool.BaseTool{
        webSearchTool,
        ragQueryTool,
        graphQueryTool,
    }

    // 3. 配置研究参数
    config := &research.ResearchConfig{
        MaxSubQuestions:     5,
        MaxConcurrency:      3,
        Timeout:             10 * time.Minute,
        ResearchDepth:       2,
        EnableFactCheck:     true,
        FactCheckSample:     5,
        ReportFormat:        "markdown",
        IncludeSources:      true,
    }

    // 4. 创建编排器
    orchestrator := research.NewOrchestrator(
        model,
        tools,
        config,
        func(p *research.ResearchProgress) {
            log.Printf("[Progress] %s: %.1f%% - %s",
                p.Stage, p.Progress*100, p.Detail)
        },
    )

    // 5. 执行研究
    ctx := context.Background()
    report, err := orchestrator.Execute(ctx, "什么是知识图谱？")
    if err != nil {
        log.Fatal(err)
    }

    // 6. 使用报告
    fmt.Printf("执行摘要: %s\n", report.ExecutiveSummary)
    fmt.Printf("关键发现: %v\n", report.KeyFindings)
    fmt.Printf("来源数: %d\n", len(report.Sources))
}
```

### 创建 UseCase

```go
// 在 wire.go 或初始化代码中
func initializeResearchUseCase(
    model model.ChatModel,
    tools []tool.BaseTool,
    progressMgr ProgressManager,
) agent.ResearchUseCase {
    return agent.NewResearchUseCaseV2(model, tools, nil, progressMgr)
}
```

## 配置选项

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `max_sub_questions` | int | 5 | 最大子问题数量 |
| `max_concurrency` | int | 3 | 并发搜索数 |
| `timeout` | duration | 10m | 总超时时间 |
| `research_depth` | int | 2 | 研究深度 (1-3) |
| `max_sources_per_query` | int | 10 | 每个查询最大来源数 |
| `enable_fact_check` | bool | true | 启用事实核查 |
| `fact_check_sample` | int | 5 | 事实核查抽样数 |
| `enable_cross_validation` | bool | true | 启用交叉验证 |
| `report_format` | string | markdown | 报告格式 |
| `include_sources` | bool | true | 包含来源列表 |

## 进度阶段

| 阶段 | 说明 |
|------|------|
| `initializing` | 初始化研究 |
| `planning` | 分解问题、制定计划 |
| `researching` | 并行执行搜索 |
| `synthesizing` | 综合信息、生成报告 |
| `fact_checking` | 事实核查 |
| `completed` | 研究完成 |
| `failed` | 研究失败 |

## 工具集成

Deep Research 可以使用以下工具：

- **web_search**: 网络搜索
- **rag_query**: RAG 检索
- **graph_query**: 知识图谱查询
- **data_query**: 数据查询
- **sql_query**: SQL 查询

工具会根据子问题类型自动选择。

## 测试

```bash
# 运行集成测试
cd link-go
export OPENAI_API_KEY=your-key
go test -v -tags=integration ./internal/application/usecases/agent/research/

# 运行特定测试
go test -v -tags=integration ./internal/application/usecases/agent/research/ -run TestOrchestrator_BasicFlow
```

## 迁移指南

### 从旧版本迁移

**旧代码：**
```go
useCase := agent.NewResearchUseCase(executor, config)
```

**新代码：**
```go
useCase := agent.NewResearchUseCaseV2(model, tools, config, progressMgr)
```

API 接口保持不变，只需要更新初始化方式。

## 最佳实践

1. **合理设置并发数**：根据 API 限制和性能要求调整 `max_concurrency`
2. **控制研究深度**：简单查询使用深度 1，复杂分析使用深度 2-3
3. **启用进度追踪**：流式 API 提供更好的用户体验
4. **配置超时**：根据查询复杂度设置合理的超时时间
5. **监控进度**：利用进度回调实现自定义进度展示

## 故障排查

### 问题：研究失败
- 检查 API Key 是否正确
- 确认网络连接正常
- 查看日志中的详细错误信息

### 问题：进度不更新
- 确认使用的是流式 API (`/research/stream`)
- 检查客户端是否正确处理 SSE 事件

### 问题：结果质量不高
- 增加 `research_depth`
- 启用 `enable_fact_check`
- 调整 `max_sources_per_query`

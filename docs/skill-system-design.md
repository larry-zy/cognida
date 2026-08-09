# Cognida Skill 系统设计方案

## 文档说明

本文档描述 Skill 系统融入 Cognida Agent 架构的设计方案。

**设计理念**: Skill 作为 Agent 能力的动态组合单元

**版本**: v1.2
**更新时间**: 2026-05-09

---

## 目录

- [一、设计目标](#一设计目标)
- [二、核心概念](#二核心概念)
- [三、系统架构](#三系统架构)
- [四、领域模型](#四领域模型)
- [五、实现设计](#五实现设计)
- [六、使用示例](#六使用示例)
- [七、元工具设计](#七元工具设计)
- [八、实现路径](#八实现路径)

---

## 一、设计目标

### 1.1 核心目标

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Skill 系统目标                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   1. 能力动态化: Agent 能力可通过 Skill 动态扩展，无需重新编译               │
│   2. 架构融合: 与现有 Tool/MCP/AgentRegistry 无缝集成                       │
│   3. 标准统一: 统一 Skill 定义、发现和调用接口                               │
│   4. 多源支持: 支持 MCP Server、本地注册、远程发现等多种 Skill 来源         │
│   5. 编排增强: 支持依赖声明、条件执行、结果组合等高级编排能力                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 概念对齐

| 概念 | 当前实现 | Skill 扩展 |
|------|---------|-----------|
| **Tool** | 可被 LLM 调用的能力 | Skill 的 LLM 调用形式 |
| **Agent Capability** | Agent 声明的能力 | Skill 聚合后的能力描述 |
| **MCP Tool** | Python 暴露的工具 | Skill 的一种来源 |
| **Hook** | 生命周期钩子 | 可封装为 Skill |

---

## 二、核心概念

### 2.1 Skill 定义

```go
// Skill 是 Agent 能力的抽象单元
type Skill struct {
    // 基础信息
    ID          string                 // skill://category/name
    Name        string                 // 显示名称
    Description string                 // 能力描述
    Category    SkillCategory          // 类别
    Version     string                 // 版本

    // 能力声明
    Capabilities []Capability          // 具体能力列表
    Dependencies []string              // 依赖的其他 Skill

    // 实现方式
    Provider    SkillProvider          // 提供者 (MCP/Local/Remote)
    Implementation SkillImplementation  // 实现细节

    // 元数据
    Metadata    map[string]interface{} // 扩展元数据
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// SkillCategory Skill 类别
type SkillCategory string

const (
    SkillCategoryData      SkillCategory = "data"       // 数据处理
    SkillCategorySearch    SkillCategory = "search"     // 检索
    SkillCategoryAnalysis  SkillCategory = "analysis"   // 分析
    SkillCategoryGeneration SkillCategory = "generation" // 生成
    SkillCategoryUtility   SkillCategory = "utility"    // 工具
    SkillCategoryWorkflow  SkillCategory = "workflow"   // 工作流
)

// Capability 能力声明
type Capability struct {
    Name        string                 // 能力名称
    Description string                 // 描述
    InputSchema map[string]interface{} // 输入 Schema
    OutputSchema map[string]interface{}// 输出 Schema
}

// SkillProvider Skill 提供者类型
type SkillProvider string

const (
    ProviderMCP     SkillProvider = "mcp"      // MCP Server 提供
    ProviderLocal   SkillProvider = "local"    // 本地实现
    ProviderRemote  SkillProvider = "remote"   // 远程 HTTP API
    ProviderComposite SkillProvider = "composite" // 组合 Skill
)

// SkillImplementation Skill 实现细节
type SkillImplementation struct {
    Type        SkillProvider
    // MCP 实现
    MCPServer   string                 // MCP Server 名称
    MCPTool     string                 // MCP Tool 名称

    // 本地实现
    Handler     SkillHandler           // 处理函数

    // 远程实现
    Endpoint    string                 // HTTP 端点
    Method      string                 // HTTP 方法

    // 组合实现
    SubSkills   []string               // 子 Skill ID
    Orchestrator string                // 编排策略
}

// SkillHandler Skill 处理函数
type SkillHandler func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)
```

### 2.2 Skill 与 Tool 的关系

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Skill → Tool 转换                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   Skill (能力抽象)                                                           │
│   │                                                                         │
│   │  ID: skill://data/analysis                                              │
│   │  Capabilities: [                                                        │
│   │    {                                                                     │
│   │      Name: "data_analyze",                                              │
│   │      InputSchema: {table_name, operations},                             │
│   │      OutputSchema: {statistics}                                          │
│   │    }                                                                     │
│   │  ]                                                                       │
│   │  Provider: MCP                                                          │
│   │                                                                         │
│   ▼                                                                         │
│   Tool (LLM 可调用)                                                          │
│   │                                                                         │
│   │  Name: "data_analyze"                                                   │
│   │  Description: "分析数据表..."                                            │
│   │  Parameters: {                                                          │
│   │    table_name: {type: "string", required: true},                        │
│   │    operations: {type: "array", required: true}                          │
│   │  }                                                                       │
│   │  Handler: → MCP Client                                                  │
│   │                                                                         │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 Skill 与 Agent 的关系

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Agent 能力组合                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   Agent: "数据分析师"                                                        │
│   ├── Skill: skill://data/analysis                                         │
│   │   └── Capability: data_analyze                                         │
│   ├── Skill: skill://data/visualization                                    │
│   │   └── Capability: plot_chart                                           │
│   └── Skill: skill://llm/judgment                                          │
│       └── Capability: llm_evaluate                                         │
│                                                                             │
│   ↓ 合并为 Agent Capabilities                                               │
│                                                                             │
│   Agent Capabilities: [                                                     │
│     {Name: "data_analysis", Skills: ["data_analyze", "stat_summary"]},     │
│     {Name: "visualization", Skills: ["plot_chart", "export_png"]},         │
│     {Name: "evaluation", Skills: ["llm_evaluate"]}                         │
│   ]                                                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、系统架构

### 3.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Skill System Architecture                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                        Agent Builder                                 │   │
│   │  ┌─────────────────────────────────────────────────────────────────┐ │   │
│   │  │  NewAgent().                                                    │ │ │   │
│   │  │    Name("分析师").                                               │ │ │   │
│   │  │    WithSkills(                                                  │ │ │   │
│   │  │      "skill://data/analysis",                                   │ │ │   │
│   │  │      "skill://data/visualization",                              │ │ │   │
│   │  │      "skill://llm/judgment"                                     │ │ │   │
│   │  │    ).                                                           │ │ │   │
│   │  │    Build()                                                      │ │ │   │
│   │  └─────────────────────────────────────────────────────────────────┘ │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│   ┌────────────────────────────────▼─────────────────────────────────────┐   │
│   │                        Skill Registry                                │   │
│   │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐             │   │
│   │  │ Register │  │  Find    │  │  List    │  │ Resolve  │             │   │
│   │  │   Skill  │  │  Skills  │  │  Skills  │  │ Chain    │             │   │
│   │  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘             │   │
│   └───────┼────────────┼────────────┼────────────┼────────────────────────┘   │
│           │            │            │            │                           │
│   ┌───────▼────────────▼────────────▼────────────▼───────────────────────────┐ │
│   │                       Skill Providers                                  │ │
│   │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐         │ │
│   │  │   MCP Provider  │  │  Local Provider │  │ Composite Prov. │         │ │
│   │  │  ┌───────────┐  │  │  ┌───────────┐  │  │  ┌───────────┐  │         │ │
│   │  │  │ MCP Tool  │  │  │  │ Go Func   │  │  │  │ Sub-Skill │  │         │ │
│   │  │  │ Discovery │  │  │  │ Handler   │  │  │  │ Chaining  │  │         │ │
│   │  │  └───────────┘  │  │  └───────────┘  │  │  └───────────┘  │         │ │
│   │  └─────────────────┘  └─────────────────┘  └─────────────────┘         │ │
│   └─────────────────────────────────────────────────────────────────────────┘ │
│                                    │                                        │
│   ┌────────────────────────────────▼─────────────────────────────────────┐   │
│   │                     Tool Registry (集成点)                            │   │
│   │                                                                         │   │
│   │   Skill.ToTool() → 注册到 Tool Registry → LLM Function Calling        │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Skill 发现流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Skill Discovery Flow                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   1. MCP Server Discovery                                                  │
│      ├── MCP Client 连接到 Server                                          │
│      ├── 调用 tools/list 获取可用工具                                       │
│      ├── 根据命名规则转换为 Skill                                          │
│      │   例: data_analyze → skill://data/analysis                         │
│      └── 注册到 Skill Registry                                             │
│                                                                             │
│   2. Local Registration                                                    │
│      ├── 代码定义 Skill                                                    │
│      ├── 调用 SkillRegistry.Register()                                     │
│      └── 直接可用                                                          │
│                                                                             │
│   3. Composite Skill                                                       │
│      ├── 定义子 Skill 组合                                                 │
│      ├── 声明编排策略 (sequence/parallel/conditional)                      │
│      └── 作为新 Skill 注册                                                 │
│                                                                             │
│   4. Remote Discovery (可选)                                               │
│      ├── HTTP GET /skills                                                  │
│      ├── 解析 Skill Manifest                                               │
│      └── 注册为 Remote Skill                                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 四、领域模型

### 4.1 领域接口

```go
// Package skill 提供 Skill 领域定义
package skill

import "context"

// SkillRegistry Skill 注册中心接口
type SkillRegistry interface {
    // Register 注册一个 Skill
    Register(ctx context.Context, skill *Skill) error

    // Unregister 注销一个 Skill
    Unregister(ctx context.Context, skillID string) error

    // Get 获取指定的 Skill
    Get(ctx context.Context, skillID string) (*Skill, error)

    // FindByCategory 按类别查找 Skill
    FindByCategory(ctx context.Context, category SkillCategory) ([]*Skill, error)

    // FindByCapability 按能力查找 Skill
    FindByCapability(ctx context.Context, capabilityName string) ([]*Skill, error)

    // ResolveDependencies 解析 Skill 依赖链
    ResolveDependencies(ctx context.Context, skillID string) ([]*Skill, error)

    // List 列出所有已注册的 Skill
    List(ctx context.Context) ([]*Skill, error)
}

// SkillExecutor Skill 执行器接口
type SkillExecutor interface {
    // Execute 执行 Skill
    Execute(ctx context.Context, skillID string, input map[string]interface{}) (map[string]interface{}, error)

    // ExecuteWithCapability 执行 Skill 的特定能力
    ExecuteWithCapability(ctx context.Context, skillID, capabilityName string, input map[string]interface{}) (map[string]interface{}, error)
}

// SkillResolver Skill 解析器接口
// 负责将 Skill 转换为 Tool
type SkillResolver interface {
    // ResolveToTool 将 Skill 解析为 Tool 定义
    ResolveToTool(ctx context.Context, skill *Skill, capabilityName string) (*ToolDefinition, error)
}

// SkillDiscoverer Skill 发现器接口
// 负责从不同来源发现 Skill
type SkillDiscoverer interface {
    // Discover 从来源发现 Skill
    Discover(ctx context.Context) ([]*Skill, error)

    // Watch 监听 Skill 变化
    Watch(ctx context.Context) (<-chan *SkillEvent, error)
}

// ToolDefinition Tool 定义 (与现有 Tool 系统对齐)
type ToolDefinition struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Parameters  map[string]interface{} `json:"parameters"`
    Handler     ToolHandler            `json:"-"`
}

// ToolHandler Tool 处理函数
type ToolHandler func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// SkillEvent Skill 变化事件
type SkillEvent struct {
    Type      EventType   // Added, Removed, Updated
    Skill     *Skill
    Timestamp time.Time
}

type EventType string

const (
    EventSkillAdded   EventType = "added"
    EventSkillRemoved EventType = "removed"
    EventSkillUpdated EventType = "updated"
)
```

### 4.2 与现有系统集成

```go
// 与 AgentRegistry 集成
type AgentRegistry interface {
    // 现有方法...

    // RegisterWithSkills 使用 Skill 集合注册 Agent
    RegisterWithSkills(
        ctx context.Context,
        agentID string,
        skillIDs []string,
    ) error

    // GetAgentCapabilities 获取 Agent 的能力描述
    // 由注册的 Skill 聚合生成
    GetAgentCapabilities(ctx context.Context, agentID string) ([]AgentCapability, error)
}

// 与 ToolRegistry 集成
type ToolRegistry interface {
    // 现有方法...

    // RegisterFromSkills 从 Skill 批量注册 Tool
    RegisterFromSkills(ctx context.Context, skills []*Skill) error
}
```

---

## 五、实现设计

### 5.1 目录结构

```
cognida-go/internal/
├── domain/
│   └── agent/
│       ├── skill.go                    # Skill 领域实体
│       ├── skill_registry.go           # Skill 注册中心接口
│       ├── skill_provider.go           # Skill 提供者接口
│       └── skill_resolver.go           # Skill 解析器接口
│
├── application/
│   └── agent/
│       └── skill/                      # Skill 应用层
│           ├── registry.go             # Skill 注册中心实现
│           ├── executor.go             # Skill 执行器实现
│           ├── resolver.go             # Skill 解析器实现
│           ├── loader.go               # 本地文件加载器
│           ├── validator.go            # Skill 定义验证器
│           ├── providers/              # Skill 提供者实现
│           │   ├── mcp.go              # MCP 提供者
│           │   ├── local.go            # 本地提供者
│           │   ├── composite.go        # 组合提供者
│           │   └── remote.go           # 远程提供者
│           └── orchestrator/           # Skill 编排器
│               ├── sequence.go         # 顺序执行
│               ├── parallel.go         # 并行执行
│               └── conditional.go      # 条件执行
│
└── infrastructure/
    └── agent/
        └── skill/                      # Skill 基础设施
            ├── storage.go              # Skill 持久化
            ├── cache.go                # Skill 缓存
            ├── watcher.go              # 文件监听器
            └── mcp_client.go           # MCP 客户端 (复用)
```

### 5.2 MCP Skill Provider

```go
// Package providers 提供 Skill 提供者实现
package providers

import (
    "context"
    "fmt"
)

// MCPSkillProvider 从 MCP Server 发现 Skill
type MCPSkillProvider struct {
    client MCPClient
    prefix string // 命名前缀，如 "python-data"
}

func NewMCPSkillProvider(client MCPClient, prefix string) *MCPSkillProvider {
    return &MCPSkillProvider{
        client: client,
        prefix: prefix,
    }
}

func (p *MCPSkillProvider) Discover(ctx context.Context) ([]*skill.Skill, error) {
    // 1. 获取 MCP Tools
    tools, err := p.client.ListTools(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to list MCP tools: %w", err)
    }

    // 2. 转换为 Skill
    var skills []*skill.Skill
    for _, tool := range tools {
        s := p.mcpToolToSkill(tool)
        skills = append(skills, s)
    }

    return skills, nil
}

func (p *MCPSkillProvider) mcpToolToSkill(tool *MCPTool) *skill.Skill {
    // 命名规则: data_analyze → skill://data/analysis
    category, name := p.parseToolName(tool.Name)

    return &skill.Skill{
        ID:          fmt.Sprintf("skill://%s/%s", category, name),
        Name:        name,
        Description: tool.Description,
        Category:    skill.SkillCategory(category),
        Version:     "1.0.0",
        Capabilities: []skill.Capability{
            {
                Name:        tool.Name,
                Description: tool.Description,
                InputSchema: tool.InputSchema,
            },
        },
        Provider: skill.ProviderMCP,
        Implementation: skill.SkillImplementation{
            Type:      skill.ProviderMCP,
            MCPServer: p.prefix,
            MCPTool:   tool.Name,
        },
        CreatedAt: time.Now(),
    }
}

func (p *MCPSkillProvider) parseToolName(toolName string) (category, name string) {
    // 简单实现: data_analyze → data, analysis
    // 可以使用更复杂的命名规则
    parts := strings.Split(toolName, "_")
    if len(parts) >= 2 {
        return parts[0], strings.Join(parts[1:], "_")
    }
    return "utility", toolName
}

func (p *MCPSkillProvider) Watch(ctx context.Context) (<-chan skill.SkillEvent, error) {
    // 监听 MCP Server 变化
    // 定期轮询或使用 SSE
    events := make(chan skill.SkillEvent)

    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        var knownTools map[string]string

        for {
            select {
            case <-ticker.C:
                tools, err := p.client.ListTools(ctx)
                if err != nil {
                    continue
                }

                current := make(map[string]string)
                for _, t := range tools {
                    current[t.Name] = t.Version

                    if knownTools == nil {
                        events <- skill.SkillEvent{
                            Type:  skill.EventSkillAdded,
                            Skill: p.mcpToolToSkill(t),
                        }
                    } else if knownTools[t.Name] != t.Version {
                        events <- skill.SkillEvent{
                            Type:  skill.EventSkillUpdated,
                            Skill: p.mcpToolToSkill(t),
                        }
                    }
                }

                // 检查删除
                for name := range knownTools {
                    if _, ok := current[name]; !ok {
                        events <- skill.SkillEvent{
                            Type: skill.EventSkillRemoved,
                            Skill: &skill.Skill{
                                ID: fmt.Sprintf("skill://%s", name),
                            },
                        }
                    }
                }

                knownTools = current

            case <-ctx.Done():
                close(events)
                return
            }
        }
    }()

    return events, nil
}
```

### 5.3 Local Skill Loader

本地 Skill 加载器从文件系统加载 Skill 定义文件，支持热重载和目录扫描。

#### 支持的文件格式

| 格式 | 扩展名 | 适用场景 |
|------|--------|----------|
| YAML | `.yaml`, `.yml` | 推荐，可读性好 |
| JSON | `.json` | 程序生成 |
| TOML | `.toml` | 配置文件风格 |

#### Skill 定义文件格式

**YAML 示例** (`skills/data/analysis.yaml`):

```yaml
# Skill 定义文件
skill:
  id: skill://data/analysis
  name: analysis
  description: 数据分析能力，提供统计描述、相关性分析、分布分析等功能
  category: data
  version: 1.0.0
  provider: local

  # 能力列表
  capabilities:
    - name: data_analyze
      description: 执行数据分析操作
      input_schema:
        type: object
        properties:
          table_name:
            type: string
            description: 数据表名
          operations:
            type: array
            items:
              type: string
              enum: [describe, correlation, distribution]
            description: 分析操作列表
        required: [table_name, operations]
      output_schema:
        type: object
        properties:
          statistics:
            type: object
            description: 统计结果
          correlations:
            type: object
            description: 相关性矩阵

    - name: data_aggregate
      description: 数据聚合操作
      input_schema:
        type: object
        properties:
          table_name:
            type: string
          group_by:
            type: string
          aggregations:
            type: array
            items:
              type: string

  # 依赖的其他 Skill
  dependencies:
    - skill://llm/judgment

  # 实现方式
  implementation:
    type: local
    handler: internal/data/analyze  # Go 代码中的处理函数路径

  # 元数据
  metadata:
    author: data-team
    tags: [data, analysis, statistics]
    timeout_ms: 30000
    cache_ttl: 300
```

**JSON 示例**:

```json
{
  "skill": {
    "id": "skill://web/search",
    "name": "search",
    "description": "网络搜索能力",
    "category": "search",
    "version": "1.0.0",
    "provider": "local",
    "capabilities": [
      {
        "name": "web_search",
        "description": "执行网络搜索",
        "input_schema": {
          "type": "object",
          "properties": {
            "query": {"type": "string"},
            "limit": {"type": "integer", "default": 10}
          },
          "required": ["query"]
        }
      }
    ],
    "implementation": {
      "type": "local",
      "handler": "internal/web/search"
    }
  }
}
```

#### 目录结构

```
skills/                              # Skill 根目录（可配置）
├── data/                            # 数据类 Skill
│   ├── analysis.yaml
│   ├── visualization.yaml
│   └── etl.yaml
├── search/                          # 检索类 Skill
│   ├── web.yaml
│   ├── kb.yaml
│   └── graph.yaml
├── llm/                             # LLM 类 Skill
│   ├── judgment.yaml
│   └── generation.yaml
└── workflow/                        # 工作流类 Skill
    ├── rag.yaml
    └── deep_research.yaml
```

#### 实现设计

```go
// LocalSkillLoader 本地 Skill 加载器
type LocalSkillLoader struct {
    registry     SkillRegistry
    baseDir      string              // Skill 文件根目录
    watcher      fsWatcher           // 文件监听器
    handlerMap   map[string]SkillHandler  // handler 路径 -> 处理函数
    validator    *SkillValidator     // Skill 验证器
}

func NewLocalSkillLoader(
    registry SkillRegistry,
    baseDir string,
) *LocalSkillLoader {
    return &LocalSkillLoader{
        registry:   registry,
        baseDir:    baseDir,
        handlerMap: make(map[string]SkillHandler),
        validator:  NewSkillValidator(),
    }
}

// LoadFromDir 从目录加载所有 Skill
func (l *LocalSkillLoader) LoadFromDir(ctx context.Context) error {
    return filepath.Walk(l.baseDir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        // 跳过目录
        if info.IsDir() {
            return nil
        }

        // 检查文件扩展名
        if !l.isSkillFile(path) {
            return nil
        }

        // 加载 Skill
        skill, err := l.LoadFromFile(ctx, path)
        if err != nil {
            log.Printf("failed to load skill from %s: %v", path, err)
            return nil // 继续处理其他文件
        }

        // 注册到 Registry
        if err := l.registry.Register(ctx, skill); err != nil {
            return fmt.Errorf("failed to register skill %s: %w", skill.ID, err)
        }

        log.Printf("loaded skill: %s from %s", skill.ID, path)
        return nil
    })
}

// LoadFromFile 从文件加载 Skill
func (l *LocalSkillLoader) LoadFromFile(ctx context.Context, path string) (*Skill, error) {
    // 1. 读取文件内容
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read file: %w", err)
    }

    // 2. 根据扩展名解析
    var skillDef SkillDefinition
    switch strings.ToLower(filepath.Ext(path)) {
    case ".yaml", ".yml":
        if err := yaml.Unmarshal(data, &skillDef); err != nil {
            return nil, fmt.Errorf("failed to parse YAML: %w", err)
        }
    case ".json":
        if err := json.Unmarshal(data, &skillDef); err != nil {
            return nil, fmt.Errorf("failed to parse JSON: %w", err)
        }
    case ".toml":
        if err := toml.Unmarshal(data, &skillDef); err != nil {
            return nil, fmt.Errorf("failed to parse TOML: %w", err)
        }
    default:
        return nil, fmt.Errorf("unsupported file format: %s", filepath.Ext(path))
    }

    // 3. 验证 Skill 定义
    if err := l.validator.Validate(skillDef); err != nil {
        return nil, fmt.Errorf("invalid skill definition: %w", err)
    }

    // 4. 构建 Skill 实体
    skill := &Skill{
        ID:          skillDef.Skill.ID,
        Name:        skillDef.Skill.Name,
        Description: skillDef.Skill.Description,
        Category:    SkillCategory(skillDef.Skill.Category),
        Version:     skillDef.Skill.Version,
        Provider:    ProviderLocal,
    }

    // 5. 添加 Capabilities
    for _, capDef := range skillDef.Skill.Capabilities {
        skill.Capabilities = append(skill.Capabilities, Capability{
            Name:         capDef.Name,
            Description:  capDef.Description,
            InputSchema:  capDef.InputSchema,
            OutputSchema: capDef.OutputSchema,
        })
    }

    // 6. 设置依赖
    skill.Dependencies = skillDef.Skill.Dependencies

    // 7. 设置实现
    handlerPath := skillDef.Skill.Implementation.Handler
    if handler, ok := l.handlerMap[handlerPath]; ok {
        skill.Implementation = SkillImplementation{
            Type:    ProviderLocal,
            Handler: handler,
        }
    } else {
        // 如果没有注册 handler，记录警告
        log.Printf("warning: no handler found for %s", handlerPath)
    }

    // 8. 设置元数据
    skill.Metadata = skillDef.Skill.Metadata
    skill.CreatedAt = time.Now()

    return skill, nil
}

// RegisterHandler 注册本地处理函数
func (l *LocalSkillLoader) RegisterHandler(path string, handler SkillHandler) {
    l.handlerMap[path] = handler
}

// Watch 监听文件变化，实现热重载
func (l *LocalSkillLoader) Watch(ctx context.Context) (<-chan SkillEvent, error) {
    events := make(chan SkillEvent)

    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return nil, err
    }

    // 递归添加监听
    filepath.Walk(l.baseDir, func(path string, info os.FileInfo, err error) error {
        if info.IsDir() {
            return watcher.Add(path)
        }
        return nil
    })

    go func() {
        defer watcher.Close()
        defer close(events)

        for {
            select {
            case event, ok := <-watcher.Events:
                if !ok {
                    return
                }

                // 只处理 Skill 文件
                if !l.isSkillFile(event.Name) {
                    continue
                }

                switch {
                case event.Op&fsnotify.Create == fsnotify.Create:
                    // 新文件，加载 Skill
                    skill, err := l.LoadFromFile(ctx, event.Name)
                    if err == nil {
                        l.registry.Register(ctx, skill)
                        events <- NewSkillEvent(SkillEventAdded, skill)
                    }

                case event.Op&fsnotify.Write == fsnotify.Write:
                    // 文件修改，重新加载
                    skill, err := l.LoadFromFile(ctx, event.Name)
                    if err == nil {
                        l.registry.Register(ctx, skill)
                        events <- NewSkillEvent(SkillEventUpdated, skill)
                    }

                case event.Op&fsnotify.Remove == fsnotify.Remove:
                    // 文件删除，注销 Skill
                    skillID := l.pathToSkillID(event.Name)
                    l.registry.Unregister(ctx, skillID)
                    events <- SkillEvent{
                        Type: SkillEventRemoved,
                        Skill: &Skill{ID: skillID},
                    }
                }

            case err, ok := <-watcher.Errors:
                if !ok {
                    return
                }
                log.Printf("watcher error: %v", err)

            case <-ctx.Done():
                return
            }
        }
    }()

    return events, nil
}

// isSkillFile 检查是否为 Skill 文件
func (l *LocalSkillLoader) isSkillFile(path string) bool {
    ext := strings.ToLower(filepath.Ext(path))
    return ext == ".yaml" || ext == ".yml" || ext == ".json" || ext == ".toml"
}

// pathToSkillID 从文件路径推断 Skill ID
func (l *LocalSkillLoader) pathToSkillID(path string) string {
    relPath, _ := filepath.Rel(l.baseDir, path)
    // skills/data/analysis.yaml -> skill://data/analysis
    parts := strings.Split(relPath, string(filepath.Separator))
    if len(parts) >= 2 {
        category := parts[len(parts)-2]
        name := strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))
        return fmt.Sprintf("skill://%s/%s", category, name)
    }
    return ""
}
```

#### Skill 验证器

```go
// SkillValidator 验证 Skill 定义
type SkillValidator struct {
    schemaLoader *schema.Loader
}

func NewSkillValidator() *SkillValidator {
    return &SkillValidator{
        schemaLoader: schema.NewLoader(),
    }
}

func (v *SkillValidator) Validate(def SkillDefinition) error {
    // 1. 基础字段验证
    if def.Skill.ID == "" {
        return fmt.Errorf("skill ID is required")
    }
    if err := ValidateSkillID(def.Skill.ID); err != nil {
        return err
    }

    if def.Skill.Name == "" {
        return fmt.Errorf("skill name is required")
    }

    if !SkillCategory(def.Skill.Category).IsValid() {
        return fmt.Errorf("invalid skill category: %s", def.Skill.Category)
    }

    // 2. Capabilities 验证
    if len(def.Skill.Capabilities) == 0 {
        return fmt.Errorf("skill must have at least one capability")
    }

    for i, cap := range def.Skill.Capabilities {
        if cap.Name == "" {
            return fmt.Errorf("capability[%d]: name is required", i)
        }
        if cap.InputSchema == nil {
            return fmt.Errorf("capability[%d]: input_schema is required", i)
        }
    }

    // 3. Implementation 验证
    if def.Skill.Implementation.Type == "" {
        return fmt.Errorf("implementation type is required")
    }

    switch def.Skill.Implementation.Type {
    case "local":
        if def.Skill.Implementation.Handler == "" {
            return fmt.Errorf("local skill must have handler")
        }
    case "mcp":
        if def.Skill.Implementation.MCPServer == "" {
            return fmt.Errorf("MCP skill must have server specified")
        }
    }

    // 4. JSON Schema 验证
    if err := v.validateSchemas(def); err != nil {
        return err
    }

    return nil
}
```

#### 使用示例

```go
// 初始化本地加载器
loader := NewLocalSkillLoader(registry, "./skills")

// 注册本地处理函数
loader.RegisterHandler("internal/data/analyze", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
    tableName := input["table_name"].(string)
    operations := input["operations"].([]string)

    // 执行数据分析逻辑...
    result := analyzeData(tableName, operations)

    return map[string]interface{}{
        "statistics": result.Statistics,
        "correlations": result.Correlations,
    }, nil
})

// 加载所有 Skill
if err := loader.LoadFromDir(ctx); err != nil {
    log.Fatalf("failed to load skills: %v", err)
}

// 启动文件监听（可选）
events, _ := loader.Watch(ctx)
go func() {
    for event := range events {
        log.Printf("skill event: %s - %s", event.Type, event.Skill.ID)
    }
}()
```

#### 配置

```yaml
# config/skills.yaml
local_loader:
  enabled: true
  directories:
    - path: ./skills
      recursive: true
      watch: true              # 启用热重载

    - path: /etc/link/skills  # 系统级 Skill 目录
      recursive: true
      watch: false

  # 文件验证
  validation:
    strict: true               # 严格模式，任何验证失败都拒绝加载
    require_examples: false    # 是否要求必须有示例

  # 安全限制
  security:
    max_file_size: 1048576     # 最大文件大小 1MB
    allowed_extensions: [yaml, yml, json, toml]
    forbid_symlinks: true      # 禁止符号链接
```

### 5.4 Composite Skill Provider

```go
// CompositeSkillProvider 支持组合 Skill
type CompositeSkillProvider struct {
    registry skill.SkillRegistry
}

func NewCompositeSkillProvider(registry skill.SkillRegistry) *CompositeSkillProvider {
    return &CompositeSkillProvider{registry: registry}
}

// RegisterComposite 注册组合 Skill
func (p *CompositeSkillProvider) RegisterComposite(
    ctx context.Context,
    def *CompositeSkillDef,
) error {
    // 验证子 Skill 存在
    for _, subID := range def.SubSkills {
        if _, err := p.registry.Get(ctx, subID); err != nil {
            return fmt.Errorf("sub-skill not found: %s", subID)
        }
    }

    s := &skill.Skill{
        ID:          def.ID,
        Name:        def.Name,
        Description: def.Description,
        Category:    skill.SkillCategoryWorkflow,
        Capabilities: def.Capabilities,
        Dependencies: def.SubSkills,
        Provider:    skill.ProviderComposite,
        Implementation: skill.SkillImplementation{
            Type:        skill.ProviderComposite,
            SubSkills:   def.SubSkills,
            Orchestrator: def.Orchestrator,
        },
    }

    return p.registry.Register(ctx, s)
}

// CompositeSkillDef 组合 Skill 定义
type CompositeSkillDef struct {
    ID           string
    Name         string
    Description  string
    SubSkills    []string
    Orchestrator string // "sequence", "parallel", "conditional"
    Capabilities []skill.Capability
}

// 执行组合 Skill
func (p *CompositeSkillProvider) Execute(
    ctx context.Context,
    skill *skill.Skill,
    input map[string]interface{},
) (map[string]interface{}, error) {
    impl := skill.Implementation

    switch impl.Orchestrator {
    case "sequence":
        return p.executeSequence(ctx, impl.SubSkills, input)
    case "parallel":
        return p.executeParallel(ctx, impl.SubSkills, input)
    case "conditional":
        return p.executeConditional(ctx, impl.SubSkills, input)
    default:
        return nil, fmt.Errorf("unknown orchestrator: %s", impl.Orchestrator)
    }
}

func (p *CompositeSkillProvider) executeSequence(
    ctx context.Context,
    subSkills []string,
    input map[string]interface{},
) (map[string]interface{}, error) {
    result := input
    for _, skillID := range subSkills {
        subSkill, err := p.registry.Get(ctx, skillID)
        if err != nil {
            return nil, err
        }

        executor := NewSkillExecutor(p.registry)
        result, err = executor.Execute(ctx, skillID, result)
        if err != nil {
            return nil, fmt.Errorf("sub-skill %s failed: %w", skillID, err)
        }
    }
    return result, nil
}

func (p *CompositeSkillProvider) executeParallel(
    ctx context.Context,
    subSkills []string,
    input map[string]interface{},
) (map[string]interface{}, error) {
    var wg sync.WaitGroup
    results := make(map[string]interface{})
    var mu sync.Mutex
    errChan := make(chan error, len(subSkills))

    for _, skillID := range subSkills {
        wg.Add(1)
        go func(sid string) {
            defer wg.Done()

            executor := NewSkillExecutor(p.registry)
            result, err := executor.Execute(ctx, sid, input)
            if err != nil {
                errChan <- err
                return
            }

            mu.Lock()
            results[sid] = result
            mu.Unlock()
        }(skillID)
    }

    wg.Wait()
    close(errChan)

    if err := <-errChan; err != nil {
        return nil, err
    }

    return map[string]interface{}{
        "results": results,
        "count":   len(results),
    }, nil
}
```

### 5.5 Skill Resolver

```go
// SkillResolver 将 Skill 解析为 Tool
type SkillResolver struct {
    registry skill.SkillRegistry
}

func NewSkillResolver(registry skill.SkillRegistry) *SkillResolver {
    return &SkillResolver{registry: registry}
}

func (r *SkillResolver) ResolveToTool(
    ctx context.Context,
    skill *skill.Skill,
    capabilityName string,
) (*ToolDefinition, error) {
    // 查找指定 Capability
    var cap *skill.Capability
    for _, c := range skill.Capabilities {
        if c.Name == capabilityName {
            cap = &c
            break
        }
    }
    if cap == nil {
        return nil, fmt.Errorf("capability not found: %s", capabilityName)
    }

    // 构建 Tool Handler
    handler := r.buildHandler(skill, capabilityName)

    return &ToolDefinition{
        Name:        cap.Name,
        Description: cap.Description,
        Parameters:  cap.InputSchema,
        Handler:     handler,
    }, nil
}

func (r *SkillResolver) buildHandler(
    skill *skill.Skill,
    capabilityName string,
) ToolHandler {
    return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
        executor := NewSkillExecutor(r.registry)
        return executor.ExecuteWithCapability(ctx, skill.ID, capabilityName, args)
    }
}

// BatchResolve 批量解析 Skill 为 Tool
func (r *SkillResolver) BatchResolve(
    ctx context.Context,
    skillIDs []string,
) ([]*ToolDefinition, error) {
    var tools []*ToolDefinition

    for _, skillID := range skillIDs {
        skill, err := r.registry.Get(ctx, skillID)
        if err != nil {
            continue
        }

        // 为每个 Capability 创建 Tool
        for _, cap := range skill.Capabilities {
            tool, err := r.ResolveToTool(ctx, skill, cap.Name)
            if err != nil {
                continue
            }
            tools = append(tools, tool)
        }
    }

    return tools, nil
}
```

---

## 六、使用示例

### 6.1 注册和使用 Skill

```go
package main

import (
    "context"
    "link/internal/application/agent/skill"
    "link/internal/application/agent/skill/providers"
)

func main() {
    ctx := context.Background()

    // 1. 创建 Skill Registry
    registry := skill.NewRegistry()

    // 2. 注册 MCP Provider
    mcpClient := NewMCPClient("python-data")
    mcpProvider := providers.NewMCPSkillProvider(mcpClient, "python-data")

    // 3. 发现并注册 MCP Skills
    mcpSkills, _ := mcpProvider.Discover(ctx)
    for _, s := range mcpSkills {
        registry.Register(ctx, s)
    }

    // 4. 注册本地 Skill
    localSkill := &skill.Skill{
        ID:          "skill://local/calc",
        Name:        "calculator",
        Description: "基础计算能力",
        Category:    skill.SkillCategoryUtility,
        Capabilities: []skill.Capability{
            {
                Name: "calculate",
                Description: "执行数学计算",
                InputSchema: map[string]interface{}{
                    "type": "object",
                    "properties": map[string]interface{}{
                        "expression": map[string]interface{}{
                            "type": "string",
                            "description": "数学表达式",
                        },
                    },
                    "required": []string{"expression"},
                },
            },
        },
        Provider: skill.ProviderLocal,
        Implementation: skill.SkillImplementation{
            Type: skill.ProviderLocal,
            Handler: func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
                // 本地实现
                expr := input["expression"].(string)
                result := evalExpression(expr)
                return map[string]interface{}{"result": result}, nil
            },
        },
    }
    registry.Register(ctx, localSkill)

    // 5. 创建组合 Skill
    compositeProvider := providers.NewCompositeSkillProvider(registry)
    compositeProvider.RegisterComposite(ctx, &providers.CompositeSkillDef{
        ID:          "skill://workflow/data_analysis",
        Name:        "数据分析工作流",
        Description: "执行数据分析和可视化",
        SubSkills: []string{
            "skill://data/analysis",
            "skill://data/visualization",
        },
        Orchestrator: "sequence",
    })

    // 6. 解析为 Tool 并注册到 Tool Registry
    resolver := skill.NewSkillResolver(registry)
    tools, _ := resolver.BatchResolve(ctx, []string{
        "skill://data/analysis",
        "skill://workflow/data_analysis",
    })

    toolRegistry := NewToolRegistry()
    for _, t := range tools {
        toolRegistry.Register(*t)
    }
}
```

### 6.2 Agent 使用 Skill

```go
// 创建带 Skill 的 Agent
func CreateAgentWithSkills(registry skill.SkillRegistry) (*Agent, error) {
    builder := NewAgentBuilder().
        Name("数据分析师").
        Type(AgentTypeAgenticRAG).
        WithSkills(
            "skill://data/analysis",
            "skill://data/visualization",
            "skill://llm/judgment",
        )

    // Builder 内部会:
    // 1. 从 Skill Registry 获取 Skills
    // 2. 解析为 Tools
    // 3. 注册到 Agent 的 Tool Registry
    // 4. 生成 Agent Capabilities 描述

    return builder.Build(ctx)
}

// LLM 调用时的 Function Calling
func (a *Agent) Chat(ctx context.Context, message string) (*Response, error) {
    // Agent 持有的 Tool Registry 已包含来自 Skills 的 Tools
    tools := a.toolRegistry.ToLLMFunctions()

    // LLM 调用
    llmResp, _ := a.llm.Generate(ctx, message, tools)

    // 如果 LLM 选择调用 Tool
    for _, call := range llmResp.ToolCalls {
        result, _ := a.toolRegistry.CallTool(ctx, call.Name, call.Arguments)
        // 返回给 LLM...
    }

    return response, nil
}
```

### 6.3 多 Agent 协作中使用 Skill

```go
// Task Decomposer 识别所需 Skill
func (d *TaskDecomposer) Decompose(ctx context.Context, query string) (*TaskPlan, error) {
    // LLM 分析任务，识别需要的 Skills
    prompt := fmt.Sprintf(`
分析任务并识别所需 Skills:

任务: %s

可用 Skills:
- skill://data/analysis: 数据分析
- skill://data/visualization: 数据可视化
- skill://web/search: 网络搜索
- skill://kb/search: 知识库检索

返回 JSON:
[
  {
    "id": "task_1",
    "query": "子任务查询",
    "required_skills": ["skill://data/analysis"]
  }
]
`, query)

    resp, _ := d.llm.Generate(ctx, prompt)
    return d.parsePlan(resp)
}

// Task Dispatcher 根据 Skill 分配任务
func (d *TaskDispatcher) Dispatch(ctx context.Context, plan *TaskPlan) (*ExecutionResult, error) {
    for _, task := range plan.SubTasks {
        // 查找具有所需 Skills 的 Agent
        agents := d.agentRegistry.FindAgentsBySkills(task.RequiredSkills)
        // 分配并执行...
    }
}
```

---

## 七、元工具设计

### 7.1 概述

元工具（Meta Tools）是 Agent 用来发现和查找系统中合适能力的特殊工具。它们赋予 Agent 自主探索和选择能力的能力。

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              元工具能力                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   Agent                                                                     │
│     │                                                                       │
│     ├─► "我需要分析销售数据，该用什么能力？"                                 │
│     │                                                                       │
│     ▼                                                                       │
│   ┌─────────────────┐                                                      │
│   │  ToolSearchTool  │  搜索所有可用 Tool，找到最匹配的                     │
│   └─────────────────┘                                                      │
│     │                                                                       │
│     ├─► 找到: data_analyze, sales_query, statistics                        │
│     │                                                                       │
│     ▼                                                                       │
│   ┌─────────────────┐                                                      │
│   │   SkillTool      │  深入了解 Skill 能力、依赖、示例                     │
│   └─────────────────┘                                                      │
│     │                                                                       │
│     └─► 返回: skill://data/analysis 的详细信息和调用示例                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 ToolSearchTool

#### 功能描述

搜索和发现系统中可用的 Tool，根据任务需求找到最合适的 Tool。

#### Tool 定义

```json
{
  "name": "tool_search",
  "description": "搜索系统中可用的工具，根据任务描述找到最合适的工具。支持按类别、关键词、能力描述搜索。",
  "parameters": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "搜索查询，描述你想要完成的任务或需要的能力"
      },
      "category": {
        "type": "string",
        "description": "可选，限制搜索的工具类别：data/search/analysis/generation/utility/workflow",
        "enum": ["data", "search", "analysis", "generation", "utility", "workflow"]
      },
      "limit": {
        "type": "integer",
        "description": "返回结果数量，默认5",
        "default": 5
      }
    },
    "required": ["query"]
  }
}
```

#### 返回格式

```json
{
  "tools": [
    {
      "name": "data_analyze",
      "description": "分析数据表，生成统计报告",
      "category": "data",
      "skill_id": "skill://data/analysis",
      "parameters": {
        "table_name": {"type": "string", "description": "数据表名"},
        "operations": {"type": "array", "description": "分析操作"}
      },
      "examples": [
        {
          "description": "分析销售表的统计信息",
          "input": {"table_name": "sales", "operations": ["describe", "correlation"]}
        }
      ],
      "relevance_score": 0.95
    }
  ],
  "total": 12,
  "search_time_ms": 15
}
```

#### 搜索策略

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           ToolSearchTool 搜索策略                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   1. 语义匹配 (优先)                                                         │
│      ├── 使用 Embedding 模型计算 query 与 tool description 的相似度        │
│      ├── 考虑 tool.name 的语义相关性                                         │
│      └── 返回相似度 > 阈值的结果                                             │
│                                                                             │
│   2. 关键词匹配 (补充)                                                       │
│      ├── 提取 query 中的关键词                                              │
│      ├── 匹配 tool.description 中的关键词                                    │
│      └── 提升命中权重                                                       │
│                                                                             │
│   3. 类别过滤                                                               │
│      ├── 如果指定 category，只在该类别内搜索                                │
│      ├── 自动识别 query 隐含的类别                                          │
│      │   例: "分析数据" → category="data"                                   │
│                                                                             │
│   4. 热度/使用频率 (排序)                                                    │
│      ├── 常用工具优先展示                                                   │
│      ├── 最近成功调用过的工具提升权重                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 实现要点

```go
// ToolSearchTool 实现
type ToolSearchTool struct {
    toolRegistry   ToolRegistry
    skillRegistry  SkillRegistry
    embeddingModel EmbeddingModel  // 用于语义搜索
}

func (t *ToolSearchTool) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
    // 1. 获取所有工具
    allTools := t.toolRegistry.List()

    // 2. 计算相似度
    queryEmbedding := t.embeddingModel.Embed(ctx, req.Query)
    for _, tool := range allTools {
        toolEmbedding := t.embeddingModel.Embed(ctx, tool.Description)
        similarity := cosineSimilarity(queryEmbedding, toolEmbedding)
        tool.Score = similarity
    }

    // 3. 类别过滤
    if req.Category != "" {
        allTools = filterByCategory(allTools, req.Category)
    }

    // 4. 排序并返回 Top-K
    sortToolsByScore(allTools)
    return topK(allTools, req.Limit), nil
}
```

### 7.3 SkillTool

#### 功能描述

深入了解指定 Skill 的详细信息，包括能力列表、依赖关系、调用示例等。

#### Tool 定义

```json
{
  "name": "skill_info",
  "description": "获取 Skill 的详细信息，包括其提供的所有能力、依赖关系、使用示例等。可用于深入了解某个 Skill 的完整功能。",
  "parameters": {
    "type": "object",
    "properties": {
      "skill_id": {
        "type": "string",
        "description": "Skill ID，格式如 skill://data/analysis。也可使用简写如 data/analysis"
      },
      "include_capabilities": {
        "type": "boolean",
        "description": "是否包含能力详情，默认 true",
        "default": true
      },
      "include_dependencies": {
        "type": "boolean",
        "description": "是否包含依赖关系，默认 true",
        "default": true
      },
      "include_examples": {
        "type": "boolean",
        "description": "是否包含使用示例，默认 true",
        "default": true
      }
    },
    "required": ["skill_id"]
  }
}
```

#### 返回格式

```json
{
  "skill": {
    "id": "skill://data/analysis",
    "name": "analysis",
    "description": "数据分析能力，提供统计描述、相关性分析、分布分析等功能",
    "category": "data",
    "version": "1.0.0",
    "provider": "mcp",
    "capabilities": [
      {
        "name": "data_analyze",
        "description": "执行数据分析操作",
        "input_schema": {
          "table_name": "数据表名",
          "operations": ["describe", "correlation", "distribution"]
        },
        "output_schema": {
          "statistics": "统计结果",
          "correlations": "相关性矩阵"
        }
      },
      {
        "name": "data_aggregate",
        "description": "数据聚合操作",
        "input_schema": {
          "table_name": "数据表名",
          "group_by": "分组字段",
          "aggregations": "聚合函数列表"
        }
      }
    ],
    "dependencies": [
      {
        "skill_id": "skill://llm/judgment",
        "reason": "用于智能分析结果解读"
      }
    ],
    "examples": [
      {
        "capability": "data_analyze",
        "description": "分析电商订单数据",
        "input": {
          "table_name": "orders",
          "operations": ["describe", "correlation"]
        },
        "expected_output": "包含订单数量、金额统计等信息的报告"
      }
    ],
    "metadata": {
      "author": "data-team",
      "tags": ["data", "analysis", "statistics"],
      "success_rate": 0.98,
      "avg_duration_ms": 250
    }
  }
}
```

#### 特殊功能

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           SkillTool 特殊功能                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   1. 模糊匹配                                                               │
│      ├── 支持简写: "data/analysis" → "skill://data/analysis"               │
│      ├── 支持名称: "analysis" → 查找所有名称包含 analysis 的 Skill          │
│      └── 支持描述搜索: "数据分析" → 搜索相关 Skill                          │
│                                                                             │
│   2. 依赖可视化                                                             │
│      ├── 展示 Skill 依赖链                                                   │
│      ├── 标识循环依赖风险                                                   │
│      └── 提示依赖版本冲突                                                   │
│                                                                             │
│   3. 使用推荐                                                               │
│      ├── 根据当前任务推荐最合适的 Capability                                │
│      ├── 提供参数填写建议                                                   │
│      └── 展示常见调用模式                                                   │
│                                                                             │
│   4. 性能信息                                                               │
│      ├── 成功率                                                             │
│      ├── 平均耗时                                                           │
│      └── 最近错误摘要                                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 实现要点

```go
// SkillTool 实现
type SkillTool struct {
    skillRegistry SkillRegistry
    toolRegistry  ToolRegistry
}

func (s *SkillTool) GetInfo(ctx context.Context, skillID string, opts *InfoOptions) (*SkillInfo, error) {
    // 1. 解析和验证 Skill ID
    normalizedID, err := s.normalizeSkillID(skillID)
    if err != nil {
        // 尝试模糊搜索
        skills, _ := s.skillRegistry.FindByDescription(ctx, skillID)
        if len(skills) > 0 {
            normalizedID = skills[0].ID
        } else {
            return nil, fmt.Errorf("skill not found: %s", skillID)
        }
    }

    // 2. 获取 Skill
    skill, err := s.skillRegistry.Get(ctx, normalizedID)
    if err != nil {
        return nil, err
    }

    // 3. 构建响应
    info := &SkillInfo{
        ID:          skill.ID,
        Name:        skill.Name,
        Description: skill.Description,
        Category:    skill.Category,
        Version:     skill.Version,
        Provider:    skill.Provider,
    }

    // 4. 添加 Capabilities
    if opts.IncludeCapabilities {
        info.Capabilities = skill.Capabilities
    }

    // 5. 解析依赖
    if opts.IncludeDependencies {
        deps := s.resolveDependencies(ctx, skill)
        info.Dependencies = deps
    }

    // 6. 生成示例
    if opts.IncludeExamples {
        examples := s.generateExamples(skill)
        info.Examples = examples
    }

    // 7. 添加元数据
    info.Metadata = s.collectMetadata(skill)

    return info, nil
}
```

### 7.4 元工具协同使用

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         元工具协同使用流程                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   用户: "帮我分析一下最近的销售数据趋势"                                     │
│                                                                             │
│   Agent Thought: "我需要先找到合适的工具"                                    │
│                                                                             │
│   Agent Action: tool_search                                                 │
│   {                                                                         │
│     "query": "销售数据分析 趋势",                                           │
│     "category": "data"                                                      │
│   }                                                                         │
│                                                                             │
│   Agent Observation: 找到以下工具                                           │
│   - data_analyze (相关度 0.92)                                              │
│   - trend_analysis (相关度 0.88)                                            │
│   - sales_query (相关度 0.75)                                               │
│                                                                             │
│   Agent Thought: "data_analyze 看起来最相关，让我了解详情"                  │
│                                                                             │
│   Agent Action: skill_info                                                  │
│   {                                                                         │
│     "skill_id": "skill://data/analysis",                                    │
│     "include_examples": true                                                │
│   }                                                                         │
│                                                                             │
│   Agent Observation: skill://data/analysis 提供以下能力                    │
│   - data_analyze: 执行数据分析                                               │
│     示例: {table_name: "sales", operations: ["describe", "trend"]}         │
│   - trend_forecast: 趋势预测                                                │
│     示例: {table_name: "sales", periods: 7}                                 │
│                                                                             │
│   Agent Thought: "我应该先用 data_analyze 分析，再用 trend_forecast 预测"    │
│                                                                             │
│   Agent Action: data_analyze                                               │
│   {table_name: "sales", operations: ["describe", "trend"]}                   │
│                                                                             │
│   ... 后续执行 ...                                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.5 元工具配置

```yaml
# config/meta_tools.yaml
meta_tools:
  tool_search:
    enabled: true
    embedding_model: "text-embedding-3-small"  # 用于语义搜索
    cache_ttl: 300                             # 缓存搜索结果5分钟
    max_results: 10
    min_similarity: 0.6                        # 最低相似度阈值

  skill_info:
    enabled: true
    cache_ttl: 600                             # 缓存 Skill 信息10分钟
    include_metrics: true                      # 包含性能指标
    max_dependencies_depth: 5                  # 最大依赖解析深度

# Agent 默认启用元工具
agents:
  default:
    meta_tools:
      - tool_search
      - skill_info
```

---

## 八、实现路径

### 8.1 阶段规划

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              实现阶段规划                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   阶段 1: 领域层设计 (Week 1)                                                │
│   ├── 定义 Skill 实体                                                       │
│   │   ├── skill.go                                                         │
│   │   ├── skill_registry.go                                                │
│   │   ├── skill_provider.go                                                │
│   │   └── skill_resolver.go                                                │
│   ├── 定义与现有系统的集成接口                                              │
│   │   ├── AgentRegistry 扩展                                               │
│   │   └── ToolRegistry 扩展                                                │
│   └── 编写单元测试                                                         │
│                                                                             │
│   阶段 2: MCP Provider 实现 (Week 2)                                        │
│   ├── MCPSkillProvider                                                     │
│   │   ├── Discover 实现                                                    │
│   │   ├── Watch 实现                                                       │
│   │   └── 命名规则转换                                                     │
│   ├── Skill Registry 实现                                                  │
│   │   ├── 内存注册表                                                       │
│   │   ├── 依赖解析                                                         │
│   │   └── 事件发布                                                         │
│   └── 与现有 MCP Client 集成                                               │
│                                                                             │
│   阶段 3: Skill Resolver 实现 (Week 3)                                     │
│   ├── Skill → Tool 转换                                                    │
│   ├── 批量解析                                                             │
│   └── 与 Tool Registry 集成                                                 │
│                                                                             │
│   阶段 4: Local Skill Loader 实现 (Week 4)                                │
│   ├── LocalSkillLoader                                                     │
│   │   ├── YAML/JSON/TOML 解析                                              │
│   │   ├── 目录扫描                                                         │
│   │   └── 文件监听 (热重载)                                                 │
│   ├── SkillValidator 实现                                                   │
│   │   ├── Schema 验证                                                      │
│   │   ├── 依赖检查                                                         │
│   │   └── 安全限制                                                         │
│   └── Handler 注册机制                                                     │
│                                                                             │
│   阶段 5: Composite Skill 实现 (Week 5)                                    │
│   ├── CompositeSkillProvider                                               │
│   ├── 编排器实现                                                           │
│   │   ├── sequence.go                                                      │
│   │   ├── parallel.go                                                      │
│   │   └── conditional.go                                                   │
│   └── 嵌套 Skill 支持                                                      │
│                                                                             │
│   阶段 6: Agent Builder 集成 (Week 6)                                       │
│   ├── WithSkills() 方法                                                    │
│   ├── 自动 Tool 注册                                                       │
│   └── Capabilities 聚合                                                    │
│                                                                             │
│   阶段 7: 元工具实现 (Week 7)                                              │
│   ├── ToolSearchTool 实现                                                  │
│   │   ├── 语义搜索                                                         │
│   │   ├── 关键词匹配                                                       │
│   │   └── 相关性排序                                                       │
│   ├── SkillTool 实现                                                       │
│   │   ├── Skill 详情获取                                                   │
│   │   ├── 依赖解析                                                         │
│   │   └── 示例生成                                                         │
│   └── 元工具与 Agent 集成                                                  │
│                                                                             │
│   阶段 8: 测试与文档 (Week 8)                                              │
│   ├── 端到端测试                                                           │
│   ├── 性能测试                                                             │
│   └── 文档完善                                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 8.2 兼容性保证

| 功能 | 现有实现 | Skill 扩展 | 兼容性 |
|------|---------|-----------|--------|
| Tool 注册 | 直接注册 Tool | Skill → Tool 转换 | ✅ 完全兼容 |
| Agent 创建 | Builder 模式 | 添加 WithSkills() | ✅ 向后兼容 |
| MCP 调用 | 直接 MCP Client | 通过 Skill 抽象 | ✅ 复用现有 |
| 多 Agent 协作 | Skill 匹配 | Skill ID 匹配 | ✅ 扩展支持 |

### 8.3 配置示例

```yaml
# config/skills.yaml
skills:
  # MCP Servers
  mcp_servers:
    - name: python-data
      transport: stdio
      command: python -m cognida_python.mcp.server
      auto_discover: true

    - name: third-party-tools
      transport: http
      url: http://tools-service:3000/mcp

  # 本地文件加载器
  local_loader:
    enabled: true
    directories:
      - path: ./skills              # 项目 Skill 目录
        recursive: true
        watch: true                 # 启用热重载

      - path: /etc/link/skills      # 系统 Skill 目录
        recursive: true
        watch: false

    # 文件验证
    validation:
      strict: true                  # 严格模式
      require_examples: false

    # 安全限制
    security:
      max_file_size: 1048576        # 1MB
      allowed_extensions: [yaml, yml, json, toml]
      forbid_symlinks: true

  # Local Skills (代码注册)
  local_skills:
    - id: skill://local/calc
      name: calculator
      handler: internal/calculator

    - id: skill://workflow/rag
      name: rag_workflow
      type: composite
      sub_skills:
        - skill://kb/search
        - skill://llm/generate

  # Agent Skills 配置
  agents:
    - name: "数据分析师"
      skills:
        - skill://data/analysis
        - skill://data/visualization
        - skill://llm/judgment

    - name: "研究助手"
      skills:
        - skill://web/search
        - skill://kb/search
        - skill://graph/query

  # 元工具配置
  meta_tools:
    tool_search:
      enabled: true
      embedding_model: "text-embedding-3-small"
      cache_ttl: 300
      max_results: 10
      min_similarity: 0.6

    skill_info:
      enabled: true
      cache_ttl: 600
      include_metrics: true
      max_dependencies_depth: 5
```

---

## 附录

### A. Skill ID 命名规范

```
格式: skill://{category}/{name}

示例:
- skill://data/analysis        - 数据分析
- skill://data/visualization   - 数据可视化
- skill://search/kb            - 知识库搜索
- skill://search/web           - 网络搜索
- skill://llm/judgment         - LLM 评判
- skill://workflow/rag         - RAG 工作流
- skill://util/calc            - 计算工具
```

### B. 版本历史

| 版本 | 日期 | 变更 |
|-----|------|------|
| v1.2 | 2026-05-09 | 新增本地 Skill 加载器设计 (LocalSkillLoader) |
| v1.1 | 2026-05-09 | 新增元工具设计 (ToolSearchTool, SkillTool) |
| v1.0 | 2026-05-09 | 初版，Skill 系统设计方案 |

---

**文档版本**: v1.2
**更新时间**: 2026-05-09

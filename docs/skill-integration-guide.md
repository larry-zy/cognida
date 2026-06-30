# Skill 系统集成指南

## 概述

Skill 系统已完成实现，提供基于 SKILL.md Markdown 文档的专业知识注入能力。

## 已实现内容

### 核心组件

| 组件 | 文件 | 功能 |
|------|------|------|
| 类型定义 | `types.go` | Skill、SkillInfo、SkillMatchResult 等核心类型 |
| 加载器 | `loader.go` | 解析 SKILL.md 文件 |
| 注册表 | `registry.go` | 存储和检索 Skill |
| 管理器 | `manager.go` | Skill 生命周期管理 |
| Agent 集成 | `agent_integration.go` | 将 Skill 格式化为 Agent 内容 |
| 中间件 | `middleware.go` | 自动匹配和注入 Skill |
| 便捷函数 | `convenience.go` | 简化使用的函数 |

### 工具集成

| 工具 | 功能 | 参数 |
|------|------|------|
| `skill_list` | 列出所有 Skill | category, tags |
| `skill_invoke` | 调用指定 Skill | skill_name, task |
| `skill_match` | 匹配相关 Skill | task, limit |

### 示例 Skill

- `code-review` - 代码审查（纯指导性）
- `rag-search` - 知识库检索（工具调用）
- `data-analysis` - 数据分析（工具调用）

---

## 集成步骤

### 1. 启动时初始化

在 `main.go` 或服务初始化代码中：

```go
import "link/internal/service/agent/skills"

func main() {
    // 初始化 Skill 系统
    if err := skills.InitializeDefault(); err != nil {
        log.Printf("[警告] Skill 系统初始化失败: %v", err)
        // 继续运行，Skill 系统非必需
    }

    // ... 其他初始化
}
```

### 2. Agent 集成

#### 方式一：自动匹配（推荐）

```go
import "link/internal/service/agent/skills"

// 创建 Agent 时添加 Skill 中间件
agent := agent.NewBuilder().
    Name("MyAgent").
    Model(llmClient).
    Tools(ragTool, searchTool).
    Middleware(skills.NewAutoSkillMiddleware(
        skills.WithMaxSkills(3), // 最多注入 3 个 Skill
    )).
    Build()
```

#### 方式二：手动调用

Agent 通过工具调用 Skill，不需要额外配置。

### 3. HTTP API 集成（可选）

如果需要通过 API 管理 Skill：

```go
// handler/skill_handler.go
type SkillHandler struct {
    skillManager skills.SkillManager
}

// ListSkills 列出所有 Skill
func (h *SkillHandler) ListSkills(c *gin.Context) {
    skills := h.skillManager.ListSkillsInfo()
    OK(c, skills)
}

// GetSkill 获取 Skill 详情
func (h *SkillHandler) GetSkill(c *gin.Context) {
    name := c.Param("name")
    skill, exists := h.skillManager.GetSkill(name)
    if !exists {
        NotFound(c, "Skill not found")
        return
    }
    OK(c, skill)
}

// MatchSkills 匹配相关 Skill
func (h *SkillHandler) MatchSkills(c *gin.Context) {
    var req struct {
        Task  string `json:"task" binding:"required"`
        Limit int    `json:"limit"`
    }
    if !BindJSON(c, &req) {
        return
    }

    if req.Limit == 0 {
        req.Limit = 3
    }

    matches := h.skillManager.MatchSkillsForTask(req.Task, req.Limit)
    OK(c, matches)
}
```

---

## 路由配置（可选）

```go
// router/skill_router.go
func RegisterSkillRoutes(r *gin.Engine, handler *SkillHandler) {
    v1 := r.Group("/api/v1/skills")
    {
        v1.GET("", handler.ListSkills)
        v1.GET("/:name", handler.GetSkill)
        v1.POST("/match", handler.MatchSkills)
        v1.GET("/categories", handler.ListCategories)
    }
}
```

---

## 目录结构

### Skill 存储目录

```
D:/link/skills/
├── code-review/
│   └── SKILL.md
├── rag-search/
│   └── SKILL.md
├── data-analysis/
│   └── SKILL.md
└── debugging/
    ├── SKILL.md
    └── examples/
        └── example.md
```

### 代码结构

```
link-go/
├── internal/service/agent/skills/
│   ├── types.go
│   ├── loader.go
│   ├── registry.go
│   ├── manager.go
│   ├── agent_integration.go
│   ├── middleware.go
│   ├── convenience.go
│   ├── init.go
│   ├── README.md
│   └── tools/
│       ├── skill_list_tool.go
│       ├── skill_invoke_tool.go
│       └── init.go
└── docs/
    ├── skill-system.md
    └── skill-integration-guide.md
```

---

## 使用示例

### Agent 使用 Skill

```go
// Agent 收到用户消息
userMessage := "请帮我审查这段代码"

// 中间件自动匹配 Skill
// - 查询 "代码审查"
// - 匹配到 code-review Skill
// - 将 Skill 内容注入到系统提示

// Agent 使用 Skill 指导生成响应
response := agent.Chat(ctx, userMessage)
```

### 工具调用 Skill

```json
// Agent 调用 skill_match 工具
{
  "tool": "skill_match",
  "parameters": {
    "task": "需要从数据库查询用户数据",
    "limit": 3
  }
}

// 返回匹配的 Skill
{
  "success": true,
  "matches": [
    {
      "name": "data-analysis",
      "relevance": 0.85,
      "reason": "description match (2/2 words)"
    }
  ]
}

// Agent 调用 skill_invoke 加载 Skill
{
  "tool": "skill_invoke",
  "parameters": {
    "skill_name": "data-analysis"
  }
}
```

---

## 配置选项

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SKILL_DIR` | `./skills` | Skill 目录 |
| `SKILL_AUTO_MATCH` | `true` | 是否自动匹配 |
| `SKILL_MAX_SKILLS` | `3` | 最大注入数量 |

### 中间件选项

```go
middleware := skills.NewAutoSkillMiddleware(
    skills.WithSkillManager(customManager),  // 自定义管理器
    skills.WithMaxSkills(5),                 // 最多注入 5 个
    skills.WithEnabled(true),                // 启用/禁用
)
```

---

## 测试

### 加载测试

```go
func TestSkillLoading(t *testing.T) {
    // 初始化
    err := skills.Initialize("D:/link/skills")
    assert.NoError(t, err)

    // 验证加载
    manager := skills.GetGlobalManager()
    assert.Greater(t, manager.Size(), 0)
}
```

### 匹配测试

```go
func TestSkillMatching(t *testing.T) {
    manager := skills.GetGlobalManager()

    matches := manager.MatchSkillsForTask("代码审查", 3)
    assert.Greater(t, len(matches), 0)

    // 验证 code-review 被匹配
    found := false
    for _, m := range matches {
        if m.Skill.Name == "code-review" {
            found = true
            assert.Greater(t, m.Relevance, 0.5)
        }
    }
    assert.True(t, found)
}
```

---

## 常见问题

### Q: Skill 未加载？

检查：
1. SKILL.md 格式是否正确（frontmatter + body）
2. 文件是否在正确的目录
3. 查看日志中的加载错误

### Q: 匹配不准确？

调整：
1. 优化 `name` 和 `description`
2. 添加相关的 `tags`
3. 使用 `when_to_use` 说明使用场景

### Q: 如何创建新 Skill？

1. 在 `D:/link/skills/` 下创建目录
2. 创建 SKILL.md 文件
3. 编写 frontmatter 和内容
4. 重启服务

---

## 下一步

1. **创建更多 Skill**：根据业务需求添加领域知识
2. **优化匹配算法**：基于实际使用反馈调整
3. **支持动态加载**：监听目录变化，自动重载
4. **性能优化**：缓存格式化内容，减少重复计算

# Skill 系统文档

## 概述

Skill 系统提供了一种通过 Markdown 文档（SKILL.md）为 Agent 提供专业知识的方法。

### 核心概念

- **Skill 是 Markdown 文档**：不是 Go 代码，是 SKILL.md 格式的文档
- **两种类型**：
  - **纯指导性 Skill**：仅提供知识，不调用工具
  - **工具调用 Skill**：提供知识并定义可使用的工具

### Skill 与工具的区别

| 方面 | Skill | Tool |
|------|-------|------|
| 本质 | Markdown 文档（指导） | 可执行函数（操作） |
| 格式 | SKILL.md | Go 代码 |
| 内容 | 领域知识、最佳实践 | 具体功能实现 |
| 调用方式 | 内容注入到系统提示 | Agent 主动调用 |

---

## SKILL.md 格式

### 基本结构

```markdown
---
name: skill-name
description: 技能描述
when_to_use: 何时使用此技能
category: 分类
tags:
  - tag1
  - tag2
allowed_tools:
  - tool1
  - tool2
---

# 技能标题

技能指导内容...
```

### Frontmatter 字段

| 字段 | 必需 | 说明 |
|------|------|------|
| `name` | ✅ | 全局唯一名称 |
| `description` | ✅ | 描述，用于搜索匹配 |
| `when_to_use` | ❌ | 何时使用此 Skill |
| `category` | ❌ | 分类（如 development, retrieval, data） |
| `tags` | ❌ | 标签列表 |
| `allowed_tools` | ❌ | 允许使用的工具白名单 |
| `disallowed_tools` | ❌ | 禁止使用的工具黑名单 |
| `model` | ❌ | 模型覆盖（"inherit" = 继承） |
| `context_mode` | ❌ | 执行模式（inline/fork） |
| `effort` | ❌ | 努力程度（1-10） |
| `disable_model_invocation` | ❌ | 禁用模型调用（纯指导性） |
| `version` | ❌ | 版本号 |
| `author` | ❌ | 作者 |
| `experimental` | ❌ | 是否实验性 |

### 支持文件目录

```
skills/my-skill/
├── SKILL.md          # 主文件（必需）
├── scripts/          # 可执行脚本（可选）
├── references/       # 参考文档（可选）
├── examples/         # 示例文件（可选）
└── assets/           # 资源文件（可选）
```

---

## 使用方式

### 1. 自动匹配（推荐）

Agent 执行时自动匹配相关 Skill 并注入：

```go
import "link/internal/service/agent/skills"

// 创建中间件
middleware := skills.NewAutoSkillMiddleware(
    skills.WithMaxSkills(3), // 最多注入 3 个 Skill
)

// 添加到 Agent
agent := builder.
    Name("MyAgent").
    Middleware(middleware).
    Build()
```

### 2. 手动调用

Agent 通过工具调用 Skill：

```go
// Agent 可以调用以下工具：
// - skill_list: 列出所有 Skill
// - skill_invoke: 调用指定 Skill
// - skill_match: 匹配相关 Skill
```

### 3. 代码中使用

```go
import "link/internal/service/agent/skills"

// 查找 Skill
skill, err := skills.FindSkill("code-review")

// 获取内容
content, err := skills.GetSkillContent("code-review")

// 按分类查找
skills := skills.FindSkillsByCategory("development")

// 生成提示
prompt, err := skills.GenerateSkillPrompt([]string{"code-review", "data-analysis"})
```

---

## 已提供的 Skill

### code-review

纯指导性 Skill，提供代码审查方法和最佳实践。

- **类型**：纯指导性
- **分类**：development
- **标签**：code-review, quality, best-practices

### rag-search

工具调用 Skill，提供知识库检索方法。

- **类型**：工具调用
- **分类**：retrieval
- **标签**：rag, retrieval, knowledge-base, search
- **可用工具**：rag_query, kb_select, kb_list

### data-analysis

工具调用 Skill，提供数据查询和分析方法。

- **类型**：工具调用
- **分类**：data
- **标签**：sql, database, analytics, reporting
- **可用工具**：sql_execute, get_schema

---

## API 参考

### 核心接口

```go
// SkillManager Skill 管理器
type SkillManager interface {
    LoadSkills(dir string) (*SkillLoadResult, error)
    GetSkill(name string) (*Skill, bool)
    ListSkills() []*Skill
    SearchSkills(query string) []*SkillMatchResult
    MatchSkillsForTask(task string, limit int) []*SkillMatchResult
}

// SkillAgentIntegration Skill 与 Agent 集成
type SkillAgentIntegration interface {
    FormatForAgent(ctx context.Context, skill *Skill) (*SkillContext, error)
    InjectIntoSystemMessage(systemMsg string, skillContext *SkillContext) string
}
```

### 便捷函数

```go
// 查找 Skill
skills.FindSkill(name string) (*Skill, error)

// 获取 Skill 内容
skills.GetSkillContent(name string) (string, error)

// 按分类查找
skills.FindSkillsByCategory(category string) []*Skill

// 按标签查找
skills.FindSkillsByTags(tags ...string) []*Skill

// 获取所有分类
skills.GetCategories() []string
```

---

## 创建新 Skill

### 步骤

1. 创建目录：`skills/my-skill/`
2. 创建 SKILL.md 文件
3. 编写 frontmatter 和内容
4. 放入 `D:/cognida/skills/` 目录
5. 重启服务或调用 `skills.Initialize()`

### 模板

```markdown
---
name: my-skill
description: 简短描述
when_to_use: 何时使用
category: 分类
tags:
  - tag1
  - tag2
version: "1.0.0"
author: Your Name
---

# Skill 标题

## 使用场景

[描述使用场景]

## 指导内容

[提供具体的指导、步骤、最佳实践]

## 注意事项

[列出需要注意的事项]
```

---

## 工具集成

### Skill 工具

系统自动注册以下工具：

1. **skill_list**
   - 列出所有可用 Skill
   - 参数：category（可选）、tags（可选）

2. **skill_invoke**
   - 调用指定 Skill
   - 参数：skill_name（必需）、task（可选）

3. **skill_match**
   - 匹配相关 Skill
   - 参数：task（必需）、limit（可选）

### 工具分组

所有 Skill 工具注册在 `skill` 分组下。

---

## 配置

### 环境变量

```bash
# Skill 目录（默认：./skills）
SKILL_DIR=/path/to/skills

# 自动匹配模式（默认：true）
SKILL_AUTO_MATCH=true

# 最大注入 Skill 数量（默认：3）
SKILL_MAX_SKILLS=3
```

### 初始化

```go
// 在 main.go 中初始化
import "link/internal/service/agent/skills"

func main() {
    // 初始化 Skill 系统
    if err := skills.InitializeDefault(); err != nil {
        log.Fatalf("Failed to initialize skills: %v", err)
    }

    // ...
}
```

---

## 最佳实践

### Skill 编写

1. **明确目标**：清楚说明 Skill 解决什么问题
2. **结构清晰**：使用标题、列表、代码块
3. **具体可操作**：提供具体步骤而非抽象概念
4. **示例丰富**：包含实际示例
5. **保持更新**：定期 review 和更新

### Skill 使用

1. **合理分类**：使用合适的 category 和 tags
2. **适度使用**：不要注入过多 Skill（建议 ≤ 3 个）
3. **验证匹配**：检查匹配的 Skill 是否真的相关
4. **监控效果**：观察 Skill 对 Agent 输出的影响

---

## 故障排除

### Skill 未加载

- 检查 SKILL.md 格式是否正确
- 确认文件在正确的目录
- 查看日志中的加载错误

### 匹配不准确

- 调整 name 和 description 使其更具体
- 添加相关的 tags
- 使用 when_to_use 字段说明使用场景

### 内容过长

- 将详细内容移到 examples/ 目录
- 在 SKILL.md 中只保留核心内容
- 使用 `WithMaxContentLength` 限制长度

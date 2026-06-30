# Skills Package

Agent Skill 系统 - 基于 SKILL.md Markdown 文档的专业知识注入。

## 快速开始

### 1. 初始化

```go
import "link/internal/service/agent/skills"

// 初始化 Skill 系统
if err := skills.InitializeDefault(); err != nil {
    log.Fatal(err)
}
```

### 2. 自动匹配

```go
// 创建自动 Skill 匹配中间件
middleware := skills.NewAutoSkillMiddleware(
    skills.WithMaxSkills(3),
)

// 添加到 Agent
agent := builder.
    Middleware(middleware).
    Build()
```

### 3. 手动调用

Agent 可以通过工具调用：

- `skill_list` - 列出所有 Skill
- `skill_invoke` - 调用指定 Skill
- `skill_match` - 匹配相关 Skill

## 包结构

```
skills/
├── types.go           # 类型定义
├── loader.go          # SKILL.md 加载器
├── registry.go        # Skill 注册表
├── manager.go         # Skill 管理器
├── agent_integration.go  # Agent 集成
├── middleware.go      # 中间件
├── convenience.go     # 便捷函数
├── init.go            # 初始化
└── tools/             # Skill 工具
    ├── skill_list_tool.go
    ├── skill_invoke_tool.go
    └── init.go
```

## 核心 API

### 查找 Skill

```go
skill, err := skills.FindSkill("code-review")
content, err := skills.GetSkillContent("code-review")
```

### 按分类查找

```go
skills := skills.FindSkillsByCategory("development")
```

### 搜索匹配

```go
matches := skills.SearchAllSkills("代码审查")
```

### 构建 Skill

```go
skill := skills.NewSkillBuilder("my-skill", "描述").
    WithCategory("development").
    WithTags("testing", "quality").
    WithContent("指导内容...").
    Build()
```

## SKILL.md 格式

```markdown
---
name: skill-name
description: 技能描述
when_to_use: 何时使用
category: 分类
tags:
  - tag1
allowed_tools:
  - tool1
---

# 技能内容

详细指导...
```

详见：`docs/skill-system.md`

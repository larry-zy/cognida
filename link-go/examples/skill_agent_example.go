// Skill Agent 使用示例
// 演示如何在 Agent 中启用自动 Skill 注入
package main

import (
	"fmt"
	"log"

	"link/internal/service/agent/skills"
)

func main() {
	// ========================================
	// 1. 初始化 Skill 系统
	// ========================================
	fmt.Println("=== 步骤 1: 初始化 Skill 系统 ===")

	// 初始化会自动加载默认目录 (./skills 或 D:/link/skills) 中的所有 Skill
	if err := skills.InitializeDefault(); err != nil {
		log.Printf("警告: Skill 初始化失败: %v", err)
	}

	// 显示已加载的 Skill
	skillNames := skills.GetSkillNames()
	fmt.Printf("已加载 %d 个 Skill: %v\n\n", len(skillNames), skillNames)

	// ========================================
	// 2. 创建带自动 Skill 注入的 Agent
	// ========================================
	fmt.Println("=== 步骤 2: 创建 Agent ===")

	// 使用便捷函数创建可直接用于 Agent 的中间件
	skillMiddleware := skills.NewAutoSkillMiddlewareForAgent(
		skills.WithMaxSkills(3), // 最多注入 3 个 Skill
	)

	// 示例：在创建 Agent 时添加中间件
	// (实际使用时需要根据具体的 Agent 框架调整)
	fmt.Printf("已创建 Skill 中间件: %T\n\n", skillMiddleware)

	// ========================================
	// 3. 演示工作流程
	// ========================================
	fmt.Println("=== 步骤 3: 工作流程演示 ===")

	fmt.Println("当用户发送消息时:")
	fmt.Println("1. Agent 收到消息")
	fmt.Println("2. Skill 中间件自动匹配相关 Skill（基于消息内容）")
	fmt.Println("3. 将 Skill 内容注入到消息中")
	fmt.Println("4. Agent 使用增强后的消息生成响应")

	// ========================================
	// 4. 演示便捷函数
	// ========================================
	fmt.Println("=== 步骤 4: 便捷函数示例 ===")

	// 查找 Skill
	if skill, err := skills.FindSkill("code-review"); err == nil {
		fmt.Printf("找到 Skill: %s\n", skill.Name)
		fmt.Printf("描述: %s\n", skill.Description)
		fmt.Printf("分类: %s\n", skill.Category)
		fmt.Printf("标签: %v\n\n", skill.Tags)
	}

	// 按分类查找
	devSkills := skills.FindSkillsByCategory("development")
	fmt.Printf("开发类 Skill: %d 个\n", len(devSkills))

	// 获取所有分类
	categories := skills.GetCategories()
	fmt.Printf("所有分类: %v\n\n", categories)

	// ========================================
	// 5. 实际使用示例
	// ========================================
	fmt.Println("=== 步骤 5: 实际使用代码示例 ===")

	fmt.Println(`// 创建带 Skill 注入的 Agent
import (
    "link/internal/service/agent/framework"
    "link/internal/service/agent/skills"
)

// 创建 Skill 中间件
skillMiddleware := skills.NewAutoSkillMiddlewareForAgent(
    skills.WithMaxSkills(3),
)

// 添加到 Agent
agent := framework.New(chatModel).
    Name("MyAgent").
    System("你是一个智能助手").
    Middleware(skillMiddleware).  // <-- 添加 Skill 中间件
    Build()

// 当用户发送 "请帮我审查这段代码" 时
// 中间件自动匹配并注入 code-review Skill`)

	fmt.Println("\n=== 总结 ===")
	fmt.Println("✅ Skill 工具自动注册 (skill_list, skill_invoke, skill_match)")
	fmt.Println("✅ Skill 系统初始化加载所有 Skill")
	fmt.Println("✅ 中间件自动匹配并注入相关 Skill")
	fmt.Println("✅ Agent 无需手动调用，自动使用 Skill 指导")
}

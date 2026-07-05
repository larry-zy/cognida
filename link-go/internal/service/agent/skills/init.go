// Package skills 提供 Skill 系统初始化
package skills

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// 全局配置
var (
	skillDirs   []string // 技能目录列表
	initialized bool
	initOnce    sync.Once
)

// ========================================
// 初始化
// ========================================

// Initialize 初始化 Skill 系统
// 从指定目录加载所有 Skill
func Initialize(dirs ...string) error {
	var initErr error
	initOnce.Do(func() {
		// 设置默认目录
		if len(dirs) == 0 {
			// 默认目录（项目根目录下的 skills 文件夹）
			dirs = []string{
				filepath.Join(".", "skills"),
				"D:/link/skills", // Windows 开发环境
			}
		}

		skillDirs = dirs

		// 加载所有目录的 Skills
		totalCount := 0
		for _, dir := range dirs {
			result, err := InitializeGlobalSkills(dir)
			if err != nil {
				log.Printf("[警告] Failed to load skills from %s: %v", dir, err)
				continue
			}

			if result.Count > 0 {
				log.Printf("[Skill] Loaded %d skills from %s", result.Count, dir)
				totalCount += result.Count
			}

			if len(result.Errors) > 0 {
				for _, e := range result.Errors {
					log.Printf("[Skill] Error loading from %s: %v", dir, e)
				}
			}
		}

		initialized = true

		log.Printf("[Skill] Skill system initialized, total skills: %d", totalCount)
	})

	return initErr
}

// InitializeDefault 初始化 Skill 系统（使用默认目录）
// 便捷函数，用于启动时自动初始化
func InitializeDefault() error {
	return Initialize()
}

// InitializeFromEnv 以健壮的目录解析初始化 Skill 系统，供服务启动时调用。
//
// 解析优先级：
//  1. 环境变量 LINK_SKILL_DIRS（逗号分隔的一或多个目录）——显式配置优先；
//  2. 否则回退到一组候选相对目录：./skills、../skills、../../skills。
//     服务通常从 link-go/ 启动，而 Skill 文件位于仓库根的 skills/（即 ../skills），
//     原默认的 ./skills 会解析到 link-go/skills 而落空——这里显式覆盖候选，修复该 CWD 陷阱。
//
// 仅加载「存在且含 SKILL.md」的目录；缺失目录由 SkillManager.LoadSkills 静默跳过。
func InitializeFromEnv() error {
	var dirs []string
	if env := strings.TrimSpace(os.Getenv("LINK_SKILL_DIRS")); env != "" {
		for _, d := range strings.Split(env, ",") {
			if d = strings.TrimSpace(d); d != "" {
				dirs = append(dirs, d)
			}
		}
	}
	if len(dirs) == 0 {
		dirs = []string{
			filepath.Join(".", "skills"),
			filepath.Join("..", "skills"),
			filepath.Join("..", "..", "skills"),
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		log.Printf("[Skill] Initializing from dirs %v (cwd=%s)", dirs, cwd)
	}
	return Initialize(dirs...)
}

// IsInitialized 检查是否已初始化
func IsInitialized() bool {
	return initialized
}

// AddSkillDir 添加 Skill 目录
func AddSkillDir(dir string) {
	skillDirs = append(skillDirs, dir)
}

// GetSkillDirs 获取所有 Skill 目录
func GetSkillDirs() []string {
	return skillDirs
}

// ReloadSkills 重新加载所有 Skills
func ReloadSkills() error {
	initialized = false
	initOnce = sync.Once{}
	return Initialize(skillDirs...)
}

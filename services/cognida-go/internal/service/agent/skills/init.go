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
		// 未显式指定时回退到规范候选目录（与 InitializeFromEnv 同源，见 DefaultSkillDirs）：
		// 服务通常从 cognida-go/ 启动而 Skill 位于 ../skills，故列多级相对候选，修复 CWD 陷阱。
		if len(dirs) == 0 {
			dirs = DefaultSkillDirs()
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
//  1. 环境变量 COGNIDA_SKILL_DIRS（逗号分隔的一或多个目录）——显式配置优先；
//  2. 否则回退到一组候选相对目录：./skills、../skills、../../skills。
//     服务通常从 cognida-go/ 启动，而 Skill 文件位于仓库根的 skills/（即 ../skills），
//     原默认的 ./skills 会解析到 cognida-go/skills 而落空——这里显式覆盖候选，修复该 CWD 陷阱。
//
// 仅加载「存在且含 SKILL.md」的目录；缺失目录由 SkillManager.LoadSkills 静默跳过。
func InitializeFromEnv() error {
	dirs := resolveSkillDirs()
	if cwd, err := os.Getwd(); err == nil {
		log.Printf("[Skill] Initializing from dirs %v (cwd=%s)", dirs, cwd)
	}
	return Initialize(dirs...)
}

// resolveSkillDirs 解析技能目录候选（COGNIDA_SKILL_DIRS 优先，否则 DefaultSkillDirs）。
// 加载侧（InitializeFromEnv）扫全部候选，写入侧（SkillDirFromEnv）取首个已存在者，二者共用此源。
func resolveSkillDirs() []string {
	var dirs []string
	if env := strings.TrimSpace(os.Getenv("COGNIDA_SKILL_DIRS")); env != "" {
		for _, d := range strings.Split(env, ",") {
			if d = strings.TrimSpace(d); d != "" {
				dirs = append(dirs, d)
			}
		}
	}
	if len(dirs) == 0 {
		dirs = DefaultSkillDirs()
	}
	return dirs
}

// SkillDirFromEnv 返回经验蒸馏落盘 SKILL.md 的目标目录：取候选中「首个已存在的目录」，
// 从而保证写入目录必落在加载扫描范围内（写 ⊆ 读），避免沉淀出的技能因目录不一致无法被复用。
// 候选全不存在时回退首个候选（由调用方负责 MkdirAll）。
func SkillDirFromEnv() string {
	dirs := resolveSkillDirs()
	for _, d := range dirs {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			return d
		}
	}
	if len(dirs) > 0 {
		return dirs[0]
	}
	return filepath.Join(".", "skills")
}

// DefaultSkillDirs 返回未显式配置 COGNIDA_SKILL_DIRS 时的候选技能目录（按优先级）。
// 服务通常从 cognida-go/ 启动，而技能文件位于仓库根 skills/（即 ../skills），故列多级候选，
// 修复「默认 ./skills 解析到 cognida-go/skills 而落空」的 CWD 陷阱。
//
// 这是候选目录的单一事实源：加载侧（InitializeFromEnv）扫描全部候选；写入侧
// （经验蒸馏 SkillDirFromEnv）取其中首个已存在者，从而保证「写入目录」必落在
// 「加载扫描范围」内——避免蒸馏出的 SKILL.md 因目录不一致而无法被加载复用。
func DefaultSkillDirs() []string {
	return []string{
		filepath.Join(".", "skills"),
		filepath.Join("..", "skills"),
		filepath.Join("..", "..", "skills"),
	}
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

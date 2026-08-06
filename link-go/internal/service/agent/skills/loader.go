// Package skills 提供 Skill 加载器
// 负责解析 SKILL.md 文件（YAML frontmatter + Markdown 正文）
package skills

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// ========================================
// SkillLoader Skill 加载器
// ========================================

// SkillLoader Skill 加载器接口
type SkillLoader interface {
	// LoadFile 从单个文件加载 Skill
	LoadFile(path string) (*Skill, error)

	// LoadDir 从目录加载所有 Skill
	LoadDir(dir string) ([]*Skill, []error)

	// LoadContent 从内容加载 Skill
	LoadContent(content string, source string) (*Skill, error)
}

// skillLoader Skill 加载器实现
type skillLoader struct {
	allowDeprecated bool
}

// NewSkillLoader 创建 Skill 加载器
func NewSkillLoader(opts ...LoaderOption) SkillLoader {
	l := &skillLoader{
		allowDeprecated: false,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// LoaderOption 加载器选项
type LoaderOption func(*skillLoader)

// WithAllowDeprecated 允许加载已废弃的 Skill
func WithAllowDeprecated(allow bool) LoaderOption {
	return func(l *skillLoader) {
		l.allowDeprecated = allow
	}
}

// LoadFile 从单个文件加载 Skill
func (l *skillLoader) LoadFile(path string) (*Skill, error) {
	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill file: %w", err)
	}

	skill, err := l.LoadContent(string(content), path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse skill file %s: %w", path, err)
	}

	// 检查是否已废弃
	if skill.Deprecated && !l.allowDeprecated {
		return nil, fmt.Errorf("skill %s is deprecated", skill.Name)
	}

	return skill, nil
}

// LoadContent 从内容加载 Skill
func (l *skillLoader) LoadContent(content string, source string) (*Skill, error) {
	// 解析 frontmatter 和正文
	frontmatter, body, err := parseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// 解析 YAML
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	// 验证必需字段
	if fm.Name == "" {
		return nil, fmt.Errorf("missing required field: name")
	}
	if fm.Description == "" {
		return nil, fmt.Errorf("missing required field: description")
	}

	// 构造 Skill
	skill := &Skill{
		Name:                   fm.Name,
		Description:            fm.Description,
		WhenToUse:              fm.WhenToUse,
		AllowedTools:           fm.AllowedTools,
		DisallowedTools:        fm.DisallowedTools,
		Model:                  fm.Model,
		ContextMode:            fm.ContextMode,
		Effort:                 fm.Effort,
		DisableModelInvocation: fm.DisableModelInvocation,
		Content:                strings.TrimSpace(body),
		Source:                 source,
		Version:                fm.Version,
		Author:                 fm.Author,
		Tags:                   fm.Tags,
		Category:               fm.Category,
		Deprecated:             fm.Deprecated,
		Experimental:           fm.Experimental,
		RawFrontmatter:        make(map[string]interface{}),
	}

	// 保留原始 frontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &skill.RawFrontmatter); err != nil {
		return nil, fmt.Errorf("failed to parse raw frontmatter: %w", err)
	}

	return skill, nil
}

// LoadDir 从目录加载所有 Skill
// 支持两种目录结构：
// 1. dir/SKILL.md - 直接在目录下
// 2. dir/subdir/SKILL.md - 在子目录下（推荐）
func (l *skillLoader) LoadDir(dir string) ([]*Skill, []error) {
	var skills []*Skill
	var errors []error

	// 遍历目录
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// 目录不存在，返回空结果
			return skills, nil
		}
		return nil, []error{fmt.Errorf("failed to read directory: %w", err)}
	}

	// 首先检查是否有直接的 SKILL.md 文件
	hasDirectSkill := false
	for _, entry := range entries {
		if entry.Name() == "SKILL.md" {
			hasDirectSkill = true
			break
		}
	}

	// 如果有直接的 SKILL.md，加载它
	if hasDirectSkill {
		path := filepath.Join(dir, "SKILL.md")
		skill, err := l.LoadFile(path)
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to load %s: %w", path, err))
		} else {
			l.scanSupportingFiles(dir, skill)
			skills = append(skills, skill)
		}
	}

	// 遍历子目录，加载每个子目录中的 SKILL.md
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		subDir := filepath.Join(dir, entry.Name())
		skillFile := filepath.Join(subDir, "SKILL.md")

		// 检查子目录中是否有 SKILL.md
		if _, err := os.Stat(skillFile); err == nil {
			skill, err := l.LoadFile(skillFile)
			if err != nil {
				errors = append(errors, fmt.Errorf("failed to load %s: %w", skillFile, err))
				continue
			}

			// 扫描支持文件
			l.scanSupportingFiles(subDir, skill)

			skills = append(skills, skill)
		}
	}

	return skills, errors
}

// scanSupportingFiles 扫描支持文件目录
func (l *skillLoader) scanSupportingFiles(skillDir string, skill *Skill) {
	// 检查 scripts/ 目录
	if scripts := l.scanDir(filepath.Join(skillDir, "scripts")); len(scripts) > 0 {
		skill.SupportingFiles.Scripts = scripts
	}

	// 检查 references/ 目录
	if refs := l.scanDir(filepath.Join(skillDir, "references")); len(refs) > 0 {
		skill.SupportingFiles.References = refs
	}

	// 检查 examples/ 目录
	if examples := l.scanDir(filepath.Join(skillDir, "examples")); len(examples) > 0 {
		skill.SupportingFiles.Examples = examples
	}

	// 检查 assets/ 目录
	if assets := l.scanDir(filepath.Join(skillDir, "assets")); len(assets) > 0 {
		skill.SupportingFiles.Assets = assets
	}
}

// scanDir 扫描目录中的文件
func (l *skillLoader) scanDir(dir string) []string {
	var files []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return files
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files
}

// ========================================
// Frontmatter 解析
// ========================================

// parseFrontmatter 解析 frontmatter 和正文
// 格式：
// ---
// YAML frontmatter
// ---
// Markdown body
func parseFrontmatter(content string) (frontmatter, body string, err error) {
	reader := bufio.NewReader(strings.NewReader(content))

	// 读取第一行
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("failed to read first line: %w", err)
	}

	line = strings.TrimSpace(line)

	// 检查是否以 --- 开始
	if line != "---" {
		return "", "", fmt.Errorf("skill file must start with ---")
	}

	// 读取 frontmatter
	var fmLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", "", fmt.Errorf("failed to read frontmatter: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")

		// 检查是否到达 frontmatter 结束
		if line == "---" {
			break
		}

		fmLines = append(fmLines, line)
	}

	frontmatter = strings.Join(fmLines, "\n")

	// 读取正文
	var bodyBuf bytes.Buffer
	for {
		chunk, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", "", fmt.Errorf("failed to read body: %w", err)
		}
		bodyBuf.WriteString(chunk)
		if err == io.EOF {
			break
		}
	}

	body = bodyBuf.String()

	return frontmatter, body, nil
}

// ========================================
// 辅助函数
// ========================================

// IsValidSkillName 检查 Skill 名称是否有效。
// 允许：任意语言的字母（含中日韩等 Unicode 字母）、数字、下划线、连字符、空格。
// 与经验落盘 slugify（保留 [a-z0-9\p{Han}]、其余折叠为连字符）保持一致——
// slugify 产出的名称必落在此集合内，避免「沉淀出来却注册不进去」的自相矛盾。
func IsValidSkillName(name string) bool {
	if name == "" {
		return false
	}
	for _, ch := range name {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' && ch != '-' && ch != ' ' {
			return false
		}
	}
	return true
}

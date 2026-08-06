package experience

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	domain_experience "link/internal/model/agent/experience"
	"link/internal/service/agent/skills"
)

// SkillSink 把「值得复用」的经验落盘为 SKILL.md，写入配置的技能根目录下
// 单层子目录 <root>/<slug>/SKILL.md（SkillLoader.LoadDir 只扫两层，故须一层），
// 落盘后触发 skills.ReloadSkills 让新技能即时可被发现。
//
// 幂等：同名 skill 目录已存在则跳过（不覆盖既有技能），返回既有名。
type SkillSink struct {
	root string // 技能根目录（写入目标）
}

// NewSkillSink 创建 skill 落盘器。root 为空时 Write 返回错误由调用方降级。
func NewSkillSink(root string) *SkillSink {
	return &SkillSink{root: strings.TrimSpace(root)}
}

// Write 依据经验生成 SKILL.md。返回生成（或既有）的 skill 名。
// 若经验不满足落盘条件（非 skill_worthy / 指引为空），返回空名且无错误。
func (s *SkillSink) Write(ctx context.Context, exp *domain_experience.Experience) (string, error) {
	if s == nil || s.root == "" {
		return "", fmt.Errorf("skill sink: 技能根目录未配置")
	}
	if exp == nil || !exp.SkillWorthy || strings.TrimSpace(exp.SkillInstructions) == "" {
		return "", nil // 不满足落盘条件：静默跳过
	}

	// 名称必须能通过注册器校验（IsValidSkillName），否则沉淀出的技能加载时会被拒。
	// slugify 保留 [a-z0-9\p{Han}]，正常与校验集合一致；这里再兜一层，
	// 万一标题折叠后为空或含非法字符，回退到稳定的 experience-<id>。
	name := slugify(exp.Title)
	if name == "" || !skills.IsValidSkillName(name) {
		name = fmt.Sprintf("experience-%d", exp.ID)
	}

	dir := filepath.Join(s.root, name)
	skillPath := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(skillPath); err == nil {
		// 同名技能已存在：不覆盖，视为幂等命中。
		return name, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("skill sink: 创建目录失败: %w", err)
	}

	content := renderSkillMarkdown(exp, name)
	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("skill sink: 写入 SKILL.md 失败: %w", err)
	}

	// 让新技能即时可被 skill_list/skill_match 发现。重载失败不致命（下次启动会加载）。
	if err := skills.ReloadSkills(); err != nil {
		// 不返回错误：文件已落盘，重载失败仅影响本进程即时可见性。
		_ = err
	}
	return name, nil
}

// renderSkillMarkdown 组装 SKILL.md：YAML frontmatter（name/description/when_to_use/
// tags/category/experimental）+ 正文（问题背景 + 可复用操作指引）。
func renderSkillMarkdown(exp *domain_experience.Experience, name string) string {
	desc := exp.Title
	if desc == "" {
		desc = firstNonEmpty(exp.Problem, "由会话经验自动沉淀的可复用技能")
	}

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", yamlScalar(name))
	fmt.Fprintf(&b, "description: %s\n", yamlScalar(desc))
	if exp.Problem != "" {
		fmt.Fprintf(&b, "when_to_use: %s\n", yamlScalar("遇到类似问题时："+oneLine(exp.Problem)))
	}
	b.WriteString("category: experience\n")
	b.WriteString("experimental: true\n")
	b.WriteString("author: experience-distill\n")
	if len(exp.Tags) > 0 {
		b.WriteString("tags:\n")
		for _, t := range exp.Tags {
			fmt.Fprintf(&b, "  - %s\n", yamlScalar(t))
		}
	}
	// disable_model_invocation：纯指导性技能（不自带工具白名单），仅供模型参考。
	b.WriteString("disable_model_invocation: true\n")
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s\n\n", firstNonEmpty(exp.Title, name))
	b.WriteString("> 本技能由会话经验自动沉淀生成，供遇到同类问题时复用。\n\n")

	if exp.Problem != "" {
		b.WriteString("## 适用场景\n\n")
		b.WriteString(exp.Problem)
		b.WriteString("\n\n")
	}
	b.WriteString("## 操作指引\n\n")
	b.WriteString(exp.SkillInstructions)
	b.WriteString("\n")

	if len(exp.Tools) > 0 {
		b.WriteString("\n## 涉及工具\n\n")
		for _, t := range exp.Tools {
			fmt.Fprintf(&b, "- `%s`\n", t)
		}
	}
	fmt.Fprintf(&b, "\n---\n_来源会话：%s，沉淀于 %s_\n", exp.SessionID, exp.CreatedAt.Format(time.RFC3339))
	return b.String()
}

var slugInvalid = regexp.MustCompile(`[^a-z0-9\p{Han}]+`)

// slugify 把标题转为文件系统友好的 slug：保留字母数字与汉字，其余折叠为连字符。
func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = slugInvalid.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = s[:60]
		s = strings.Trim(s, "-")
	}
	return s
}

// yamlScalar 把字符串包装为安全的 YAML 双引号标量（转义引号与反斜杠）。
func yamlScalar(s string) string {
	s = oneLine(s)
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}

// oneLine 把多行折叠为单行（换行替换为空格），供 frontmatter 单行标量使用。
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// firstNonEmpty 返回首个非空字符串。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

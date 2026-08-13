// 本文件是经验沉淀的「技能」环节：把被标记 skill_worthy 的会话经验落成一份
// 可复用的 SKILL.md，供技能系统在未来同类问题时加载复用。与图谱沉淀（graph_sink.go）
// 并列，同属一次蒸馏 pass 的下游产物；能力（渲染/落盘/近重复合并纯逻辑）在此，
// 业务阈值/门控与「写哪个目录、注册进哪个管理器」在 config.go 与组合根装配。
package experience

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	domain_experience "cognida/internal/model/agent/experience"
	"cognida/internal/service/agent/skills"
)

// skillNameMaxRunes 落盘技能名（目录名）最长 rune 数，避免超长标题撑出畸形目录名。
const skillNameMaxRunes = 60

// SkillRegistrar 把新落盘的 SKILL.md 即时注册进运行中的技能管理器（运行期可见），
// 由组合根注入；为 nil 时仅落盘，靠加载侧下次扫描/热更新兜底生效。
type SkillRegistrar func(name, path string) error

// SkillSink 把 skill_worthy 的经验渲染为 SKILL.md 并幂等落盘。
type SkillSink struct {
	dir       string                                 // 落盘根目录（须 ⊆ 加载扫描范围，见 skills.SkillDirFromEnv）
	repo      domain_experience.ExperienceRepository // 近重复合并判定；可空
	registrar SkillRegistrar                         // 运行期即时注册；可空
	// mu 串行化「查重→落盘」临界区：后台 Worker 以 goroutine 并发蒸馏多个会话
	// （scanOnce 的 go func + 信号量），两个同租户同标题会话会同时通过 FindSkillWorthyByTenant
	// 去重检查（此刻双方 skill_name 都还没落库）→ 都写同一 slug 目录（last-write-wins）。
	// 进程内加锁闭合该竞态窗口；跨实例仍需 DB 唯一约束，属多实例范畴另议。
	mu sync.Mutex
}

// NewSkillSink 创建技能落盘器。dir 为空时 Write 会报错（调用方须给出有效目录）。
func NewSkillSink(dir string, repo domain_experience.ExperienceRepository, registrar SkillRegistrar) *SkillSink {
	return &SkillSink{dir: strings.TrimSpace(dir), repo: repo, registrar: registrar}
}

// Write 把一条 skill_worthy 经验落成 SKILL.md，返回落盘的技能名（供回填 SkillName）。
// 非 skill_worthy、无指引正文、或名称非法时静默返回空名不报错（视为「本条不产技能」）。
func (s *SkillSink) Write(ctx context.Context, exp *domain_experience.Experience) (string, error) {
	if s == nil || exp == nil || !exp.SkillWorthy {
		return "", nil
	}
	if strings.TrimSpace(exp.SkillInstructions) == "" {
		return "", nil
	}
	name := slugify(exp.Title)
	// 与加载/注册侧 skills.IsValidSkillName 对齐：名称落不进注册表就别落盘，避免「沉淀却注册不进」。
	if name == "" || !skills.IsValidSkillName(name) {
		return "", nil
	}
	if s.dir == "" {
		return "", fmt.Errorf("skill sink: 落盘目录未配置")
	}

	// 查重→落盘为一个原子临界区，防止并发蒸馏的两个同标题会话都通过去重后重复落盘（见 mu 注释）。
	s.mu.Lock()
	defer s.mu.Unlock()

	// 近重复合并：同租户已有相同 slug 的技能（多为不同会话得出的同名解法）→ 复用既有技能名，
	// 本次不再另落一份内容雷同的技能，避免目录堆同义重复。
	if s.repo != nil {
		if existing, err := s.repo.FindSkillWorthyByTenant(ctx, exp.TenantID, 200); err == nil {
			for _, e := range existing {
				if e == nil || e.SessionID == exp.SessionID {
					continue
				}
				if e.SkillName != "" && slugify(e.Title) == name {
					return e.SkillName, nil
				}
			}
		}
	}

	dir := filepath.Join(s.dir, name)
	path := filepath.Join(dir, "SKILL.md")
	content := renderSkillMarkdown(name, exp)

	// 幂等落盘：内容未变则跳过写入，保持文件 mtime 稳定（加载侧据 mtime 判 TTL 过期）。
	if prev, err := os.ReadFile(path); err == nil && string(prev) == content {
		return name, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("skill sink: 建目录 %s 失败: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("skill sink: 落盘 %s 失败: %w", path, err)
	}

	// 运行期可见：即时注册进运行中的管理器（best-effort，失败靠加载侧下次扫描兜底）。
	if s.registrar != nil {
		_ = s.registrar(name, path)
	}
	return name, nil
}

// renderSkillMarkdown 渲染确定性的 SKILL.md 文本（YAML frontmatter + 指引正文）。
// 输出必须确定：同一 exp 恒产出同一字节串，才能支撑上面的幂等（内容未变不重写）判定。
func renderSkillMarkdown(name string, exp *domain_experience.Experience) string {
	desc := strings.TrimSpace(exp.Title)
	if desc == "" {
		desc = strings.TrimSpace(exp.Problem)
	}
	if desc == "" {
		desc = name
	}

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", yamlInline(name))
	fmt.Fprintf(&b, "description: %s\n", yamlInline(desc))
	if wt := strings.TrimSpace(exp.Problem); wt != "" {
		fmt.Fprintf(&b, "when_to_use: %s\n", yamlInline(wt))
	}
	fmt.Fprintf(&b, "author: %s\n", yamlInline(skills.DistilledSkillAuthor))
	b.WriteString("experimental: true\n")
	if len(exp.Tags) > 0 {
		b.WriteString("tags:\n")
		for _, t := range exp.Tags {
			if t = strings.TrimSpace(t); t != "" {
				fmt.Fprintf(&b, "  - %s\n", yamlInline(t))
			}
		}
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(exp.SkillInstructions))
	b.WriteString("\n")
	return b.String()
}

// yamlInline 把短字符串安全地表示为 YAML 单行双引号标量（折叠换行、转义引号/反斜杠），
// 避免标题/标签里的冒号、井号等破坏 frontmatter 解析。
func yamlInline(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + strings.TrimSpace(s) + `"`
}

// slugify 把标题折叠为技能名/目录名：保留小写字母、数字、汉字，其余（空格/标点/大写等）
// 折叠为单个连字符并去除首尾连字符。产出集合 [a-z0-9\p{Han}-] 必落在 skills.IsValidSkillName
// 允许范围内——二者是「沉淀得出即注册得进」的同一契约的两侧。
func slugify(s string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastHyphen = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || unicode.Is(unicode.Han, r):
			b.WriteRune(r)
			lastHyphen = false
		default:
			if b.Len() > 0 && !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")

	// 限长（按 rune），并再次去尾连字符，避免截断处留下悬空连字符。
	if runes := []rune(slug); len(runes) > skillNameMaxRunes {
		slug = strings.Trim(string(runes[:skillNameMaxRunes]), "-")
	}
	return slug
}

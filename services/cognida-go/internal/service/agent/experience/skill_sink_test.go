package experience

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain_experience "cognida/internal/model/agent/experience"
	"cognida/internal/service/agent/skills"
)

// ---- slugify ----

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"按城市统计月度营收":       "按城市统计月度营收",
		"Hello World!":    "hello-world",
		"  多余  空格 & 符号  ": "多余-空格-符号",
		"---前后连字符---":     "前后连字符",
		"RFM 分群 v2":       "rfm-分群-v2",
		"!!!":             "", // 全标点 → 空
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

// slugify 产出必须落在 skills.IsValidSkillName 允许范围内（沉淀得出即注册得进）。
func TestSlugify_SatisfiesValidSkillName(t *testing.T) {
	for _, in := range []string{"按城市统计月度营收", "Hello World", "RFM 分群 v2", "a-b_c"} {
		got := slugify(in)
		if got == "" {
			t.Errorf("slugify(%q) 意外为空", in)
			continue
		}
		if !skills.IsValidSkillName(got) {
			t.Errorf("slugify(%q)=%q 未通过 IsValidSkillName", in, got)
		}
	}
}

func TestSlugify_TruncatesLong(t *testing.T) {
	got := slugify(strings.Repeat("a", skillNameMaxRunes+50))
	if n := len([]rune(got)); n > skillNameMaxRunes {
		t.Errorf("slugify 未限长: %d > %d", n, skillNameMaxRunes)
	}
}

// ---- renderSkillMarkdown 往返可被加载器解析 ----

func TestRenderSkillMarkdown_RoundTrip(t *testing.T) {
	exp := &domain_experience.Experience{
		Title:             "按城市统计月度营收",
		Problem:           "用户想看上月各城市营收: 需按 city 聚合",
		Tags:              []string{"电商分析", "SQL"},
		SkillInstructions: "1. 先 get_schema 确认 city 字段\n2. 用 sql_execute 按 city 聚合\n3. render_ui 出图",
	}
	content := renderSkillMarkdown("按城市统计月度营收", exp)

	skill, err := skills.NewSkillLoader().LoadContent(content, "test")
	if err != nil {
		t.Fatalf("渲染出的 SKILL.md 无法被加载器解析: %v\n---\n%s", err, content)
	}
	if skill.Name != "按城市统计月度营收" {
		t.Errorf("name = %q", skill.Name)
	}
	if skill.Author != distilledSkillAuthor {
		t.Errorf("author = %q, want %q（须与加载侧 TTL 识别一致）", skill.Author, distilledSkillAuthor)
	}
	if !skill.Experimental {
		t.Error("沉淀技能应标记 experimental=true")
	}
	if !strings.Contains(skill.Content, "get_schema") {
		t.Errorf("正文丢失指引: %q", skill.Content)
	}
	if len(skill.Tags) != 2 {
		t.Errorf("tags = %v", skill.Tags)
	}
}

// ---- Write 行为 ----

// fakeExpRepo 仅用于 Write 测试；只实现近重复查询，其余为占位。
type fakeExpRepo struct {
	skillWorthy []*domain_experience.Experience
}

func (f *fakeExpRepo) FindIdleUnsummarizedSessions(context.Context, time.Time, time.Time, int, int) ([]*domain_experience.IdleSession, error) {
	return nil, nil
}
func (f *fakeExpRepo) Save(context.Context, *domain_experience.Experience) error { return nil }
func (f *fakeExpRepo) ExistsBySession(context.Context, string) (bool, error)     { return false, nil }
func (f *fakeExpRepo) FindSkillWorthyByTenant(context.Context, int64, int) ([]*domain_experience.Experience, error) {
	return f.skillWorthy, nil
}

func worthyExp() *domain_experience.Experience {
	return &domain_experience.Experience{
		SessionID:         "sess-1",
		TenantID:          1,
		Title:             "按城市统计月度营收",
		Problem:           "按 city 聚合上月营收",
		Tags:              []string{"SQL"},
		SkillWorthy:       true,
		SkillInstructions: "1. get_schema\n2. sql_execute 聚合\n3. render_ui",
	}
}

func TestSkillSink_WritesFile(t *testing.T) {
	dir := t.TempDir()
	sink := NewSkillSink(dir, nil, nil)
	name, err := sink.Write(context.Background(), worthyExp())
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if name != "按城市统计月度营收" {
		t.Fatalf("name = %q", name)
	}
	path := filepath.Join(dir, name, "SKILL.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("SKILL.md 未落盘: %v", err)
	}
}

func TestSkillSink_NotWorthyNoop(t *testing.T) {
	dir := t.TempDir()
	exp := worthyExp()
	exp.SkillWorthy = false
	name, err := NewSkillSink(dir, nil, nil).Write(context.Background(), exp)
	if err != nil || name != "" {
		t.Fatalf("非 skill_worthy 应静默跳过, got name=%q err=%v", name, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("不应落盘任何文件, got %d 项", len(entries))
	}
}

func TestSkillSink_EmptyInstructionsNoop(t *testing.T) {
	dir := t.TempDir()
	exp := worthyExp()
	exp.SkillInstructions = "   "
	name, err := NewSkillSink(dir, nil, nil).Write(context.Background(), exp)
	if err != nil || name != "" {
		t.Fatalf("空指引应跳过, got name=%q err=%v", name, err)
	}
}

// 幂等落盘：内容未变时不重写文件，mtime 保持稳定（加载侧据 mtime 判 TTL）。
func TestSkillSink_IdempotentPreservesMtime(t *testing.T) {
	dir := t.TempDir()
	sink := NewSkillSink(dir, nil, nil)
	name, err := sink.Write(context.Background(), worthyExp())
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	path := filepath.Join(dir, name, "SKILL.md")

	// 把 mtime 拨到过去，再次以相同内容 Write —— 应跳过重写，mtime 不被刷新。
	past := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if _, err := sink.Write(context.Background(), worthyExp()); err != nil {
		t.Fatalf("second write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.ModTime().Truncate(time.Second).Equal(past) {
		t.Errorf("内容未变却刷新了 mtime: got %v want %v", info.ModTime(), past)
	}
}

// 近重复合并：同租户已有相同 slug 的技能 → 复用既有名、不落新文件。
func TestSkillSink_NearDupReusesName(t *testing.T) {
	dir := t.TempDir()
	repo := &fakeExpRepo{skillWorthy: []*domain_experience.Experience{
		{SessionID: "other", TenantID: 1, Title: "按城市统计月度营收", SkillName: "按城市统计月度营收"},
	}}
	sink := NewSkillSink(dir, repo, nil)

	exp := worthyExp() // 同标题、不同 SessionID
	name, err := sink.Write(context.Background(), exp)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if name != "按城市统计月度营收" {
		t.Errorf("应复用既有技能名, got %q", name)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("近重复不应落新文件, got %d 项", len(entries))
	}
}

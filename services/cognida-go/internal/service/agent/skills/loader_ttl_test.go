package skills

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSkillFile 在 <dir>/<name>/SKILL.md 写一个技能，并把文件 mtime 设为 modTime。
func writeSkillFile(t *testing.T, root, name, author string, modTime time.Time) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "SKILL.md")
	content := "---\nname: " + name + "\ndescription: d\nauthor: " + author + "\nexperimental: true\ncategory: experience\n---\n\n# body\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	return path
}

// withFixedNow 固定 nowFunc 供 TTL 判定，返回还原函数。
func withFixedNow(t *testing.T, now time.Time) func() {
	t.Helper()
	prev := nowFunc
	nowFunc = func() time.Time { return now }
	return func() { nowFunc = prev }
}

// TestLoadFile_ExpiredDistilledSkipped 超过 TTL 的自动沉淀技能加载时返回 ErrSkillExpired。
func TestLoadFile_ExpiredDistilledSkipped(t *testing.T) {
	now := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	defer withFixedNow(t, now)()

	root := t.TempDir()
	old := now.Add(-distilledSkillTTL - time.Hour)
	path := writeSkillFile(t, root, "exp-old", distilledSkillAuthor, old)

	loader := NewSkillLoader()
	_, err := loader.LoadFile(path)
	if err != ErrSkillExpired {
		t.Fatalf("过期沉淀技能应返回 ErrSkillExpired, got %v", err)
	}
}

// TestLoadFile_FreshDistilledLoaded TTL 内的自动沉淀技能正常加载。
func TestLoadFile_FreshDistilledLoaded(t *testing.T) {
	now := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	defer withFixedNow(t, now)()

	root := t.TempDir()
	fresh := now.Add(-distilledSkillTTL + time.Hour) // 差一小时到期，仍在存活期内
	path := writeSkillFile(t, root, "exp-fresh", distilledSkillAuthor, fresh)

	loader := NewSkillLoader()
	skill, err := loader.LoadFile(path)
	if err != nil {
		t.Fatalf("存活期内技能应正常加载, got %v", err)
	}
	if skill.Name != "exp-fresh" {
		t.Fatalf("技能名异常: %q", skill.Name)
	}
}

// TestLoadFile_CuratedNeverExpires 非 experience-distill 的技能永不因时间过期。
func TestLoadFile_CuratedNeverExpires(t *testing.T) {
	now := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	defer withFixedNow(t, now)()

	root := t.TempDir()
	ancient := now.Add(-distilledSkillTTL * 10)
	path := writeSkillFile(t, root, "curated-old", "human", ancient)

	loader := NewSkillLoader()
	if _, err := loader.LoadFile(path); err != nil {
		t.Fatalf("策展技能不应因时间过期, got %v", err)
	}
}

// TestLoadDir_SkipsExpiredSilently LoadDir 跳过过期沉淀技能且不计入错误，新鲜技能照常加载。
func TestLoadDir_SkipsExpiredSilently(t *testing.T) {
	now := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	defer withFixedNow(t, now)()

	root := t.TempDir()
	writeSkillFile(t, root, "exp-old", distilledSkillAuthor, now.Add(-distilledSkillTTL-time.Hour))
	writeSkillFile(t, root, "exp-fresh", distilledSkillAuthor, now.Add(-time.Hour))

	loader := NewSkillLoader()
	loaded, errs := loader.LoadDir(root)
	if len(errs) != 0 {
		t.Fatalf("过期技能应静默跳过、不产生错误, got %v", errs)
	}
	if len(loaded) != 1 || loaded[0].Name != "exp-fresh" {
		names := make([]string, len(loaded))
		for i, s := range loaded {
			names[i] = s.Name
		}
		t.Fatalf("应只加载新鲜技能 exp-fresh, got %v", names)
	}
}

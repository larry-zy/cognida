package skills

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSkill(t *testing.T, dir, name, desc string) {
	t.Helper()
	sub := filepath.Join(dir, name)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func TestWatchReloadsOnChange(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "alpha", "first skill")

	m := NewSkillManager(WithWatchInterval(20 * time.Millisecond)).(*skillManager)
	if _, err := m.LoadSkills(dir); err != nil {
		t.Fatalf("initial load: %v", err)
	}
	if _, ok := m.GetSkill("alpha"); !ok {
		t.Fatalf("alpha should be loaded")
	}

	if err := m.Watch(dir); err != nil {
		t.Fatalf("watch: %v", err)
	}
	defer m.StopWatch(dir)

	// 新增一个 skill 文件，watch 应在轮询后自动加载
	writeSkill(t, dir, "beta", "second skill")

	deadline := time.After(2 * time.Second)
	for {
		if _, ok := m.GetSkill("beta"); ok {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("beta was not auto-loaded after change")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestWatchDuplicateRejected(t *testing.T) {
	dir := t.TempDir()
	m := NewSkillManager(WithWatchInterval(50 * time.Millisecond))

	if err := m.Watch(dir); err != nil {
		t.Fatalf("first watch: %v", err)
	}
	defer m.StopWatch(dir)

	if err := m.Watch(dir); err == nil {
		t.Errorf("expected error watching same dir twice")
	}
}

func TestStopWatchUnknownDir(t *testing.T) {
	m := NewSkillManager()
	if err := m.StopWatch(t.TempDir()); err == nil {
		t.Errorf("expected error stopping unwatched dir")
	}
}

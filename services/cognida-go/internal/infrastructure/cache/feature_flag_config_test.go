package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadConfigFromFile_ParsesGlobalAndAgents(t *testing.T) {
	const yamlContent = `
enabled: true
threshold: 0.85
ttl: 24h
top_k: 5
agents:
  rag_agent:
    enabled: true
    threshold: 0.90
    ttl: 12h
    top_k: 3
  quiet_agent:
    enabled: false
`
	path := writeTempConfig(t, yamlContent)

	strategy, err := loadConfigFromFile(path)
	if err != nil {
		t.Fatalf("loadConfigFromFile: %v", err)
	}

	if !strategy.Global.Enabled {
		t.Errorf("expected global enabled")
	}
	if strategy.Global.Threshold != 0.85 {
		t.Errorf("global threshold = %v, want 0.85", strategy.Global.Threshold)
	}
	if strategy.Global.TTL != 24*time.Hour {
		t.Errorf("global ttl = %v, want 24h", strategy.Global.TTL)
	}
	if strategy.Global.TopK != 5 {
		t.Errorf("global top_k = %d, want 5", strategy.Global.TopK)
	}

	rag, ok := strategy.Agents["rag_agent"]
	if !ok {
		t.Fatalf("rag_agent missing")
	}
	if !rag.Enabled || rag.Threshold != 0.90 || rag.TTL != 12*time.Hour || rag.TopK != 3 {
		t.Errorf("rag_agent = %+v, unexpected", rag)
	}

	// quiet_agent 未设置的字段应回落到 defaults
	quiet, ok := strategy.Agents["quiet_agent"]
	if !ok {
		t.Fatalf("quiet_agent missing")
	}
	if quiet.Enabled {
		t.Errorf("quiet_agent should be disabled")
	}
	if quiet.Threshold != 0.85 || quiet.TTL != 24*time.Hour || quiet.TopK != 5 {
		t.Errorf("quiet_agent should inherit defaults, got %+v", quiet)
	}
}

func TestLoadConfigFromFile_AppliesToFeatureFlag(t *testing.T) {
	const yamlContent = `
enabled: true
threshold: 0.8
ttl: 1h
top_k: 4
agents:
  rag_agent:
    enabled: true
`
	path := writeTempConfig(t, yamlContent)

	ff := NewFeatureFlag()
	if err := LoadConfigFromFile(ff, path); err != nil {
		t.Fatalf("LoadConfigFromFile: %v", err)
	}

	if !ff.GetGlobal() {
		t.Errorf("expected global enabled after load")
	}
	if !ff.IsEnabled("rag_agent") {
		t.Errorf("expected rag_agent enabled")
	}
	if _, ok := ff.GetAgentConfig("rag_agent"); !ok {
		t.Errorf("expected rag_agent config present")
	}
}

func TestLoadConfigFromFile_InvalidTTL(t *testing.T) {
	path := writeTempConfig(t, "enabled: true\nttl: not-a-duration\n")
	if _, err := loadConfigFromFile(path); err == nil {
		t.Errorf("expected error for invalid ttl")
	}
}

func TestLoadConfigFromFile_MissingFile(t *testing.T) {
	if _, err := loadConfigFromFile(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Errorf("expected error for missing file")
	}
}

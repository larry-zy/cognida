package prompt

import "testing"

// TestLoadExtractionTemplate_AllUsedNames 锁定 llm_extractor 用到的全部抽取模板名都能从
// 内嵌 extraction_templates/ 成功加载且非空，避免「改了目录/文件名却没人发现」的静默漂移。
func TestLoadExtractionTemplate_AllUsedNames(t *testing.T) {
	names := []string{
		"entity_extraction",
		"entity_extraction_query",
		"relation_extraction",
		"relation_extraction_query",
		"graph_extraction",
		"graph_incremental",
	}
	for _, name := range names {
		content, err := LoadExtractionTemplate(name)
		if err != nil {
			t.Errorf("LoadExtractionTemplate(%q) 出错: %v", name, err)
			continue
		}
		if content == "" {
			t.Errorf("LoadExtractionTemplate(%q) 返回空内容", name)
		}
	}
}

// TestLoadExtractionTemplate_Missing 缺失模板应返回错误而非 panic。
func TestLoadExtractionTemplate_Missing(t *testing.T) {
	if _, err := LoadExtractionTemplate("no_such_template"); err == nil {
		t.Fatal("加载不存在的模板应返回错误，实际为 nil")
	}
}

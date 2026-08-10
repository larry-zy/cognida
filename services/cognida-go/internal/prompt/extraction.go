package prompt

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

// extractionFS 内嵌知识图谱抽取/查询改写用的提示词模板。
//
// 这类模板的 YAML 结构是「templates 列表（含 id/content 等字段）」，与 registry.go
// 的扁平「key: 正文」格式不同，故独立目录、独立解析，不与 templates/*.yaml 混用。
//
//go:embed extraction_templates/*.yaml
var extractionFS embed.FS

// extractionTemplateFile 对应 extraction_templates/<name>.yaml 的结构，只取首条 content。
type extractionTemplateFile struct {
	Templates []struct {
		ID      string `yaml:"id"`
		Content string `yaml:"content"`
	} `yaml:"templates"`
}

// LoadExtractionTemplate 按名称加载抽取模板并返回其首条 content。
//
// 模板经 go:embed 编译期内嵌，零运行时路径依赖（取代了原 config.LoadPromptTemplate
// 那套按 cwd 猜多个相对路径读文件的脆弱实现）。name 不含扩展名，如 "graph_extraction"。
func LoadExtractionTemplate(name string) (string, error) {
	data, err := extractionFS.ReadFile("extraction_templates/" + name + ".yaml")
	if err != nil {
		return "", fmt.Errorf("prompt: 读取抽取模板 %q 失败: %w", name, err)
	}
	var f extractionTemplateFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return "", fmt.Errorf("prompt: 解析抽取模板 %q 失败: %w", name, err)
	}
	if len(f.Templates) == 0 {
		return "", fmt.Errorf("prompt: 抽取模板 %q 内无 templates 条目", name)
	}
	return f.Templates[0].Content, nil
}

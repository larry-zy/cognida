package genui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	prompts "cognida/internal/prompt"
)

// LLM 是本包所需的最小生成接口；eino 的 model.BaseChatModel / ToolCallingChatModel 均满足，
// 单测也可用轻量 fake 实现，无需实现 Stream。
type LLM interface {
	Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

// llmSystemPrompt 正文集中于 internal/prompt/templates/genui.yaml。
var llmSystemPrompt = prompts.MustGet("genui", "system")

// LLMCompose 是 Level 2：让 LLM 依据可用数据路径设计组件布局，Go 端组装 DataModel 并校验。
// 任一环节失败（生成、解析、校验）都返回 error，由调用方回退到 TemplateCompose。
func LLMCompose(ctx context.Context, llm LLM, dm *DataModel, question string) (*UISpec, error) {
	root, err := toGeneric(dm)
	if err != nil {
		return nil, fmt.Errorf("datamodel serialize: %w", err)
	}
	userPrompt := buildUserPrompt(dm, root, question)

	resp, err := llm.Generate(ctx, []*schema.Message{
		schema.SystemMessage(llmSystemPrompt),
		schema.UserMessage(userPrompt),
	})
	if err != nil {
		return nil, fmt.Errorf("llm generate: %w", err)
	}

	var parsed struct {
		Components []Component `json:"components"`
	}
	if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &parsed); err != nil {
		return nil, fmt.Errorf("llm output not valid JSON: %w", err)
	}

	spec := &UISpec{
		Surface:    "text2sql",
		Title:      titleFor(dm, question),
		Catalog:    Catalog,
		GenMode:    GenModeLLM,
		Components: parsed.Components,
		DataModel:  dm,
	}
	if err := Validate(spec); err != nil {
		return nil, fmt.Errorf("llm spec invalid: %w", err)
	}
	return spec, nil
}

// buildUserPrompt 组装用户提示：问题 + 可用数据路径清单（从真实 DataModel 枚举）。
func buildUserPrompt(dm *DataModel, root interface{}, question string) string {
	var b strings.Builder
	if question != "" {
		fmt.Fprintf(&b, "用户问题：%s\n\n", question)
	}
	if at, ok := dm.Meta["analysis_type"].(string); ok && at != "" {
		fmt.Fprintf(&b, "分析类型：%s\n\n", at)
	}
	b.WriteString("可用数据路径（只能引用这些 {path}）：\n")
	paths := enumeratePaths(root)
	for _, p := range paths {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	b.WriteString("\ncatalog: " + strings.Join(Catalog, ", ") + "\n")
	b.WriteString("\n请输出 {\"components\": [...]}，务必包含 id=\"root\" 容器，Table 绑定 /table，")
	b.WriteString("时序用 LineChart、分类对比用 BarChart（均绑定 /series），有 /scatter 时用 ScatterChart 看相关性，")
	b.WriteString("分类占比用 pie_chart、有序阶段转化用 funnel（均绑定 /series），多卡片可用 grid 栅格布局，")
	b.WriteString("关键指标用 MetricCard 绑定对应 /metrics/* 路径。")
	b.WriteString("注意：MetricCard 的 value 只能绑定 /metrics/* 标量路径，切勿绑定 /table、/series 等容器路径")
	b.WriteString("（否则卡片无值）；若上面没有任何 /metrics/* 路径，则不要使用 MetricCard。")
	return b.String()
}

// enumeratePaths 枚举 DataModel 中「可作为绑定目标」的 JSON Pointer：
// 容器级路径（/table、/series）+ 每个标量指标（/metrics/<key>）。
func enumeratePaths(root interface{}) []string {
	m, ok := root.(map[string]interface{})
	if !ok {
		return nil
	}
	var paths []string
	if _, ok := m["table"]; ok {
		paths = append(paths, "/table")
	}
	if _, ok := m["series"]; ok {
		paths = append(paths, "/series")
	}
	if _, ok := m["scatter"]; ok {
		paths = append(paths, "/scatter")
	}
	if metrics, ok := m["metrics"].(map[string]interface{}); ok {
		keys := make([]string, 0, len(metrics))
		for k := range metrics {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			paths = append(paths, "/metrics/"+jsonPtrEscape(k))
		}
	}
	return paths
}

// extractJSON 从可能带围栏/前后缀的文本中截出首个平衡的 JSON 对象。
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	if start < 0 {
		return s
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s[start:]
}

package genui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// LLM 是本包所需的最小生成接口；eino 的 model.BaseChatModel / ToolCallingChatModel 均满足，
// 单测也可用轻量 fake 实现，无需实现 Stream。
type LLM interface {
	Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

const llmSystemPrompt = `你是「生成式 UI」布局设计器。你的唯一职责是为一份数据分析结果设计组件布局，
输出一个 JSON 对象 {"components": [...]}（组件邻接表）。

【铁律】
1. 你绝不输出任何具体数字或数据值。所有数据一律用绑定引用真实数据模型：{"path": "/..."}（JSON Pointer）。
2. 只能使用 catalog 中的组件类型，且必须有且仅有一个 id 为 "root" 的容器节点。
3. 每个组件形如：{"id": "唯一id", "type": "类型", "props": {...}, "children": ["子id", ...]}。
   - 容器（Column/Row）用 children 组织子节点；叶子组件用 props。
4. {path} 只能引用「可用数据路径」清单中列出的路径，不得臆造路径。

【catalog 组件】
- Column / Row：布局容器（纵向 / 横向），用 children。
- Text：文本，props.text 可为字符串或 {path}，props.variant ∈ title|subtitle|body|caption。
- MetricCard：指标卡，props.label（字符串标题）、props.value（{path} 绑定）、props.unit（可选）。
- Callout：结论条，props.title、props.text、props.tone ∈ info|success|warning。
- Table：数据表，props.data 必须绑定 {path} 指向 {columns, rows}。
- LineChart：折线图，props.title、props.series 必须绑定 {path} 指向 {labels, actual, forecast}。

只输出 JSON，不要解释，不要 markdown 代码围栏。`

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
	b.WriteString("有序列时用 LineChart 绑定 /series，关键指标用 MetricCard 绑定对应 /metrics/* 路径。")
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

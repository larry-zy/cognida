package chat

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// DeepSeek 原生工具调用（DSML）解析
// ============================================================
//
// 部分 DeepSeek 模型（如 V3.1/V3.2 thinking）在 OpenAI 兼容端点上并不把工具调用放进
// 结构化的 `delta.tool_calls`，而是以自有的 DSML 标记内联在 `content` 文本里，形如：
//
//	<｜｜DSML｜｜tool_calls>
//	<｜｜DSML｜｜invoke name="data_analysis">
//	<｜｜DSML｜｜parameter name="analysis_type" string="true">report</｜｜DSML｜｜parameter>
//	<｜｜DSML｜｜parameter name="insights" string="false">["G","A"]</｜｜DSML｜｜parameter>
//	</｜｜DSML｜｜invoke>
//	</｜｜DSML｜｜tool_calls>
//
// 其中 `｜` 是全角竖线 U+FF5C。适配层若只读结构化 tool_calls，这段就会当作正文原样下发，
// 工具永不派发，ReAct 循环空转直至超时（线上曾出现 duration_ms≈71.5s 的卡死）。
//
// 作为中台，LLM 适配层必须把这种原生格式解析回结构化 schema.ToolCall，并从用户可见正文里
// 剥掉标记文本。以下解析器同时服务非流式（整段 content）与流式（跨分片累积）两条路径。
//
// `string="true"` 表示参数值按字面字符串处理；`string="false"` 表示值本身是 JSON
// （数值/布尔/数组/对象）。缺省时按能否 JSON 解析自动判定。

// 标记正则：容忍 1+ 个竖线（全角 U+FF5C 或 ASCII `|`），兼容不同版本的分隔符变体。
// (?s) 让 `.` 匹配换行，因为参数值/整块可能跨行。
var (
	dsmlToolCallsOpenRe = regexp.MustCompile(`<[｜|]+DSML[｜|]+tool_calls>`)
	dsmlInvokeRe        = regexp.MustCompile(`(?s)<[｜|]+DSML[｜|]+invoke\s+name="([^"]+)"\s*>(.*?)</[｜|]+DSML[｜|]+invoke>`)
	dsmlParamRe         = regexp.MustCompile(`(?s)<[｜|]+DSML[｜|]+parameter\s+name="([^"]+)"(?:\s+string="([^"]*)")?\s*>(.*?)</[｜|]+DSML[｜|]+parameter>`)
	// dsmlBlockRe 用于从正文里整体剥除 <tool_calls>…</tool_calls> 区间。
	dsmlBlockRe = regexp.MustCompile(`(?s)<[｜|]+DSML[｜|]+tool_calls>.*?</[｜|]+DSML[｜|]+tool_calls>`)
)

// 流式安全前缀判定用的字面标记（线上实际输出为双全角竖线）。
const (
	dsmlOpenMarker  = "<｜｜DSML｜｜tool_calls>"
	dsmlCloseMarker = "</｜｜DSML｜｜tool_calls>"
)

// containsDSMLToolCalls 判断一段文本是否含有 DSML 工具调用起始标记。
func containsDSMLToolCalls(s string) bool {
	return dsmlToolCallsOpenRe.MatchString(s)
}

// parseDSMLToolCalls 从文本中解析 DSML 原生工具调用。
//   - cleaned：剥除 DSML 区间后的用户可见正文（trim 收尾空白）。
//   - calls：解析出的结构化工具调用；原生格式无 ID，这里补发确定性合成 ID。
//   - found：是否至少解析出一个工具调用。
//
// 解析以 invoke 块为单位，不强依赖外层 tool_calls 闭合标签，从而对被截断的流也尽量兜住。
func parseDSMLToolCalls(content string) (cleaned string, calls []schema.ToolCall, found bool) {
	invokes := dsmlInvokeRe.FindAllStringSubmatch(content, -1)
	if len(invokes) == 0 {
		return content, nil, false
	}

	calls = make([]schema.ToolCall, 0, len(invokes))
	for i, inv := range invokes {
		name := inv[1]
		body := inv[2]
		args := parseDSMLParams(body)
		argsJSON, err := json.Marshal(args)
		if err != nil {
			// 理论上 map[string]interface{} 恒可序列化；防御性跳过。
			continue
		}
		calls = append(calls, schema.ToolCall{
			ID:   fmt.Sprintf("call_dsml_%d", i),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      name,
				Arguments: string(argsJSON),
			},
		})
	}
	if len(calls) == 0 {
		return content, nil, false
	}

	cleaned = strings.TrimSpace(stripDSMLBlocks(content))
	return cleaned, calls, true
}

// stripDSMLBlocks 从正文里剥除 DSML 工具调用标记文本。
// 优先整体剥除完整的 <tool_calls>…</tool_calls> 区间；若模型未闭合外层标签，
// 则退化为剥除各 invoke 块。用于「含标记但未解析出有效 invoke」时也不把原始标记泄漏给用户。
func stripDSMLBlocks(content string) string {
	cleaned := dsmlBlockRe.ReplaceAllString(content, "")
	if cleaned == content {
		cleaned = dsmlInvokeRe.ReplaceAllString(content, "")
	}
	return cleaned
}

// parseDSMLParams 解析单个 invoke 体内的全部 <parameter> 为参数 map。
func parseDSMLParams(body string) map[string]interface{} {
	params := dsmlParamRe.FindAllStringSubmatch(body, -1)
	out := make(map[string]interface{}, len(params))
	for _, p := range params {
		name := p[1]
		stringAttr := p[2] // "true" | "false" | ""
		raw := p[3]

		switch stringAttr {
		case "true":
			out[name] = raw
		case "false":
			out[name] = decodeJSONValue(raw)
		default:
			// 未标注 string 属性：尝试按 JSON 解析，失败则当字面字符串。
			out[name] = decodeJSONValue(raw)
		}
	}
	return out
}

// decodeJSONValue 尝试把原始文本解析为 JSON 值；失败则原样作为字符串返回。
func decodeJSONValue(raw string) interface{} {
	trimmed := strings.TrimSpace(raw)
	var v interface{}
	if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
		return v
	}
	return raw
}

// ============================================================
// 流式过滤器：跨 content 分片检测并抽取 DSML 工具调用块
// ============================================================
//
// 流式下 content 逐片到达，DSML 标记可能被切在任意分片边界上。dsmlFilter 维护一个
// 待发缓冲：未进入捕获态时，只下发「确定不可能是起始标记前缀」的部分，其余暂留；一旦缓冲
// 里出现完整起始标记，则进入捕获态、把标记及其后内容改存入捕获缓冲，直到收到闭合标记再整体
// 解析为工具调用。这样既不泄露标记文本，也不误吞正常正文。

type dsmlFilter struct {
	capturing bool
	pending   strings.Builder // 非捕获态：尚不能安全下发的正文尾巴
	capture   strings.Builder // 捕获态：已累积的 DSML 块字节
	calls     []schema.ToolCall
}

// feed 处理一个 content 分片，返回此刻可安全下发给用户的正文。
func (f *dsmlFilter) feed(chunk string) string {
	if chunk == "" {
		return ""
	}
	if f.capturing {
		f.capture.WriteString(chunk)
		return f.tryClose()
	}

	f.pending.WriteString(chunk)
	s := f.pending.String()

	if idx := strings.Index(s, dsmlOpenMarker); idx >= 0 {
		emit := s[:idx]
		f.pending.Reset()
		f.capturing = true
		f.capture.WriteString(s[idx:])
		return emit + f.tryClose()
	}

	// 暂留可能是起始标记前缀的尾部，其余下发。
	hold := holdbackLen(s, dsmlOpenMarker)
	emit := s[:len(s)-hold]
	f.pending.Reset()
	f.pending.WriteString(s[len(s)-hold:])
	return emit
}

// tryClose 在捕获态下检查是否已收到闭合标记；是则解析该块并退出捕获态，
// 闭合标记之后的残余文本重新走 feed（可能还有下一块或正常正文）。
func (f *dsmlFilter) tryClose() string {
	s := f.capture.String()
	idx := strings.Index(s, dsmlCloseMarker)
	if idx < 0 {
		return ""
	}
	end := idx + len(dsmlCloseMarker)
	block := s[:end]
	trailing := s[end:]

	if _, calls, ok := parseDSMLToolCalls(block); ok {
		f.calls = append(f.calls, calls...)
	}
	f.capturing = false
	f.capture.Reset()

	if trailing == "" {
		return ""
	}
	return f.feed(trailing)
}

// finish 在流结束时收尾：返回残余待发正文与已解析出的全部工具调用。
// 若结束时仍处于捕获态（外层标签被截断），尽力对已捕获内容做一次宽松解析。
func (f *dsmlFilter) finish() (emit string, calls []schema.ToolCall) {
	if f.capturing {
		if _, c, ok := parseDSMLToolCalls(f.capture.String()); ok {
			f.calls = append(f.calls, c...)
		}
		f.capture.Reset()
		f.capturing = false
		return "", f.calls
	}
	emit = f.pending.String()
	f.pending.Reset()
	return emit, f.calls
}

// holdbackLen 返回 s 尾部应暂留的字节数：其为 marker 的某个前缀，可能是跨分片到达的标记开头。
// 采用字节级比较，正确处理被切在多字节字符（全角竖线）中间的情形。
func holdbackLen(s, marker string) int {
	n := len(marker) - 1
	if n > len(s) {
		n = len(s)
	}
	for ; n > 0; n-- {
		if strings.HasSuffix(s, marker[:n]) {
			return n
		}
	}
	return 0
}

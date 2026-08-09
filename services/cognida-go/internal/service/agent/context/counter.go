package context

// ApproxTokenCounter 是无外部依赖的近似 token 计数器：中文按 0.7 token/字符、
// 其余按 0.25 token/字符估算，与基础设施层 llm.tokenCounter 的启发式保持一致。
//
// 它是本上下文工程能力包的自带默认计数器，使窗口/预算治理自足、不反向依赖基础设施层
// （能力与业务分离）。需要更精确计数的调用方可注入自己的 TokenCounter 实现。
type ApproxTokenCounter struct{}

// Count 估算文本 token 数（非空文本至少返回 1）。
func (ApproxTokenCounter) Count(text string) int {
	if text == "" {
		return 0
	}
	chinese, other := 0, 0
	for _, r := range text {
		if r >= 0x4e00 && r <= 0x9fff {
			chinese++
		} else {
			other++
		}
	}
	tokens := int(float64(chinese)*0.7) + int(float64(other)*0.25)
	if tokens == 0 {
		return 1
	}
	return tokens
}

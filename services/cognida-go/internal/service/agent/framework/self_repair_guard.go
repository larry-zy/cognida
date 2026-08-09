package framework

import (
	"fmt"
	"sort"
	"strings"
)

// 自我修复护栏（strategic 层）：把工具层已分级的失败按「工具名:error_kind」聚合计数，
// 在同一失败签名反复出现时从「盲目重试」升级为「触发再规划」，累计失败过多则提前收尾。
// 与 token 预算/maxIter 正交：那两者约束总量，本护栏约束「原地打转」。
const (
	// TerminatedByRepairExhausted 表示因反复失败（自我修复未收敛）而提前收尾。
	TerminatedByRepairExhausted = "repair_exhausted"

	// replanThreshold：同一失败签名累计达到此次数 → 注入一次再规划提示（引导换路径而非重试）。
	replanThreshold = 2
	// windDownThreshold：本轮累计失败达到此数 → 判定原地打转，提前进入 wind-down 收尾。
	windDownThreshold = 4
)

// failureGuard 是「每次运行」私有的失败签名计数器——绝不挂在共享 agentImpl 上（并发运行会串号）。
type failureGuard struct {
	counts    map[string]int  // 失败签名 -> 连续失败次数（同工具成功即清零）
	replanned map[string]bool // 失败签名 -> 是否已注入过再规划提示（避免每步重复注入）
	total     int             // 本轮累计失败次数（跨签名）
}

func newFailureGuard() *failureGuard {
	return &failureGuard{
		counts:    make(map[string]int),
		replanned: make(map[string]bool),
	}
}

// failureSignature 构造失败签名：有 error_kind 用「tool:kind」，否则退化为「tool」。
func failureSignature(tool, kind string) string {
	if kind == "" {
		return tool
	}
	return tool + ":" + kind
}

// recordFailure 记一次工具失败（kind 为工具层分级，普通报错传空）。
func (g *failureGuard) recordFailure(tool, kind string) {
	if g == nil {
		return
	}
	g.counts[failureSignature(tool, kind)]++
	g.total++
}

// recordSuccess 记一次工具成功：清零「该工具」名下所有失败签名——一次进展即打破其失败循环，
// 不牵连其它工具的计数（各工具的修复进度相互独立）。
func (g *failureGuard) recordSuccess(tool string) {
	if g == nil {
		return
	}
	prefix := tool + ":"
	for sig := range g.counts {
		if sig == tool || strings.HasPrefix(sig, prefix) {
			delete(g.counts, sig)
			delete(g.replanned, sig)
		}
	}
}

// replanNote 返回本轮需要注入的再规划提示（仅针对刚跨过阈值、尚未提示过的签名），
// 无则返回空串。同一签名只提示一次，避免每步刷屏。
func (g *failureGuard) replanNote() string {
	if g == nil {
		return ""
	}
	var hit []string
	for sig, n := range g.counts {
		if n >= replanThreshold && !g.replanned[sig] {
			g.replanned[sig] = true
			hit = append(hit, sig)
		}
	}
	if len(hit) == 0 {
		return ""
	}
	sort.Strings(hit) // 稳定输出，便于测试与日志比对
	return fmt.Sprintf(
		"检测到以下调用已重复失败：%s。请勿再用相同参数重试同一工具。"+
			"先阅读上一步观察里的 error_kind/hint 定向修正（改列名/改表名/修语法/缩小范围/更换数据源或工具），"+
			"若仍无进展，请重新规划任务路径或如实说明受阻原因与已获得的部分结论。",
		strings.Join(hit, "、"),
	)
}

// shouldWindDown 判断累计失败是否已达到「原地打转」阈值，应提前收尾。
func (g *failureGuard) shouldWindDown() bool {
	return g != nil && g.total >= windDownThreshold
}

package bpecount

import (
	"sync"
	"testing"

	ctxeng "cognida/internal/service/agent/context"
)

// 基准来自 HuggingFace 官方 tokenizers（0.22.x）对本目录内嵌的同一份 DeepSeek-V3 tokenizer.json
// 逐样本 encode(add_special_tokens=False) 的 token 数。sugarme 加载时剔除了那条 RE2 不支持、
// 且计数中性的负向先行断言分支（见 bpecount.go 注释），故与 HF 允许极小边界差异（±2）。
var goldenSamples = []struct {
	name   string
	text   string
	hfTok  int // HuggingFace 官方 token 数
}{
	{"zh_pure", "上个月华东区各品类的销售额环比整体增长明显", 12},
	{"zh_mixed", "家电环比 +18.3% 领跑，生鲜 -4.1% 下滑，大盘环比 +6.2%，客单价上移约7个百分点。", 36},
	{"sql", "SELECT category, SUM(amount) FROM orders WHERE region='east' GROUP BY category ORDER BY 2 DESC", 23},
	{"en", "Please summarize the month over month growth by category.", 10},
	{"rid", "查询完成，结果见 rs_east2026q2，家电增量集中在大家电与厨卫。", 22},
}

const tolerance = 2

// TestBPECount_MatchesHuggingFaceGroundTruth 验证内嵌 DeepSeek 分词器的 token 计数
// 与 HuggingFace 官方实现逐样本一致（允许 ±2 的计数中性补丁边界差异）。
func TestBPECount_MatchesHuggingFaceGroundTruth(t *testing.T) {
	c, ok := NewOrApprox()
	if !ok {
		t.Fatalf("内嵌 DeepSeek 分词器加载失败，降级到了 ApproxTokenCounter（词表损坏？）")
	}
	for _, s := range goldenSamples {
		got := c.Count(s.text)
		diff := got - s.hfTok
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Errorf("样本 %q: BPE=%d, HF 基准=%d, 差 %d 超出容差 ±%d", s.name, got, s.hfTok, s.hfTok-got, tolerance)
		} else {
			t.Logf("样本 %-8s: BPE=%2d  HF=%2d  (Δ=%d) ✅", s.name, got, s.hfTok, got-s.hfTok)
		}
	}
}

// TestBPECount_FixesHeuristicUndercount 证明真实 BPE 修正了字符启发式对「中文夹数字/百分号」
// 分析型文本的系统性低估：zh_mixed 上 ApproxTokenCounter 明显偏低，BPE 贴合 HF 基准。
// 这正是引入真实分词的核心动机（折叠/剥离触发线不再偏晚）。
func TestBPECount_FixesHeuristicUndercount(t *testing.T) {
	c, ok := NewOrApprox()
	if !ok {
		t.Fatalf("分词器加载失败")
	}
	approx := ctxeng.ApproxTokenCounter{}
	const mixed = "家电环比 +18.3% 领跑，生鲜 -4.1% 下滑，大盘环比 +6.2%，客单价上移约7个百分点。"
	const hfTok = 36

	bpe := c.Count(mixed)
	heur := approx.Count(mixed)
	if heur >= bpe {
		t.Errorf("预期字符启发式(%d)对混合文本低估、真实 BPE(%d) 更高，未复现", heur, bpe)
	}
	// 量化低估幅度，落进日志便于回看动机。
	undercount := float64(hfTok-heur) * 100 / float64(hfTok)
	t.Logf("zh_mixed: HF=%d  BPE=%d  启发式=%d → 启发式低估 %.1f%%", hfTok, bpe, heur, undercount)
	if undercount < 15 {
		t.Errorf("预期启发式对该混合样本低估显著(>15%%)，实测仅 %.1f%%", undercount)
	}
}

// TestBPECount_ConcurrentSafe 并发调用 Count（-race 下）验证底层 BPE 合并缓存的
// RWMutex 保护成立——生产里多个 ReAct 循环会并发计数。
func TestBPECount_ConcurrentSafe(t *testing.T) {
	c, ok := NewOrApprox()
	if !ok {
		t.Fatalf("分词器加载失败")
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := goldenSamples[i%len(goldenSamples)]
			if got := c.Count(s.text); got <= 0 {
				t.Errorf("并发 Count(%q)=%d，应为正", s.name, got)
			}
		}(i)
	}
	wg.Wait()
}

// TestBPECount_EmptyAndMinOne 空串返回 0，非空至少返回 1（与 ApproxTokenCounter 口径一致）。
func TestBPECount_EmptyAndMinOne(t *testing.T) {
	c, _ := NewOrApprox()
	if got := c.Count(""); got != 0 {
		t.Errorf("空串应为 0，got %d", got)
	}
	if got := c.Count("a"); got < 1 {
		t.Errorf("非空至少 1，got %d", got)
	}
}

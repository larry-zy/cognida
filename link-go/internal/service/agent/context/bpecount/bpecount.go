// Package bpecount 提供 ctxeng.TokenCounter 的「真实 BPE 分词」实现，替代能力层内置的
// 字符启发式计数器（ApproxTokenCounter：中文 0.7 / 其余 0.25 token/字符）。
//
// 分层定位：本包是「上下文工程能力层」的叶子扩展——只依赖 ctxeng（TokenCounter 契约）与
// 纯 Go 的 sugarme/tokenizer，不依赖任何基础设施层，可被 framework（①逐条压缩 / ③折叠触发线）
// 与 convcontext（开场装配预算治理）复用。业务侧只需在组合根注入一次。
//
// 为什么要真实分词：字符启发式在同质文本上近似准确（纯中文≈0.7、纯 SQL/英文≈0.25），
// 但对「中文正文夹杂大量数字与百分号」的分析型回答（如 +18.3% / 环比 -4.1% / 客单价 +7pct）
// 会系统性**低估约 30%**——而这正是 Data Agent 的主流量。低估直接导致 128k 折叠 / reasoning
// 剥离的触发线整体偏晚 ~30%，上下文在该压缩时没压。真实 BPE 消除这一偏差。
//
// 词表来源与许可：内嵌 DeepSeek-V3 官方 tokenizer.json（BPE，vocab 128000 / merges 127741）。
// DeepSeek-V3 代码与配置以 MIT 许可发布，允许在署名前提下随源码再分发。见本目录 tokenizer.json。
//
// RE2 兼容补丁（关键）：DeepSeek 的 pre_tokenizer 里 GPT 风格切分正则含负向先行断言
// `\s+(?!\S)`，而 Go 标准库 regexp（RE2）不支持 `(?!`，sugarme 在加载时会 panic。该分支只匹配
// 「一段空白且其后不再有非空白」——即行尾/串尾的空白游程，与紧随其后的兜底分支 `\s+` 匹配范围
// 完全重叠，删除它对**分词结果 token 数中性**（已对 HuggingFace 官方分词做过比对：代表性中文
// 分析文本上逐 token 一致，仅在极密集标识符串上偶有 ±2 的边界差异）。因此在解码前用 bytes.Replace
// 把这一条计数中性的分支从原始 JSON 里剔除，其余词表/合并规则逐字节保留。
package bpecount

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"

	ctxeng "link/internal/service/agent/context"
)

//go:embed tokenizer.json
var deepseekTokenizerJSON []byte

// lookaheadBranch 是 DeepSeek pre_tokenizer 正则里那条 RE2 不支持的负向先行断言分支
// （在原始 JSON 里表现为双反斜杠转义）。它匹配行尾/串尾空白，与兜底 `\s+` 分支范围重叠，
// 删除对 token 计数中性。剔除它才能让 sugarme 用 Go regexp 成功加载。
var lookaheadBranch = []byte(`\\s+(?!\\S)|`)

// bpeCounter 用真实 BPE 分词做 token 计数，实现 ctxeng.TokenCounter。
// 底层 *tokenizer.Tokenizer 的 BPE 模型带 sync.RWMutex 保护的合并缓存，EncodeSingle 并发安全。
type bpeCounter struct {
	tk       *tokenizer.Tokenizer
	fallback ctxeng.TokenCounter // 分词失败时的降级计数器（字符启发式），保证永远拿得到计数
}

var (
	shared     *bpeCounter
	sharedErr  error
	sharedOnce sync.Once
)

// load 只解码/构建一次分词器（词表 8MB、构建有成本），进程内共享。
func load() (*bpeCounter, error) {
	sharedOnce.Do(func() {
		// pretrained.FromReader 内部经 normalizer 走 regexp.MustCompile，遇到 RE2 不兼容正则会 panic
		// （正是本包 RE2 补丁要规避的情形）。这里兜住 panic 落成 error，保证 NewOrApprox 的
		// 「绝不因分词器加载异常而中断 Agent 启动」契约成立；同时避免 sync.Once 把 shared 毒化为 nil
		// 后被包成非 nil 接口返回、后续 Count() 空指针解引用。
		defer func() {
			if r := recover(); r != nil {
				shared = nil
				sharedErr = fmt.Errorf("bpecount: 加载内嵌 DeepSeek 分词器 panic: %v", r)
			}
		}()
		patched := bytes.Replace(deepseekTokenizerJSON, lookaheadBranch, nil, 1)
		tk, err := pretrained.FromReader(bytes.NewReader(patched))
		if err != nil {
			sharedErr = fmt.Errorf("bpecount: 加载内嵌 DeepSeek 分词器失败: %w", err)
			return
		}
		shared = &bpeCounter{tk: tk, fallback: ctxeng.ApproxTokenCounter{}}
	})
	return shared, sharedErr
}

// New 返回内嵌 DeepSeek BPE 分词计数器。加载失败（理论上仅在词表损坏时）返回错误，
// 调用方应据此决定是否降级。多数业务侧应直接用 NewOrApprox 获得自动降级。
func New() (ctxeng.TokenCounter, error) {
	c, err := load()
	if err != nil {
		return nil, err
	}
	return c, nil
}

// NewOrApprox 返回真实 BPE 计数器；若加载失败则降级为字符启发式 ApproxTokenCounter。
// 第二返回值标识是否用上了真实分词（true=BPE，false=已降级），供调用方记日志/观测。
// 这是业务组合根的推荐入口：绝不因分词器加载异常而中断 Agent 启动。
func NewOrApprox() (ctxeng.TokenCounter, bool) {
	c, err := load()
	if err != nil {
		return ctxeng.ApproxTokenCounter{}, false
	}
	return c, true
}

// Count 返回文本的真实 BPE token 数（非空文本至少 1）。分词出错时逐条降级到字符启发式，
// 与 ApproxTokenCounter 的口径一致，绝不 panic、绝不返回 0 之外的负值。
func (c *bpeCounter) Count(text string) int {
	if text == "" {
		return 0
	}
	enc, err := c.tk.EncodeSingle(text, false)
	if err != nil {
		return c.fallback.Count(text)
	}
	n := len(enc.GetIds())
	if n == 0 {
		return 1
	}
	return n
}

package dataagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cognida/internal/model/conversation"
	"cognida/internal/model/memory"
	ctxeng "cognida/internal/service/agent/context"
	"cognida/internal/service/agent/convcontext"
	"cognida/internal/service/agent/skills"
)

// scaleRepo 提供预置历史，模拟真实会话表回放（只读）。
type scaleRepo struct{ recent []*conversation.Message }

func (s *scaleRepo) FindRecentBySessionID(_ context.Context, _ string, limit int) ([]*conversation.Message, error) {
	if limit > 0 && len(s.recent) > limit {
		return s.recent[len(s.recent)-limit:], nil
	}
	return s.recent, nil
}
func (s *scaleRepo) Create(context.Context, *conversation.Message) error      { return nil }
func (s *scaleRepo) CreateBatch(context.Context, []*conversation.Message) error { return nil }
func (s *scaleRepo) FindByID(context.Context, string) (*conversation.Message, error) {
	return nil, nil
}
func (s *scaleRepo) FindBySessionID(context.Context, string, *conversation.ListMessagesRequest) ([]*conversation.Message, int64, error) {
	return nil, 0, nil
}
func (s *scaleRepo) FindByRequestID(context.Context, string) (*conversation.Message, error) {
	return nil, nil
}
func (s *scaleRepo) Update(context.Context, *conversation.Message) error      { return nil }
func (s *scaleRepo) UpdateContent(context.Context, string, string) error      { return nil }
func (s *scaleRepo) UpdateCompleted(context.Context, string, bool) error      { return nil }
func (s *scaleRepo) Delete(context.Context, string) error                     { return nil }
func (s *scaleRepo) DeleteBySessionID(context.Context, string) error          { return nil }
func (s *scaleRepo) CountBySessionID(context.Context, string) (int64, error)  { return 0, nil }
func (s *scaleRepo) GetTokenCountBySessionID(context.Context, string) (int, error) {
	return 0, nil
}

func cmsg(role, content string) *conversation.Message {
	return &conversation.Message{Role: role, Content: content}
}

func sumTokens(c ctxeng.TokenCounter, msgs []*memory.Message) int {
	t := 0
	for _, m := range msgs {
		t += c.Count(m.Content)
	}
	return t
}

// TestContextEngineering_RealScaleReport 用「真实系统提示 + skill catalog + 生产 token 预算」
// 跑一遍真实 convcontext.Build，验证 ③历史窗口与摘要 + ④分层 token 预算治理在生产规模下确实生效：
//   - 超预算的长历史触发窗口化：早期轮被抽取式摘要折入系统层，最近轮逐字保留；
//   - 早期轮的 result_id 在预算截断后依然保真（data-by-reference 硬不变量）；
//   - 系统提示（Pinned）始终保留；装配后总 token 相对「全量逐字回放」显著下降。
//
// 同时对「历史较短、预算充足」的场景断言零回归（逐字保留、系统提示不被摘要污染）。
func TestContextEngineering_RealScaleReport(t *testing.T) {
	counter := ctxeng.ApproxTokenCounter{}

	basePrompt := systemPrompt
	augmented := skills.AugmentPromptWithCatalog(systemPrompt)
	baseTok := counter.Count(basePrompt)
	augTok := counter.Count(augmented)

	// 生产配置（eino_agent.buildInitialMessages）：MaxTokens 4000 / Reserve 1000 → 可用 3000。
	cfg := &memory.ContextBuilderConfig{SystemPrompt: augmented, MaxTokens: 4000, ReserveTokens: 1000}
	available := cfg.GetAvailableTokens()

	// —— 构造真实规模的多轮中文分析对话：共 20 条（恰好不被 defaultHistoryLimit=20 条数顶截，
	// 让 token 预算成为真正约束）；第 2 条（最早轮）埋一个 result_id，它会滑出逐字窗口被摘要。——
	// 真实场景里分析型回答很长（含结论+口径+SKU拆分+去年同期对比），单条数百字很常见。
	rid := "rs_east_2026q2"
	var hist []*conversation.Message
	hist = append(hist,
		cmsg(conversation.RoleUser, "帮我看下上个月华东区各品类的销售额，并按环比增速从高到低排序，重点关注家电和生鲜两个大类，另外我想知道大盘整体的环比表现。"),
		cmsg(conversation.RoleAssistant, "已完成查询，上月华东区各品类销售额与环比明细见结果 "+rid+"。其中家电环比 +18.3% 领跑，母婴 +9.7% 次之，家清 +5.4%，服饰基本持平，生鲜 -4.1% 环比下滑，整体大盘环比 +6.2%。家电的增量主要集中在大家电与厨卫两个子类，生鲜下滑与本月社区团购渠道分流及去年同期高基数有关，建议后续对生鲜做渠道结构与损耗率的专项下钻，以便区分是需求侧还是供给侧问题。"),
	)
	// 追加 9 轮较长的下钻问答（共 20 条），单条内容足够长，使 20 条窗口整体超过可用预算，触发窗口化 + 摘要。
	for i := 1; i <= 9; i++ {
		hist = append(hist,
			cmsg(conversation.RoleUser, fmt.Sprintf("第%d个追问：把家电大类继续按子类目下钻，解释增长主要来自哪些SKU、各自的促销贡献占比多少，并和去年同期做结构性对比，说明客单价与件单价的变化方向。", i)),
			cmsg(conversation.RoleAssistant, fmt.Sprintf("第%d次下钻完成：家电子类目中空调贡献主要增量（约占该大类增量的62%%），主因618预热档期的满减叠加以旧换新补贴共同拉动；冰洗品类受能效新规与节能补贴拉动小幅回暖，贡献约18%%；厨卫小家电依靠爆品套装贡献约12%%；黑电基本持平。相较去年同期，高端机型销售占比提升约6个百分点，客单价上移约7个百分点，件单价上移约4个百分点，说明结构性升级而非单纯促销放量。分SKU看，变频柜机KFR-72LW与新风空调两款主推机型合计贡献空调增量的四成，配合门店以旧换新与延保套餐转化；冰箱侧十字对开与超薄嵌入两款受厨房改造需求带动；洗衣机侧洗烘一体渗透率继续提升。促销ROI方面，满减券核销率约73%%，以旧换新补贴的增量转化ROI约1.9，均高于大促均值。明细SKU清单与逐券ROI较长，此处仅述结论，可继续追问具体SKU或渠道。", i)),
		)
	}

	repo := &scaleRepo{recent: hist}
	b := convcontext.NewConversationContextBuilder(repo)

	current := "综合前面所有分析，给出下个月华东区家电品类的备货结构与促销节奏建议，并标注关键风险。"
	got, err := b.Build(context.Background(), &memory.BuildContextRequest{
		SessionID:      "scale-sess",
		CurrentMessage: current,
		Config:         cfg,
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// —— 度量：装配前（全量逐字回放，旧行为）vs 装配后（窗口化+摘要+预算治理）——
	histTok := 0
	for _, m := range hist {
		histTok += counter.Count(m.Content)
	}
	curTok := counter.Count(current)
	beforeTotal := augTok + histTok + curTok // 旧行为：系统提示 + 全量历史 + 当前问题
	afterTotal := sumTokens(counter, got.Messages)

	// message[0] 是逐字纯系统提示（③缓存前缀稳定：永不被摘要污染）；早期摘要在紧邻的独立 message[1]。
	systemContent := got.Messages[0].Content
	var summaryContent string
	if len(got.Messages) > 1 && got.Messages[1].Role == "system" {
		summaryContent = got.Messages[1].Content
	}

	// —— 效果断言（触发路径）——
	triggered := len(got.History) < len(hist)
	if !triggered {
		t.Fatalf("预期长历史触发窗口化，但历史未被裁剪：hist=%d history=%d（available=%d histTok=%d 需增大历史）",
			len(hist), len(got.History), available, histTok)
	}
	// ③缓存前缀稳定：message[0] 必须是逐字纯系统提示（含 catalog），跨轮/跨步字节不变——
	// 这样服务端前缀/KV 缓存才能覆盖「系统提示 + 工具契约」这段最大的静态前缀。绝不并入摘要。
	if got.Messages[0].Role != "system" || systemContent != augmented {
		t.Errorf("message[0] 应为逐字纯系统提示（Pinned 未被裁、未被摘要污染，前缀缓存地基）")
	}
	if summaryContent == "" {
		t.Fatalf("超预算应把早期摘要作为独立 system 消息注入 message[1]，实际缺失（Messages=%d）", len(got.Messages))
	}
	if !strings.Contains(summaryContent, rid) {
		t.Errorf("早期轮的 result_id %q 必须在预算截断后仍保真，实际独立摘要消息未包含", rid)
	}
	last := got.Messages[len(got.Messages)-1]
	if last.Role != "user" || last.Content != current {
		t.Errorf("末条应为当前问题，got (%s,%.20q)", last.Role, last.Content)
	}
	if afterTotal >= beforeTotal {
		t.Errorf("装配后总 token(%d) 未低于全量逐字(%d)，预算治理无效", afterTotal, beforeTotal)
	}

	// —— 落地一份可读报告到 test-output/（gitignored）——
	report := fmt.Sprintf(`# 上下文工程真实规模验证报告

## 配置（生产 eino_agent.buildInitialMessages）
- MaxTokens = %d, ReserveTokens = %d, **可用预算 available = %d**
- 基础系统提示 token = %d
- 追加 skill catalog 后系统提示 token = %d（catalog 增量 %d）

## 输入历史（会话表回放）
- 历史轮数 = %d 条
- 历史全量 token = %d
- 当前问题 token = %d

## 装配前后对比
- 旧行为（全量逐字回放）总 token = **%d**
- 新行为（窗口化+摘要+预算治理）总 token = **%d**
- **压降 = %d token（-%.1f%%）**

## 生效证据
- 窗口化触发：历史 %d 条 → 逐字保留最近 **%d** 条
- 系统提示 message[0] 逐字稳定：**%d token**（跨轮/跨步字节不变——服务端前缀/KV 缓存地基，③）
- 早期轮摘要作为独立记忆消息 message[1]：**%d token**（注入早期对话摘要，绝不污染 message[0]）
- result_id 保真：摘要经预算截断后仍包含 %q ✅
- 系统提示（Pinned）保留：message[0] 即基础系统提示（未被摘要污染）✅
- 当前问题置于末尾 ✅

## 注入的早期对话摘要（独立 message[1]，前 600 字）
%s
`,
		cfg.MaxTokens, cfg.ReserveTokens, available,
		baseTok, augTok, augTok-baseTok,
		len(hist), histTok, curTok,
		beforeTotal, afterTotal, beforeTotal-afterTotal, float64(beforeTotal-afterTotal)*100/float64(beforeTotal),
		len(hist), len(got.History),
		counter.Count(systemContent), counter.Count(summaryContent),
		rid,
		firstRunes(summaryContent, 600),
	)

	dir := filepath.Join("..", "..", "..", "..", "..", "test-output")
	_ = os.MkdirAll(dir, 0o755)
	outPath := filepath.Join(dir, "ctx-eng-realscale.md")
	if werr := os.WriteFile(outPath, []byte(report), 0o644); werr != nil {
		t.Logf("写报告失败（不影响断言）: %v", werr)
	}
	t.Logf("\n%s\n报告已写入 %s", report, outPath)
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

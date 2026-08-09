package chat

import (
	"context"
	"testing"

	"cognida/internal/model/llm"
)

// fakeModelRepo 仅实现本测试用到的方法。
type fakeModelRepo struct {
	llm.ModelRepository
	byID    map[int64]*llm.ModelConfig
	byType  []*llm.ModelConfig
	typeErr error
}

func (r *fakeModelRepo) GetByID(_ context.Context, id int64) (*llm.ModelConfig, error) {
	return r.byID[id], nil
}
func (r *fakeModelRepo) GetByType(context.Context, int64, llm.ModelType) ([]*llm.ModelConfig, error) {
	return r.byType, r.typeErr
}

// capturingFactory 记录收到的降级链。
type capturingFactory struct {
	llm.ModelFactory
	got []*llm.ModelConfig
}

func (f *capturingFactory) CreateChatModelChain(configs []*llm.ModelConfig) (llm.LLMClient, error) {
	f.got = configs
	return nil, nil
}

func chatCfg(id int64, enabled, isDefault bool) *llm.ModelConfig {
	return &llm.ModelConfig{ID: id, TenantID: 1, ModelType: llm.ModelTypeChat, Enabled: enabled, IsDefault: isDefault}
}

func TestBuildChatChain_OrdersFallbacks(t *testing.T) {
	primary := chatCfg(10, true, false)
	repo := &fakeModelRepo{
		byID: map[int64]*llm.ModelConfig{10: primary},
		byType: []*llm.ModelConfig{
			primary,
			chatCfg(20, true, false),  // 普通后备
			chatCfg(5, true, true),    // 默认模型 → 应排后备首位
			chatCfg(30, false, false), // 禁用 → 剔除
		},
	}
	f := &capturingFactory{}
	svc := NewModelService(repo, f)

	if _, err := svc.CreateChatModel(context.Background(), 1, 10); err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, len(f.got))
	for i, c := range f.got {
		ids[i] = c.ID
	}
	want := []int64{10, 5, 20} // 主 → 默认后备 → 普通后备（禁用剔除）
	if len(ids) != len(want) {
		t.Fatalf("chain=%v want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("chain[%d]=%d want %d (full=%v)", i, ids[i], want[i], ids)
		}
	}
}

func TestBuildChatChain_RepoErrorFallsBackToPrimaryOnly(t *testing.T) {
	primary := chatCfg(10, true, false)
	repo := &fakeModelRepo{
		byID:    map[int64]*llm.ModelConfig{10: primary},
		typeErr: context.DeadlineExceeded,
	}
	f := &capturingFactory{}
	svc := NewModelService(repo, f)

	if _, err := svc.CreateChatModel(context.Background(), 1, 10); err != nil {
		t.Fatal(err)
	}
	if len(f.got) != 1 || f.got[0].ID != 10 {
		t.Fatalf("expected primary-only chain, got %v", f.got)
	}
}

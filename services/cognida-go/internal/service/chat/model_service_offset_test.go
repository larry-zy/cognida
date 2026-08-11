package chat

import (
	"context"
	"testing"

	"cognida/internal/model/llm"
)

// offsetModelRepo 记录 List 收到的过滤/分页参数，并回放预置的 configs 与 total，
// 用于验证 ListModels 偏移路径把过滤全部下推仓储、且 total 直用仓储返回值〔R2-2〕。
type offsetModelRepo struct {
	llm.ModelRepository
	gotModelType string
	gotEnabled   *bool
	gotOffset    int
	gotLimit     int
	configs      []*llm.ModelConfig
	total        int64
}

func (r *offsetModelRepo) List(_ context.Context, _ int64, modelType string, enabled *bool, offset, limit int) ([]*llm.ModelConfig, int64, error) {
	r.gotModelType = modelType
	r.gotEnabled = enabled
	r.gotOffset = offset
	r.gotLimit = limit
	return r.configs, r.total, nil
}

// 偏移路径：类型+启用过滤透传仓储，分页参数正确换算，total 用仓储返回值（不再被内存过滤破坏）。
func TestListModels_OffsetPushesFiltersAndUsesRepoTotal(t *testing.T) {
	enabled := true
	repo := &offsetModelRepo{
		configs: []*llm.ModelConfig{
			{ID: 3, TenantID: 1, Name: "m3", Enabled: true},
			{ID: 2, TenantID: 1, Name: "m2", Enabled: true},
		},
		total: 42, // 仓储在同一过滤口径下算出的总数
	}
	svc := NewModelService(repo, nil)

	resp, err := svc.ListModels(context.Background(), 1, &ListModelsRequestDTO{
		ModelType: "chat",
		Enabled:   &enabled,
		Page:      3,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListModels error = %v", err)
	}

	// 过滤条件必须下推到仓储，而非在 service 内存里过滤。
	if repo.gotModelType != "chat" {
		t.Errorf("modelType 未下推：got %q want chat", repo.gotModelType)
	}
	if repo.gotEnabled == nil || *repo.gotEnabled != true {
		t.Errorf("enabled 未下推：got %v want true", repo.gotEnabled)
	}
	// 分页换算：page=3,size=10 → offset=20,limit=10（含类型查询也必须分页，修复旧「类型分支不分页」）。
	if repo.gotOffset != 20 || repo.gotLimit != 10 {
		t.Errorf("分页参数换算错误：offset=%d limit=%d want 20/10", repo.gotOffset, repo.gotLimit)
	}
	// total 必须是仓储返回值，而非过滤后返回集的长度（旧实现的 total 失真缺陷）。
	if resp.Total != 42 {
		t.Errorf("total = %d want 42（应直用仓储总数，不被内存过滤破坏）", resp.Total)
	}
	if len(resp.Models) != 2 {
		t.Errorf("返回集长度 = %d want 2", len(resp.Models))
	}
	if resp.Page != 3 || resp.PageSize != 10 {
		t.Errorf("回显分页 = %d/%d want 3/10", resp.Page, resp.PageSize)
	}
}

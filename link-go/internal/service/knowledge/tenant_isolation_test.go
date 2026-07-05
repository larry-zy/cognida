// Package knowledge: 跨租户 IDOR 防护单元测试。
// tenant A 用 tenant B 的资源 id 访问，必须得到与"不存在"相同的错误（不泄露资源存在性），
// 且写操作（删除等）不得触达底层仓储。
package knowledge

import (
	"context"
	"fmt"
	"testing"

	domain_knowledge "link/internal/model/knowledge"
)

// tenantFakeKBRepo 租户感知的 KnowledgeBaseRepository 假实现：
// 只按 FindByIDForTenant 语义做 id+tenant 双键匹配，并记录写调用次数。
type tenantFakeKBRepo struct {
	kbs         map[string]*domain_knowledge.KnowledgeBase
	deleteCalls int
	updateCalls int
}

func newTenantFakeKBRepo() *tenantFakeKBRepo {
	return &tenantFakeKBRepo{kbs: map[string]*domain_knowledge.KnowledgeBase{}}
}

func (f *tenantFakeKBRepo) Create(_ context.Context, kb *domain_knowledge.KnowledgeBase) error {
	f.kbs[kb.ID] = kb
	return nil
}

func (f *tenantFakeKBRepo) FindByID(_ context.Context, id string) (*domain_knowledge.KnowledgeBase, error) {
	if kb, ok := f.kbs[id]; ok {
		return kb, nil
	}
	return nil, fmt.Errorf("知识库不存在")
}

func (f *tenantFakeKBRepo) FindByIDForTenant(_ context.Context, id string, tenantID int64) (*domain_knowledge.KnowledgeBase, error) {
	if kb, ok := f.kbs[id]; ok && tenantID > 0 && kb.TenantID == tenantID {
		return kb, nil
	}
	return nil, fmt.Errorf("知识库不存在")
}

func (f *tenantFakeKBRepo) FindByTenantID(context.Context, int64, int, int) ([]*domain_knowledge.KnowledgeBase, int64, error) {
	return nil, 0, nil
}

func (f *tenantFakeKBRepo) FindByUser(context.Context, int64, int, int) ([]*domain_knowledge.KnowledgeBase, int64, error) {
	return nil, 0, nil
}

func (f *tenantFakeKBRepo) Update(context.Context, *domain_knowledge.KnowledgeBase) error {
	f.updateCalls++
	return nil
}

func (f *tenantFakeKBRepo) UpdateStats(context.Context, string, int, int, int64) error {
	return nil
}

func (f *tenantFakeKBRepo) Delete(context.Context, string) error {
	f.deleteCalls++
	return nil
}

func (f *tenantFakeKBRepo) HardDelete(context.Context, string) error {
	f.deleteCalls++
	return nil
}

func (f *tenantFakeKBRepo) Exists(_ context.Context, id string) (bool, error) {
	_, ok := f.kbs[id]
	return ok, nil
}

func (f *tenantFakeKBRepo) GetStorageStats(context.Context, int64) (int64, int64, error) {
	return 0, 0, nil
}

// newIDORFixture 造一个属于 tenant B(=2) 的知识库 kb-b，返回 service 与假仓储。
// 其余依赖传 nil：被测的越权拒绝路径在 requireKB 处即返回，不触达它们。
func newIDORFixture() (KnowledgeBaseService, *tenantFakeKBRepo) {
	repo := newTenantFakeKBRepo()
	repo.kbs["kb-b"] = &domain_knowledge.KnowledgeBase{
		ID:       "kb-b",
		TenantID: 2,
		Name:     "tenant B 的知识库",
	}
	svc := NewKnowledgeBaseService(repo, nil, nil, nil, nil, nil, nil)
	return svc, repo
}

// TestIDOR_CrossTenantRead_NotFound tenant A(=1) 用 tenant B 的 kb id 读取 → not found，
// 且错误信息与真正不存在时完全一致（不泄露资源存在性）。
func TestIDOR_CrossTenantRead_NotFound(t *testing.T) {
	svc, _ := newIDORFixture()
	ctx := context.Background()

	_, errCross := svc.FindByID(ctx, "kb-b", 1)
	if errCross == nil {
		t.Fatal("期望跨租户读取被拒绝，却成功了")
	}
	_, errMissing := svc.FindByID(ctx, "kb-nonexistent", 1)
	if errMissing == nil {
		t.Fatal("期望不存在的 kb 返回错误")
	}
	if errCross.Error() != errMissing.Error() {
		t.Errorf("跨租户与不存在的错误信息应一致（不泄露存在性）: %q vs %q", errCross, errMissing)
	}
}

// TestIDOR_SameTenantRead_Allowed 归属租户可正常读取。
func TestIDOR_SameTenantRead_Allowed(t *testing.T) {
	svc, _ := newIDORFixture()

	kb, err := svc.FindByID(context.Background(), "kb-b", 2)
	if err != nil {
		t.Fatalf("归属租户读取应成功: %v", err)
	}
	if kb.ID != "kb-b" {
		t.Errorf("kb.ID = %s, 期望 kb-b", kb.ID)
	}
}

// TestIDOR_CrossTenantDelete_NoRowsAffected 跨租户删除被拒，且底层仓储 0 次写调用（0 行受影响）。
func TestIDOR_CrossTenantDelete_NoRowsAffected(t *testing.T) {
	svc, repo := newIDORFixture()

	if err := svc.Delete(context.Background(), "kb-b", 1); err == nil {
		t.Fatal("期望跨租户删除被拒绝，却成功了")
	}
	if repo.deleteCalls != 0 {
		t.Errorf("越权删除被拒后仓储 Delete 调用次数 = %d, 期望 0", repo.deleteCalls)
	}
	if _, ok := repo.kbs["kb-b"]; !ok {
		t.Error("越权删除被拒后，知识库不应被删除")
	}
}

// TestIDOR_CrossTenantUpdate_NoRowsAffected 跨租户更新被拒，且底层仓储 0 次写调用。
func TestIDOR_CrossTenantUpdate_NoRowsAffected(t *testing.T) {
	svc, repo := newIDORFixture()

	name := "hacked"
	_, err := svc.UpdateFromRequest(context.Background(), "kb-b", 1, &UpdateKnowledgeBaseRequest{Name: &name})
	if err == nil {
		t.Fatal("期望跨租户更新被拒绝，却成功了")
	}
	if repo.updateCalls != 0 {
		t.Errorf("越权更新被拒后仓储 Update 调用次数 = %d, 期望 0", repo.updateCalls)
	}
	if repo.kbs["kb-b"].Name == "hacked" {
		t.Error("越权更新被拒后，名称不应被修改")
	}
}

// TestIDOR_ZeroTenant_Denied tenantID 非法（0/负数）时 fail-closed 拒绝。
func TestIDOR_ZeroTenant_Denied(t *testing.T) {
	svc, _ := newIDORFixture()

	if _, err := svc.FindByID(context.Background(), "kb-b", 0); err == nil {
		t.Error("tenantID=0 应被拒绝")
	}
	if _, err := svc.FindByID(context.Background(), "kb-b", -1); err == nil {
		t.Error("tenantID=-1 应被拒绝")
	}
}

// TestIDOR_Search_SkipsUnauthorizedKB 搜索静默剔除越权 kbID，不报错也不返回他租户数据。
func TestIDOR_Search_SkipsUnauthorizedKB(t *testing.T) {
	svc, _ := newIDORFixture()

	chunks, err := svc.Search(context.Background(), []string{"kb-b"}, 1, "query", 5, 0)
	if err != nil {
		t.Fatalf("越权 kbID 应被静默剔除而非报错: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("越权搜索不应返回数据, got %d chunks", len(chunks))
	}
}

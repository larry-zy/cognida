// 会话摘要 write-through 测试（4.1.4）：
// UpdateSummary 先落 MySQL 再写缓存；GetSummary 缓存未命中回源 MySQL 并回填；
// miss（"", nil）与 error（err）语义区分。
package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"link/internal/model/memory"
	redisStore "link/internal/repository/redis"
)

// fakeSummaryRepo 仅实现摘要相关方法的内存版 MemoryRepository。
type fakeSummaryRepo struct {
	memory.MemoryRepository // 未实现的方法 panic（测试不应触达）

	summaries map[string]*memory.Summary // sessionID → 最新摘要
	loadErr   error                      // 注入 LoadLatestSummary 的真实错误
	saveCalls int
	updateCalls int
}

func newFakeSummaryRepo() *fakeSummaryRepo {
	return &fakeSummaryRepo{summaries: map[string]*memory.Summary{}}
}

func (r *fakeSummaryRepo) LoadLatestSummary(_ context.Context, sessionID string) (*memory.Summary, error) {
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	s, ok := r.summaries[sessionID]
	if !ok {
		return nil, memory.ErrSummaryNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *fakeSummaryRepo) SaveSummary(_ context.Context, s *memory.Summary) error {
	r.saveCalls++
	cp := *s
	r.summaries[s.SessionID] = &cp
	return nil
}

func (r *fakeSummaryRepo) UpdateSummary(_ context.Context, s *memory.Summary) error {
	r.updateCalls++
	cp := *s
	r.summaries[s.SessionID] = &cp
	return nil
}

// newTestMemoryService miniredis 作缓存 + fake repo 作主存储。
func newTestMemoryService(t *testing.T, repo memory.MemoryRepository) (memory.MemoryService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := redisStore.NewRedisMemoryStore(client, time.Hour)
	return NewMemoryService(repo, cache, nil, nil, nil, nil), mr
}

// TestUpdateSummary_WriteThroughThenCacheClearFallsBack write-through 后清空缓存，
// GetSummary 必须回源 MySQL 命中并回填缓存。旧实现只写 Redis，清缓存即永久丢失。
func TestUpdateSummary_WriteThroughThenCacheClearFallsBack(t *testing.T) {
	repo := newFakeSummaryRepo()
	svc, mr := newTestMemoryService(t, repo)
	ctx := context.Background()

	if err := svc.UpdateSummary(ctx, "sess-1", "第一轮摘要"); err != nil {
		t.Fatalf("UpdateSummary: %v", err)
	}
	// 先落 MySQL
	if repo.saveCalls != 1 {
		t.Fatalf("SaveSummary 调用 %d 次, 期望 1（未落库）", repo.saveCalls)
	}
	if got := repo.summaries["sess-1"].Content; got != "第一轮摘要" {
		t.Fatalf("MySQL 摘要 = %q, 期望 第一轮摘要", got)
	}

	// 模拟 Redis 重启/TTL 过期
	mr.FlushAll()

	got, err := svc.GetSummary(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetSummary 回源失败: %v", err)
	}
	if got != "第一轮摘要" {
		t.Fatalf("回源摘要 = %q, 期望 第一轮摘要", got)
	}
	// 回填缓存
	if cached, err := mr.Get("session:summary:sess-1"); err != nil || cached != "第一轮摘要" {
		t.Fatalf("缓存回填 = %q err=%v, 期望 第一轮摘要", cached, err)
	}
}

// TestUpdateSummary_RollingUpdate 二次更新走 UpdateSummary（滚动），不新建行。
func TestUpdateSummary_RollingUpdate(t *testing.T) {
	repo := newFakeSummaryRepo()
	svc, _ := newTestMemoryService(t, repo)
	ctx := context.Background()

	if err := svc.UpdateSummary(ctx, "sess-1", "v1"); err != nil {
		t.Fatalf("UpdateSummary v1: %v", err)
	}
	if err := svc.UpdateSummary(ctx, "sess-1", "v2"); err != nil {
		t.Fatalf("UpdateSummary v2: %v", err)
	}
	if repo.saveCalls != 1 || repo.updateCalls != 1 {
		t.Fatalf("saveCalls=%d updateCalls=%d, 期望 1/1（滚动更新而非重复新建）", repo.saveCalls, repo.updateCalls)
	}
	if got := repo.summaries["sess-1"].Content; got != "v2" {
		t.Fatalf("MySQL 摘要 = %q, 期望 v2", got)
	}
}

// TestGetSummary_MissReturnsEmptyNil 缓存与 MySQL 都无摘要 → 真 miss（"", nil）。
func TestGetSummary_MissReturnsEmptyNil(t *testing.T) {
	repo := newFakeSummaryRepo()
	svc, _ := newTestMemoryService(t, repo)

	got, err := svc.GetSummary(context.Background(), "no-such-session")
	if err != nil {
		t.Fatalf("真 miss 不应返回错误: %v", err)
	}
	if got != "" {
		t.Fatalf("真 miss 应返回空串, got %q", got)
	}
}

// TestGetSummary_RealErrorReturned 缓存 miss + MySQL 真实故障 → 必须返回 error。
// 旧实现把一切错误吞成空摘要，静默丢跨轮记忆。
func TestGetSummary_RealErrorReturned(t *testing.T) {
	repo := newFakeSummaryRepo()
	repo.loadErr = fmt.Errorf("mysql connection refused")
	svc, _ := newTestMemoryService(t, repo)

	_, err := svc.GetSummary(context.Background(), "sess-1")
	if err == nil {
		t.Fatal("MySQL 真实故障被吞成 miss（应返回 error）")
	}
}

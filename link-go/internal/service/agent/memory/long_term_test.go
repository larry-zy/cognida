// Package memory 长期记忆用例测试：ID 唯一性 + 访问统计无 data race。
package memory

import (
	"context"
	"sync"
	"testing"
	"time"

	"link/internal/model/memory"
)

// ========================================
// 3.1 ID 生成唯一性
// ========================================

// TestGenerateID_ConcurrentNoDuplicates -race 下并发生成大量 ID 断言无重复。
// 旧实现每个字节取 time.Now().UnixNano()%len(charset)，同一纳秒内必然重复。
func TestGenerateID_ConcurrentNoDuplicates(t *testing.T) {
	const (
		goroutines = 16
		perG       = 2000
	)

	var mu sync.Mutex
	seen := make(map[string]struct{}, goroutines*perG)
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]string, 0, perG)
			for i := 0; i < perG; i++ {
				local = append(local, generateID())
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range local {
				if _, dup := seen[id]; dup {
					t.Errorf("重复 ID: %s", id)
					return
				}
				seen[id] = struct{}{}
			}
		}()
	}
	wg.Wait()

	if len(seen) != goroutines*perG {
		t.Errorf("唯一 ID 数 = %d, 期望 %d", len(seen), goroutines*perG)
	}
}

// TestRandomString_Charset 随机串只包含合法字符且长度正确。
func TestRandomString_Charset(t *testing.T) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	for i := 0; i < 100; i++ {
		s := randomString(8)
		if len(s) != 8 {
			t.Fatalf("len(%q) = %d, 期望 8", s, len(s))
		}
		for _, c := range s {
			found := false
			for _, valid := range charset {
				if c == valid {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("非法字符 %q in %q", c, s)
			}
		}
	}
}

// ========================================
// 3.2 访问统计无 data race
// ========================================

// raceFakeRepo 内存版 LongTermMemoryRepository：RecordAccess 原子计数。
type raceFakeRepo struct {
	mu          sync.Mutex
	memories    map[string]*memory.LongTermMemory
	accessCalls map[string]int
	updateCalls int
}

func newRaceFakeRepo() *raceFakeRepo {
	return &raceFakeRepo{
		memories:    make(map[string]*memory.LongTermMemory),
		accessCalls: make(map[string]int),
	}
}

func (r *raceFakeRepo) Store(_ context.Context, mem *memory.LongTermMemory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.memories[mem.ID] = mem
	return nil
}

func (r *raceFakeRepo) Retrieve(_ context.Context, id string) (*memory.LongTermMemory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.memories[id], nil
}

func (r *raceFakeRepo) Search(_ context.Context, _ *memory.MemorySearchQuery) ([]*memory.LongTermMemory, error) {
	return nil, nil
}

func (r *raceFakeRepo) Update(_ context.Context, _ *memory.LongTermMemory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateCalls++
	return nil
}

func (r *raceFakeRepo) RecordAccess(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accessCalls[id]++
	return nil
}

func (r *raceFakeRepo) Delete(_ context.Context, _ string) error { return nil }

func (r *raceFakeRepo) ListByCategory(_ context.Context, _ int64, _ string, _ int) ([]*memory.LongTermMemory, error) {
	return nil, nil
}

func (r *raceFakeRepo) ListByUser(_ context.Context, _, _ int64, _ int) ([]*memory.LongTermMemory, error) {
	return nil, nil
}

// TestRetrieve_NoDataRaceOnReturnedMemory -race 下并发 Retrieve 并读取返回实体的字段，
// 断言后台访问统计不改写已返回的 *mem（改写会被 race detector 捕获）。
func TestRetrieve_NoDataRaceOnReturnedMemory(t *testing.T) {
	repo := newRaceFakeRepo()
	stored := &memory.LongTermMemory{
		ID:      "mem-1",
		Content: "用户喜欢简洁回答",
	}
	_ = repo.Store(context.Background(), stored)

	uc := NewRetrieveMemoryUseCase(repo)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mem, err := uc.Execute(context.Background(), "mem-1")
			if err != nil {
				t.Errorf("Retrieve: %v", err)
				return
			}
			// 并发读取返回实体的字段：若后台 goroutine 改写 *mem，-race 必报
			_ = mem.AccessCount
			_ = mem.LastAccessAt
			_ = mem.Content
		}()
	}
	wg.Wait()

	// 等后台 RecordAccess goroutine 完成
	deadline := time.Now().Add(2 * time.Second)
	for {
		repo.mu.Lock()
		calls := repo.accessCalls["mem-1"]
		repo.mu.Unlock()
		if calls == 32 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.accessCalls["mem-1"] != 32 {
		t.Errorf("RecordAccess 调用数 = %d, 期望 32", repo.accessCalls["mem-1"])
	}
	// 访问统计必须走原子自增接口，不走整实体 Update
	if repo.updateCalls != 0 {
		t.Errorf("Update 调用数 = %d, 期望 0（应改用 RecordAccess）", repo.updateCalls)
	}
}

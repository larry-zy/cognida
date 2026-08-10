package cache

import (
	"sync"
	"testing"

	domaincache "cognida/internal/model/cache"
)

// TestHotReloader_ConcurrentReloadAndRead 覆盖 H11：热重载写入必须与读者共用 ff.mu，
// 否则 ReloadFromStrategy 重置 agentFlags map 时与 IsEnabled/GetAll/GetMetrics 的读迭代
// 构成 concurrent map read+write。需在 -race 下运行以捕获回归。
func TestHotReloader_ConcurrentReloadAndRead(t *testing.T) {
	hr := NewHotReloader(&domaincache.AgentCacheStrategy{
		Global: domaincache.SemanticCacheConfig{Enabled: true},
		Agents: map[string]domaincache.AgentCacheConfig{},
	})
	ff := hr.GetFeatureFlag()

	const iterations = 2000
	var wg sync.WaitGroup

	// 写者：反复热重载，每轮替换整份 agentFlags map
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			hr.ReloadFromStrategy(&domaincache.AgentCacheStrategy{
				Global: domaincache.SemanticCacheConfig{Enabled: i%2 == 0},
				Agents: map[string]domaincache.AgentCacheConfig{
					"rag_agent":  {Enabled: true},
					"chat_agent": {Enabled: i%3 == 0},
				},
			})
			_ = hr.ToggleAgent("rag_agent", i%2 == 0)
			_ = hr.ToggleGlobal(i%2 == 1)
		}
	}()

	// 读者：并发读，触发对 agentFlags 的迭代
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = ff.IsEnabled("rag_agent")
				_ = ff.GetAll()
				_ = ff.GetMetrics()
			}
		}()
	}

	wg.Wait()
}

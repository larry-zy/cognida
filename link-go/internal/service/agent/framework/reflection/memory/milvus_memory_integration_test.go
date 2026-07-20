//go:build integration

// Package memory 的集成测试：验证自我进化记忆闭环在真实 Milvus + 真实 embedder 下端到端可用。
// 运行：
//
//	cd link-go && set -a && source .env && set +a && \
//	  go test -tags=integration ./internal/service/agent/framework/reflection/memory/ -run TestMilvusReflectionMemory_Integration -v
package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"link/internal/infrastructure/config"
	embeddingfactory "link/internal/infrastructure/llm/embedding"
	reflectionmodel "link/internal/model/agent/reflection"
	"link/internal/repository/milvus"
)

// TestMilvusReflectionMemory_Integration 覆盖内联反思 Hook 记忆的真实基础设施路径：
// 建 embedder → 探测维度 → 建 MilvusReflectionMemory（真实建集合/索引）→ Store 一条失败经验 →
// Retrieve 按 agent+语义命中。证明反思记忆的「沉淀→检索」闭环在真实 Milvus 下打通。
func TestMilvusReflectionMemory_Integration(t *testing.T) {
	ctx := context.Background()

	// 1) 真实 Milvus 连接（.env 需已 source 到进程环境）。
	require.NoError(t, milvus.InitMilvus(config.LoadMilvusConfig()), "Milvus 初始化失败，请确认本地 Milvus 已启动且 .env 已 source")
	t.Cleanup(func() { _ = milvus.CloseMilvus() })

	// 2) 真实 embedder（DashScope）。
	embedder, err := embeddingfactory.NewEmbedder(config.LoadEmbeddingConfig())
	require.NoError(t, err, "创建 embedder 失败")

	// 3) 探测维度校正 VectorDim（避免 Milvus 插入维度不匹配）。
	vecs, err := embedder.EmbedStrings(ctx, []string{"dim probe"})
	require.NoError(t, err)
	require.NotEmpty(t, vecs)
	dim := len(vecs[0])
	require.Positive(t, dim)
	t.Logf("embedder 输出维度=%d", dim)

	memCfg := reflectionmodel.DefaultReflectionConfig().Memory
	memCfg.VectorDim = dim

	// 4) 建 MilvusReflectionMemory（真实建集合/索引，幂等）。
	mem, err := NewMilvusReflectionMemory(embedder, memCfg)
	require.NoError(t, err, "创建 MilvusReflectionMemory 失败")

	// 5) Store 一条失败经验（Retrieve 只召回 success==false 的记录）。
	// agentID 带纳秒后缀隔离本次运行，避免历史脏数据干扰断言。
	agentID := "it-agent-" + time.Now().Format("20060102150405.000000000")
	task := "统计上月各品类销售额并给出同比"
	record := &reflectionmodel.ReflectionRecord{
		AgentID:    agentID,
		Task:       task,
		Lesson:     "同比需对齐去年同月口径，勿混用自然月与财月",
		Success:    false,
		Iterations: 3,
	}
	require.NoError(t, mem.Store(ctx, record), "Store 失败")
	require.NotEmpty(t, record.ID)

	// 6) Retrieve 按 agent + 语义召回。
	got, err := mem.Retrieve(ctx, agentID, task, 3)
	require.NoError(t, err, "Retrieve 失败")
	require.NotEmpty(t, got, "应召回刚存入的失败经验")

	var found bool
	for _, r := range got {
		if r.ID == record.ID {
			found = true
			require.Equal(t, agentID, r.AgentID)
			require.Equal(t, record.Lesson, r.Lesson)
			require.False(t, r.Success)
		}
	}
	require.True(t, found, "召回结果应包含本次存入的记录 ID=%s", record.ID)
}

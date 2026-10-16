package neo4j

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"cognida/internal/model/knowledge"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

var _ knowledge.GraphRelationStatsRepository = (*Neo4jRepository)(nil)

// relationStatsQuery counts each directed relationship once, including a
// self-loop, and excludes malformed edges crossing a tenant or KB boundary.
// chunk_ids is JSON text in writeGraphs; grouping it keeps aggregation in Neo4j
// while permitting strict JSON decoding without requiring an APOC installation.
const relationStatsQuery = `
	MATCH (s:Entity {tenant_id: $tenantId, kb_id: $kbId})
	      -[r:RELATION {tenant_id: $tenantId, kb_id: $kbId}]->
	      (t:Entity {tenant_id: $tenantId, kb_id: $kbId})
	RETURN coalesce(r.type, '') AS relationType,
	       toInteger(coalesce(r.strength, 0)) AS strengthBucket,
	       r.chunk_ids AS chunkIDs,
	       count(r) AS relationCount,
	       sum(coalesce(r.strength, 0)) AS strengthSum,
	       sum(coalesce(r.weight, 0)) AS weightSum
`

func (r *Neo4jRepository) GetRelationStats(ctx context.Context, namespace knowledge.NameSpace) (*knowledge.RelationStatistics, error) {
	tenantID, err := strconv.ParseInt(namespace.TenantID, 10, 64)
	if err != nil || tenantID <= 0 || strings.TrimSpace(namespace.KnowledgeBaseID) == "" {
		return nil, fmt.Errorf("关系统计需要有效的租户和知识库范围")
	}
	if r == nil || r.driver == nil {
		return nil, fmt.Errorf("Neo4j 关系统计仓储未配置")
	}
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName, AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.Run(ctx, relationStatsQuery, map[string]interface{}{
		"tenantId": namespace.TenantID, "kbId": namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("聚合图谱关系失败: %w", err)
	}
	stats := &knowledge.RelationStatistics{StrengthDistribution: map[int]int{}, TypeDistribution: map[string]int{}}
	var strengthSum, weightSum, chunkSum float64
	for result.Next(ctx) {
		record := result.Record()
		kind, ok := relationStatsValue(record, "relationType").(string)
		if !ok {
			return nil, fmt.Errorf("图谱关系类型不是字符串")
		}
		bucket, ok := relationStatsValue(record, "strengthBucket").(int64)
		if !ok || int64(int(bucket)) != bucket {
			return nil, fmt.Errorf("图谱关系强度分桶无效")
		}
		count, ok := relationStatsValue(record, "relationCount").(int64)
		if !ok || count <= 0 || count > int64(math.MaxInt-stats.TotalRelations) {
			return nil, fmt.Errorf("图谱关系计数无效或溢出")
		}
		strength, err := relationStatsNumber(relationStatsValue(record, "strengthSum"))
		if err != nil {
			return nil, fmt.Errorf("图谱关系强度无效: %w", err)
		}
		weight, err := relationStatsNumber(relationStatsValue(record, "weightSum"))
		if err != nil {
			return nil, fmt.Errorf("图谱关系权重无效: %w", err)
		}
		chunks, err := relationStatsChunkCount(relationStatsValue(record, "chunkIDs"))
		if err != nil {
			return nil, fmt.Errorf("图谱关系分块引用无效: %w", err)
		}
		stats.TotalRelations += int(count)
		stats.StrengthDistribution[int(bucket)] += int(count)
		stats.TypeDistribution[kind] += int(count)
		strengthSum += strength
		weightSum += weight
		chunkSum += float64(chunks) * float64(count)
	}
	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("读取图谱关系统计失败: %w", err)
	}
	if math.IsNaN(strengthSum) || math.IsInf(strengthSum, 0) || math.IsNaN(weightSum) || math.IsInf(weightSum, 0) || math.IsInf(chunkSum, 0) {
		return nil, fmt.Errorf("图谱关系统计聚合溢出")
	}
	if stats.TotalRelations > 0 {
		stats.AverageStrength = strengthSum / float64(stats.TotalRelations)
		stats.AverageWeight = weightSum / float64(stats.TotalRelations)
		stats.AverageChunkCount = chunkSum / float64(stats.TotalRelations)
	}
	return stats, nil
}

func relationStatsValue(record *neo4j.Record, key string) any {
	if record == nil {
		return nil
	}
	value, _ := record.Get(key)
	return value
}

func relationStatsNumber(value any) (float64, error) {
	var number float64
	switch v := value.(type) {
	case int64:
		number = float64(v)
	case float64:
		number = v
	default:
		return 0, fmt.Errorf("预期数值聚合结果")
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("预期有限数值")
	}
	return number, nil
}

func relationStatsChunkCount(value any) (int, error) {
	switch v := value.(type) {
	case nil:
		return 0, nil
	case string:
		// Decode as generic JSON first: decoding [null] into []string would
		// silently turn the corrupt reference into an empty string.
		var decoded any
		if err := json.Unmarshal([]byte(v), &decoded); err != nil {
			return 0, err
		}
		if decoded == nil {
			return 0, nil
		}
		items, ok := decoded.([]any)
		if !ok {
			return 0, fmt.Errorf("预期 JSON 字符串数组")
		}
		return relationStatsChunkCount(items)
	case []any:
		for _, item := range v {
			if _, ok := item.(string); !ok {
				return 0, fmt.Errorf("分块引用必须为字符串")
			}
		}
		return len(v), nil
	case []string:
		return len(v), nil
	default:
		return 0, fmt.Errorf("预期分块引用数组或 JSON 文本")
	}
}

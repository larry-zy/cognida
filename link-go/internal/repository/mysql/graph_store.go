// Package mysql 提供 Graph 领域的 MySQL 仓储实现。
//
// graph_store.go 实现了一个完整的、基于 MySQL 的图谱存储，作为 Neo4j 不可用时的
// 降级方案。它在关系表上重建邻接结构，并在 Go 侧计算图算法（最短路径、K 最短
// 路径、连通分量、聚类系数等），从而在没有 Neo4j 的部署中依然提供图谱能力。
//
// 存储模型（4 张表）：
//   - graph_nodes             实体节点，按 (tenant_id, kb_id, name) 唯一
//   - graph_relations         关系边，按 (tenant_id, kb_id, rel_id) 唯一
//   - graph_communities       社区检测结果
//   - graph_community_members 社区成员关系
//
// 语义对齐 Neo4j 实现（internal/repository/neo4j/graph_repo.go）：
//   - 节点按 name 在 scope 内合并：chunks/attributes 取并集
//   - 关系按 id 合并：chunk_ids 取并集，strength 取均值
//   - 空 ID 的节点/关系在批量写入时跳过
package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domain_knowledge "link/internal/model/knowledge"
)

// ========================================
// GORM Models
// ========================================

// graphNodeModel 图谱节点持久化模型
type graphNodeModel struct {
	ID          uint    `gorm:"primaryKey;autoIncrement"`
	TenantID    string  `gorm:"column:tenant_id;size:64;index:idx_gn_scope,priority:1;uniqueIndex:uq_gn_name,priority:1"`
	KBID        string  `gorm:"column:kb_id;size:64;index:idx_gn_scope,priority:2;uniqueIndex:uq_gn_name,priority:2"`
	NodeID      string  `gorm:"column:node_id;size:128;index"`
	Name        string  `gorm:"column:name;size:191;uniqueIndex:uq_gn_name,priority:3"`
	EntityType  string  `gorm:"column:entity_type;size:128;index"`
	Attributes  string  `gorm:"column:attributes;type:json"`
	Chunks      string  `gorm:"column:chunks;type:json"`
	Properties  string  `gorm:"column:properties;type:json"`
	Pagerank    float64 `gorm:"column:pagerank"`
	Betweenness float64 `gorm:"column:betweenness"`
	CommunityID string  `gorm:"column:community_id;size:128;index"`
}

// TableName 指定表名
func (graphNodeModel) TableName() string { return "graph_nodes" }

// graphRelationModel 图谱关系持久化模型
type graphRelationModel struct {
	ID             uint    `gorm:"primaryKey;autoIncrement"`
	TenantID       string  `gorm:"column:tenant_id;size:64;index:idx_gr_scope,priority:1;uniqueIndex:uq_gr_id,priority:1"`
	KBID           string  `gorm:"column:kb_id;size:64;index:idx_gr_scope,priority:2;uniqueIndex:uq_gr_id,priority:2"`
	RelID          string  `gorm:"column:rel_id;size:128;uniqueIndex:uq_gr_id,priority:3"`
	Source         string  `gorm:"column:source;size:191;index"`
	Target         string  `gorm:"column:target;size:191;index"`
	Type           string  `gorm:"column:type;size:128;index"`
	Strength       float64 `gorm:"column:strength"`
	Weight         float64 `gorm:"column:weight"`
	ChunkIDs       string  `gorm:"column:chunk_ids;type:json"`
	Properties     string  `gorm:"column:properties;type:json"`
	CombinedDegree int     `gorm:"column:combined_degree"`
	PMI            float64 `gorm:"column:pmi"`
	Description    string  `gorm:"column:description;type:text"`
}

// TableName 指定表名
func (graphRelationModel) TableName() string { return "graph_relations" }

// graphCommunityModel 社区持久化模型
type graphCommunityModel struct {
	ID          uint      `gorm:"primaryKey;autoIncrement"`
	TenantID    string    `gorm:"column:tenant_id;size:64;index:idx_gc_scope,priority:1;uniqueIndex:uq_gc_id,priority:1"`
	KBID        string    `gorm:"column:kb_id;size:64;index:idx_gc_scope,priority:2;uniqueIndex:uq_gc_id,priority:2"`
	CommunityID string    `gorm:"column:community_id;size:128;uniqueIndex:uq_gc_id,priority:3"`
	Name        string    `gorm:"column:name;size:255"`
	Size        int       `gorm:"column:size"`
	Modularity  float64   `gorm:"column:modularity"`
	Labels      string    `gorm:"column:labels;type:json"`
	Properties  string    `gorm:"column:properties;type:json"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

// TableName 指定表名
func (graphCommunityModel) TableName() string { return "graph_communities" }

// graphCommunityMemberModel 社区成员持久化模型
type graphCommunityMemberModel struct {
	ID              uint    `gorm:"primaryKey;autoIncrement"`
	TenantID        string  `gorm:"column:tenant_id;size:64;index:idx_gcm_scope,priority:1;uniqueIndex:uq_gcm,priority:1"`
	KBID            string  `gorm:"column:kb_id;size:64;index:idx_gcm_scope,priority:2;uniqueIndex:uq_gcm,priority:2"`
	CommunityID     string  `gorm:"column:community_id;size:128;index;uniqueIndex:uq_gcm,priority:3"`
	EntityID        string  `gorm:"column:entity_id;size:128;uniqueIndex:uq_gcm,priority:4"`
	EntityName      string  `gorm:"column:entity_name;size:191;index"`
	MembershipScore float64 `gorm:"column:membership_score"`
}

// TableName 指定表名
func (graphCommunityMemberModel) TableName() string { return "graph_community_members" }

// ========================================
// JSON helpers
// ========================================

func marshalSlice(s []string) string {
	if len(s) == 0 {
		return "[]"
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalSlice(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func marshalStringMap(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func unmarshalStringMap(s string) map[string]string {
	if s == "" {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// defaultSearchNodeLimit 是 SearchNodes 在调用方未显式指定 Limit 时的默认上限，
// 防止一次性物化整个 namespace 的节点。
const defaultSearchNodeLimit = 200

// escapeLike 转义 MySQL LIKE 的通配符（% _）及转义符（\），使待匹配文本按字面量
// 参与 LIKE，而非被当作通配符放大命中范围。
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// jsonElemPattern 构造在 JSON 字符串数组列（形如 ["a","b"]）上做 LIKE 预筛选的
// 模式，匹配 val 作为独立数组元素（被双引号包裹）出现。闭合的双引号可避免 "c1"
// 误命中 "c12"。该模式仅用于缩小候选集，精确判定由调用方在 Go 侧按元素比对完成。
func jsonElemPattern(val string) string {
	return "%\"" + escapeLike(val) + "\"%"
}

// unionStrings 返回 a、b 去重后的并集，保持 a 的顺序在前。
func unionStrings(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, v := range a {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	for _, v := range b {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// ========================================
// Model <-> Domain converters
// ========================================

func (m *graphNodeModel) toDomain() *domain_knowledge.GraphNode {
	return &domain_knowledge.GraphNode{
		ID:         m.NodeID,
		Name:       m.Name,
		EntityType: m.EntityType,
		Attributes: unmarshalSlice(m.Attributes),
		Chunks:     unmarshalSlice(m.Chunks),
		Properties: unmarshalStringMap(m.Properties),
	}
}

func nodeModelFromDomain(ns domain_knowledge.NameSpace, n *domain_knowledge.GraphNode) *graphNodeModel {
	return &graphNodeModel{
		TenantID:    ns.TenantID,
		KBID:        ns.KnowledgeBaseID,
		NodeID:      n.ID,
		Name:        n.Name,
		EntityType:  n.EntityType,
		Attributes:  marshalSlice(n.Attributes),
		Chunks:      marshalSlice(n.Chunks),
		Properties:  marshalStringMap(n.Properties),
		CommunityID: "",
	}
}

func (m *graphRelationModel) toDomain() *domain_knowledge.GraphRelation {
	return &domain_knowledge.GraphRelation{
		ID:             m.RelID,
		Source:         m.Source,
		Target:         m.Target,
		Type:           m.Type,
		Strength:       m.Strength,
		Weight:         m.Weight,
		ChunkIDs:       unmarshalSlice(m.ChunkIDs),
		Properties:     unmarshalStringMap(m.Properties),
		CombinedDegree: m.CombinedDegree,
		PMI:            m.PMI,
		Description:    m.Description,
	}
}

func relationModelFromDomain(ns domain_knowledge.NameSpace, r *domain_knowledge.GraphRelation) *graphRelationModel {
	return &graphRelationModel{
		TenantID:       ns.TenantID,
		KBID:           ns.KnowledgeBaseID,
		RelID:          r.ID,
		Source:         r.Source,
		Target:         r.Target,
		Type:           r.Type,
		Strength:       r.Strength,
		Weight:         r.Weight,
		ChunkIDs:       marshalSlice(r.ChunkIDs),
		Properties:     marshalStringMap(r.Properties),
		CombinedDegree: r.CombinedDegree,
		PMI:            r.PMI,
		Description:    r.Description,
	}
}

// ========================================
// Repository
// ========================================

// graphMetaRepository 基于 MySQL 的图谱仓储实现（Neo4j 降级方案）。
type graphMetaRepository struct {
	db          *gorm.DB
	migrateOnce sync.Once
	migrateErr  error
}

// NewGraphMetaRepository 创建基于 MySQL 的图谱仓储。
func NewGraphMetaRepository(db *gorm.DB) domain_knowledge.GraphRepository {
	return &graphMetaRepository{db: db}
}

// ensureSchema 懒加载式建表，确保图谱相关表存在（幂等）。
func (r *graphMetaRepository) ensureSchema(ctx context.Context) error {
	r.migrateOnce.Do(func() {
		r.migrateErr = r.db.WithContext(ctx).AutoMigrate(
			&graphNodeModel{},
			&graphRelationModel{},
			&graphCommunityModel{},
			&graphCommunityMemberModel{},
		)
	})
	return r.migrateErr
}

// scope 返回限定到当前 namespace 的查询。
func (r *graphMetaRepository) scope(ctx context.Context, ns domain_knowledge.NameSpace, model interface{}) *gorm.DB {
	return r.db.WithContext(ctx).Model(model).
		Where("tenant_id = ? AND kb_id = ?", ns.TenantID, ns.KnowledgeBaseID)
}

// InitIndexes 初始化（建表）图谱存储，幂等。
func (r *graphMetaRepository) InitIndexes(ctx context.Context) error {
	return r.ensureSchema(ctx)
}

// ========================================
// Write Operations
// ========================================

// AddGraph 批量写入图谱数据，节点按 name 合并、关系按 id 合并。
func (r *graphMetaRepository) AddGraph(ctx context.Context, ns domain_knowledge.NameSpace, graphs []*domain_knowledge.GraphData) error {
	if err := r.ensureSchema(ctx); err != nil {
		return err
	}
	if len(graphs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, g := range graphs {
			if g == nil {
				continue
			}
			for _, node := range g.Node {
				if node == nil || node.ID == "" || node.Name == "" {
					continue // 跳过空 ID/空名称节点，对齐 Neo4j 语义
				}
				if err := r.upsertNodeTx(ctx, tx, ns, node); err != nil {
					return err
				}
			}
			for _, rel := range g.Relation {
				if rel == nil || rel.ID == "" {
					continue // 跳过空 ID 关系，对齐 Neo4j 语义
				}
				if err := r.upsertRelationTx(ctx, tx, ns, rel); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// upsertNodeTx 在事务内按 name 合并写入单个节点。
func (r *graphMetaRepository) upsertNodeTx(ctx context.Context, tx *gorm.DB, ns domain_knowledge.NameSpace, node *domain_knowledge.GraphNode) error {
	var existing graphNodeModel
	// FOR UPDATE：在唯一索引 (tenant_id, kb_id, name) 上加 next-key 锁，使并发
	// AddGraph 对同一节点的 read-then-merge 串行化，避免重复键插入失败。
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_id = ? AND kb_id = ? AND name = ?", ns.TenantID, ns.KnowledgeBaseID, node.Name).
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		m := nodeModelFromDomain(ns, node)
		return tx.WithContext(ctx).Create(m).Error
	}
	if err != nil {
		return fmt.Errorf("查询节点失败: %w", err)
	}

	// 合并：chunks/attributes 取并集，entity_type/properties 在有值时更新。
	existing.Chunks = marshalSlice(unionStrings(unmarshalSlice(existing.Chunks), node.Chunks))
	existing.Attributes = marshalSlice(unionStrings(unmarshalSlice(existing.Attributes), node.Attributes))
	if node.EntityType != "" {
		existing.EntityType = node.EntityType
	}
	if node.ID != "" {
		existing.NodeID = node.ID
	}
	if len(node.Properties) > 0 {
		merged := unmarshalStringMap(existing.Properties)
		if merged == nil {
			merged = make(map[string]string)
		}
		for k, v := range node.Properties {
			merged[k] = v
		}
		existing.Properties = marshalStringMap(merged)
	}
	return tx.WithContext(ctx).Save(&existing).Error
}

// upsertRelationTx 在事务内按 id 合并写入单个关系。
func (r *graphMetaRepository) upsertRelationTx(ctx context.Context, tx *gorm.DB, ns domain_knowledge.NameSpace, rel *domain_knowledge.GraphRelation) error {
	var existing graphRelationModel
	// FOR UPDATE：与节点 upsert 同理，锁住唯一索引行避免并发重复键。
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_id = ? AND kb_id = ? AND rel_id = ?", ns.TenantID, ns.KnowledgeBaseID, rel.ID).
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		m := relationModelFromDomain(ns, rel)
		return tx.WithContext(ctx).Create(m).Error
	}
	if err != nil {
		return fmt.Errorf("查询关系失败: %w", err)
	}

	existing.ChunkIDs = marshalSlice(unionStrings(unmarshalSlice(existing.ChunkIDs), rel.ChunkIDs))
	// strength 取均值，对齐 Neo4j (old+new)/2
	if existing.Strength > 0 && rel.Strength > 0 {
		existing.Strength = (existing.Strength + rel.Strength) / 2
	} else if rel.Strength > 0 {
		existing.Strength = rel.Strength
	}
	if rel.Weight > 0 {
		existing.Weight = rel.Weight
	}
	if rel.Type != "" {
		existing.Type = rel.Type
	}
	if rel.Source != "" {
		existing.Source = rel.Source
	}
	if rel.Target != "" {
		existing.Target = rel.Target
	}
	if rel.Description != "" {
		existing.Description = rel.Description
	}
	if rel.CombinedDegree != 0 {
		existing.CombinedDegree = rel.CombinedDegree
	}
	if rel.PMI != 0 {
		existing.PMI = rel.PMI
	}
	return tx.WithContext(ctx).Save(&existing).Error
}

// AddNode 写入单个节点（合并语义）。
func (r *graphMetaRepository) AddNode(ctx context.Context, ns domain_knowledge.NameSpace, node *domain_knowledge.GraphNode) error {
	if err := r.ensureSchema(ctx); err != nil {
		return err
	}
	if node == nil || node.Name == "" {
		return fmt.Errorf("节点名称不能为空")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return r.upsertNodeTx(ctx, tx, ns, node)
	})
}

// AddRelation 写入单个关系（合并语义），返回存储后的关系。
func (r *graphMetaRepository) AddRelation(ctx context.Context, ns domain_knowledge.NameSpace, relation *domain_knowledge.GraphRelation) (*domain_knowledge.GraphRelation, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	if relation == nil || relation.ID == "" {
		return nil, fmt.Errorf("关系 ID 不能为空")
	}
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return r.upsertRelationTx(ctx, tx, ns, relation)
	}); err != nil {
		return nil, err
	}
	var stored graphRelationModel
	if err := r.scope(ctx, ns, &graphRelationModel{}).
		Where("rel_id = ?", relation.ID).First(&stored).Error; err != nil {
		return nil, err
	}
	return stored.toDomain(), nil
}

// UpdateNode 更新节点属性（按 name 定位）。
func (r *graphMetaRepository) UpdateNode(ctx context.Context, ns domain_knowledge.NameSpace, node *domain_knowledge.GraphNode) error {
	if err := r.ensureSchema(ctx); err != nil {
		return err
	}
	if node == nil || node.Name == "" {
		return fmt.Errorf("节点名称不能为空")
	}
	updates := map[string]interface{}{
		"entity_type": node.EntityType,
		"attributes":  marshalSlice(node.Attributes),
		"chunks":      marshalSlice(node.Chunks),
		"properties":  marshalStringMap(node.Properties),
	}
	if node.ID != "" {
		updates["node_id"] = node.ID
	}
	res := r.scope(ctx, ns, &graphNodeModel{}).Where("name = ?", node.Name).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("节点不存在: %s", node.Name)
	}
	return nil
}

// UpdateRelation 更新关系属性（按 rel_id 定位），返回更新后的关系。
func (r *graphMetaRepository) UpdateRelation(ctx context.Context, ns domain_knowledge.NameSpace, relation *domain_knowledge.GraphRelation) (*domain_knowledge.GraphRelation, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	if relation == nil || relation.ID == "" {
		return nil, fmt.Errorf("关系 ID 不能为空")
	}
	updates := map[string]interface{}{
		"source":          relation.Source,
		"target":          relation.Target,
		"type":            relation.Type,
		"strength":        relation.Strength,
		"weight":          relation.Weight,
		"chunk_ids":       marshalSlice(relation.ChunkIDs),
		"properties":      marshalStringMap(relation.Properties),
		"combined_degree": relation.CombinedDegree,
		"pmi":             relation.PMI,
		"description":     relation.Description,
	}
	res := r.scope(ctx, ns, &graphRelationModel{}).Where("rel_id = ?", relation.ID).Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, fmt.Errorf("关系不存在: %s", relation.ID)
	}
	var stored graphRelationModel
	if err := r.scope(ctx, ns, &graphRelationModel{}).Where("rel_id = ?", relation.ID).First(&stored).Error; err != nil {
		return nil, err
	}
	return stored.toDomain(), nil
}

// DeleteNode 删除节点（按 node_id 或 name），并删除与之相连的关系。
func (r *graphMetaRepository) DeleteNode(ctx context.Context, ns domain_knowledge.NameSpace, nodeID string) error {
	if err := r.ensureSchema(ctx); err != nil {
		return err
	}
	var node graphNodeModel
	err := r.scope(ctx, ns, &graphNodeModel{}).
		Where("node_id = ? OR name = ?", nodeID, nodeID).First(&node).Error
	if err == gorm.ErrRecordNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND kb_id = ? AND (source = ? OR target = ?)",
			ns.TenantID, ns.KnowledgeBaseID, node.Name, node.Name).
			Delete(&graphRelationModel{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id = ? AND kb_id = ? AND id = ?", ns.TenantID, ns.KnowledgeBaseID, node.ID).
			Delete(&graphNodeModel{}).Error
	})
}

// DeleteRelation 删除单个关系（按 rel_id）。
func (r *graphMetaRepository) DeleteRelation(ctx context.Context, ns domain_knowledge.NameSpace, relationID string) error {
	if err := r.ensureSchema(ctx); err != nil {
		return err
	}
	return r.scope(ctx, ns, &graphRelationModel{}).Where("rel_id = ?", relationID).
		Delete(&graphRelationModel{}).Error
}

// DeleteByScope 按 scope 删除 (kb_id 或 knowledge_id)。
func (r *graphMetaRepository) DeleteByScope(ctx context.Context, scopeType string, scopeID string) error {
	if err := r.ensureSchema(ctx); err != nil {
		return err
	}
	switch scopeType {
	case "kb_id":
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("kb_id = ?", scopeID).Delete(&graphRelationModel{}).Error; err != nil {
				return err
			}
			if err := tx.Where("kb_id = ?", scopeID).Delete(&graphNodeModel{}).Error; err != nil {
				return err
			}
			if err := tx.Where("kb_id = ?", scopeID).Delete(&graphCommunityMemberModel{}).Error; err != nil {
				return err
			}
			return tx.Where("kb_id = ?", scopeID).Delete(&graphCommunityModel{}).Error
		})
	case "knowledge_id":
		// 删除 chunks 中存在以 scopeID 为前缀的节点（对齐 Neo4j STARTS WITH 语义）。
		// 注意：与 Neo4j 实现一致，本方法不携带 tenant/kb（接口未提供），按全局 scope 匹配。
		// 关系删除按"每个被删节点所在 (tenant_id, kb_id)"分组，等价于 Neo4j 的 DETACH DELETE，
		// 避免不同租户/知识库下同名节点的关系被误删。
		var nodes []graphNodeModel
		if err := r.db.WithContext(ctx).Find(&nodes).Error; err != nil {
			return err
		}
		// scopeKey -> 该 scope 下命中的节点名集合与行 id 集合。
		type scopeDel struct {
			tenantID string
			kbID     string
			names    []string
			ids      []uint
		}
		matched := make(map[string]*scopeDel)
		for i := range nodes {
			hit := false
			for _, c := range unmarshalSlice(nodes[i].Chunks) {
				if strings.HasPrefix(c, scopeID) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
			key := nodes[i].TenantID + "\x00" + nodes[i].KBID
			sd := matched[key]
			if sd == nil {
				sd = &scopeDel{tenantID: nodes[i].TenantID, kbID: nodes[i].KBID}
				matched[key] = sd
			}
			sd.names = append(sd.names, nodes[i].Name)
			sd.ids = append(sd.ids, nodes[i].ID)
		}
		if len(matched) == 0 {
			return nil
		}
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for _, sd := range matched {
				if err := tx.Where("tenant_id = ? AND kb_id = ? AND (source IN ? OR target IN ?)",
					sd.tenantID, sd.kbID, sd.names, sd.names).Delete(&graphRelationModel{}).Error; err != nil {
					return err
				}
				if err := tx.Where("id IN ?", sd.ids).Delete(&graphNodeModel{}).Error; err != nil {
					return err
				}
			}
			return nil
		})
	default:
		return fmt.Errorf("不支持的 scope 类型: %s", scopeType)
	}
}

// DeleteByChunkID 从节点/关系中剔除指定 chunkID，并清理孤立节点及其关系。
func (r *graphMetaRepository) DeleteByChunkID(ctx context.Context, ns domain_knowledge.NameSpace, chunkID string) error {
	if err := r.ensureSchema(ctx); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 仅加载 chunks JSON 中可能包含该 chunkID 的候选节点，避免全量扫描整个
		// namespace。LIKE 只用于缩小候选集，精确判定仍在 Go 侧逐元素比对，因此
		// 即便 LIKE 命中多余行也不影响正确性。
		var nodes []graphNodeModel
		if err := tx.Where("tenant_id = ? AND kb_id = ? AND chunks LIKE ?",
			ns.TenantID, ns.KnowledgeBaseID, jsonElemPattern(chunkID)).Find(&nodes).Error; err != nil {
			return err
		}
		emptyNodeNames := make(map[string]struct{})
		for i := range nodes {
			chunks := unmarshalSlice(nodes[i].Chunks)
			filtered := make([]string, 0, len(chunks))
			changed := false
			for _, c := range chunks {
				if c == chunkID {
					changed = true
					continue
				}
				filtered = append(filtered, c)
			}
			if !changed {
				continue
			}
			if len(filtered) == 0 {
				emptyNodeNames[nodes[i].Name] = struct{}{}
				if err := tx.Delete(&graphNodeModel{}, nodes[i].ID).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Model(&graphNodeModel{}).Where("id = ?", nodes[i].ID).
				Update("chunks", marshalSlice(filtered)).Error; err != nil {
				return err
			}
		}

		// 批量删除连接到已删除（孤立）节点的关系，一条语句替代逐行删除。
		if len(emptyNodeNames) > 0 {
			names := make([]string, 0, len(emptyNodeNames))
			for n := range emptyNodeNames {
				names = append(names, n)
			}
			if err := tx.Where("tenant_id = ? AND kb_id = ? AND (source IN ? OR target IN ?)",
				ns.TenantID, ns.KnowledgeBaseID, names, names).
				Delete(&graphRelationModel{}).Error; err != nil {
				return err
			}
		}

		// 从仍存活、且 chunk_ids 中包含该 chunkID 的关系里剔除该 chunkID（同样按
		// LIKE 预筛选候选集）。连接到已删除节点的关系已在上一步移除，这里跳过。
		var rels []graphRelationModel
		if err := tx.Where("tenant_id = ? AND kb_id = ? AND chunk_ids LIKE ?",
			ns.TenantID, ns.KnowledgeBaseID, jsonElemPattern(chunkID)).Find(&rels).Error; err != nil {
			return err
		}
		for i := range rels {
			if _, gone := emptyNodeNames[rels[i].Source]; gone {
				continue
			}
			if _, gone := emptyNodeNames[rels[i].Target]; gone {
				continue
			}
			cids := unmarshalSlice(rels[i].ChunkIDs)
			filtered := make([]string, 0, len(cids))
			changed := false
			for _, c := range cids {
				if c == chunkID {
					changed = true
					continue
				}
				filtered = append(filtered, c)
			}
			if changed {
				if err := tx.Model(&graphRelationModel{}).Where("id = ?", rels[i].ID).
					Update("chunk_ids", marshalSlice(filtered)).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// DeleteByKnowledgeID 按 knowledge_id 删除（对齐 Neo4j，复用 chunk 前缀语义）。
func (r *graphMetaRepository) DeleteByKnowledgeID(ctx context.Context, ns domain_knowledge.NameSpace, knowledgeID string) error {
	return r.DeleteByChunkID(ctx, ns, knowledgeID)
}

// DeleteGraph 删除指定 namespace 下的全部图谱数据。
func (r *graphMetaRepository) DeleteGraph(ctx context.Context, namespaces []domain_knowledge.NameSpace) error {
	if err := r.ensureSchema(ctx); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, ns := range namespaces {
			cond := tx.Where("tenant_id = ? AND kb_id = ?", ns.TenantID, ns.KnowledgeBaseID)
			if err := cond.Session(&gorm.Session{}).Delete(&graphRelationModel{}).Error; err != nil {
				return err
			}
			if err := tx.Where("tenant_id = ? AND kb_id = ?", ns.TenantID, ns.KnowledgeBaseID).Delete(&graphNodeModel{}).Error; err != nil {
				return err
			}
			if err := tx.Where("tenant_id = ? AND kb_id = ?", ns.TenantID, ns.KnowledgeBaseID).Delete(&graphCommunityMemberModel{}).Error; err != nil {
				return err
			}
			if err := tx.Where("tenant_id = ? AND kb_id = ?", ns.TenantID, ns.KnowledgeBaseID).Delete(&graphCommunityModel{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ========================================
// Query Operations
// ========================================

// loadNodes 加载 namespace 下全部节点。
func (r *graphMetaRepository) loadNodes(ctx context.Context, ns domain_knowledge.NameSpace) ([]graphNodeModel, error) {
	var nodes []graphNodeModel
	err := r.scope(ctx, ns, &graphNodeModel{}).Find(&nodes).Error
	return nodes, err
}

// loadRelations 加载 namespace 下全部关系。
func (r *graphMetaRepository) loadRelations(ctx context.Context, ns domain_knowledge.NameSpace) ([]graphRelationModel, error) {
	var rels []graphRelationModel
	err := r.scope(ctx, ns, &graphRelationModel{}).Find(&rels).Error
	return rels, err
}

// GetGraph 获取完整图谱数据。
func (r *graphMetaRepository) GetGraph(ctx context.Context, ns domain_knowledge.NameSpace) (*domain_knowledge.GraphData, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	nodes, err := r.loadNodes(ctx, ns)
	if err != nil {
		return nil, err
	}
	rels, err := r.loadRelations(ctx, ns)
	if err != nil {
		return nil, err
	}
	data := &domain_knowledge.GraphData{
		Node:     make([]*domain_knowledge.GraphNode, 0, len(nodes)),
		Relation: make([]*domain_knowledge.GraphRelation, 0, len(rels)),
	}
	for i := range nodes {
		data.Node = append(data.Node, nodes[i].toDomain())
	}
	for i := range rels {
		data.Relation = append(data.Relation, rels[i].toDomain())
	}
	return data, nil
}

// SearchNodes 按名称模糊匹配搜索节点，支持类型过滤与分页。
func (r *graphMetaRepository) SearchNodes(ctx context.Context, ns domain_knowledge.NameSpace, query string, opts *domain_knowledge.NodeQueryOptions) ([]*domain_knowledge.GraphNode, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	q := r.scope(ctx, ns, &graphNodeModel{})
	if query != "" {
		// 转义 LIKE 通配符，使 query 中的 %/_ 按字面量匹配，而非放大命中范围。
		q = q.Where("name LIKE ?", "%"+escapeLike(query)+"%")
	}
	// 默认对结果集封顶，避免调用方未传 Limit 时一次性物化整个 namespace 的节点。
	limit := defaultSearchNodeLimit
	offset := 0
	if opts != nil {
		if len(opts.EntityTypes) > 0 {
			q = q.Where("entity_type IN ?", opts.EntityTypes)
		}
		if opts.Limit > 0 {
			limit = opts.Limit
		}
		if opts.Offset > 0 {
			offset = opts.Offset
		}
	}
	q = q.Limit(limit).Offset(offset)
	var nodes []graphNodeModel
	if err := q.Find(&nodes).Error; err != nil {
		return nil, err
	}
	out := make([]*domain_knowledge.GraphNode, 0, len(nodes))
	for i := range nodes {
		out = append(out, nodes[i].toDomain())
	}
	return out, nil
}

// GetNeighbors 获取指定节点的邻居及连接关系。
func (r *graphMetaRepository) GetNeighbors(ctx context.Context, ns domain_knowledge.NameSpace, nodeName string, opts *domain_knowledge.RelationQueryOptions) (*domain_knowledge.NodeQueryResult, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	var center graphNodeModel
	err := r.scope(ctx, ns, &graphNodeModel{}).Where("name = ?", nodeName).First(&center).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("节点不存在: %s", nodeName)
	}
	if err != nil {
		return nil, err
	}

	relQuery := r.scope(ctx, ns, &graphRelationModel{}).Where("source = ? OR target = ?", nodeName, nodeName)
	if opts != nil {
		if len(opts.Types) > 0 {
			relQuery = relQuery.Where("type IN ?", opts.Types)
		}
		if opts.MinWeight > 0 {
			relQuery = relQuery.Where("weight >= ?", opts.MinWeight)
		}
		if opts.MinStrength > 0 {
			relQuery = relQuery.Where("strength >= ?", opts.MinStrength)
		}
	}
	var rels []graphRelationModel
	if err := relQuery.Find(&rels).Error; err != nil {
		return nil, err
	}

	degree := len(rels)

	// 分页（在内存中按 offset/limit 截取，确保 degree 反映全部连接数）。
	if opts != nil {
		if opts.Offset > 0 && opts.Offset < len(rels) {
			rels = rels[opts.Offset:]
		} else if opts != nil && opts.Offset >= len(rels) {
			rels = nil
		}
		if opts.Limit > 0 && opts.Limit < len(rels) {
			rels = rels[:opts.Limit]
		}
	}

	neighborNames := make([]string, 0, len(rels))
	nameSet := make(map[string]struct{})
	domainRels := make([]*domain_knowledge.GraphRelation, 0, len(rels))
	for i := range rels {
		domainRels = append(domainRels, rels[i].toDomain())
		other := rels[i].Target
		if rels[i].Target == nodeName {
			other = rels[i].Source
		}
		if other == "" || other == nodeName {
			continue
		}
		if _, ok := nameSet[other]; !ok {
			nameSet[other] = struct{}{}
			neighborNames = append(neighborNames, other)
		}
	}

	neighbors := make([]*domain_knowledge.GraphNode, 0, len(neighborNames))
	if len(neighborNames) > 0 {
		var nbModels []graphNodeModel
		if err := r.scope(ctx, ns, &graphNodeModel{}).Where("name IN ?", neighborNames).Find(&nbModels).Error; err != nil {
			return nil, err
		}
		for i := range nbModels {
			neighbors = append(neighbors, nbModels[i].toDomain())
		}
	}

	return &domain_knowledge.NodeQueryResult{
		Node:      center.toDomain(),
		Neighbors: neighbors,
		Relations: domainRels,
		Degree:    degree,
	}, nil
}

// GetNodeCommunity 获取节点所属社区。
func (r *graphMetaRepository) GetNodeCommunity(ctx context.Context, ns domain_knowledge.NameSpace, nodeName string) (*domain_knowledge.Community, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	var member graphCommunityMemberModel
	err := r.scope(ctx, ns, &graphCommunityMemberModel{}).Where("entity_name = ?", nodeName).First(&member).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("节点未归属任何社区: %s", nodeName)
	}
	if err != nil {
		return nil, err
	}
	var comm graphCommunityModel
	if err := r.scope(ctx, ns, &graphCommunityModel{}).Where("community_id = ?", member.CommunityID).First(&comm).Error; err != nil {
		return nil, err
	}
	return &domain_knowledge.Community{
		ID:              comm.CommunityID,
		KnowledgeBaseID: ns.KnowledgeBaseID,
		Name:            comm.Name,
		Size:            comm.Size,
		Modularity:      comm.Modularity,
		Labels:          unmarshalSlice(comm.Labels),
		CreatedAt:       comm.CreatedAt,
		Properties:      unmarshalStringMap(comm.Properties),
	}, nil
}

// GetEntityContext 获取增量抽取所需的实体上下文。
func (r *graphMetaRepository) GetEntityContext(ctx context.Context, ns domain_knowledge.NameSpace) (*domain_knowledge.ExtractionContext, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	nodes, err := r.loadNodes(ctx, ns)
	if err != nil {
		return nil, err
	}
	rels, err := r.loadRelations(ctx, ns)
	if err != nil {
		return nil, err
	}

	entityTypes := make(map[string]int)
	existing := make([]string, 0, len(nodes))
	samples := make([]domain_knowledge.EntitySample, 0, len(nodes))
	for i := range nodes {
		entityTypes[nodes[i].EntityType]++
		if nodes[i].Name != "" {
			existing = append(existing, nodes[i].Name)
			if len(samples) < 100 {
				samples = append(samples, domain_knowledge.EntitySample{
					Name:       nodes[i].Name,
					EntityType: nodes[i].EntityType,
					Attributes: unmarshalSlice(nodes[i].Attributes),
				})
			}
		}
	}

	relationTypes := make(map[string]int)
	for i := range rels {
		relationTypes[rels[i].Type]++
	}

	return &domain_knowledge.ExtractionContext{
		KnowledgeBaseID:  ns.KnowledgeBaseID,
		ExistingEntities: existing,
		EntityTypes:      entityTypes,
		RelationTypes:    relationTypes,
		SampleEntities:   samples,
	}, nil
}

// ========================================
// Graph Algorithms (computed in Go)
// ========================================

// adjacency 内存邻接表（无向），用于路径与图算法。
type adjacency struct {
	nodesByName map[string]*domain_knowledge.GraphNode
	edges       map[string][]adjEdge
}

type adjEdge struct {
	to  string
	rel *domain_knowledge.GraphRelation
}

// buildAdjacency 从 MySQL 载入并构建无向邻接表，可按关系类型过滤。
func (r *graphMetaRepository) buildAdjacency(ctx context.Context, ns domain_knowledge.NameSpace, relationTypes []string) (*adjacency, error) {
	nodes, err := r.loadNodes(ctx, ns)
	if err != nil {
		return nil, err
	}
	rels, err := r.loadRelations(ctx, ns)
	if err != nil {
		return nil, err
	}

	typeFilter := make(map[string]struct{}, len(relationTypes))
	for _, t := range relationTypes {
		typeFilter[t] = struct{}{}
	}

	adj := &adjacency{
		nodesByName: make(map[string]*domain_knowledge.GraphNode, len(nodes)),
		edges:       make(map[string][]adjEdge),
	}
	for i := range nodes {
		adj.nodesByName[nodes[i].Name] = nodes[i].toDomain()
	}
	for i := range rels {
		if len(typeFilter) > 0 {
			if _, ok := typeFilter[rels[i].Type]; !ok {
				continue
			}
		}
		dr := rels[i].toDomain()
		adj.edges[dr.Source] = append(adj.edges[dr.Source], adjEdge{to: dr.Target, rel: dr})
		adj.edges[dr.Target] = append(adj.edges[dr.Target], adjEdge{to: dr.Source, rel: dr})
	}
	return adj, nil
}

// pathToResult 将节点名序列与边序列转换为 PathQueryResult。
func (adj *adjacency) pathToResult(names []string, edges []*domain_knowledge.GraphRelation) *domain_knowledge.PathQueryResult {
	nodes := make([]*domain_knowledge.GraphNode, 0, len(names))
	for _, n := range names {
		if node, ok := adj.nodesByName[n]; ok {
			nodes = append(nodes, node)
		} else {
			nodes = append(nodes, &domain_knowledge.GraphNode{Name: n})
		}
	}
	weight := 0.0
	for _, e := range edges {
		weight += e.Weight
	}
	return &domain_knowledge.PathQueryResult{
		Nodes:     nodes,
		Relations: edges,
		Length:    len(edges),
		Weight:    weight,
	}
}

// FindShortestPath 使用 BFS 计算两节点间最短路径。
func (r *graphMetaRepository) FindShortestPath(ctx context.Context, ns domain_knowledge.NameSpace, startNode, endNode string, opts *domain_knowledge.PathQueryOptions) (*domain_knowledge.PathQueryResult, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	maxDepth := 0
	var relTypes []string
	if opts != nil {
		maxDepth = opts.MaxDepth
		relTypes = opts.RelationTypes
	}
	adj, err := r.buildAdjacency(ctx, ns, relTypes)
	if err != nil {
		return nil, err
	}
	names, edges, found := bfsShortestPath(adj, startNode, endNode, maxDepth)
	if !found {
		return nil, fmt.Errorf("未找到从 %s 到 %s 的路径", startNode, endNode)
	}
	return adj.pathToResult(names, edges), nil
}

// bfsState 记录 BFS 过程中到达某节点的前驱及使用的边。
type bfsState struct {
	prev    string
	prevRel *domain_knowledge.GraphRelation
	depth   int
}

// bfsShortestPath 标准 BFS，返回节点名序列与边序列。
func bfsShortestPath(adj *adjacency, start, end string, maxDepth int) ([]string, []*domain_knowledge.GraphRelation, bool) {
	if start == end {
		return []string{start}, nil, true
	}
	visited := map[string]bfsState{start: {prev: "", prevRel: nil, depth: 0}}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		curDepth := visited[cur].depth
		if maxDepth > 0 && curDepth >= maxDepth {
			continue
		}
		for _, e := range adj.edges[cur] {
			if _, seen := visited[e.to]; seen {
				continue
			}
			visited[e.to] = bfsState{prev: cur, prevRel: e.rel, depth: curDepth + 1}
			if e.to == end {
				return reconstructPath(visited, start, end)
			}
			queue = append(queue, e.to)
		}
	}
	return nil, nil, false
}

func reconstructPath(visited map[string]bfsState, start, end string) ([]string, []*domain_knowledge.GraphRelation, bool) {
	names := []string{end}
	var edges []*domain_knowledge.GraphRelation
	cur := end
	for cur != start {
		st := visited[cur]
		edges = append(edges, st.prevRel)
		names = append(names, st.prev)
		cur = st.prev
	}
	// 反转
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}
	for i, j := 0, len(edges)-1; i < j; i, j = i+1, j-1 {
		edges[i], edges[j] = edges[j], edges[i]
	}
	return names, edges, true
}

// FindKShortestPaths 枚举简单路径并按长度/权重排序返回前 K 条。
func (r *graphMetaRepository) FindKShortestPaths(ctx context.Context, ns domain_knowledge.NameSpace, startNode, endNode string, k int, opts *domain_knowledge.PathQueryOptions) ([]*domain_knowledge.PathQueryResult, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	if k <= 0 {
		k = 1
	}
	maxDepth := 6
	var relTypes []string
	if opts != nil {
		if opts.MaxDepth > 0 {
			maxDepth = opts.MaxDepth
		}
		relTypes = opts.RelationTypes
	}
	adj, err := r.buildAdjacency(ctx, ns, relTypes)
	if err != nil {
		return nil, err
	}
	paths := enumerateSimplePaths(adj, startNode, endNode, maxDepth)
	sortPaths(paths)
	if len(paths) > k {
		paths = paths[:k]
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("未找到从 %s 到 %s 的路径", startNode, endNode)
	}
	return paths, nil
}

// FindPathWithTypes 在关系类型约束下枚举路径。
func (r *graphMetaRepository) FindPathWithTypes(ctx context.Context, ns domain_knowledge.NameSpace, startNode, endNode string, relationTypes []string, maxDepth int) ([]*domain_knowledge.PathQueryResult, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	if maxDepth <= 0 {
		maxDepth = 6
	}
	adj, err := r.buildAdjacency(ctx, ns, relationTypes)
	if err != nil {
		return nil, err
	}
	paths := enumerateSimplePaths(adj, startNode, endNode, maxDepth)
	sortPaths(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("未找到从 %s 到 %s 的路径", startNode, endNode)
	}
	return paths, nil
}

const maxEnumeratedPaths = 1000

// enumerateSimplePaths 使用 DFS 枚举所有简单路径（带深度与数量上限）。
func enumerateSimplePaths(adj *adjacency, start, end string, maxDepth int) []*domain_knowledge.PathQueryResult {
	var results []*domain_knowledge.PathQueryResult
	visited := map[string]bool{start: true}
	nameStack := []string{start}
	var edgeStack []*domain_knowledge.GraphRelation

	var dfs func(cur string, depth int)
	dfs = func(cur string, depth int) {
		if len(results) >= maxEnumeratedPaths {
			return
		}
		if cur == end && len(edgeStack) > 0 {
			names := append([]string(nil), nameStack...)
			edges := append([]*domain_knowledge.GraphRelation(nil), edgeStack...)
			results = append(results, adj.pathToResult(names, edges))
			return
		}
		if depth >= maxDepth {
			return
		}
		for _, e := range adj.edges[cur] {
			if visited[e.to] {
				continue
			}
			visited[e.to] = true
			nameStack = append(nameStack, e.to)
			edgeStack = append(edgeStack, e.rel)
			dfs(e.to, depth+1)
			edgeStack = edgeStack[:len(edgeStack)-1]
			nameStack = nameStack[:len(nameStack)-1]
			delete(visited, e.to)
		}
	}
	dfs(start, 0)
	return results
}

// sortPaths 按长度升序、权重降序排序。
func sortPaths(paths []*domain_knowledge.PathQueryResult) {
	sort.SliceStable(paths, func(i, j int) bool {
		if paths[i].Length != paths[j].Length {
			return paths[i].Length < paths[j].Length
		}
		return paths[i].Weight > paths[j].Weight
	})
}

// ========================================
// Statistics Operations
// ========================================

// GetGraphStats 获取基础图谱统计。
func (r *graphMetaRepository) GetGraphStats(ctx context.Context, ns domain_knowledge.NameSpace) (*domain_knowledge.GraphStats, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	// 统计只需 chunks 列与节点数，故仅 SELECT chunks，避免加载 attributes/
	// properties 等大字段；关系数与节点数分别用 COUNT/行数得到。
	var rows []struct{ Chunks string }
	if err := r.scope(ctx, ns, &graphNodeModel{}).Select("chunks").Find(&rows).Error; err != nil {
		return nil, err
	}
	var relCount int64
	if err := r.scope(ctx, ns, &graphRelationModel{}).Count(&relCount).Error; err != nil {
		return nil, err
	}
	chunkSet := make(map[string]struct{})
	for i := range rows {
		for _, c := range unmarshalSlice(rows[i].Chunks) {
			chunkSet[c] = struct{}{}
		}
	}
	chunkIDs := make([]string, 0, len(chunkSet))
	for c := range chunkSet {
		chunkIDs = append(chunkIDs, c)
	}
	sort.Strings(chunkIDs)
	return &domain_knowledge.GraphStats{
		NodeCount:     int64(len(rows)),
		RelationCount: relCount,
		ChunkCount:    int64(len(chunkIDs)),
		ChunkIDs:      chunkIDs,
	}, nil
}

// degreeMap 计算每个节点的度（无向，重复边计入）。
func degreeMap(nodes []graphNodeModel, rels []graphRelationModel) map[string]int {
	deg := make(map[string]int, len(nodes))
	for i := range nodes {
		deg[nodes[i].Name] = 0
	}
	for i := range rels {
		deg[rels[i].Source]++
		deg[rels[i].Target]++
	}
	return deg
}

// GetDegreeStats 获取度统计。
func (r *graphMetaRepository) GetDegreeStats(ctx context.Context, ns domain_knowledge.NameSpace) (*domain_knowledge.DegreeStats, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	nodes, err := r.loadNodes(ctx, ns)
	if err != nil {
		return nil, err
	}
	rels, err := r.loadRelations(ctx, ns)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return &domain_knowledge.DegreeStats{}, nil
	}
	deg := degreeMap(nodes, rels)
	total := 0
	maxDeg, minDeg := 0, int(^uint(0)>>1)
	isolated := 0
	for _, d := range deg {
		total += d
		if d > maxDeg {
			maxDeg = d
		}
		if d < minDeg {
			minDeg = d
		}
		if d == 0 {
			isolated++
		}
	}
	avg := float64(total) / float64(len(deg))
	high := 0
	for _, d := range deg {
		if float64(d) > 2*avg && avg > 0 {
			high++
		}
	}
	if minDeg == int(^uint(0)>>1) {
		minDeg = 0
	}
	return &domain_knowledge.DegreeStats{
		AverageDegree:   avg,
		MaxDegree:       maxDeg,
		MinDegree:       minDeg,
		IsolatedNodes:   isolated,
		HighDegreeNodes: high,
	}, nil
}

// GetTypeDistribution 获取类型分布统计。
func (r *graphMetaRepository) GetTypeDistribution(ctx context.Context, ns domain_knowledge.NameSpace) (*domain_knowledge.TypeDistribution, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	nodes, err := r.loadNodes(ctx, ns)
	if err != nil {
		return nil, err
	}
	rels, err := r.loadRelations(ctx, ns)
	if err != nil {
		return nil, err
	}
	entityDist := make(map[string]int)
	for i := range nodes {
		entityDist[nodes[i].EntityType]++
	}
	relDist := make(map[string]int)
	for i := range rels {
		relDist[rels[i].Type]++
	}
	return &domain_knowledge.TypeDistribution{
		EntityTypeDistribution:   entityDist,
		RelationTypeDistribution: relDist,
		TopEntityTypes:           topTypeCounts(entityDist, 10),
		TopRelationTypes:         topTypeCounts(relDist, 10),
	}, nil
}

// topTypeCounts 返回出现次数最多的前 n 个类型。
func topTypeCounts(dist map[string]int, n int) []domain_knowledge.TypeCount {
	counts := make([]domain_knowledge.TypeCount, 0, len(dist))
	for t, c := range dist {
		counts = append(counts, domain_knowledge.TypeCount{Type: t, Count: c})
	}
	sort.SliceStable(counts, func(i, j int) bool {
		if counts[i].Count != counts[j].Count {
			return counts[i].Count > counts[j].Count
		}
		return counts[i].Type < counts[j].Type
	})
	if len(counts) > n {
		counts = counts[:n]
	}
	return counts
}

// GetDensityMetrics 获取密度指标（密度、连通分量、聚类系数）。
func (r *graphMetaRepository) GetDensityMetrics(ctx context.Context, ns domain_knowledge.NameSpace) (*domain_knowledge.DensityMetrics, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	nodes, err := r.loadNodes(ctx, ns)
	if err != nil {
		return nil, err
	}
	rels, err := r.loadRelations(ctx, ns)
	if err != nil {
		return nil, err
	}
	n := len(nodes)
	if n == 0 {
		return &domain_knowledge.DensityMetrics{}, nil
	}

	// 无向邻接集合（去重边）。
	neighborSet := make(map[string]map[string]struct{}, n)
	for i := range nodes {
		neighborSet[nodes[i].Name] = make(map[string]struct{})
	}
	edgeCount := 0
	for i := range rels {
		s, t := rels[i].Source, rels[i].Target
		if s == t {
			continue
		}
		if _, ok := neighborSet[s]; !ok {
			neighborSet[s] = make(map[string]struct{})
		}
		if _, ok := neighborSet[t]; !ok {
			neighborSet[t] = make(map[string]struct{})
		}
		if _, dup := neighborSet[s][t]; !dup {
			edgeCount++
		}
		neighborSet[s][t] = struct{}{}
		neighborSet[t][s] = struct{}{}
	}

	density := 0.0
	if n > 1 {
		density = 2 * float64(edgeCount) / (float64(n) * float64(n-1))
	}

	// 连通分量（并查集）。
	componentCount, largest := connectedComponents(nodes, neighborSet)

	// 平均局部聚类系数。
	clustering := averageClusteringCoefficient(neighborSet)

	return &domain_knowledge.DensityMetrics{
		Density:               density,
		ComponentCount:        componentCount,
		LargestComponentSize:  largest,
		ClusteringCoefficient: clustering,
	}, nil
}

// connectedComponents 计算连通分量数量与最大分量规模。
func connectedComponents(nodes []graphNodeModel, neighborSet map[string]map[string]struct{}) (int, int) {
	parent := make(map[string]string, len(nodes))
	var find func(x string) string
	find = func(x string) string {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	for i := range nodes {
		parent[nodes[i].Name] = nodes[i].Name
	}
	for name, nbrs := range neighborSet {
		if _, ok := parent[name]; !ok {
			parent[name] = name
		}
		for nb := range nbrs {
			if _, ok := parent[nb]; !ok {
				parent[nb] = nb
			}
			union(name, nb)
		}
	}
	sizes := make(map[string]int)
	for name := range parent {
		root := find(name)
		sizes[root]++
	}
	largest := 0
	for _, s := range sizes {
		if s > largest {
			largest = s
		}
	}
	return len(sizes), largest
}

// averageClusteringCoefficient 计算平均局部聚类系数。
func averageClusteringCoefficient(neighborSet map[string]map[string]struct{}) float64 {
	if len(neighborSet) == 0 {
		return 0
	}
	total := 0.0
	for _, nbrs := range neighborSet {
		k := len(nbrs)
		if k < 2 {
			continue
		}
		links := 0
		nbrList := make([]string, 0, k)
		for nb := range nbrs {
			nbrList = append(nbrList, nb)
		}
		for i := 0; i < len(nbrList); i++ {
			for j := i + 1; j < len(nbrList); j++ {
				if _, ok := neighborSet[nbrList[i]][nbrList[j]]; ok {
					links++
				}
			}
		}
		possible := float64(k*(k-1)) / 2
		total += float64(links) / possible
	}
	return total / float64(len(neighborSet))
}

// GetCentralitySummaries 获取中心性摘要（pagerank/betweenness），无存储值时回退到度中心性。
func (r *graphMetaRepository) GetCentralitySummaries(ctx context.Context, ns domain_knowledge.NameSpace, centralityType string) (*domain_knowledge.CentralitySummary, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	nodes, err := r.loadNodes(ctx, ns)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return &domain_knowledge.CentralitySummary{}, nil
	}

	type scored struct {
		id, name string
		score    float64
	}
	scores := make([]scored, 0, len(nodes))
	allZero := true
	for i := range nodes {
		var v float64
		switch strings.ToLower(centralityType) {
		case "betweenness":
			v = nodes[i].Betweenness
		default:
			v = nodes[i].Pagerank
		}
		if v != 0 {
			allZero = false
		}
		scores = append(scores, scored{id: nodes[i].NodeID, name: nodes[i].Name, score: v})
	}

	// 无已计算的中心性时，回退到归一化度中心性。
	if allZero {
		rels, err := r.loadRelations(ctx, ns)
		if err != nil {
			return nil, err
		}
		deg := degreeMap(nodes, rels)
		denom := float64(len(nodes) - 1)
		if denom <= 0 {
			denom = 1
		}
		for i := range scores {
			scores[i].score = float64(deg[scores[i].name]) / denom
		}
	}

	sort.SliceStable(scores, func(i, j int) bool { return scores[i].score > scores[j].score })

	minV, maxV, sum := scores[len(scores)-1].score, scores[0].score, 0.0
	for _, s := range scores {
		sum += s.score
	}
	topN := 10
	if len(scores) < topN {
		topN = len(scores)
	}
	top := make([]domain_knowledge.CentralityNode, 0, topN)
	for i := 0; i < topN; i++ {
		top = append(top, domain_knowledge.CentralityNode{
			NodeID:   scores[i].id,
			NodeName: scores[i].name,
			Score:    scores[i].score,
			Rank:     i + 1,
		})
	}
	return &domain_knowledge.CentralitySummary{
		Min:      minV,
		Max:      maxV,
		Average:  sum / float64(len(scores)),
		TopNodes: top,
	}, nil
}

// GetCommunityStats 获取社区检测统计。
func (r *graphMetaRepository) GetCommunityStats(ctx context.Context, ns domain_knowledge.NameSpace) (*domain_knowledge.CommunityStats, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	var comms []graphCommunityModel
	if err := r.scope(ctx, ns, &graphCommunityModel{}).Find(&comms).Error; err != nil {
		return nil, err
	}
	if len(comms) == 0 {
		return &domain_knowledge.CommunityStats{}, nil
	}
	totalSize := 0
	largest := 0
	modularity := 0.0
	for i := range comms {
		totalSize += comms[i].Size
		if comms[i].Size > largest {
			largest = comms[i].Size
		}
		modularity += comms[i].Modularity
	}
	return &domain_knowledge.CommunityStats{
		CommunityCount:       len(comms),
		Modularity:           modularity,
		AverageCommunitySize: float64(totalSize) / float64(len(comms)),
		LargestCommunitySize: largest,
	}, nil
}

// GetStatsByEntityType 获取按实体类型过滤的统计。
func (r *graphMetaRepository) GetStatsByEntityType(ctx context.Context, ns domain_knowledge.NameSpace, entityType string) (*domain_knowledge.GraphStats, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	var nodes []graphNodeModel
	if err := r.scope(ctx, ns, &graphNodeModel{}).Where("entity_type = ?", entityType).Find(&nodes).Error; err != nil {
		return nil, err
	}
	nameSet := make(map[string]struct{}, len(nodes))
	chunkSet := make(map[string]struct{})
	for i := range nodes {
		nameSet[nodes[i].Name] = struct{}{}
		for _, c := range unmarshalSlice(nodes[i].Chunks) {
			chunkSet[c] = struct{}{}
		}
	}
	rels, err := r.loadRelations(ctx, ns)
	if err != nil {
		return nil, err
	}
	var relCount int64
	for i := range rels {
		_, s := nameSet[rels[i].Source]
		_, t := nameSet[rels[i].Target]
		if s || t {
			relCount++
		}
	}
	chunkIDs := make([]string, 0, len(chunkSet))
	for c := range chunkSet {
		chunkIDs = append(chunkIDs, c)
	}
	sort.Strings(chunkIDs)
	return &domain_knowledge.GraphStats{
		NodeCount:     int64(len(nodes)),
		RelationCount: relCount,
		ChunkCount:    int64(len(chunkIDs)),
		ChunkIDs:      chunkIDs,
	}, nil
}

// ========================================
// Analytics Result Storage
// ========================================

// StoreCommunity 存储社区检测结果（按 community_id upsert）。
func (r *graphMetaRepository) StoreCommunity(ctx context.Context, ns domain_knowledge.NameSpace, community *domain_knowledge.Community) error {
	if err := r.ensureSchema(ctx); err != nil {
		return err
	}
	if community == nil || community.ID == "" {
		return fmt.Errorf("社区 ID 不能为空")
	}
	createdAt := community.CreatedAt
	if createdAt.IsZero() {
		// 调用方未设置时间时，交由数据库默认；这里保持零值由 GORM 处理。
		createdAt = time.Time{}
	}
	var existing graphCommunityModel
	err := r.scope(ctx, ns, &graphCommunityModel{}).Where("community_id = ?", community.ID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		m := &graphCommunityModel{
			TenantID:    ns.TenantID,
			KBID:        ns.KnowledgeBaseID,
			CommunityID: community.ID,
			Name:        community.Name,
			Size:        community.Size,
			Modularity:  community.Modularity,
			Labels:      marshalSlice(community.Labels),
			Properties:  marshalStringMap(community.Properties),
			CreatedAt:   createdAt,
		}
		return r.db.WithContext(ctx).Create(m).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"name":       community.Name,
		"size":       community.Size,
		"modularity": community.Modularity,
		"labels":     marshalSlice(community.Labels),
		"properties": marshalStringMap(community.Properties),
	}
	return r.scope(ctx, ns, &graphCommunityModel{}).Where("community_id = ?", community.ID).Updates(updates).Error
}

// StoreCommunityMembers 存储社区成员关系（按 community_id+entity_id upsert）。
func (r *graphMetaRepository) StoreCommunityMembers(ctx context.Context, ns domain_knowledge.NameSpace, members []*domain_knowledge.CommunityMember) error {
	if err := r.ensureSchema(ctx); err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, m := range members {
			if m == nil || m.CommunityID == "" || m.EntityID == "" {
				continue
			}
			var existing graphCommunityMemberModel
			err := tx.Where("tenant_id = ? AND kb_id = ? AND community_id = ? AND entity_id = ?",
				ns.TenantID, ns.KnowledgeBaseID, m.CommunityID, m.EntityID).First(&existing).Error
			if err == gorm.ErrRecordNotFound {
				if err := tx.Create(&graphCommunityMemberModel{
					TenantID:        ns.TenantID,
					KBID:            ns.KnowledgeBaseID,
					CommunityID:     m.CommunityID,
					EntityID:        m.EntityID,
					EntityName:      m.EntityName,
					MembershipScore: m.MembershipScore,
				}).Error; err != nil {
					return err
				}
				// 同步节点的 community_id 便于反查（节点可能尚未写入，匹配 0 行不视为错误）。
				if err := tx.Model(&graphNodeModel{}).
					Where("tenant_id = ? AND kb_id = ? AND name = ?", ns.TenantID, ns.KnowledgeBaseID, m.EntityName).
					Update("community_id", m.CommunityID).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			if err := tx.Model(&graphCommunityMemberModel{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
				"entity_name":      m.EntityName,
				"membership_score": m.MembershipScore,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateCentralityScores 更新节点中心性分数。
func (r *graphMetaRepository) UpdateCentralityScores(ctx context.Context, ns domain_knowledge.NameSpace, scores map[string]float64, scoreType string) error {
	if err := r.ensureSchema(ctx); err != nil {
		return err
	}
	if len(scores) == 0 {
		return nil
	}
	column := "pagerank"
	if strings.ToLower(scoreType) == "betweenness" {
		column = "betweenness"
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for name, score := range scores {
			if err := tx.Model(&graphNodeModel{}).
				Where("tenant_id = ? AND kb_id = ? AND name = ?", ns.TenantID, ns.KnowledgeBaseID, name).
				Update(column, score).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ========================================
// Neo4j-compatible helpers (back-compat)
// ========================================

// SearchNode 搜索节点（返回包含命中节点及其关系的子图）。
func (r *graphMetaRepository) SearchNode(ctx context.Context, ns domain_knowledge.NameSpace, nodes []string) (*domain_knowledge.GraphData, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return &domain_knowledge.GraphData{}, nil
	}
	var nodeModels []graphNodeModel
	if err := r.scope(ctx, ns, &graphNodeModel{}).Where("name IN ?", nodes).Find(&nodeModels).Error; err != nil {
		return nil, err
	}
	var relModels []graphRelationModel
	if err := r.scope(ctx, ns, &graphRelationModel{}).Where("source IN ? OR target IN ?", nodes, nodes).Find(&relModels).Error; err != nil {
		return nil, err
	}
	data := &domain_knowledge.GraphData{
		Node:     make([]*domain_knowledge.GraphNode, 0, len(nodeModels)),
		Relation: make([]*domain_knowledge.GraphRelation, 0, len(relModels)),
	}
	for i := range nodeModels {
		data.Node = append(data.Node, nodeModels[i].toDomain())
	}
	for i := range relModels {
		data.Relation = append(data.Relation, relModels[i].toDomain())
	}
	return data, nil
}

// SearchNodeV2 与 SearchNode 等价（MySQL 实现下行为一致）。
func (r *graphMetaRepository) SearchNodeV2(ctx context.Context, ns domain_knowledge.NameSpace, nodes []string) (*domain_knowledge.GraphData, error) {
	return r.SearchNode(ctx, ns, nodes)
}

// SearchPath 搜索两节点间路径，返回路径子图列表。
func (r *graphMetaRepository) SearchPath(ctx context.Context, ns domain_knowledge.NameSpace, startNode, endNode string, maxDepth int) ([]*domain_knowledge.GraphData, error) {
	if maxDepth <= 0 {
		maxDepth = 6
	}
	paths, err := r.FindPathWithTypes(ctx, ns, startNode, endNode, nil, maxDepth)
	if err != nil {
		return nil, err
	}
	out := make([]*domain_knowledge.GraphData, 0, len(paths))
	for _, p := range paths {
		out = append(out, &domain_knowledge.GraphData{Node: p.Nodes, Relation: p.Relations})
	}
	return out, nil
}

// ========================================
// Health & Connection
// ========================================

// CheckHealth 检查底层 MySQL 连接。
func (r *graphMetaRepository) CheckHealth(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close MySQL 连接由连接池管理，无需显式关闭。
func (r *graphMetaRepository) Close(ctx context.Context) error {
	return nil
}

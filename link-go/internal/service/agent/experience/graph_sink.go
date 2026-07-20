package experience

import (
	"context"
	"fmt"
	"strconv"

	domain_experience "link/internal/model/agent/experience"
	"link/internal/model/knowledge"
)

// GraphSink 把一条经验写入知识图谱：建一个 experience 节点，并向其涉及的
// 工具/标签节点连 RELATED_TO 边，供后续图谱检索复用历史经验。
//
// 命名空间与术语接地一致：TenantID 用会话租户、kb_id="" （租户全域）。
// Neo4j 不可用时可注入 MySQL 版 GraphRepository（graphMetaRepository），行为一致。
type GraphSink struct {
	repo knowledge.GraphRepository
}

// NewGraphSink 创建图谱写入器。repo 为 nil 时 Write 直接返回 nil（视为未接线，静默跳过）。
func NewGraphSink(repo knowledge.GraphRepository) *GraphSink {
	return &GraphSink{repo: repo}
}

// Write 把经验落图谱。exp.ID 已由持久化层赋值，作为节点稳定 ID 的一部分。
func (g *GraphSink) Write(ctx context.Context, exp *domain_experience.Experience) error {
	if g == nil || g.repo == nil {
		return nil // 未接线图谱：静默跳过（沉淀主流程不因图谱缺失而失败）
	}
	if exp == nil {
		return fmt.Errorf("graph sink: 经验为空")
	}

	expNodeName := experienceNodeName(exp)
	data := &knowledge.GraphData{}

	// 经验主节点：正文摘要挂在 Properties，便于图谱侧观测/检索。
	data.Node = append(data.Node, &knowledge.GraphNode{
		ID:         "exp_" + strconv.FormatInt(exp.ID, 10),
		Name:       expNodeName,
		EntityType: "experience",
		Properties: map[string]string{
			"source":     "experience-distill",
			"session_id": exp.SessionID,
			"problem":    truncate(exp.Problem, 500),
			"solution":   truncate(exp.Solution, 500),
			"agent_type": exp.AgentType,
		},
	})

	// 工具/标签节点 + RELATED_TO 边：节点按 name MERGE 幂等，边按 id 幂等。
	nodeSeen := map[string]bool{expNodeName: true}
	relIdx := 0
	addRelated := func(name, etype string) {
		if name == "" {
			return
		}
		if !nodeSeen[name] {
			nodeSeen[name] = true
			data.Node = append(data.Node, &knowledge.GraphNode{
				ID:         etype + "_" + name,
				Name:       name,
				EntityType: etype,
				Properties: map[string]string{"source": "experience-distill"},
			})
		}
		data.Relation = append(data.Relation, &knowledge.GraphRelation{
			ID:       fmt.Sprintf("exp_%d_rel_%d", exp.ID, relIdx),
			Source:   expNodeName,
			Target:   name,
			Type:     string(knowledge.RelationTypeRelatedTo),
			Strength: 5,
		})
		relIdx++
	}
	for _, t := range exp.Tools {
		addRelated(t, "tool")
	}
	for _, t := range exp.Tags {
		addRelated(t, "topic")
	}

	ns := knowledge.NameSpace{TenantID: strconv.FormatInt(exp.TenantID, 10), KnowledgeBaseID: ""}
	if err := g.repo.AddGraph(ctx, ns, []*knowledge.GraphData{data}); err != nil {
		return fmt.Errorf("graph sink: 写入图谱失败: %w", err)
	}
	return nil
}

// experienceNodeName 生成经验节点的可读名：优先用标题，缺失时回落到「经验#ID」。
func experienceNodeName(exp *domain_experience.Experience) string {
	if exp.Title != "" {
		return "经验：" + exp.Title
	}
	return "经验#" + strconv.FormatInt(exp.ID, 10)
}

// truncate 截断字符串到最多 n 个「符文」（rune），避免在多字节字符中间切断
// 导致图谱属性存入非法 UTF-8（用于图谱属性防超长）。
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

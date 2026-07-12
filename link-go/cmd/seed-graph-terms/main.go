// 工具：向知识图谱（Neo4j）注入「业务口语术语 → 受治理规范名」的 SIMILAR_TO 桥接，
// 为指标语义层的术语接地（termgrounding）补齐第二层（图谱层）证据。
//
// 背景：接地分两层——(1) 模型内同义词精确匹配；(2) 模型内未命中时，经 Neo4j 图谱把
// 术语解析为其同义/相关规范名（如 "流水" —SIMILAR_TO→ "营收"），再回落到模型内索引。
// seed-semantic 已灌好模型内同义词；本工具灌的是「模型里没登记、但业务口语常用」的词，
// 唯有它们才会真正触发并验证图谱层（登记过的词会在第一层命中，永远走不到图谱）。
//
// 用法：cd link-go && set -a && source .env && set +a && go run ./cmd/seed-graph-terms
// 落库对象：租户 tenant_id="1"（dev 用户）、kb_id=""（租户全域，与运行时 GraphAdapter
// 的检索命名空间一致）。节点按 name 幂等 MERGE、关系按 id 幂等，可反复执行。
//
// 前置：cmd/seed-semantic 已灌入语义模型（桥接的规范名须是模型内已登记的名/同义词），
// 且 Neo4j 可连（NEO4J_URI/USERNAME/PASSWORD）；连不上时接地自动退化为仅模型内同义词。
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"link/internal/model/knowledge"
	neo4jrepo "link/internal/repository/neo4j"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// seedTenant 目标租户：dev 用户 tenant_id=1（图谱命名空间用字符串形态）。
const seedTenant = "1"

// bridge 是一条「口语术语 → 受治理规范名」的图谱桥接。
// Canonical 必须精确等于某个语义模型里已登记的规范名或同义词（接地第二层会据此回落）。
// Colloquial 则须是模型内「未登记」的词，否则第一层就命中、永远走不到图谱层。
type bridge struct {
	Colloquial string // 业务口语词（模型内未登记）
	Canonical  string // 受治理规范名/同义词（模型内已登记）
	Note       string // 说明（写入节点属性，便于图谱侧观测）
}

// bridges 电商销售 + 商品销售两套模型的口语术语桥接。
// 全部经人工核对：Colloquial 均不在 seed-semantic 的 name/synonyms 中。
var bridges = []bridge{
	// —— 电商销售（营收/客单价/折扣率/城市/支付方式）——
	{"流水", "营收", "口语：日/月流水 = 营收"},
	{"单均", "客单价", "口语：单均 = 客单价"},
	{"让利", "折扣率", "口语：让利力度 = 折扣率"},
	{"地域", "城市", "口语：地域/地区 = 城市维度"},
	{"付款方式", "支付方式", "口语：付款方式 = 支付方式维度"},
	// —— 商品销售（销量/商品销售额/均价/品类/品牌）——
	{"出货量", "销量", "口语：出货量 = 销量"},
	{"货值", "商品销售额", "口语：货值 = 商品销售额"},
	{"单件均价", "均价", "口语：单件均价 = 均价"},
	{"类目", "品类", "口语：类目 = 品类维度"},
	{"牌子", "品牌", "口语：牌子 = 品牌维度"},
}

func main() {
	repo, err := neo4jrepo.NewNeo4jRepository(
		env("NEO4J_URI", "bolt://localhost:7687"),
		env("NEO4J_USERNAME", "neo4j"),
		env("NEO4J_PASSWORD", ""),
		// dbName 留空：与运行时 GraphAdapter（cfg.Neo4j.DatabaseName 未设→""）一致，
		// 确保注入落在同一物理库、运行时检索得到。
		"",
	)
	if err != nil {
		log.Fatalf("connect neo4j failed: %v", err)
	}

	ns := knowledge.NameSpace{TenantID: seedTenant, KnowledgeBaseID: ""}
	ctx := context.Background()

	// 组装节点 + SIMILAR_TO 关系：口语词与规范名各建一个节点，MERGE 幂等。
	nodeSeen := make(map[string]bool)
	data := &knowledge.GraphData{}
	addNode := func(name, etype, note string) {
		if name == "" || nodeSeen[name] {
			return
		}
		nodeSeen[name] = true
		data.Node = append(data.Node, &knowledge.GraphNode{
			ID:         "gt_" + name, // 名内嵌 ID：稳定且可读；节点本身按 name MERGE 幂等
			Name:       name,
			EntityType: etype,
			Properties: map[string]string{"source": "seed-graph-terms", "note": note},
		})
	}
	for i, b := range bridges {
		addNode(b.Colloquial, "业务术语", b.Note)
		addNode(b.Canonical, "受治理规范名", b.Note)
		data.Relation = append(data.Relation, &knowledge.GraphRelation{
			ID:          fmt.Sprintf("gt_rel_%d", i),
			Source:      b.Colloquial,
			Target:      b.Canonical,
			Type:        string(knowledge.RelationTypeSimilarTo), // SIMILAR_TO：接地视为同义/别名的边
			Strength:    9,
			Description: b.Note,
		})
	}

	if err := repo.AddGraph(ctx, ns, []*knowledge.GraphData{data}); err != nil {
		log.Fatalf("inject graph terms failed: %v", err)
	}

	fmt.Printf("OK: injected %d 术语节点 + %d SIMILAR_TO 桥接 (tenant=%s, kb_id=\"\")\n",
		len(data.Node), len(data.Relation), seedTenant)
	for _, b := range bridges {
		fmt.Printf("  %s —SIMILAR_TO→ %s\n", b.Colloquial, b.Canonical)
	}
	fmt.Println("DONE: 术语接地图谱层已就绪（模型内未命中的口语词将经图谱回落到规范名）")
}

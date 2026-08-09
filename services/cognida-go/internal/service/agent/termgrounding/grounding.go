// Package termgrounding 把用户口语化的业务术语「接地」到指标语义层里受治理的
// 指标/维度/度量名，供意图识别阶段消歧。
//
// 接地分两层，均为确定性、可观测：
//  1. 模型内同义词（in-model）：直接用 ModelBundle 的 name/synonyms 建索引精确匹配，
//     这是既有 NL2Semantics 能力的复用与扩展，无需外部依赖即可工作。
//  2. 知识图谱/血缘（GraphPort，可选）：模型内未命中时，经 Neo4j 图谱把术语解析为
//     其同义/相关的规范名（如 "GMV" —SIMILAR_TO→ "营收"），再回落到模型内索引。
//
// 关键原则：一个术语命中「多个不同」受治理概念时判为「歧义」，只回传候选、不硬选，
// 交由上层反问用户（歧义反问不硬猜）。
package termgrounding

import (
	"context"
	"sort"
	"strings"

	"cognida/internal/model/semantic"
)

// Kind 是受治理概念的类别。
type Kind string

const (
	KindMetric    Kind = "metric"
	KindDimension Kind = "dimension"
	KindMeasure   Kind = "measure"
)

// Source 标注一个候选是如何被解析出来的（可观测）。
type Source string

const (
	// SourceName 精确命中受治理概念的规范名。
	SourceName Source = "name"
	// SourceSynonym 命中模型内登记的业务同义词。
	SourceSynonym Source = "synonym"
	// SourceGraph 经知识图谱/血缘解析得到。
	SourceGraph Source = "graph"
)

// Candidate 是一个术语可能对应的受治理概念。
type Candidate struct {
	Name   string `json:"name"`   // 受治理的规范语义名
	Kind   Kind   `json:"kind"`   // metric / dimension / measure
	Source Source `json:"source"` // 命中来源（可观测）
	Via    string `json:"via,omitempty"`
}

// Grounding 是单个输入术语的接地结果。
type Grounding struct {
	Term       string      `json:"term"`                 // 原始术语
	Resolved   string      `json:"resolved,omitempty"`   // 唯一命中的规范名（歧义/未命中时空）
	Kind       Kind        `json:"kind,omitempty"`       // 命中概念的类别
	Ambiguous  bool        `json:"ambiguous,omitempty"`  // 命中多个不同概念，须反问不硬选
	Unresolved bool        `json:"unresolved,omitempty"` // 无任何候选
	Candidates []Candidate `json:"candidates,omitempty"` // 全部候选（歧义时供反问，命中时供观测）
}

// GraphPort 是接地用到的知识图谱窄端口：把原始术语解析为图谱中同义/相关的规范名。
// 由 GraphAdapter 适配既有 knowledge.GraphRepository（复用血缘图谱而非另起一套）。
type GraphPort interface {
	// ResolveAliases 返回 term 在图谱中的候选规范名（含节点自身名与其同义/相关邻居名）。
	// 无图谱或无命中时返回空切片。tenantID 用于租户隔离。
	ResolveAliases(ctx context.Context, tenantID int64, term string) ([]string, error)
}

// Grounder 依据语义模型（+ 可选图谱）执行术语接地。
type Grounder struct {
	graph GraphPort // 可为 nil：仅用模型内同义词接地
}

// NewGrounder 构造接地器；graph 可为 nil（图谱是可选增强）。
func NewGrounder(graph GraphPort) *Grounder {
	return &Grounder{graph: graph}
}

// concept 是索引里的一个受治理概念条目。
type concept struct {
	name string
	kind Kind
}

// bundleIndex 是 name/synonym（归一化）→ 概念集合 的倒排索引。
type bundleIndex map[string][]concept

// buildIndex 从 bundle 装配倒排索引：指标/度量/维度的规范名与同义词均登记。
func buildIndex(bundle *semantic.ModelBundle) bundleIndex {
	idx := make(bundleIndex)
	add := func(term string, c concept) {
		k := norm(term)
		if k == "" {
			return
		}
		for _, ex := range idx[k] {
			if ex.name == c.name && ex.kind == c.kind {
				return // 去重：同一概念的重复登记
			}
		}
		idx[k] = append(idx[k], c)
	}
	for _, m := range bundle.Metrics {
		c := concept{name: m.Name, kind: KindMetric}
		add(m.Name, c)
		for _, s := range m.Synonyms {
			add(s, c)
		}
	}
	for _, ms := range bundle.Measures {
		add(ms.Name, concept{name: ms.Name, kind: KindMeasure})
	}
	for _, d := range bundle.Dimensions {
		c := concept{name: d.Name, kind: KindDimension}
		add(d.Name, c)
		for _, s := range d.Synonyms {
			add(s, c)
		}
	}
	return idx
}

// Ground 对一批术语接地。tenantID 供图谱端口做租户隔离。
func (g *Grounder) Ground(ctx context.Context, tenantID int64, bundle *semantic.ModelBundle, terms []string) []Grounding {
	idx := buildIndex(bundle)
	out := make([]Grounding, 0, len(terms))
	for _, t := range terms {
		out = append(out, g.groundOne(ctx, tenantID, idx, t))
	}
	return out
}

func (g *Grounder) groundOne(ctx context.Context, tenantID int64, idx bundleIndex, term string) Grounding {
	gr := Grounding{Term: term}

	// 第一层：模型内精确命中（规范名优先于同义词）。
	if cands := lookup(idx, term); len(cands) > 0 {
		return finalize(gr, cands)
	}

	// 第二层：图谱解析别名后回落到模型内索引。
	if g.graph != nil {
		aliases, err := g.graph.ResolveAliases(ctx, tenantID, term)
		if err == nil {
			var cands []Candidate
			seen := make(map[string]bool)
			for _, alias := range aliases {
				for _, c := range idx[norm(alias)] {
					key := string(c.kind) + "|" + c.name
					if seen[key] {
						continue
					}
					seen[key] = true
					cands = append(cands, Candidate{Name: c.name, Kind: c.kind, Source: SourceGraph, Via: alias})
				}
			}
			if len(cands) > 0 {
				return finalize(gr, cands)
			}
		}
	}

	gr.Unresolved = true
	return gr
}

// lookup 在模型内索引中查术语；命中时标注来源为 name 还是 synonym。
func lookup(idx bundleIndex, term string) []Candidate {
	concepts, ok := idx[norm(term)]
	if !ok {
		return nil
	}
	cands := make([]Candidate, 0, len(concepts))
	for _, c := range concepts {
		src := SourceSynonym
		if norm(c.name) == norm(term) {
			src = SourceName // 术语本身即规范名
		}
		cands = append(cands, Candidate{Name: c.name, Kind: c.kind, Source: src})
	}
	return cands
}

// finalize 依候选集判定唯一命中还是歧义（按不同概念数计）。
func finalize(gr Grounding, cands []Candidate) Grounding {
	sortCandidates(cands)
	gr.Candidates = cands
	distinct := make(map[string]bool)
	for _, c := range cands {
		distinct[string(c.Kind)+"|"+c.Name] = true
	}
	if len(distinct) == 1 {
		gr.Resolved = cands[0].Name
		gr.Kind = cands[0].Kind
		return gr
	}
	gr.Ambiguous = true // 多个不同概念 → 反问不硬选
	return gr
}

// sortCandidates 让候选稳定有序：来源可信度（name > synonym > graph），再按名。
func sortCandidates(cands []Candidate) {
	rank := map[Source]int{SourceName: 0, SourceSynonym: 1, SourceGraph: 2}
	sort.SliceStable(cands, func(i, j int) bool {
		if rank[cands[i].Source] != rank[cands[j].Source] {
			return rank[cands[i].Source] < rank[cands[j].Source]
		}
		return cands[i].Name < cands[j].Name
	})
}

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

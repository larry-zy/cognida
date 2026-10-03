package catalog

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	c "cognida/internal/model/catalog"
)

type ProfileRisk struct {
	Level   string `json:"level"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}
type RelatedAsset struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}
type ProfileCard struct {
	AssetID         string         `json:"asset_id"`
	Summary         string         `json:"summary"`
	Method          string         `json:"method"`
	QualityScore    *float64       `json:"quality_score"`
	ProfiledColumns int            `json:"profiled_columns"`
	ActiveColumns   int            `json:"active_columns"`
	SuitableFor     []string       `json:"suitable_for"`
	Recommendations []string       `json:"recommendations"`
	Risks           []ProfileRisk  `json:"risks"`
	Related         []RelatedAsset `json:"related"`
}

// ProfileCard reports measured sample completeness, never an invented overall
// quality/model confidence. Related assets are explainable metadata matches.
func (s *Service) ProfileCard(ctx context.Context, tenant int64, id string) (*ProfileCard, error) {
	asset, e := s.Get(ctx, tenant, id)
	if e != nil {
		return nil, e
	}
	card := describeAsset(*asset)
	page, e := s.Repo.List(ctx, tenant, c.Filter{DatasourceID: asset.DatasourceID, Limit: 100})
	if e != nil {
		return nil, e
	}
	names := map[string]bool{}
	for _, col := range asset.Columns {
		if !col.Deprecated {
			names[strings.ToLower(col.Name)] = true
		}
	}
	type match struct {
		asset  c.AssetTable
		fields int
	}
	matches := []match{}
	ids := make([]string, 0, len(page.Items))
	for _, candidate := range page.Items {
		if candidate.ID != id && !candidate.Deprecated {
			ids = append(ids, candidate.ID)
		}
	}
	columns, e := s.Repo.ActiveColumnNames(ctx, tenant, ids)
	if e != nil {
		return nil, e
	}
	for _, candidate := range page.Items {
		if candidate.ID == id || candidate.Deprecated {
			continue
		}
		count := 0
		for _, name := range columns[candidate.ID] {
			if names[strings.ToLower(name)] {
				count++
			}
		}
		if count > 0 {
			matches = append(matches, match{candidate, count})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].fields == matches[j].fields {
			return matches[i].asset.ID < matches[j].asset.ID
		}
		return matches[i].fields > matches[j].fields
	})
	for i, m := range matches {
		if i == 5 {
			break
		}
		card.Related = append(card.Related, RelatedAsset{m.asset.ID, m.asset.TableName, fmt.Sprintf("同数据源，%d 个同名字段；关联关系仍需验证", m.fields)})
	}
	if page.HasMore {
		card.Recommendations = append(card.Recommendations, "相关数据候选来自当前数据源前 100 个资产，不代表完整排序。")
	}
	return &card, nil
}
func describeAsset(a c.AssetTable) ProfileCard {
	card := ProfileCard{AssetID: a.ID, Method: "sample_completeness_v1", SuitableFor: []string{}, Recommendations: []string{}, Risks: []ProfileRisk{}, Related: []RelatedAsset{}}
	sum := 0.0
	for _, col := range a.Columns {
		if col.Deprecated {
			continue
		}
		card.ActiveColumns++
		if col.Profiled && col.SampleSize > 0 && col.NullRate != nil {
			card.ProfiledColumns++
			sum += *col.NullRate
			if *col.NullRate > .2 {
				card.Risks = append(card.Risks, ProfileRisk{"warning", col.Name, fmt.Sprintf("样本缺失率 %.1f%%；分析前明确缺失值处理口径。", *col.NullRate*100)})
			}
		}
		name := strings.ToLower(col.Name)
		if col.Classification == "secret" || col.Classification == "confidential" || strings.Contains(name, "password") || strings.Contains(name, "token") || strings.Contains(name, "email") || strings.Contains(name, "phone") {
			card.Risks = append(card.Risks, ProfileRisk{"warning", col.Name, "可能包含敏感信息；字段名启发式提示，需人工确认并配置分级/脱敏。"})
		}
	}
	card.Summary = fmt.Sprintf("%s.%s：%d 个有效字段，源端估计 %d 行；已采样 %d 个字段。", a.Schema, a.TableName, card.ActiveColumns, a.RowCount, card.ProfiledColumns)
	if a.Comment != "" {
		card.Summary += " " + a.Comment
	}
	if card.ProfiledColumns > 0 {
		score := math.Round(100*(1-sum/float64(card.ProfiledColumns))*100) / 100
		card.QualityScore = &score
		card.SuitableFor = append(card.SuitableFor, "exploratory_analysis")
		card.Recommendations = append(card.Recommendations, "分数只衡量已采样字段的完整性，不证明准确性、代表性或训练适用性。")
	} else {
		card.Recommendations = append(card.Recommendations, "执行带采样的同步后可计算完整性分数；当前质量未知。")
	}
	if a.Deprecated {
		card.Risks = append(card.Risks, ProfileRisk{"error", "", "源表已消失；引用前重新同步或选择有效资产。"})
		card.SuitableFor = []string{}
	}
	if a.Classification == "secret" || a.Classification == "confidential" {
		card.Risks = append(card.Risks, ProfileRisk{"warning", "", "资产受敏感访问与发布审批限制。"})
	}
	card.Recommendations = append(card.Recommendations, "用于建模或训练前验证时间覆盖、分布偏差及业务口径。")
	return card
}

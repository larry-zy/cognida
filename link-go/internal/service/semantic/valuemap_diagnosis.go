// Package semantic —— 列画像反哺语义层 ValueMap 的一致性诊断（画像 → 语义闭环）。
//
// 背景：维度 ValueMap（业务标签 → 物理枚举值，如 已完成→completed）此前只能靠人工在
// seed 脚本里手写，无从校验其物理值是否与真实数据一致。而列画像（data facts）恰好扫描出
// 每个低基数列的真实物理枚举分布（TopValues）。本诊断把二者按物理坐标对齐，纯数据驱动地
// 检出两类治理缺陷，但绝不臆造业务标签（那需人/LLM 语义知识，属二期自动补全）：
//
//   - 死映射（DeadMapping）：ValueMap 里映射到的物理值在真实数据中根本不存在（拼写错/
//     值已废弃），该映射永远匹配 0 行——正是 Bug C 之后仍会悄悄失效的隐患。
//   - 覆盖缺口（CoverageGap）：真实数据里存在、却没有任何 ValueMap 业务标签指向的物理枚举
//     值（如新增状态无人配映射），LLM 拼过滤时缺业务锚点。
//
// 只读、非破坏；未接线画像（profiles==nil）时返回未启用而非报错——零回归。
package semantic

import (
	"context"
	"sort"
	"strings"

	"link/internal/model/dataprofile"
	model "link/internal/model/semantic"
)

// ProfileReader 是列画像的只读端口（消费侧最小接口，避免语义层耦合具体仓储）。
// mysql.columnProfileRepository 天然满足；nil 表示画像未接线。
type ProfileReader interface {
	ListByDatasourceTable(ctx context.Context, tenantID int64, datasourceID, table string) ([]*dataprofile.ColumnProfile, error)
}

// 诊断状态：一个维度值映射与真实画像对照后的判定结果。
const (
	DiagStatusOK        = "ok"         // 已成功对照画像
	DiagStatusNoProfile = "no_profile" // 该列尚无画像（冷表/未被 get_schema 探查过）
	DiagStatusNotColumn = "not_column" // Expr 是表达式而非裸列名，无法对齐画像
	DiagStatusNotEnum   = "not_enum"   // 有画像但非「已完整采集的枚举列」（高基数/未截全/空表）
	DiagStatusNoTable   = "no_table"   // 维度引用的逻辑表缺失（悬挂引用）
	DiagStatusAmbiguous = "ambiguous"  // 同数据源下多个 schema 存在同名表，坐标多义
)

// DeadMapping 一条失效的值映射：业务标签指向的物理值在真实数据中不存在。
type DeadMapping struct {
	Label         string `json:"label"`          // 业务标签（ValueMap 的键）
	PhysicalValue string `json:"physical_value"` // 映射到的物理值（真实数据中缺失）
}

// DimensionValueMapDiagnosis 单个维度的值映射诊断结果。
type DimensionValueMapDiagnosis struct {
	DimensionID   string `json:"dimension_id"`
	DimensionName string `json:"dimension_name"`
	LogicalTable  string `json:"logical_table,omitempty"`
	PhysicalTable string `json:"physical_table,omitempty"`
	Column        string `json:"column"`
	Status        string `json:"status"`

	MappedCount int64 `json:"mapped_count"` // ValueMap 条目数
	Distinct    int64 `json:"distinct"`     // 真实基数（画像）
	RowCount    int64 `json:"row_count"`    // 画像时表行数

	DeadMappings []DeadMapping                `json:"dead_mappings,omitempty"`
	CoverageGaps []dataprofile.ValueFrequency `json:"coverage_gaps,omitempty"`
}

// ValueMapDiagnosisReport 一个语义模型全部维度的值映射诊断汇总。
type ValueMapDiagnosisReport struct {
	ModelID    string                       `json:"model_id"`
	Dimensions []DimensionValueMapDiagnosis `json:"dimensions"`
}

// DiagnoseValueMaps 对某语义模型逐维度做画像↔ValueMap 一致性诊断。
//
// 报告范围：① 所有配了 ValueMap 的维度（校验其物理值是否命中真实数据）；② 未配 ValueMap
// 但物理列是「已完整采集的枚举列」的维度（其真实枚举值即全量覆盖缺口，提示应补映射）。
// 其余维度（无映射且非枚举列）不入报告，避免噪声。
func (s *Service) DiagnoseValueMaps(ctx context.Context, tenantID int64, modelID string) (*ValueMapDiagnosisReport, error) {
	if s.profiles == nil {
		return nil, ErrProfileDisabled
	}
	bundle, err := s.Get(ctx, tenantID, modelID)
	if err != nil {
		return nil, err
	}

	ltByID := make(map[string]*model.LogicalTable, len(bundle.LogicalTables))
	for _, lt := range bundle.LogicalTables {
		ltByID[lt.ID] = lt
	}

	report := &ValueMapDiagnosisReport{ModelID: modelID, Dimensions: []DimensionValueMapDiagnosis{}}
	for _, dim := range bundle.Dimensions {
		hasVM := len(dim.ValueMap) > 0
		diag := DimensionValueMapDiagnosis{
			DimensionID:   dim.ID,
			DimensionName: dim.Name,
			Column:        dim.Expr,
			MappedCount:   int64(len(dim.ValueMap)),
		}

		lt := ltByID[dim.LogicalTableID]
		if lt == nil {
			if hasVM {
				diag.Status = DiagStatusNoTable
				report.Dimensions = append(report.Dimensions, diag)
			}
			continue
		}
		diag.LogicalTable = lt.Name
		diag.PhysicalTable = lt.PhysicalTable

		if !isBareColumn(dim.Expr) {
			if hasVM {
				diag.Status = DiagStatusNotColumn
				report.Dimensions = append(report.Dimensions, diag)
			}
			continue
		}

		profs, err := s.profiles.ListByDatasourceTable(ctx, tenantID, lt.DatabaseID, lt.PhysicalTable)
		if err != nil {
			return nil, err
		}
		colProf, schemaCount := pickColumnProfile(profs, dim.Expr)
		if schemaCount > 1 {
			if hasVM {
				diag.Status = DiagStatusAmbiguous
				report.Dimensions = append(report.Dimensions, diag)
			}
			continue
		}
		if colProf == nil {
			if hasVM {
				diag.Status = DiagStatusNoProfile
				report.Dimensions = append(report.Dimensions, diag)
			}
			continue
		}

		diag.Distinct = colProf.Distinct
		diag.RowCount = colProf.RowCount
		dead, gaps, status := diagnoseValueMap(dim.ValueMap, colProf)
		diag.Status = status
		diag.DeadMappings = dead
		diag.CoverageGaps = gaps

		// 未配映射的维度：仅当其列确是已采集枚举（status==ok）且存在缺口时才入报告，
		// 提示「该列有真实枚举值但无 ValueMap」；非枚举列则跳过（数值/日期等无需映射）。
		if hasVM || (status == DiagStatusOK && len(gaps) > 0) {
			report.Dimensions = append(report.Dimensions, diag)
		}
	}
	return report, nil
}

// diagnoseValueMap 是纯比对逻辑（无 I/O，便于单测）：把维度 ValueMap 的物理值一侧与画像的
// 真实枚举分布对照，产出死映射与覆盖缺口。仅当画像是「已完整采集的枚举列」时才可靠对照，
// 否则返回相应非 ok 状态且不产出条目（避免因截断/空表误报）。
//
// 物理值归一化用 normPhys（仅 ToLower、不 Trim）以贴合运行时真实语义：指标引擎
// mapDimensionValues 把物理值「原样」拼进 `col = 'phys'`，故 MySQL 比较口径才是准绳——
// 默认 _ci collation 大小写不敏感（据此折叠大小写，避免把大小写差异误判为死映射），但
// 前导/尾随空白在运行时是显著的（MySQL 的 `=` 从不左裁空白，NO PAD collation 连尾随空白
// 也显著）。故此处保留空白敏感：带空白错漏的映射在运行时确实恒匹配 0 行，应如实报为死映射。
// 局限：区分大小写的 collation（_bin/_cs）下纯大小写笔误会漏报——为守住「_ci 下零误报」而
// 接受的取舍。
func diagnoseValueMap(valueMap map[string]string, p *dataprofile.ColumnProfile) (dead []DeadMapping, gaps []dataprofile.ValueFrequency, status string) {
	// 可靠性门槛：必须有行、有枚举分布，且分布已采全（Distinct 未超过已截取的条数）。
	captured := len(p.TopValues) > 0 && int64(len(p.TopValues)) >= p.Distinct
	if p.RowCount == 0 || p.Distinct == 0 || !captured {
		return nil, nil, DiagStatusNotEnum
	}

	realSet := make(map[string]struct{}, len(p.TopValues))
	for _, v := range p.TopValues {
		realSet[normPhys(v.Value)] = struct{}{}
	}
	mappedSet := make(map[string]struct{}, len(valueMap))
	for _, phys := range valueMap {
		mappedSet[normPhys(phys)] = struct{}{}
	}

	// 死映射：ValueMap 指向的物理值不在真实枚举集合里。map 迭代序不稳定，按标签排序保证
	// 报告输出确定（供 HTTP/前端消费，避免无谓 diff 抖动）。
	for label, phys := range valueMap {
		if _, ok := realSet[normPhys(phys)]; !ok {
			dead = append(dead, DeadMapping{Label: label, PhysicalValue: phys})
		}
	}
	sort.Slice(dead, func(i, j int) bool { return dead[i].Label < dead[j].Label })
	// 覆盖缺口：真实枚举值未被任何 ValueMap 物理值覆盖（按频次降序，画像已排序）。
	for _, v := range p.TopValues {
		if _, ok := mappedSet[normPhys(v.Value)]; !ok {
			gaps = append(gaps, v)
		}
	}
	return dead, gaps, DiagStatusOK
}

// pickColumnProfile 从「按数据源+表」检索到的画像里取指定列的画像，并返回该列命中的
// 不同 schema 数（>1 表示跨库同名表多义，坐标不可靠）。列名匹配大小写不敏感。
func pickColumnProfile(profs []*dataprofile.ColumnProfile, column string) (*dataprofile.ColumnProfile, int) {
	target := normValue(column)
	var picked *dataprofile.ColumnProfile
	schemas := make(map[string]struct{}, 2)
	for _, p := range profs {
		if normValue(p.ColumnName) != target {
			continue
		}
		schemas[p.SchemaName] = struct{}{}
		if picked == nil {
			picked = p
		}
	}
	return picked, len(schemas)
}

// isBareColumn 判断 Expr 是否为裸列名（可选反引号包裹），而非 LOWER(x)/a+b 之类的表达式。
// 只有裸列名才能对齐画像坐标（画像以物理列名为键）。
func isBareColumn(expr string) bool {
	s := strings.TrimSpace(expr)
	s = strings.TrimPrefix(s, "`")
	s = strings.TrimSuffix(s, "`")
	if s == "" {
		return false
	}
	for i, r := range s {
		// 与 MySQL 未加引号标识符口径对齐：[0-9a-zA-Z$_] 及 U+0080 以上（含 CJK）。
		// 本库物理列名常为中文，若不放行 CJK/$ 会把它们误判为表达式而永不诊断。
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == '$' || r >= 0x80
		isDigit := r >= '0' && r <= '9'
		if i == 0 && !isLetter {
			return false
		}
		if !isLetter && !isDigit {
			return false
		}
	}
	return true
}

// normValue 归一化标识符（列名）用于集合比对：小写 + 去首尾空白。列名无空白语义，可安全裁剪。
func normValue(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// normPhys 归一化物理枚举「取值」用于与真实数据比对：仅折叠大小写、不裁空白。
// 见 diagnoseValueMap 注释——运行时物理值原样入 SQL，空白在 MySQL `=` 下是显著的。
func normPhys(s string) string {
	return strings.ToLower(s)
}

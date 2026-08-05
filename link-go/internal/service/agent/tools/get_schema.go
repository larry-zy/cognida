// Package tools 提供 Schema 获取工具
package tools

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"link/internal/model/dataprofile"
	model_datasource "link/internal/model/datasource"
)

// ========================================
// Get Schema Tool
// ========================================

// GetSchemaRequest Schema 获取请求
type GetSchemaRequest struct {
	// DatabaseID 数据源ID（可选）。空=当前业务库（或会话选定的数据源）；非空=已注册外部数据源 ID
	DatabaseID string `json:"database_id" jsonschema:"description=数据源ID，可选。空为当前库，非空为已注册外部数据源ID"`

	// TableName 表名（可选，指定则精确返回该表的完整结构）
	TableName string `json:"table_name" jsonschema:"description=表名，可选，指定则精确返回该表的完整结构"`

	// Keywords 关键词/查询意图（可选）。未指定 table_name 时用于在全部表描述中
	// 按相关度筛选候选表子集，避免把全库结构一次性灌入。
	Keywords string `json:"keywords" jsonschema:"description=关键词或查询意图，可选。未指定表名时据此从全部表描述中按相关度返回候选表子集"`
}

// TableSchema 表结构定义
type TableSchema struct {
	TableName  string         `json:"table_name"`
	Description string         `json:"description,omitempty"`
	Columns    []ColumnSchema `json:"columns"`
	PrimaryKey string         `json:"primary_key,omitempty"`
}

// ColumnSchema 列结构定义
type ColumnSchema struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Nullable    bool   `json:"nullable"`
	Description string `json:"description,omitempty"`
	// Facts 数据事实（空值率/基数/枚举分布），仅在精确返回单表且已接线列画像存储时附上；
	// 缺失表示未接线或尚未画像。供 LLM 用真实取值替代猜测，提升 Text2SQL 准确率。
	Facts *ColumnFacts `json:"facts,omitempty"`
}

// GetSchemaResult Schema 获取结果
type GetSchemaResult struct {
	// Tables 表结构列表
	Tables []TableSchema `json:"tables"`

	// Database 数据库名
	Database string `json:"database"`

	// Note 选表说明（如"按相关度返回候选子集""未找到相关表，返回目录"），便于 LLM 决策。
	Note string `json:"note,omitempty"`
}

// 未指定表名时的选表上限：禁止无上限全库注入。
const (
	// maxRelevantTables 关键词命中时返回的详细候选表上限
	maxRelevantTables = 15
	// maxCatalogTables 无关键词时返回的「表目录」（仅名+描述）上限
	maxCatalogTables = 300
)

// NewGetSchemaTool 创建 Schema 获取工具
// 使用基类 TypedBaseTool 实现类型安全；db 业务库、dsp 外部数据源提供者（可为 nil）、
// profileStore 列画像存储（可为 nil，缺失则不带数据事实、零回归）经参数注入。
func NewGetSchemaTool(db *gorm.DB, dsp model_datasource.ConnectionProvider, profileStore dataprofile.Store) *TypedBaseTool[GetSchemaRequest, GetSchemaResult] {
	handler := func(ctx context.Context, req *GetSchemaRequest) (*GetSchemaResult, error) {
		return getSchema(ctx, req, db, dsp, profileStore)
	}
	return NewTypedBaseTool("get_schema",
		`获取数据库表结构信息，用于生成 SQL 查询。

用法（避免一次性拉全库）：
- 已知目标表：传 table_name，精确返回该表完整列结构。
- 只有查询意图：传 keywords（如"区域 销售额"），从全部表描述中按相关度返回候选表子集及其结构。
- 都不传：返回轻量「表目录」（表名+描述，不含列），据此再用 table_name/keywords 收敛。

参数：
- database_id: 数据源ID（可选，空为当前库，非空为已注册外部数据源）
- table_name: 表名（可选，精确返回）
- keywords: 关键词/查询意图（可选，按相关度选表）`,
		handler,
	)
}

// FetchSchema 导出的 Schema 查询入口，供 HTTP handler 等外部调用方复用（前端 schema 浏览器）。
// 与 agent 工具不同：未指定表名时返回全库全部表的完整结构（不做相关度收敛）。
func FetchSchema(ctx context.Context, db *gorm.DB, dsp model_datasource.ConnectionProvider, databaseID, tableName string) (*GetSchemaResult, error) {
	target, err := resolveQueryTarget(ctx, databaseID, db, dsp)
	if err != nil {
		return nil, err
	}
	if _, err := target.databaseName(); err != nil {
		return nil, err
	}

	var names []string
	if tableName != "" {
		names = []string{tableName}
	} else {
		if names, err = listTableNames(ctx, target); err != nil {
			return nil, err
		}
	}
	tables, err := queryTableSchemas(ctx, target, names)
	if err != nil {
		return nil, err
	}
	return &GetSchemaResult{Tables: tables, Database: target.dbName}, nil
}

// getSchema 是 agent 工具处理器：施加「有界选表」策略，未指定表名时禁止全库详细注入。
// database_id 非空时经 ConnectionProvider 路由到外部数据源；无效 id 显式报错不回落业务库。
func getSchema(ctx context.Context, req *GetSchemaRequest, businessDB *gorm.DB, dsp model_datasource.ConnectionProvider, profileStore dataprofile.Store) (*GetSchemaResult, error) {
	target, err := resolveQueryTarget(ctx, req.DatabaseID, businessDB, dsp)
	if err != nil {
		return nil, err
	}
	if _, err := target.databaseName(); err != nil {
		return nil, err
	}

	// 1) 指定表名：精确返回该表完整结构，并惰性附上数据事实（写通刷新过期画像）。
	if req.TableName != "" {
		tables, err := queryTableSchemas(ctx, target, []string{req.TableName})
		if err != nil {
			return nil, err
		}
		if len(tables) == 1 {
			attachAndRefreshProfiles(ctx, profileStore, businessDB, dsp, target, req.DatabaseID, &tables[0])
		}
		return &GetSchemaResult{Tables: tables, Database: target.dbName}, nil
	}

	// 载入全部表描述卡片（名/表注释/列名/列注释），作为选表与目录的共同数据源。
	cards, err := loadTableCards(ctx, target)
	if err != nil {
		return nil, err
	}

	// 2) 有关键词：从全部表描述中按相关度选出候选子集，返回其完整结构。
	if kw := strings.TrimSpace(req.Keywords); kw != "" {
		selected := rankTablesByRelevance(cards, kw, maxRelevantTables)
		if len(selected) > 0 {
			tables, err := queryTableSchemas(ctx, target, selected)
			if err != nil {
				return nil, err
			}
			return &GetSchemaResult{
				Tables:   tables,
				Database: target.dbName,
				Note:     fmt.Sprintf("按关键词相关度从 %d 张表中选出 %d 张候选（如需其它表请指定 table_name）", len(cards), len(tables)),
			}, nil
		}
		// 3) 无命中：返回受上限约束的轻量目录，绝不无上限全库详细回退。
		return catalogResult(target.dbName, cards, "未找到与关键词明显相关的表，返回轻量表目录（表名+描述），请据此指定 table_name")
	}

	// 4) 无表名无关键词：返回轻量「表目录」（表名+描述，不含列）。
	return catalogResult(target.dbName, cards, "返回轻量表目录（表名+描述，不含列）；请用 table_name 或 keywords 收敛到具体表")
}

// listTableNames 返回库内全部基础表名。
func listTableNames(ctx context.Context, target *queryTarget) ([]string, error) {
	rows, err := target.db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = ? AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`, target.dbName)
	if err != nil {
		return nil, fmt.Errorf("查询表列表失败: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("查询表列表失败: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// queryTableSchemas 返回指定表的完整结构（列/主键）。names 为空返回空。
func queryTableSchemas(ctx context.Context, target *queryTarget, names []string) ([]TableSchema, error) {
	result := make([]TableSchema, 0, len(names))
	for _, tableName := range names {
		colSchemas, err := queryTableColumns(ctx, target, tableName)
		if err != nil {
			return nil, err
		}
		// 无列即表不存在（MySQL 表至少一列），跳过——省去单独的存在性查询。
		if len(colSchemas) == 0 {
			continue
		}

		pk, err := queryPrimaryKey(ctx, target, tableName)
		if err != nil {
			// 主键信息缺失不阻断结构返回
			pk = ""
		}

		result = append(result, TableSchema{
			TableName:  tableName,
			Columns:    colSchemas,
			PrimaryKey: pk,
		})
	}
	return result, nil
}

// queryTableColumns 查询单表列结构。
func queryTableColumns(ctx context.Context, target *queryTarget, tableName string) ([]ColumnSchema, error) {
	rows, err := target.db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable, column_comment
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ?
		ORDER BY ordinal_position
	`, target.dbName, tableName)
	if err != nil {
		return nil, fmt.Errorf("查询列信息失败: %w", err)
	}
	defer rows.Close()

	var colSchemas []ColumnSchema
	for rows.Next() {
		var name, dataType, isNullable string
		var comment sql.NullString
		if err := rows.Scan(&name, &dataType, &isNullable, &comment); err != nil {
			return nil, fmt.Errorf("查询列信息失败: %w", err)
		}
		colSchemas = append(colSchemas, ColumnSchema{
			Name:        name,
			Type:        dataType,
			Nullable:    strings.EqualFold(isNullable, "YES"),
			Description: comment.String,
		})
	}
	return colSchemas, rows.Err()
}

// queryPrimaryKey 查询单表主键首列。
func queryPrimaryKey(ctx context.Context, target *queryTarget, tableName string) (string, error) {
	rows, err := target.db.QueryContext(ctx, `
		SELECT column_name
		FROM information_schema.key_column_usage
		WHERE table_schema = ? AND table_name = ? AND constraint_name = 'PRIMARY'
		ORDER BY ordinal_position
	`, target.dbName, tableName)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	if rows.Next() {
		var pk string
		if err := rows.Scan(&pk); err != nil {
			return "", err
		}
		return pk, nil
	}
	return "", rows.Err()
}

// tableCard 是一张表的「描述卡」：用于相关度打分与轻量目录，避免为选表加载完整结构。
type tableCard struct {
	Name    string
	Comment string
	Columns []string // 列名 + 列注释拼接文本
}

// searchText 返回卡片的可检索文本（表名分词 + 表注释 + 列名/列注释）。
func (c tableCard) searchText() string {
	parts := make([]string, 0, len(c.Columns)+2)
	parts = append(parts, strings.ReplaceAll(c.Name, "_", " "), c.Comment)
	parts = append(parts, c.Columns...)
	return strings.ToLower(strings.Join(parts, " "))
}

// loadTableCards 一次性载入全部表的描述卡（2 次 information_schema 查询）。
func loadTableCards(ctx context.Context, target *queryTarget) ([]tableCard, error) {
	tRows, err := target.db.QueryContext(ctx, `
		SELECT table_name, table_comment
		FROM information_schema.tables
		WHERE table_schema = ? AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`, target.dbName)
	if err != nil {
		return nil, fmt.Errorf("查询表列表失败: %w", err)
	}
	defer tRows.Close()

	var cards []tableCard
	for tRows.Next() {
		var name string
		var comment sql.NullString
		if err := tRows.Scan(&name, &comment); err != nil {
			return nil, fmt.Errorf("查询表列表失败: %w", err)
		}
		cards = append(cards, tableCard{Name: name, Comment: comment.String})
	}
	if err := tRows.Err(); err != nil {
		return nil, fmt.Errorf("查询表列表失败: %w", err)
	}

	cRows, err := target.db.QueryContext(ctx, `
		SELECT table_name, column_name, column_comment
		FROM information_schema.columns
		WHERE table_schema = ?
		ORDER BY table_name, ordinal_position
	`, target.dbName)
	if err != nil {
		return nil, fmt.Errorf("查询列信息失败: %w", err)
	}
	defer cRows.Close()

	byName := make(map[string]*tableCard, len(cards))
	for i := range cards {
		byName[cards[i].Name] = &cards[i]
	}
	for cRows.Next() {
		var tableName, columnName string
		var columnComment sql.NullString
		if err := cRows.Scan(&tableName, &columnName, &columnComment); err != nil {
			return nil, fmt.Errorf("查询列信息失败: %w", err)
		}
		if card, ok := byName[tableName]; ok {
			frag := columnName
			if columnComment.String != "" {
				frag = frag + " " + columnComment.String
			}
			card.Columns = append(card.Columns, strings.ReplaceAll(frag, "_", " "))
		}
	}
	if err := cRows.Err(); err != nil {
		return nil, fmt.Errorf("查询列信息失败: %w", err)
	}
	return cards, nil
}

// rankTablesByRelevance 用关键词分词对表描述卡做词法相关度打分，返回 Top-K 表名。
// 打分：表名命中权重更高（×3），其余（注释/列）命中计 1。无命中（score=0）不入选。
func rankTablesByRelevance(cards []tableCard, keywords string, topK int) []string {
	terms := tokenize(keywords)
	if len(terms) == 0 {
		return nil
	}
	type scored struct {
		name  string
		score int
	}
	ranked := make([]scored, 0, len(cards))
	for _, card := range cards {
		nameText := strings.ToLower(strings.ReplaceAll(card.Name, "_", " "))
		bodyText := card.searchText()
		score := 0
		for _, term := range terms {
			if strings.Contains(nameText, term) {
				score += 3
			} else if strings.Contains(bodyText, term) {
				score++
			}
		}
		if score > 0 {
			ranked = append(ranked, scored{name: card.Name, score: score})
		}
	}
	// 稳定排序：分数降序，同分按表名字典序，保证结果确定性。
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].name < ranked[j].name
	})
	if len(ranked) > topK {
		ranked = ranked[:topK]
	}
	names := make([]string, 0, len(ranked))
	for _, r := range ranked {
		names = append(names, r.name)
	}
	return names
}

// catalogResult 构造轻量表目录（表名+描述，不含列），受 maxCatalogTables 上限约束。
func catalogResult(dbName string, cards []tableCard, note string) (*GetSchemaResult, error) {
	limit := len(cards)
	truncated := false
	if limit > maxCatalogTables {
		limit = maxCatalogTables
		truncated = true
	}
	tables := make([]TableSchema, 0, limit)
	for _, card := range cards[:limit] {
		tables = append(tables, TableSchema{TableName: card.Name, Description: card.Comment})
	}
	if truncated {
		note = fmt.Sprintf("%s（表数 %d 超过上限，仅列出前 %d 张，请用 keywords 收敛）", note, len(cards), maxCatalogTables)
	}
	return &GetSchemaResult{Tables: tables, Database: dbName, Note: note}, nil
}

// tokenize 把关键词切成小写词元（按空白与常见分隔符/下划线）。
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', ',', '，', '、', '/', '_', '-', '.', ';', '；', ':', '：':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		if f == "" {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return out
}

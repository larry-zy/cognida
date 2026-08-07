// Package tools —— 列画像（数据事实）的采集与写通。
//
// get_schema 精确返回某表结构时，除了 information_schema 的静态元数据（列名/类型/注释），
// 还惰性附上「数据事实」：空值率、基数、以及低基数列的实际枚举取值分布。这些事实只有
// 扫描真实数据才能知道，能把 Agent 从「猜 status='已完成'」纠正为「用真实值 completed」。
//
// 写通策略（lazy write-through）：读时先查缓存并附上；缓存缺失或过期则后台异步画像 +
// upsert，绝不阻塞本次取结构。画像走一次聚合扫描（COUNT(*) + 各列空值/基数），枚举候选
// 列再各跑一次 GROUP BY 取 Top-N 分布，全部带超时与列级容错。
package tools

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	agentctx "link/internal/model/agent"
	"link/internal/model/dataprofile"
	model_datasource "link/internal/model/datasource"
)

const (
	// profileTTL 画像时效：超过则视为过期，触发后台重算（数据分布会随时间漂移）。
	profileTTL = 24 * time.Hour
	// profileTimeout 单次后台画像的总超时（聚合 + 枚举扫描共用）。
	profileTimeout = 60 * time.Second
	// enumDistinctThreshold 基数不超过此值的列才视为枚举候选，采集 Top 取值分布。
	enumDistinctThreshold = 50
	// topValuesLimit 单枚举列返回的取值分布条数上限。
	topValuesLimit = 20
	// maxEnumColumns 单表最多为多少个枚举候选列跑 GROUP BY，防宽表画像放大扫描。
	maxEnumColumns = 16
	// maxProfileRefreshPerCall 单次 get_schema（关键词选表）最多触发多少张表的后台画像。
	// 候选表可达 maxRelevantTables 张；冷库首次选表若对每张都触发全表聚合+枚举扫描，会形成
	// 扫描风暴。故缓存事实对全部候选表照挂（廉价），但后台刷新按此上限限流——未刷到的表
	// 仍挂着旧/空快照，下次选表继续预热，跨调用逐步收敛。
	maxProfileRefreshPerCall = 6
)

// nonEnumTypes 明确不作为枚举候选的物理类型（连续型/大文本/时间/二进制）：
// 对它们枚举取值无意义且体积大，跳过 GROUP BY。
var nonEnumTypes = map[string]struct{}{
	"text": {}, "tinytext": {}, "mediumtext": {}, "longtext": {},
	"blob": {}, "tinyblob": {}, "mediumblob": {}, "longblob": {},
	"json": {}, "date": {}, "datetime": {}, "timestamp": {}, "time": {}, "year": {},
	"decimal": {}, "float": {}, "double": {}, "bit": {},
	"binary": {}, "varbinary": {}, "geometry": {},
}

// spatialTypes 几何/空间类型无法参与比较排序，故不能 COUNT(DISTINCT)。聚合扫描是
// 整表一条语句：只要有一个几何列，COUNT(DISTINCT geom) 就会让整条聚合报错
// （Illegal parameter data types geometry），拖垮全表所有列的画像并陷入反复重扫。
// 对这些列跳过基数统计（以 NULL 占位保持列序），空值率仍可正常采集。
var spatialTypes = map[string]struct{}{
	"geometry": {}, "point": {}, "linestring": {}, "polygon": {},
	"multipoint": {}, "multilinestring": {}, "multipolygon": {}, "geometrycollection": {},
}

// ColumnFacts 是附给 LLM 的列数据事实（get_schema 精确返回时随列结构下发）。
type ColumnFacts struct {
	RowCount  int64                        `json:"row_count"`
	NullRate  float64                      `json:"null_rate"`
	Distinct  int64                        `json:"distinct"`
	TopValues []dataprofile.ValueFrequency `json:"top_values,omitempty"`
	// Stale 表示读到的画像已过期、后台正在刷新；本次仍返回旧快照供参考。
	Stale bool `json:"stale,omitempty"`
}

// profileInFlight 去重后台画像：同一物理坐标同时只允许一个画像任务在跑，
// 避免同表被并发探查时重复全表扫描。
var profileInFlight sync.Map // key: coordinate string -> struct{}

// triggerProfileRefresh 指向后台画像触发器；抽成变量以便单测观测触发次数
// （验证批量选表的刷新限流）。生产恒为 triggerBackgroundProfile。
var triggerProfileRefresh = triggerBackgroundProfile

// effectiveDatasourceID 解析画像归属的数据源 ID：显式 database_id 优先，
// 否则回落会话选定的数据源；仍为空表示当前业务库（以空串入库消歧）。
func effectiveDatasourceID(ctx context.Context, databaseID string) string {
	if databaseID != "" {
		return databaseID
	}
	return agentctx.MustGetDatasourceID(ctx)
}

// attachCachedFacts 读缓存画像并按列名挂到表结构上（不触发后台刷新），返回该表画像是否过期
// （需刷新）以及本次是否适用（store 未接线/空表返回 ok=false）。是「挂缓存」与「决定是否刷新」
// 的共享底座，供单表精确路径与多候选表批量路径复用。
func attachCachedFacts(ctx context.Context, store dataprofile.Store, target *queryTarget, databaseID string, table *TableSchema) (stale, ok bool) {
	if store == nil || table == nil || len(table.Columns) == 0 {
		return false, false
	}
	tenantID := agentctx.MustGetTenantID(ctx)
	dsID := effectiveDatasourceID(ctx, databaseID)
	schema := target.dbName

	cached, err := store.ListByTable(ctx, tenantID, dsID, schema, table.TableName)
	if err != nil {
		// 读画像失败不影响结构返回，仅记告警。
		log.Printf("[column_profile] 读画像失败 %s.%s: %v", schema, table.TableName, err)
		cached = nil
	}

	stale = profilesStale(cached)
	attachColumnFacts(table, cached, stale)
	return stale, true
}

// attachAndRefreshProfiles 为精确返回的单表附上缓存中的数据事实，并在缺失/过期时
// 触发后台刷新。store 为 nil（未接线）或非单表时静默跳过——零回归。
// businessDB/dsp 供后台画像重新路由目标库（而非捕获请求期 target.db，见 resolveProfileDB）。
func attachAndRefreshProfiles(ctx context.Context, store dataprofile.Store, businessDB *gorm.DB, dsp model_datasource.ConnectionProvider, target *queryTarget, databaseID string, table *TableSchema) {
	stale, ok := attachCachedFacts(ctx, store, target, databaseID, table)
	if !ok || !stale {
		return
	}
	triggerProfileRefresh(store, businessDB, dsp, agentctx.MustGetTenantID(ctx), effectiveDatasourceID(ctx, databaseID), target.dbName, *table)
}

// attachAndRefreshProfilesBatch 为关键词选出的候选表批量附上缓存数据事实。缓存命中即挂
// （廉价，只读缓存无扫描），让 Agent 写 SQL 前就能看到真实枚举值/空值率、不再猜值；过期表按
// maxProfileRefreshPerCall 上限触发后台刷新，避免冷库首次选表对十余张表同时全表扫描（扫描风暴）。
// 超限未刷新的表仍挂着旧/空快照，下次选表继续预热。
func attachAndRefreshProfilesBatch(ctx context.Context, store dataprofile.Store, businessDB *gorm.DB, dsp model_datasource.ConnectionProvider, target *queryTarget, databaseID string, tables []TableSchema) {
	if store == nil {
		return
	}
	refreshed := 0
	for i := range tables {
		stale, ok := attachCachedFacts(ctx, store, target, databaseID, &tables[i])
		if !ok || !stale {
			continue
		}
		if refreshed >= maxProfileRefreshPerCall {
			continue // 已挂旧/空快照，仅限流后台刷新
		}
		triggerProfileRefresh(store, businessDB, dsp, agentctx.MustGetTenantID(ctx), effectiveDatasourceID(ctx, databaseID), target.dbName, tables[i])
		refreshed++
	}
}

// resolveProfileDB 后台画像执行时（重新）取目标库连接池，而非捕获请求期的 target.db：
//   - 业务库取 gorm 进程级池（长生命周期，永不被关，安全）；
//   - 外部数据源经 provider 按坐标重新 Acquire——受管池即便被空闲回收或配置变更
//     (Invalidate) 关闭，也能拿到当前版本的池，且 Acquire 会刷新 lastUsed 免被误回收。
//
// 消除「把受管连接池句柄捕获过异步边界」的隐患：请求返回后 target.db 可能被关闭，
// 而这里在 goroutine 内按同一坐标重取，始终是有效句柄。
func resolveProfileDB(ctx context.Context, businessDB *gorm.DB, dsp model_datasource.ConnectionProvider, tenantID int64, datasourceID string) (*sql.DB, error) {
	if datasourceID == "" {
		if businessDB == nil {
			return nil, fmt.Errorf("业务库未初始化")
		}
		return businessDB.DB()
	}
	if dsp == nil {
		return nil, fmt.Errorf("外部数据源提供者未注入")
	}
	db, _, err := dsp.Acquire(ctx, tenantID, datasourceID)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// profilesStale 判定画像是否需要（重新）采集：为空或最新一条已超过 TTL 即过期。
func profilesStale(profiles []*dataprofile.ColumnProfile) bool {
	if len(profiles) == 0 {
		return true
	}
	newest := profiles[0].ProfiledAt
	for _, p := range profiles[1:] {
		if p.ProfiledAt.After(newest) {
			newest = p.ProfiledAt
		}
	}
	return time.Since(newest) > profileTTL
}

// attachColumnFacts 把画像按列名对齐挂到列结构上（缺画像的列保持无事实）。
func attachColumnFacts(table *TableSchema, profiles []*dataprofile.ColumnProfile, stale bool) {
	if len(profiles) == 0 {
		return
	}
	byCol := make(map[string]*dataprofile.ColumnProfile, len(profiles))
	for _, p := range profiles {
		byCol[p.ColumnName] = p
	}
	for i := range table.Columns {
		p, ok := byCol[table.Columns[i].Name]
		if !ok {
			continue
		}
		table.Columns[i].Facts = &ColumnFacts{
			RowCount:  p.RowCount,
			NullRate:  p.NullRate,
			Distinct:  p.Distinct,
			TopValues: p.TopValues,
			Stale:     stale,
		}
	}
}

// triggerBackgroundProfile 异步画像并写回缓存；用独立 background 上下文（本请求 ctx
// 会随工具返回被取消），带超时与坐标级去重。目标库在 goroutine 内按坐标重新取（不捕获
// 请求期句柄，见 resolveProfileDB）。失败仅记日志，不反哺调用方。
func triggerBackgroundProfile(store dataprofile.Store, businessDB *gorm.DB, dsp model_datasource.ConnectionProvider, tenantID int64, datasourceID, schema string, table TableSchema) {
	key := profileKey(tenantID, datasourceID, schema, table.TableName)
	if _, loaded := profileInFlight.LoadOrStore(key, struct{}{}); loaded {
		return // 同坐标已有画像在跑
	}
	go func() {
		defer profileInFlight.Delete(key)
		// 后台 goroutine 脱离请求栈，无上层 HTTP recover 兜底；任何 panic（驱动/扫描）
		// 未捕获会拖垮整个进程，这里就地兜住只降级为一次画像失败。
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[column_profile] 画像 goroutine panic %s.%s: %v", schema, table.TableName, r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), profileTimeout)
		defer cancel()

		db, err := resolveProfileDB(ctx, businessDB, dsp, tenantID, datasourceID)
		if err != nil {
			log.Printf("[column_profile] 取目标库失败 %s.%s: %v", schema, table.TableName, err)
			return
		}
		profiles, err := computeColumnProfiles(ctx, db, tenantID, datasourceID, schema, table)
		if err != nil {
			log.Printf("[column_profile] 画像失败 %s.%s: %v", schema, table.TableName, err)
			return
		}
		if err := store.Upsert(ctx, profiles); err != nil {
			log.Printf("[column_profile] 画像写库失败 %s.%s: %v", schema, table.TableName, err)
		}
	}()
}

func profileKey(tenantID int64, datasourceID, schema, table string) string {
	return fmt.Sprintf("%d\x00%s\x00%s\x00%s", tenantID, datasourceID, schema, table)
}

// computeColumnProfiles 对单表执行一次数据事实采集：
//  1. 一次聚合扫描得表行数 + 各列空值数 + 各列基数；
//  2. 对低基数枚举候选列各跑一次 GROUP BY 取 Top-N 取值分布（列级容错）。
//
// 返回每列一条快照。空表（0 行）也返回零值快照，避免反复重扫。
func computeColumnProfiles(ctx context.Context, db *sql.DB, tenantID int64, datasourceID, schema string, table TableSchema) ([]*dataprofile.ColumnProfile, error) {
	now := time.Now()
	cols := table.Columns
	tableRef := quoteMySQLIdent(schema) + "." + quoteMySQLIdent(table.TableName)

	// 1) 聚合扫描：COUNT(*) + 每列 SUM(col IS NULL) + COUNT(DISTINCT col)。
	selects := make([]string, 0, len(cols)*2+1)
	selects = append(selects, "COUNT(*)")
	for _, c := range cols {
		q := quoteMySQLIdent(c.Name)
		distinctExpr := "COUNT(DISTINCT " + q + ")"
		if _, spatial := spatialTypes[baseType(c.Type)]; spatial {
			distinctExpr = "NULL" // 几何列不可 DISTINCT，占位保持列序，基数留 0
		}
		selects = append(selects, "SUM("+q+" IS NULL)", distinctExpr)
	}
	aggSQL := "SELECT " + strings.Join(selects, ", ") + " FROM " + tableRef

	row := db.QueryRowContext(ctx, aggSQL)
	scanDst := make([]interface{}, len(selects))
	scanVals := make([]sql.NullInt64, len(selects))
	for i := range scanVals {
		scanDst[i] = &scanVals[i]
	}
	if err := row.Scan(scanDst...); err != nil {
		return nil, fmt.Errorf("聚合画像扫描失败: %w", err)
	}

	rowCount := scanVals[0].Int64
	profiles := make([]*dataprofile.ColumnProfile, 0, len(cols))
	for i, c := range cols {
		nullCount := scanVals[1+i*2].Int64
		distinct := scanVals[2+i*2].Int64
		nullRate := 0.0
		if rowCount > 0 {
			nullRate = float64(nullCount) / float64(rowCount)
		}
		profiles = append(profiles, &dataprofile.ColumnProfile{
			TenantID:     tenantID,
			DatasourceID: datasourceID,
			SchemaName:   schema,
			TableName:    table.TableName,
			ColumnName:   c.Name,
			RowCount:     rowCount,
			NullCount:    nullCount,
			NullRate:     nullRate,
			Distinct:     distinct,
			ProfiledAt:   now,
		})
	}

	// 2) 枚举候选列 Top-N 取值分布（有界、列级容错）。
	if rowCount > 0 {
		enumBudget := maxEnumColumns
		for i, c := range cols {
			if enumBudget <= 0 {
				break
			}
			distinct := profiles[i].Distinct
			if !isEnumCandidate(c, distinct, rowCount) {
				continue
			}
			enumBudget--
			top, err := computeTopValues(ctx, db, tableRef, c.Name)
			if err != nil {
				log.Printf("[column_profile] 枚举取值采集失败 %s.%s.%s: %v", schema, table.TableName, c.Name, err)
				continue
			}
			profiles[i].TopValues = top
		}
	}
	return profiles, nil
}

// isEnumCandidate 判断列是否值得采集枚举分布：低基数、非唯一（排除标识列）、
// 且物理类型不属于连续/大文本/时间/二进制。
func isEnumCandidate(c ColumnSchema, distinct, rowCount int64) bool {
	if distinct <= 0 || distinct > enumDistinctThreshold {
		return false
	}
	if distinct >= rowCount { // 每行一值≈唯一标识，枚举无意义
		return false
	}
	base := baseType(c.Type)
	if _, bad := nonEnumTypes[base]; bad {
		return false
	}
	if _, spatial := spatialTypes[base]; spatial {
		return false
	}
	return true
}

// baseType 归一化物理类型名：小写、去空格、剥掉 varchar(64)/decimal(10,2) 之类的括号后缀，
// 便于按裸类型名查表（枚举/几何白名单）。
func baseType(colType string) string {
	base := strings.ToLower(strings.TrimSpace(colType))
	if idx := strings.IndexByte(base, '('); idx >= 0 {
		base = base[:idx]
	}
	return strings.TrimSpace(base)
}

// computeTopValues 取单列按频次降序的 Top-N 非空取值分布。
func computeTopValues(ctx context.Context, db *sql.DB, tableRef, column string) ([]dataprofile.ValueFrequency, error) {
	q := quoteMySQLIdent(column)
	sqlText := fmt.Sprintf(
		"SELECT %s AS v, COUNT(*) AS c FROM %s WHERE %s IS NOT NULL GROUP BY %s ORDER BY c DESC, v ASC LIMIT %d",
		q, tableRef, q, q, topValuesLimit,
	)
	rows, err := db.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []dataprofile.ValueFrequency
	for rows.Next() {
		var v sql.NullString
		var c int64
		if err := rows.Scan(&v, &c); err != nil {
			return nil, err
		}
		out = append(out, dataprofile.ValueFrequency{Value: v.String, Count: c})
	}
	return out, rows.Err()
}

// quoteMySQLIdent 以反引号转义 MySQL 标识符（内部反引号翻倍），防注入。
// get_schema 全链路本就假定 MySQL（information_schema.column_comment 等为 MySQL 专有），
// 画像沿用同一方言。
func quoteMySQLIdent(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

package datasource

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"strconv"
	"time"
)

const (
	// defaultSampleSize 抽样默认行数
	defaultSampleSize = 100
	// maxSampleSize 抽样行数上限（防止把大表整表拉进内存）
	maxSampleSize = 10000
	// sampleQueryTimeout 抽样查询超时
	sampleQueryTimeout = 30 * time.Second
)

// SampleCSV 从外部数据源指定表抽样最多 limit 行，序列化为 CSV 返回（含表头）。
// 供数据质量模块把样本推给 Python 分析——Python 侧无需、也不应直连外部数据库。
//
// 安全边界：表名先经真实表清单白名单校验再用 driver.QuoteIdent 转义，杜绝注入/越权；
// 只做 SELECT，套用受管连接池与查询超时。返回实际抽样到的行数。
func (s *Service) SampleCSV(ctx context.Context, tenantID int64, datasourceID, table string, limit int) ([]byte, int, error) {
	if limit <= 0 {
		limit = defaultSampleSize
	}
	if limit > maxSampleSize {
		limit = maxSampleSize
	}

	db, ds, err := s.cm.Acquire(ctx, tenantID, datasourceID)
	if err != nil {
		return nil, 0, err
	}
	drv, err := DriverFor(ds.Type)
	if err != nil {
		return nil, 0, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, sampleQueryTimeout)
	defer cancel()

	// 白名单校验：表必须在数据源真实表清单中，防注入与越权取数
	tables, err := drv.ListTables(queryCtx, db, ds.DatabaseName)
	if err != nil {
		return nil, 0, fmt.Errorf("datasource: 读取表清单失败: %w", err)
	}
	valid := false
	for _, t := range tables {
		if t.Name == table {
			valid = true
			break
		}
	}
	if !valid {
		return nil, 0, fmt.Errorf("datasource: 表 %q 不存在或不可访问", table)
	}

	// 表名已白名单校验并转义、limit 为经上限收敛的整数，拼接安全（无外部字符串入 SQL）
	// #nosec G202 -- 表名已对照数据源真实表清单白名单校验并经 drv.QuoteIdent 转义，limit 为整型，无外部字符串入 SQL
	query := "SELECT * FROM " + drv.QuoteIdent(table) + " LIMIT " + strconv.Itoa(limit)
	rows, err := db.QueryContext(queryCtx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("datasource: 抽样查询失败: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, 0, err
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(cols); err != nil {
		return nil, 0, err
	}

	raw := make([]sql.RawBytes, len(cols))
	scan := make([]any, len(cols))
	for i := range raw {
		scan[i] = &raw[i]
	}

	n := 0
	record := make([]string, len(cols))
	for rows.Next() {
		if err := rows.Scan(scan...); err != nil {
			return nil, 0, err
		}
		for i, rb := range raw {
			if rb == nil {
				record[i] = "" // NULL → 空串
			} else {
				record[i] = string(rb)
			}
		}
		if err := w.Write(record); err != nil {
			return nil, 0, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), n, nil
}

package datasource

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

// SampleRecords preserves SQL NULL, unlike CSV, and bounds both rows and bytes.
// Connections remain owned by ConnectionManager.
func (s *Service) SampleRecords(ctx context.Context, tenant int64, id, table string, limit int) ([]string, [][]any, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	db, ds, err := s.cm.Acquire(ctx, tenant, id)
	if err != nil {
		return nil, nil, err
	}
	drv, err := DriverFor(ds.Type)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, sampleQueryTimeout)
	defer cancel()
	tables, err := drv.ListTables(ctx, db, ds.DatabaseName)
	if err != nil {
		return nil, nil, err
	}
	found := false
	for _, t := range tables {
		if t.Name == table {
			found = true
			break
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("datasource: unknown table")
	}
	// #nosec G202 -- table is matched against live metadata above, driver-quoted, and LIMIT is a bounded integer.
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+drv.QuoteIdent(table)+" LIMIT "+strconv.Itoa(limit))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	if len(columns) > 512 {
		return nil, nil, fmt.Errorf("datasource: too many columns")
	}
	out := [][]any{}
	size := 0
	for rows.Next() {
		raw := make([]sql.RawBytes, len(columns))
		dest := make([]any, len(raw))
		for i := range raw {
			dest[i] = &raw[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, nil, err
		}
		record := make([]any, len(raw))
		for i, v := range raw {
			size += len(v)
			if len(v) > 65536 || size > 8<<20 {
				return nil, nil, fmt.Errorf("datasource: sample exceeds byte limit")
			}
			if v != nil {
				record[i] = string(v)
			}
		}
		out = append(out, record)
	}
	return columns, out, rows.Err()
}

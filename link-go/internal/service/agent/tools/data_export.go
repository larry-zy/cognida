// Package tools 提供 data_export 导出工具（Phase 4）。
package tools

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/xuri/excelize/v2"

	agentctx "link/internal/model/agent"
	"link/internal/model/agent/operations"
	"link/internal/service/agent/resultstore"
)

// ========================================
// Data Export Tool
// ========================================

// DataExportRequest 导出请求：按 result_id 导出为文件，返回文件引用而非内容。
type DataExportRequest struct {
	// ResultID 要导出的结果集引用
	ResultID string `json:"result_id" jsonschema:"required,description=要导出的结果集引用result_id"`

	// Format 导出格式：csv（默认）或 excel
	Format string `json:"format" jsonschema:"description=导出格式csv或excel，默认csv,enum=csv,enum=excel"`

	// Filename 文件名（可选，不含扩展名；默认按 result_id 生成）
	Filename string `json:"filename" jsonschema:"description=文件名，可选，不含扩展名"`
}

// DataExportResult 导出结果——只回文件引用，绝不回文件内容。
type DataExportResult struct {
	// Status success / rejected
	Status string `json:"status"`

	// FilePath 导出文件路径（文件引用）
	FilePath string `json:"file_path,omitempty"`

	// FileName 文件名
	FileName string `json:"file_name,omitempty"`

	// Format csv / excel
	Format string `json:"format,omitempty"`

	// RowCount 导出行数（不含表头）
	RowCount int `json:"row_count,omitempty"`

	// SizeBytes 文件大小
	SizeBytes int64 `json:"size_bytes,omitempty"`

	// Message 说明
	Message string `json:"message,omitempty"`

	// LatencyMs 耗时（毫秒）
	LatencyMs int64 `json:"latency_ms"`
}

// NewDataExportTool 创建导出工具；o 操作工具依赖（结果存储/审计/导出目录）经参数注入。
func NewDataExportTool(o *opTools) *TypedBaseTool[DataExportRequest, DataExportResult] {
	handler := func(ctx context.Context, req *DataExportRequest) (*DataExportResult, error) {
		return o.dataExport(ctx, req)
	}
	return NewTypedBaseTool("data_export",
		`把结果集导出为文件（CSV/Excel）。

说明：
- 按 result_id 从结果存储取本会话的完整结果集
- 返回文件引用（路径/大小/行数），绝不把文件内容回灌对话
- 属于白名单安全操作，自动执行；所有导出写入操作审计

参数：
- result_id: 结果集引用（必需）
- format: csv 或 excel（可选，默认 csv）
- filename: 文件名，不含扩展名（可选）`,
		handler,
	)
}

// dataExport 按 result_id 导出文件。白名单安全操作：自动执行，无需人机确认。
func (o *opTools) dataExport(ctx context.Context, req *DataExportRequest) (*DataExportResult, error) {
	startTime := time.Now()

	if req.ResultID == "" {
		return nil, fmt.Errorf("result_id 不能为空")
	}
	if o.resultStore == nil {
		return nil, fmt.Errorf("结果存储未启用，无法导出")
	}

	format := req.Format
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "excel" {
		return nil, fmt.Errorf("不支持的导出格式 %q，仅支持 csv / excel", format)
	}

	// 归属校验：只允许导出本会话的结果集
	owner := resultstore.OwnerKey(agentctx.MustGetTenantID(ctx), agentctx.MustGetSessionID(ctx))
	stored, err := o.resultStore.Get(ctx, owner, req.ResultID)
	if err != nil {
		var msg string
		switch err {
		case resultstore.ErrNotFound:
			msg = fmt.Sprintf("result_id %s 不存在或已过期，请重新取数", req.ResultID)
		case resultstore.ErrUnauthorized:
			msg = fmt.Sprintf("result_id %s 不属于当前会话", req.ResultID)
		default:
			return nil, fmt.Errorf("读取结果存储失败: %w", err)
		}
		o.recordAudit(ctx, operations.OpExport, req.ResultID, "", "",
			operations.StatusRejected, msg, map[string]interface{}{"format": format})
		return &DataExportResult{
			Status:    operations.StatusRejected,
			Message:   msg,
			LatencyMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 导出目录
	dir := o.cfg.ExportDir
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "link-agent-exports")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建导出目录失败: %w", err)
	}

	base := req.Filename
	if base == "" {
		base = "export_" + req.ResultID
	}
	base = sanitizeFilename(base)

	var filePath string
	switch format {
	case "csv":
		filePath = filepath.Join(dir, base+".csv")
		err = writeCSV(filePath, stored.Columns, stored.Rows)
	case "excel":
		filePath = filepath.Join(dir, base+".xlsx")
		err = writeXLSX(filePath, stored.Columns, stored.Rows)
	}
	if err != nil {
		o.recordAudit(ctx, operations.OpExport, req.ResultID, "", "",
			operations.StatusFailed, err.Error(), map[string]interface{}{"format": format})
		return nil, fmt.Errorf("导出失败: %w", err)
	}

	var size int64
	if fi, statErr := os.Stat(filePath); statErr == nil {
		size = fi.Size()
	}
	o.recordAudit(ctx, operations.OpExport, filePath, "", "",
		operations.StatusSuccess, fmt.Sprintf("导出 %d 行 → %s", len(stored.Rows), filePath),
		map[string]interface{}{"result_id": req.ResultID, "format": format, "row_count": len(stored.Rows), "size_bytes": size})

	return &DataExportResult{
		Status:    operations.StatusSuccess,
		FilePath:  filePath,
		FileName:  filepath.Base(filePath),
		Format:    format,
		RowCount:  len(stored.Rows),
		SizeBytes: size,
		LatencyMs: time.Since(startTime).Milliseconds(),
	}, nil
}

// sanitizeFilename 文件名白名单化：只保留字母/数字/下划线/连字符。
func sanitizeFilename(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "export"
	}
	return string(out)
}

// cellString 单元格值转字符串。
func cellString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case float64:
		return fmtNum(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// writeCSV 写 CSV 文件（UTF-8 BOM，Excel 直接打开不乱码）。
func writeCSV(path string, columns []string, rows []map[string]interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	w := csv.NewWriter(f)
	if err := w.Write(columns); err != nil {
		return err
	}
	record := make([]string, len(columns))
	for _, row := range rows {
		for i, col := range columns {
			record[i] = cellString(row[col])
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// writeXLSX 写 Excel 文件。
func writeXLSX(path string, columns []string, rows []map[string]interface{}) error {
	f := excelize.NewFile()
	defer f.Close()

	const sheet = "Sheet1"
	header := make([]interface{}, len(columns))
	for i, c := range columns {
		header[i] = c
	}
	if err := f.SetSheetRow(sheet, "A1", &header); err != nil {
		return err
	}
	for i, row := range rows {
		record := make([]interface{}, len(columns))
		for j, col := range columns {
			record[j] = row[col]
		}
		cell, err := excelize.CoordinatesToCellName(1, i+2)
		if err != nil {
			return err
		}
		if err := f.SetSheetRow(sheet, cell, &record); err != nil {
			return err
		}
	}
	return f.SaveAs(path)
}

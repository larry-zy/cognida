// Package handler 提供数据质量管理中心的 HTTP 处理器。
package handler

import (
	"errors"
	"io"
	"strings"

	"github.com/gin-gonic/gin"

	domain_quality "cognida/internal/model/quality"
	app_quality "cognida/internal/service/quality"
)

// respondEvalError 区分质量评估错误的 HTTP 语义：Python 因输入数据不合法而拒绝
// （InputRejectedError）属客户端错误，返回 400；其余（Python 不可用/超时/落库失败）
// 属服务端错误，返回 500。避免把用户粘贴的畸形 CSV 记成服务器故障。
func respondEvalError(c *gin.Context, err error) {
	var inputErr *app_quality.InputRejectedError
	if errors.As(err, &inputErr) {
		BadRequest(c, err.Error())
		return
	}
	// 未知质量维度属客户端输入问题（拼写/用错类型），返回 400。
	var dimErr *app_quality.InvalidDimensionError
	if errors.As(err, &dimErr) {
		BadRequest(c, err.Error())
		return
	}
	InternalError(c, err.Error())
}

// QualityHandler 数据质量处理器
type QualityHandler struct {
	service *app_quality.Service
}

// NewQualityHandler 创建数据质量处理器
func NewQualityHandler(service *app_quality.Service) *QualityHandler {
	return &QualityHandler{service: service}
}

const maxQualityUploadSize = 20 << 20 // 20MB

// readCSVInput 从请求提取结构化数据字节、来源名、维度与输入格式(csv/json)。
// 优先 multipart 文件(file)，否则回退 JSON body 里的 csv_data 文本。
// format 为空时按 csv 处理；json 表示 csv_data 内是对象数组文本，由 Python 端解析。
func (h *QualityHandler) readCSVInput(c *gin.Context) (data []byte, sourceName string, dimensions []string, format string, ok bool) {
	contentType := c.ContentType()
	if strings.HasPrefix(contentType, "multipart/form-data") {
		fileHeader, err := c.FormFile("file")
		if err != nil {
			BadRequest(c, "缺少上传文件 file")
			return nil, "", nil, "", false
		}
		if fileHeader.Size > maxQualityUploadSize {
			BadRequest(c, "文件超过 20MB 上限")
			return nil, "", nil, "", false
		}
		f, err := fileHeader.Open()
		if err != nil {
			InternalError(c, "打开上传文件失败")
			return nil, "", nil, "", false
		}
		defer f.Close()
		fileBytes, err := io.ReadAll(f)
		if err != nil {
			InternalError(c, "读取上传文件失败")
			return nil, "", nil, "", false
		}
		if dims := c.PostForm("dimensions"); dims != "" {
			dimensions = splitCSVParam(dims)
		}
		return fileBytes, fileHeader.Filename, dimensions, c.PostForm("format"), true
	}

	// JSON: { "csv_data": "...", "source_name": "...", "dimensions": [...], "format": "csv|json" }
	var req struct {
		CSVData    string   `json:"csv_data"`
		SourceName string   `json:"source_name"`
		Dimensions []string `json:"dimensions"`
		Format     string   `json:"format"`
	}
	if !BindJSON(c, &req) {
		return nil, "", nil, "", false
	}
	if strings.TrimSpace(req.CSVData) == "" {
		BadRequest(c, "数据不能为空")
		return nil, "", nil, "", false
	}
	name := req.SourceName
	if name == "" {
		name = "粘贴数据"
	}
	return []byte(req.CSVData), name, req.Dimensions, req.Format, true
}

func splitCSVParam(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// EvaluateStructured 结构化数据质量评估
// @Router /api/v1/quality/evaluate/structured [post]
func (h *QualityHandler) EvaluateStructured(c *gin.Context) {
	data, sourceName, dimensions, format, ok := h.readCSVInput(c)
	if !ok {
		return
	}
	report, err := h.service.EvaluateStructured(
		c.Request.Context(), GetTenantID(c), GetUserID(c), sourceName, data, format, dimensions)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	OK(c, report)
}

// EvaluateDatasource 数据源直评：从外部数据源指定表抽样后评估结构化质量。
// 取数全在 Go 侧完成（受管连接池 + 只读 SELECT），Python 不直连数据库。
// @Router /api/v1/quality/evaluate/datasource [post]
func (h *QualityHandler) EvaluateDatasource(c *gin.Context) {
	var req struct {
		DatasourceID string   `json:"datasource_id"`
		Table        string   `json:"table"`
		SampleSize   int      `json:"sample_size"`
		Dimensions   []string `json:"dimensions"`
	}
	if !BindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.DatasourceID) == "" {
		BadRequest(c, "datasource_id 不能为空")
		return
	}
	if strings.TrimSpace(req.Table) == "" {
		BadRequest(c, "table 不能为空")
		return
	}
	report, err := h.service.EvaluateDatasource(
		c.Request.Context(), GetTenantID(c), GetUserID(c),
		req.DatasourceID, req.Table, req.SampleSize, req.Dimensions)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	OK(c, report)
}

// EvaluateUnstructured 非结构化文本质量评估
// @Router /api/v1/quality/evaluate/unstructured [post]
func (h *QualityHandler) EvaluateUnstructured(c *gin.Context) {
	var req struct {
		Text       string   `json:"text"`
		SourceName string   `json:"source_name"`
		Dimensions []string `json:"dimensions"`
	}
	if !BindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		BadRequest(c, "text 不能为空")
		return
	}
	name := req.SourceName
	if name == "" {
		name = "粘贴文本"
	}
	report, err := h.service.EvaluateUnstructured(
		c.Request.Context(), GetTenantID(c), GetUserID(c), name, req.Text, req.Dimensions)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	OK(c, report)
}

// CleanData 数据清洗
// @Router /api/v1/quality/clean [post]
func (h *QualityHandler) CleanData(c *gin.Context) {
	// 复用数据提取逻辑：dimensions 位在清洗场景不用；format 为输入格式；
	// cleaners 与 output_format（导出格式）单独取。
	data, sourceName, _, format, ok := h.readCSVInput(c)
	if !ok {
		return
	}
	// body 已被 readCSVInput 读走，cleaners/output_format 从表单或 query 兜底获取。
	var cleaners []string
	var outputFormat string
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		if cl := c.PostForm("cleaners"); cl != "" {
			cleaners = splitCSVParam(cl)
		}
		outputFormat = c.PostForm("output_format")
	} else {
		if cl := c.Query("cleaners"); cl != "" {
			cleaners = splitCSVParam(cl)
		}
		outputFormat = c.Query("output_format")
	}
	report, err := h.service.Clean(
		c.Request.Context(), GetTenantID(c), GetUserID(c), sourceName, data, cleaners, format, outputFormat)
	if err != nil {
		respondEvalError(c, err)
		return
	}
	OK(c, report)
}

// ListDimensions 支持的质量维度
// @Router /api/v1/quality/dimensions [get]
func (h *QualityHandler) ListDimensions(c *gin.Context) {
	dims, err := h.service.ListDimensions(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	OK(c, gin.H{"dimensions": dims})
}

// ListRecords 历史质检记录列表
// @Router /api/v1/quality/records [get]
func (h *QualityHandler) ListRecords(c *gin.Context) {
	page, size := GetPageParams(c)
	filter := domain_quality.ListFilter{
		TenantID: GetTenantID(c),
		Type:     domain_quality.CheckType(c.Query("type")),
		Limit:    size,
		Offset:   (page - 1) * size,
	}
	records, total, err := h.service.ListRecords(c.Request.Context(), filter)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	// 列表不返回完整报告 JSON，避免体积过大。
	items := make([]gin.H, 0, len(records))
	for _, r := range records {
		items = append(items, gin.H{
			"id":            r.ID,
			"type":          r.Type,
			"source_name":   r.SourceName,
			"overall_score": r.OverallScore,
			"record_count":  r.RecordCount,
			"summary":       r.Summary,
			"created_at":    r.CreatedAt,
		})
	}
	PageSuccessJSON(c, total, items, page, size)
}

// GetRecord 历史质检记录详情（含完整报告快照）
// @Router /api/v1/quality/records/:id [get]
func (h *QualityHandler) GetRecord(c *gin.Context) {
	id := c.Param("id")
	record, err := h.service.GetRecord(c.Request.Context(), id, GetTenantID(c))
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if record == nil {
		NotFound(c, "质检记录不存在")
		return
	}
	OK(c, gin.H{
		"id":            record.ID,
		"type":          record.Type,
		"source_name":   record.SourceName,
		"overall_score": record.OverallScore,
		"record_count":  record.RecordCount,
		"summary":       record.Summary,
		"report":        rawJSON(record.ReportJSON),
		"created_at":    record.CreatedAt,
	})
}

// DeleteRecord 删除历史质检记录
// @Router /api/v1/quality/records/:id [delete]
func (h *QualityHandler) DeleteRecord(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DeleteRecord(c.Request.Context(), id, GetTenantID(c)); err != nil {
		InternalError(c, err.Error())
		return
	}
	OK(c, gin.H{"deleted": id})
}

// rawJSON 把存储的报告 JSON 原样透传给前端（避免二次转义）。
func rawJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return jsonRawMessage(b)
}

// jsonRawMessage 让 gin 直接输出原始 JSON 字节。
type jsonRawMessage []byte

func (m jsonRawMessage) MarshalJSON() ([]byte, error) {
	if len(m) == 0 {
		return []byte("null"), nil
	}
	return m, nil
}

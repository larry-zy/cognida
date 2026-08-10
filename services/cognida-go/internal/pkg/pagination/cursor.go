// Package pagination 提供可复用的「不透明游标（keyset）」分页原语〔M5〕。
//
// 背景：项目约定「分页使用 cursor 而非 offset」，但既有列表接口普遍以
// `page/offset` 实现——offset 在大偏移下需扫描并丢弃前 N 行，深翻页性能劣化，
// 且在并发写入时会漏读/重读（页与页之间行数漂移）。keyset 游标以「上一页末行的
// 排序位置」为锚点，翻页恒定代价且对并发写入稳定。
//
// 本包只提供机制，不绑定具体表：
//   - Cursor 是**不透明**的线格式（对客户端不可解析，base64url(JSON)）；
//   - KeysetPredicate 生成 `(sortCol, idCol)` 复合 keyset 的 WHERE 片段，
//     由调用方拼到既有 GORM 查询上。
//
// 迁移策略（避免破坏前端既有 `page/total` 契约）：新接口/自然按时间或自增 id
// 排序的接口优先采用游标；存量 offset 接口以「携带 cursor 参数即走 keyset、
// 否则维持 offset」的增量方式接入（参考 audit 日志列表），逐步收敛。
package pagination

import (
	"encoding/base64"
	"encoding/json"
)

// 每页条数边界：非正值回落 DefaultLimit，超上限截断到 MaxLimit。
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Direction 排序方向。keyset 谓词方向必须与 ORDER BY 方向一致。
type Direction string

const (
	Desc Direction = "desc" // 由新到旧 / 由大到小
	Asc  Direction = "asc"  // 由旧到新 / 由小到大
)

// Cursor 不透明游标：编码「上一页末行」的 keyset 位置。
//
//	Sort —— 主排序列的值（字符串化，由调用方按列类型自行序列化/还原，
//	        如时间用 RFC3339Nano、数值用十进制）；
//	ID   —— 唯一 tie-breaker（通常为主键），保证同排序值下顺序确定、不漏不重。
type Cursor struct {
	Sort string `json:"s"`
	ID   int64  `json:"i"`
}

// IsZero 表示首页（无游标）。
func (c Cursor) IsZero() bool {
	return c.ID == 0 && c.Sort == ""
}

// Encode 生成 URL-safe 的不透明游标串；零游标编码为空串（首页语义）。
func (c Cursor) Encode() string {
	if c.IsZero() {
		return ""
	}
	b, _ := json.Marshal(c) // Cursor 字段均为可序列化基本类型，不会出错
	return base64.RawURLEncoding.EncodeToString(b)
}

// Decode 解析游标串。空串返回零游标 + nil（首页）；非法串返回 error，
// 调用方据此回 400，切勿把错误游标当首页静默处理（否则客户端翻页会莫名回到首页）。
func Decode(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, err
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}, err
	}
	return c, nil
}

// NormalizeLimit 将每页条数归一化到 [1, MaxLimit]，非正值回落 DefaultLimit。
func NormalizeLimit(n int) int {
	switch {
	case n <= 0:
		return DefaultLimit
	case n > MaxLimit:
		return MaxLimit
	default:
		return n
	}
}

// KeysetPredicate 生成按复合键 (sortCol, idCol) 翻页的 WHERE 片段与参数。
//
// 谓词取「上一页末行之后」的一侧，与 ORDER BY 同向：
//
//	Desc → sortCol < v OR (sortCol = v AND idCol < id)
//	Asc  → sortCol > v OR (sortCol = v AND idCol > id)
//
// sortVal 应传入与列类型匹配的**已定型**值（如 time.Time），而非字符串，
// 交由驱动做类型化绑定，避免字符串比较的格式歧义。列名由调用方提供，
// 须为可信常量（勿用外部输入拼列名，防注入）。
func KeysetPredicate(sortCol, idCol string, sortVal any, id int64, dir Direction) (string, []any) {
	op := "<"
	if dir == Asc {
		op = ">"
	}
	clause := "(" + sortCol + " " + op + " ? OR (" + sortCol + " = ? AND " + idCol + " " + op + " ?))"
	return clause, []any{sortVal, sortVal, id}
}

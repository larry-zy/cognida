package resultstore

import (
	"strings"
	"sync"
	"time"
)

// SessionRegistry 记录每个 owner（tenant:session）最近一次 sql_execute 取数的 result_id，
// 以及「归一化 SQL 签名 → result_id」去重索引。进程内、owner 隔离、并发安全、TTL 惰性过期。
//
// 解决「取数复用」的两个诉求：
//   - ③ 去重：会话内相同 SQL（同库同上限）在 TTL 内重复执行时直接复用已存 result_id，
//     跳过重复打库——正是「卡住」复现里同一句归因取数被反复重跑 3× 的根因。
//   - ① 最近结果回退：data_analysis 缺 result_id 且无内联 data 时，回退到本会话最近一次取数
//     结果，让「取数→分析」的委派链在 result_id 未显式串联时也不 dead-end（Analysis 空转）。
//
// 定位是「单进程内的取数复用索引」——结果本体仍由 Store（Redis/内存）持有并做 owner 校验，
// 本注册表只存 id 映射；多实例部署时退化为各实例各自复用，功能不受损（仍会各自重跑一次）。
type SessionRegistry struct {
	mu     sync.Mutex
	ttl    time.Duration
	owners map[string]*ownerFetches
}

// ownerFetches 单个 owner 的取数索引：最近一次结果 + 按 SQL 签名的去重表。
type ownerFetches struct {
	latestID  string
	expiresAt int64             // latestID 的过期 unix 秒
	bySQL     map[string]sqlHit // 归一化 SQL 签名 → 命中
}

// sqlHit 一条 SQL 去重命中（result_id + 独立过期时间）。
type sqlHit struct {
	resultID  string
	expiresAt int64
}

// NewSessionRegistry 构造会话取数注册表；ttl<=0 时用 DefaultTTL（与 Result Store 对齐）。
func NewSessionRegistry(ttl time.Duration) *SessionRegistry {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &SessionRegistry{ttl: ttl, owners: make(map[string]*ownerFetches)}
}

// SQLSignature 把「数据源 + 执行 SQL」归一化为稳定去重键：仅折叠空白（不改大小写，
// 避免把字符串字面量里大小写不同的过滤条件误判为同一查询）。空 SQL 返回空串，调用方据此跳过。
func SQLSignature(databaseID, executedSQL string) string {
	norm := strings.Join(strings.Fields(executedSQL), " ")
	if norm == "" {
		return ""
	}
	return databaseID + "|" + norm
}

// RecordFetch 记录一次成功取数：更新 owner 的最近结果，并（sqlSig 非空时）写入 SQL 去重索引。
// nil 接收者、空 owner/resultID 均为安全空操作，便于调用点无脑调用。
func (r *SessionRegistry) RecordFetch(owner, sqlSig, resultID string) {
	if r == nil || owner == "" || resultID == "" {
		return
	}
	exp := nowUnix() + int64(r.ttl.Seconds())
	r.mu.Lock()
	defer r.mu.Unlock()
	of := r.owners[owner]
	if of == nil {
		of = &ownerFetches{bySQL: make(map[string]sqlHit)}
		r.owners[owner] = of
	}
	of.latestID = resultID
	of.expiresAt = exp
	if sqlSig != "" {
		of.bySQL[sqlSig] = sqlHit{resultID: resultID, expiresAt: exp}
	}
}

// Latest 返回 owner 最近一次取数的 result_id；无记录或已过期返回 ok=false。
func (r *SessionRegistry) Latest(owner string) (string, bool) {
	if r == nil || owner == "" {
		return "", false
	}
	now := nowUnix()
	r.mu.Lock()
	defer r.mu.Unlock()
	of := r.owners[owner]
	if of == nil || of.latestID == "" || now > of.expiresAt {
		return "", false
	}
	return of.latestID, true
}

// LiveFetches 返回 owner 名下所有 TTL 内未过期、按 SQL 签名去重后的取数 result_id（顺序不保证），
// 并惰性清理过期项。用于 data_analysis 缺来源回退时判定歧义：多于一份即「本会话存在多份候选结果」，
// 应显式消歧而非盲取 latest（宁拒不猜）。仅统计带 SQL 签名的取数（sql_execute 均带签名）；签名为空的
// 历史取数不进本索引，由调用方另经 Latest 兜底。
func (r *SessionRegistry) LiveFetches(owner string) []string {
	if r == nil || owner == "" {
		return nil
	}
	now := nowUnix()
	r.mu.Lock()
	defer r.mu.Unlock()
	of := r.owners[owner]
	if of == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(of.bySQL))
	ids := make([]string, 0, len(of.bySQL))
	for sig, hit := range of.bySQL {
		if now > hit.expiresAt {
			delete(of.bySQL, sig) // 惰性清理，与 LookupSQL 一致
			continue
		}
		if _, dup := seen[hit.resultID]; dup {
			continue // 不同签名映射到同一 result_id（去重复用）时只计一次
		}
		seen[hit.resultID] = struct{}{}
		ids = append(ids, hit.resultID)
	}
	return ids
}

// LookupSQL 按归一化 SQL 签名查 owner 的去重命中；无命中或已过期返回 ok=false（并惰性清理过期项）。
func (r *SessionRegistry) LookupSQL(owner, sqlSig string) (string, bool) {
	if r == nil || owner == "" || sqlSig == "" {
		return "", false
	}
	now := nowUnix()
	r.mu.Lock()
	defer r.mu.Unlock()
	of := r.owners[owner]
	if of == nil {
		return "", false
	}
	hit, ok := of.bySQL[sqlSig]
	if !ok {
		return "", false
	}
	if now > hit.expiresAt {
		delete(of.bySQL, sqlSig)
		return "", false
	}
	return hit.resultID, true
}

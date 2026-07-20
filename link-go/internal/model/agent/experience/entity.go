// Package experience 定义「会话经验自动沉淀」的领域模型与仓储接口。
//
// 背景：每次 Agent 会话结束（以空闲超时判定）后，后台异步把该会话的问题解决过程
// 蒸馏为一条结构化「经验」——既写入知识图谱（experience 节点 + RELATED_TO 边），
// 又在满足条件时蒸馏为可复用的 SKILL.md。agent_experiences 表同时充当幂等标记：
// 一条会话至多沉淀一次（已存在记录即跳过）。
package experience

import (
	"context"
	"time"
)

// Status 沉淀状态：worker 处理生命周期的显式状态机。
const (
	// StatusPending 已入库待处理（占位记录，充当幂等标记）
	StatusPending = "pending"
	// StatusDone 沉淀完成（图谱 + skill 均已尝试写入）
	StatusDone = "done"
	// StatusFailed 沉淀失败（LLM/落库出错，保留错误便于排查，不再重试）
	StatusFailed = "failed"
	// StatusSkipped 会话不值得沉淀（LLM 判定无可复用经验）
	StatusSkipped = "skipped"
)

// Experience 是一条从会话蒸馏出的结构化经验。
// 其中 Title/Problem/Solution/Tools/Tags/SkillWorthy/SkillInstructions 由 LLM 产出，
// 其余为 worker 采集的元数据与落库状态。
type Experience struct {
	// 主键与来源
	ID        int64  `json:"id"`
	SessionID string `json:"session_id"` // 来源会话（唯一，充当幂等键）
	TenantID  int64  `json:"tenant_id"`
	UserID    int64  `json:"user_id"`
	AgentType string `json:"agent_type"`

	// LLM 蒸馏产出
	Title             string   `json:"title"`              // 一句话概括该会话解决了什么
	Problem           string   `json:"problem"`            // 用户遇到的问题/诉求
	Solution          string   `json:"solution"`           // 采用的解法/关键步骤
	Tools             []string `json:"tools"`              // 过程中用到的工具名
	Tags              []string `json:"tags"`               // 主题标签（用于图谱关联与检索）
	SkillWorthy       bool     `json:"skill_worthy"`       // 是否值得蒸馏为可复用 skill
	SkillInstructions string   `json:"skill_instructions"` // skill_worthy 时：可复用的操作指引正文

	// 落库状态
	Status    string `json:"status"`               // pending/done/failed/skipped
	SkillName string `json:"skill_name,omitempty"` // 已生成的 SKILL.md 名（无则空）
	Error     string `json:"error,omitempty"`      // 失败原因

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IdleSession 是待沉淀会话的轻量视图：worker 扫描 sessions 表挑出空闲已结束、
// 且尚未沉淀过的会话，只带回沉淀所需的最小字段。
type IdleSession struct {
	SessionID    string
	TenantID     int64
	UserID       int64
	AgentType    string
	MessageCount int
}

// ExperienceRepository 是经验沉淀的持久化端口。
type ExperienceRepository interface {
	// FindIdleUnsummarizedSessions 挑出「空闲已结束且未沉淀」的会话：
	//   Status=Active AND UpdatedAt < idleBefore AND MessageCount>=minMessages
	//   AND (createdAfter 零值 OR CreatedAt >= createdAfter)
	//   AND NOT EXISTS(agent_experiences WHERE session_id = sessions.id)
	// 按 UpdatedAt 升序（最早空闲的先处理），至多 limit 条。
	// createdAfter 为起始时间下限：只沉淀该时刻之后新建的会话，历史存量不回溯（零值=不限）。
	FindIdleUnsummarizedSessions(ctx context.Context, idleBefore, createdAfter time.Time, minMessages, limit int) ([]*IdleSession, error)

	// Save 插入或更新一条经验记录（按 session_id 幂等 upsert）。
	Save(ctx context.Context, exp *Experience) error

	// ExistsBySession 是否已存在该会话的沉淀记录（占位或完成均算）。
	ExistsBySession(ctx context.Context, sessionID string) (bool, error)

	// FindSkillWorthyByTenant 返回某租户下「已生成 skill」的经验（Tools/Tags 已解析），
	// 供近重复合并判定使用：按最近更新倒序，至多 limit 条。
	FindSkillWorthyByTenant(ctx context.Context, tenantID int64, limit int) ([]*Experience, error)
}

package experience

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// ConfigFromEnv 从环境变量装配 Worker 配置（缺省回落 DefaultConfig）：
//   - EXPERIENCE_DISTILL_ENABLED：总开关（true/1 开启；默认关）
//   - EXPERIENCE_IDLE_TIMEOUT：空闲判定阈值（如 15m；默认 15m）
//   - EXPERIENCE_SCAN_INTERVAL：扫描间隔（如 1m；默认 1m）
//   - EXPERIENCE_MIN_MESSAGES：最少消息数（默认 2）
//   - EXPERIENCE_BATCH_SIZE：单轮批量（默认 20）
//   - EXPERIENCE_MAX_CONCURRENT：并发蒸馏上限（默认 2）
//   - EXPERIENCE_START_FROM：起始时间下限，只沉淀该时刻后新建的会话（默认空=不限，历史存量不回溯）
//   - EXPERIENCE_MIN_CONFIDENCE：质量门，低于此置信度记 skipped 不落图谱（默认 60；0=关闭该门）
//   - EXPERIENCE_PREGATE_ENABLED：客观失败前置门总开关（默认开，true/1；false/0 关闭）
//   - EXPERIENCE_SKILL_MIN_CONFIDENCE：技能沉淀置信门，skill_worthy 且置信度≥此值才落 SKILL.md（默认 80；0=不额外设限）
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	if v := strings.TrimSpace(os.Getenv("EXPERIENCE_DISTILL_ENABLED")); v != "" {
		cfg.Enabled = v == "true" || v == "1"
	}
	cfg.IdleTimeout = durationEnv("EXPERIENCE_IDLE_TIMEOUT", cfg.IdleTimeout)
	cfg.ScanInterval = durationEnv("EXPERIENCE_SCAN_INTERVAL", cfg.ScanInterval)
	cfg.MinMessages = intEnv("EXPERIENCE_MIN_MESSAGES", cfg.MinMessages)
	cfg.BatchSize = intEnv("EXPERIENCE_BATCH_SIZE", cfg.BatchSize)
	cfg.MaxConcurrent = intEnv("EXPERIENCE_MAX_CONCURRENT", cfg.MaxConcurrent)
	cfg.StartFrom = timeEnv("EXPERIENCE_START_FROM", cfg.StartFrom)
	// MinConfidence 允许 0（显式关闭质量门），故用 nonNegIntEnv 而非要求正数的 intEnv。
	cfg.MinConfidence = nonNegIntEnv("EXPERIENCE_MIN_CONFIDENCE", cfg.MinConfidence)
	if v := strings.TrimSpace(os.Getenv("EXPERIENCE_PREGATE_ENABLED")); v != "" {
		cfg.PreGateEnabled = v == "true" || v == "1"
	}
	// SkillMinConfidence 允许 0（不额外设限），故用 nonNegIntEnv。
	cfg.SkillMinConfidence = nonNegIntEnv("EXPERIENCE_SKILL_MIN_CONFIDENCE", cfg.SkillMinConfidence)
	return cfg
}

// SkillSinkEnabledFromEnv 读技能沉淀开关（与写侧 EXPERIENCE_DISTILL_ENABLED 相互独立）：
//   - EXPERIENCE_SKILL_SINK_ENABLED：true/1 开启把 skill_worthy 经验落成 SKILL.md；默认关。
//
// 独立门控便于「先只攒经验/图谱、确认质量后再放开生成技能文件」——技能会落进 git 跟踪的
// skills/ 目录并进未来 system prompt，副作用大于图谱沉淀，故默认关、单独开。
func SkillSinkEnabledFromEnv() bool {
	v := strings.TrimSpace(os.Getenv("EXPERIENCE_SKILL_SINK_ENABLED"))
	return v == "true" || v == "1"
}

// nonNegIntEnv 解析非负整数环境变量（区别于 intEnv 要求正数）：允许 0 用于「显式关闭」语义。
func nonNegIntEnv(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		log.Printf("[ExperienceWorker] %s=%q 非法（需非负整数），回落 %d", key, v, def)
		return def
	}
	return n
}

// RecallEnabledFromEnv 读经验召回开关（读侧，与写侧 EXPERIENCE_DISTILL_ENABLED 相互独立）：
//   - EXPERIENCE_RECALL_ENABLED：true/1 开启首答注入历史经验；默认关。
//
// 读写分离便于「先攒一段时间沉淀、再放开召回」，也避免空图谱时的无谓开销。
func RecallEnabledFromEnv() bool {
	v := strings.TrimSpace(os.Getenv("EXPERIENCE_RECALL_ENABLED"))
	return v == "true" || v == "1"
}

// timeEnv 解析起始时间下限：接受日期 2006-01-02、日期时间 2006-01-02 15:04:05
// 或 RFC3339；均按本地时区解释。为空或非法则回落 def（通常为零值=不限）。
func timeEnv(key string, def time.Time) time.Time {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t
		}
	}
	log.Printf("[ExperienceWorker] %s=%q 非法（需 2006-01-02 / 2006-01-02 15:04:05 / RFC3339），回落不限", key, v)
	return def
}

func durationEnv(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Printf("[ExperienceWorker] %s=%q 非法，回落 %s", key, v, def)
		return def
	}
	return d
}

func intEnv(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		log.Printf("[ExperienceWorker] %s=%q 非法，回落 %d", key, v, def)
		return def
	}
	return n
}

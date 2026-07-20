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
//   - EXPERIENCE_DEDUP_THRESHOLD：近重复合并的 Jaccard 阈值（默认 0.8；<=0 关闭）
//   - EXPERIENCE_START_FROM：起始时间下限，只沉淀该时刻后新建的会话（默认空=不限，历史存量不回溯）
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
	cfg.DedupThreshold = floatEnv("EXPERIENCE_DEDUP_THRESHOLD", cfg.DedupThreshold)
	cfg.StartFrom = timeEnv("EXPERIENCE_START_FROM", cfg.StartFrom)
	return cfg
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

// SkillDirFromEnv 解析 skill 落盘目录：优先 EXPERIENCE_SKILL_DIR，
// 否则回落到 LINK_SKILL_DIRS 的第一个目录，再否则 "./skills"。
func SkillDirFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("EXPERIENCE_SKILL_DIR")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("LINK_SKILL_DIRS")); v != "" {
		if first := strings.TrimSpace(strings.Split(v, ",")[0]); first != "" {
			return first
		}
	}
	return "./skills"
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

// floatEnv 解析浮点环境变量；允许 0（用于关闭去重），仅拒绝负数与非法值。
func floatEnv(key string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		log.Printf("[ExperienceWorker] %s=%q 非法，回落 %v", key, v, def)
		return def
	}
	return f
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

package experience

import (
	"context"
	"log"
	"sync"
	"time"

	domain_experience "link/internal/model/agent/experience"
	domain_conversation "link/internal/model/conversation"
)

// Config 会话经验沉淀 Worker 配置（由环境变量装配）。
type Config struct {
	// Enabled 总开关（EXPERIENCE_DISTILL_ENABLED，默认 false）。
	Enabled bool
	// IdleTimeout 会话空闲多久视为已结束（EXPERIENCE_IDLE_TIMEOUT，默认 15m）。
	IdleTimeout time.Duration
	// ScanInterval 后台扫描间隔（EXPERIENCE_SCAN_INTERVAL，默认 1m）。
	ScanInterval time.Duration
	// MinMessages 至少多少条消息才值得沉淀（默认 2）。
	MinMessages int
	// BatchSize 单次扫描至多处理的会话数（默认 20）。
	BatchSize int
	// MaxConcurrent 并发蒸馏上限（每次沉淀是一次独立 LLM 调用，需限流；默认 2）。
	MaxConcurrent int
	// DedupThreshold tools+tags 的 Jaccard 相似度阈值：达到则复用既有 skill、不再新建
	//（避免同类套路反复生成 SKILL.md）。默认 0.8；<=0 关闭去重。
	DedupThreshold float64
	// StartFrom 起始时间下限（EXPERIENCE_START_FROM，默认零值=不限）：
	// 只沉淀该时刻之后新建（created_at）的会话，历史存量不回溯分析。
	StartFrom time.Time
}

// DefaultConfig 返回带默认值的配置。
func DefaultConfig() Config {
	return Config{
		Enabled:        false,
		IdleTimeout:    15 * time.Minute,
		ScanInterval:   1 * time.Minute,
		MinMessages:    2,
		BatchSize:      20,
		MaxConcurrent:  2,
		DedupThreshold: 0.8,
	}
}

// Worker 后台扫描空闲已结束会话并异步蒸馏经验（进程内 goroutine 队列，无外部依赖）。
type Worker struct {
	cfg        Config
	expRepo    domain_experience.ExperienceRepository
	msgRepo    domain_conversation.MessageRepository
	summarizer *Summarizer
	graphSink  *GraphSink
	skillSink  *SkillSink
	dedup      *Deduplicator

	sem      chan struct{}       // 并发限流信号量
	inFlight map[string]struct{} // 处理中的会话（防同一会话被多轮扫描重复入队）
	mu       sync.Mutex

	ctx     context.Context // 生命周期上下文；Stop 时取消，令在途蒸馏尽快退出
	cancel  context.CancelFunc
	stopCh  chan struct{}
	wg      sync.WaitGroup
	running bool
	runMu   sync.Mutex
}

// NewWorker 组装经验沉淀 Worker。graphSink/skillSink 允许为 nil（对应能力未接线时静默跳过）。
func NewWorker(
	cfg Config,
	expRepo domain_experience.ExperienceRepository,
	msgRepo domain_conversation.MessageRepository,
	summarizer *Summarizer,
	graphSink *GraphSink,
	skillSink *SkillSink,
) *Worker {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 1
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 20
	}
	if cfg.MinMessages <= 0 {
		cfg.MinMessages = 2
	}
	return &Worker{
		cfg:        cfg,
		expRepo:    expRepo,
		msgRepo:    msgRepo,
		summarizer: summarizer,
		graphSink:  graphSink,
		skillSink:  skillSink,
		dedup:      NewDeduplicator(cfg.DedupThreshold),
		sem:        make(chan struct{}, cfg.MaxConcurrent),
		inFlight:   make(map[string]struct{}),
		stopCh:     make(chan struct{}),
	}
}

// Run 启动后台扫描循环（非阻塞：内部起 goroutine）。
func (w *Worker) Run() error {
	w.runMu.Lock()
	if w.running {
		w.runMu.Unlock()
		return nil
	}
	w.running = true
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.runMu.Unlock()

	startFrom := "不限（含历史存量）"
	if !w.cfg.StartFrom.IsZero() {
		startFrom = w.cfg.StartFrom.Format(time.RFC3339)
	}
	log.Printf("[ExperienceWorker] 启动：idleTimeout=%s scanInterval=%s maxConcurrent=%d startFrom=%s",
		w.cfg.IdleTimeout, w.cfg.ScanInterval, w.cfg.MaxConcurrent, startFrom)

	w.wg.Add(1)
	go w.scanLoop()
	return nil
}

// scanLoop 周期扫描待沉淀会话并异步处理。
func (w *Worker) scanLoop() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.cfg.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.scanOnce()
		}
	}
}

// scanOnce 执行一次扫描：挑出空闲未沉淀会话，逐个限流入队。
func (w *Worker) scanOnce() {
	ctx := context.Background()
	idleBefore := time.Now().Add(-w.cfg.IdleTimeout)
	sessions, err := w.expRepo.FindIdleUnsummarizedSessions(ctx, idleBefore, w.cfg.StartFrom, w.cfg.MinMessages, w.cfg.BatchSize)
	if err != nil {
		log.Printf("[ExperienceWorker] 扫描待沉淀会话失败: %v", err)
		return
	}
	if len(sessions) == 0 {
		return
	}
	log.Printf("[ExperienceWorker] 本轮发现 %d 个待沉淀会话", len(sessions))

	for _, sess := range sessions {
		if !w.claim(sess.SessionID) {
			continue // 已在处理中，跳过
		}
		select {
		case <-w.stopCh:
			w.release(sess.SessionID)
			return
		default:
		}
		w.wg.Add(1)
		go func(s *domain_experience.IdleSession) {
			defer w.wg.Done()
			defer w.release(s.SessionID)
			// 获取并发槽；Stop 期间（stopCh 关闭）直接放弃，避免阻塞关停。
			select {
			case w.sem <- struct{}{}:
				defer func() { <-w.sem }()
			case <-w.stopCh:
				return
			}
			w.process(s)
		}(sess)
	}
}

// process 蒸馏单个会话：拉消息 → LLM 蒸馏 → 落库 → 写图谱 → 写 skill。
func (w *Worker) process(sess *domain_experience.IdleSession) {
	base := w.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, 2*time.Minute)
	defer cancel()

	// 二次幂等校验（扫描到入队之间可能已被处理）。
	if exists, err := w.expRepo.ExistsBySession(ctx, sess.SessionID); err == nil && exists {
		return
	}

	messages, err := w.msgRepo.FindRecentBySessionID(ctx, sess.SessionID, 100)
	if err != nil {
		log.Printf("[ExperienceWorker] 会话 %s 拉取消息失败: %v", sess.SessionID, err)
		return
	}
	if len(messages) < w.cfg.MinMessages {
		return
	}

	exp, err := w.summarizer.Summarize(ctx, messages)
	if err != nil {
		log.Printf("[ExperienceWorker] 会话 %s 蒸馏失败: %v", sess.SessionID, err)
		// 落一条 failed 记录，避免下轮反复重试同一失败会话。
		w.saveStatus(ctx, sess, domain_experience.StatusFailed, err.Error())
		return
	}

	// 补齐落库元数据。
	exp.SessionID = sess.SessionID
	exp.TenantID = sess.TenantID
	exp.UserID = sess.UserID
	exp.AgentType = sess.AgentType

	// LLM 判定无可复用经验：记 skipped，仍写入以占位（幂等）。
	if exp.Title == "" && exp.Problem == "" && exp.Solution == "" {
		exp.Status = domain_experience.StatusSkipped
		if err := w.expRepo.Save(ctx, exp); err != nil {
			log.Printf("[ExperienceWorker] 会话 %s 保存 skipped 失败: %v", sess.SessionID, err)
		}
		return
	}

	exp.Status = domain_experience.StatusDone
	// 先保存（拿到自增 ID，供图谱节点 ID 使用），随后写图谱/skill 再更新。
	if err := w.expRepo.Save(ctx, exp); err != nil {
		log.Printf("[ExperienceWorker] 会话 %s 保存经验失败: %v", sess.SessionID, err)
		return
	}

	// 写图谱（失败不致命）。
	if w.graphSink != nil {
		if err := w.graphSink.Write(ctx, exp); err != nil {
			log.Printf("[ExperienceWorker] 会话 %s 写图谱失败: %v", sess.SessionID, err)
		}
	}

	// 写 skill（仅 skill_worthy；失败不致命）。
	// 近重复命中时复用既有 skill，避免同类套路反复生成一堆内容雷同的 SKILL.md。
	if w.skillSink != nil && exp.SkillWorthy {
		if dup := w.maybeDuplicate(ctx, exp); dup != nil {
			exp.SkillName = dup.SkillName
			log.Printf("[ExperienceWorker] 会话 %s 近重复命中，复用 skill=%q（源=%s）",
				sess.SessionID, dup.SkillName, dup.SessionID)
			if err := w.expRepo.Save(ctx, exp); err != nil {
				log.Printf("[ExperienceWorker] 会话 %s 回写复用 skill_name 失败: %v", sess.SessionID, err)
			}
		} else {
			name, err := w.skillSink.Write(ctx, exp)
			if err != nil {
				log.Printf("[ExperienceWorker] 会话 %s 写 skill 失败: %v", sess.SessionID, err)
			} else if name != "" {
				exp.SkillName = name
				if err := w.expRepo.Save(ctx, exp); err != nil {
					log.Printf("[ExperienceWorker] 会话 %s 回写 skill_name 失败: %v", sess.SessionID, err)
				}
			}
		}
	}

	log.Printf("[ExperienceWorker] 会话 %s 沉淀完成 title=%q skill=%q", sess.SessionID, exp.Title, exp.SkillName)
}

// maybeDuplicate 在同租户已生成 skill 的经验中找与 exp 近重复者（tools+tags 高度重合）。
// 去重关闭或查询失败时返回 nil（降级为正常新建 skill，不阻断主流程）。
func (w *Worker) maybeDuplicate(ctx context.Context, exp *domain_experience.Experience) *domain_experience.Experience {
	if !w.dedup.Enabled() {
		return nil
	}
	candidates, err := w.expRepo.FindSkillWorthyByTenant(ctx, exp.TenantID, 200)
	if err != nil {
		log.Printf("[ExperienceWorker] 会话 %s 查询近重复候选失败: %v", exp.SessionID, err)
		return nil
	}
	return w.dedup.FindDuplicate(exp, candidates)
}

// saveStatus 落一条仅含状态/错误的占位记录（用于 failed/skipped，避免反复重试）。
func (w *Worker) saveStatus(ctx context.Context, sess *domain_experience.IdleSession, status, errMsg string) {
	exp := &domain_experience.Experience{
		SessionID: sess.SessionID,
		TenantID:  sess.TenantID,
		UserID:    sess.UserID,
		AgentType: sess.AgentType,
		Status:    status,
		Error:     errMsg,
	}
	if err := w.expRepo.Save(ctx, exp); err != nil {
		log.Printf("[ExperienceWorker] 会话 %s 保存状态 %s 失败: %v", sess.SessionID, status, err)
	}
}

// claim 尝试认领会话；已在处理中返回 false。
func (w *Worker) claim(sessionID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.inFlight[sessionID]; ok {
		return false
	}
	w.inFlight[sessionID] = struct{}{}
	return true
}

// release 释放会话认领。
func (w *Worker) release(sessionID string) {
	w.mu.Lock()
	delete(w.inFlight, sessionID)
	w.mu.Unlock()
}

// Stop 停止 Worker，等待在途蒸馏完成。
func (w *Worker) Stop() {
	w.runMu.Lock()
	if !w.running {
		w.runMu.Unlock()
		return
	}
	w.running = false
	if w.cancel != nil {
		w.cancel() // 取消在途蒸馏的上下文，令其尽快退出
	}
	w.runMu.Unlock()

	close(w.stopCh)
	w.wg.Wait()
	log.Printf("[ExperienceWorker] 已停止")
}

// IsRunning 是否运行中。
func (w *Worker) IsRunning() bool {
	w.runMu.Lock()
	defer w.runMu.Unlock()
	return w.running
}

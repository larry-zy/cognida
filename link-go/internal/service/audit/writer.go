// Package audit 提供请求审计的异步批量落库能力。
//
// 设计目标：审计写入绝不放大请求延迟。请求路径只做非阻塞入队，
// 由后台单 goroutine 按「批量大小」或「定时」两条件之一 flush 到 MySQL。
// 缓冲满时丢弃并计数（审计尽力而为，不因落库抖动拖垮在线请求）。
package audit

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	auditmodel "link/internal/model/audit"
)

// WriterConfig 异步批量写入器配置
type WriterConfig struct {
	BufferSize    int           // 入队缓冲容量（满则丢弃）
	BatchSize     int           // 单批最大落库条数
	FlushInterval time.Duration // 定时 flush 周期（不足一批也落库）
	WriteTimeout  time.Duration // 单批落库超时
}

// DefaultWriterConfig 返回默认配置
func DefaultWriterConfig() WriterConfig {
	return WriterConfig{
		BufferSize:    4096,
		BatchSize:     128,
		FlushInterval: 2 * time.Second,
		WriteTimeout:  5 * time.Second,
	}
}

// Writer 审计日志异步批量写入器
type Writer struct {
	repo    auditmodel.Repository
	ch      chan *auditmodel.AuditLog
	cfg     WriterConfig
	done    chan struct{}
	dropped atomic.Int64
	closed  atomic.Bool
}

// NewWriter 创建并启动审计写入器。
// 后台 flush goroutine 在此启动，须在应用关闭时调用 Close 优雅收尾。
func NewWriter(repo auditmodel.Repository, cfg WriterConfig) *Writer {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 4096
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 128
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 2 * time.Second
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 5 * time.Second
	}
	w := &Writer{
		repo: repo,
		ch:   make(chan *auditmodel.AuditLog, cfg.BufferSize),
		cfg:  cfg,
		done: make(chan struct{}),
	}
	go w.loop()
	return w
}

// Enqueue 非阻塞入队。缓冲满则丢弃并计数，绝不阻塞调用方（请求路径）。
func (w *Writer) Enqueue(entry *auditmodel.AuditLog) {
	if entry == nil || w.closed.Load() {
		return
	}
	select {
	case w.ch <- entry:
	default:
		n := w.dropped.Add(1)
		// 采样告警：避免丢弃风暴刷屏，仅每 100 条提示一次。
		if n%100 == 1 {
			log.Printf("[AuditWriter] 审计缓冲已满，累计丢弃 %d 条（建议增大 BufferSize 或排查落库延迟）", n)
		}
	}
}

// Dropped 返回累计丢弃条数（可用于监控暴露）。
func (w *Writer) Dropped() int64 {
	return w.dropped.Load()
}

func (w *Writer) loop() {
	defer close(w.done)
	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]*auditmodel.AuditLog, 0, w.cfg.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		w.flush(batch)
		batch = batch[:0]
	}

	for {
		select {
		case entry, ok := <-w.ch:
			if !ok {
				// 通道关闭：把剩余积压落库后退出（优雅关闭）。
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= w.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// flush 将一批审计逐条落库。单条失败仅记录日志，不阻断其余条目。
func (w *Writer) flush(batch []*auditmodel.AuditLog) {
	ctx, cancel := context.WithTimeout(context.Background(), w.cfg.WriteTimeout)
	defer cancel()
	for _, e := range batch {
		if err := w.repo.Create(ctx, e); err != nil {
			log.Printf("[AuditWriter] 审计落库失败: %v", err)
		}
	}
}

// Close 停止入队并把积压落库后返回；ctx 超时则提前返回其错误。
func (w *Writer) Close(ctx context.Context) error {
	if w.closed.Swap(true) {
		return nil
	}
	close(w.ch)
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

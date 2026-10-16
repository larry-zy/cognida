package huggingface

import (
	"context"
	"log"
	"time"
)

type Worker struct{ s *Service }

func NewWorker(s *Service) *Worker { return &Worker{s: s} }

// Run blocks until cancellation. The composition root owns its wait group.
func (w *Worker) Run(ctx context.Context) {
	if w == nil || w.s == nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if e := w.s.Tick(ctx); e != nil && ctx.Err() == nil {
			log.Print("[HuggingFace] durable import reconciliation incomplete")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

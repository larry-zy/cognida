package feedback

import (
	"context"
	"log"
	"time"
)

// Worker only invalidates and reconciles cleanup. It never creates repair,
// validation or publication actions on behalf of a user.
type Worker struct{ service *Service }

func NewWorker(s *Service) *Worker { return &Worker{s} }
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	cursor := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			next, e := w.service.ReconcileSources(ctx, cursor)
			if e != nil {
				log.Printf("feedback source reconciliation failed: %v", e)
			} else {
				cursor = next
			}
		}
	}
}

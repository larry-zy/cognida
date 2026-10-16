package handler

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	d "cognida/internal/model/taskcontrol"
	"github.com/gin-gonic/gin"
)

// TaskEvents observes persisted facts only: disconnect/reconnect cannot start
// a run or spend a model call. A snapshot digest is a replay cursor, not an
// assertion that every transient state was observed.
func (h *TaskControlHandler) TaskEvents(c *gin.Context) {
	if h.supervisor == nil || h.runtime == nil {
		BadRequest(c, "任务执行服务不可用")
		return
	}
	last := c.GetHeader("Last-Event-ID")
	if len(last) > 64 {
		BadRequest(c, "事件游标不合法")
		return
	}
	started := false
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		summary, err := h.supervisor.Summarize(c.Request.Context(), taskControlScope(c), c.Param("id"))
		if err == nil {
			err = h.runtime.DecorateSummary(c.Request.Context(), taskControlScope(c), summary)
		}
		if err != nil && !errors.Is(err, d.ErrConflict) {
			if !started {
				taskControlError(c, err)
			} else {
				c.SSEvent("error", gin.H{"message": "任务或来源已不可访问，请重新加载"})
				c.Writer.Flush()
			}
			return
		}
		if err == nil {
			if !started {
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-store")
				c.Header("X-Accel-Buffering", "no")
				started = true
			}
			body, err := json.Marshal(summary)
			if err != nil {
				return
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(body))
			if digest != last {
				fmt.Fprintf(c.Writer, "id: %s\nevent: state\ndata: %s\n\n", digest, body)
				c.Writer.Flush()
				last = digest
			}
			if summary.Terminal {
				c.SSEvent("terminal", gin.H{"status": summary.Status})
				c.Writer.Flush()
				return
			}
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-heartbeat.C:
			if started {
				fmt.Fprint(c.Writer, ": heartbeat\n\n")
				c.Writer.Flush()
			}
		case <-tick.C:
		}
	}
}

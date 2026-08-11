package sse

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

// lockstepWriter 记录并发写入次数，且不与外部锁绑定——用于 -race 下暴露对同一
// ResponseWriter 的无保护并发写。所有写都必须已被 *Writer 的锁串行化。
type lockstepWriter struct {
	header http.Header
	writes int
}

func (l *lockstepWriter) Header() http.Header {
	if l.header == nil {
		l.header = make(http.Header)
	}
	return l.header
}

func (l *lockstepWriter) Write(p []byte) (int, error) {
	l.writes++ // 若写入未被串行化，-race 会在此处报数据竞争
	return len(p), nil
}

func (l *lockstepWriter) WriteHeader(int) {}

// Flush 实现 http.Flusher，覆盖心跳与数据帧里的 Flush 分支。
func (l *lockstepWriter) Flush() {}

// TestWriter_ConcurrentSendAndHeartbeatNoRace 在 -race 下验证：高频心跳与数据帧
// 并发写同一 ResponseWriter 时，Writer 的锁把二者串行化，不产生数据竞争〔R2-3〕。
func TestWriter_ConcurrentSendAndHeartbeatNoRace(t *testing.T) {
	lw := &lockstepWriter{}
	sw := NewWriter(lw)

	// 1ms 心跳：测试窗口内会与数据帧密集交错。
	stop := sw.StartHeartbeat(context.Background(), &HeartbeatConfig{
		Interval: time.Millisecond,
		Comment:  "keepalive",
	})

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				sw.Send(EventTypeContent, ContentChunk{Content: "x"})
			}
		}()
	}
	wg.Wait()

	// 停止心跳并等待其 goroutine 完全退出——建立 happens-before，
	// 之后主 goroutine 读取 lw.writes 不再与心跳写入竞争。
	stop()

	if lw.writes == 0 {
		t.Fatal("expected writes to underlying ResponseWriter")
	}
}

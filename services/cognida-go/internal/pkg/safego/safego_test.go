package safego

import (
	"sync"
	"testing"
)

// TestGo_RecoversPanic 验证 Go 包装的 goroutine panic 不会外泄（否则测试进程崩溃）。
func TestGo_RecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	Go("test-panic", func() {
		defer wg.Done()
		panic("boom")
	})
	wg.Wait() // 若 panic 未被 recover，进程已崩溃，走不到这里。
}

// TestGo_RunsNormally 验证正常函数照常执行。
func TestGo_RunsNormally(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	ran := false
	Go("test-normal", func() {
		defer wg.Done()
		ran = true
	})
	wg.Wait()
	if !ran {
		t.Fatal("fn did not run")
	}
}

// TestRecover_PreservesLaterDefers 验证作为首个 defer 时，其余 defer（如 close）仍会执行。
func TestRecover_PreservesLaterDefers(t *testing.T) {
	ch := make(chan struct{})
	go func() {
		defer Recover("producer")
		defer close(ch) // LIFO：先于 Recover 执行，channel 被正常关闭
		panic("producer boom")
	}()
	<-ch // channel 关闭后 receive 立即返回；若未关闭则死锁，测试超时失败。
}

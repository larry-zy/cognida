// Package safego 提供带 panic 兜底的 goroutine 启动助手。
//
// Go 语言里 goroutine 内未被 recover 的 panic 会直接终止整个进程——一个后台任务、
// 一个流转发 goroutine 的意外 panic 足以拖垮整个服务。本包统一在此类 goroutine 边界
// 处 recover 并记录堆栈，把故障隔离在单个 goroutine 内。
//
// 两种用法：
//   - safego.Go(name, fn)：无 channel 契约的「即发即忘」后台 goroutine，直接包装。
//   - defer safego.Recover(name)：需保留自身 channel 关闭/清理 defer 的生产者
//     goroutine，把它作为 goroutine 函数体的第一个 defer（LIFO 下最后执行），
//     既让 close(ch) 等既有 defer 照常运行，又能兜住 panic。
package safego

import (
	"log"
	"runtime/debug"
)

// Go 在独立 goroutine 中运行 fn，并 recover 其 panic（记录名称、panic 值与堆栈）。
// 适用于结果不经 channel 回传的后台任务；需要保留 channel 语义的场景改用 Recover。
func Go(name string, fn func()) {
	go func() {
		defer Recover(name)
		fn()
	}()
}

// Recover 作为 goroutine 内的 defer 使用，捕获并记录 panic：`defer safego.Recover("xxx")`。
// 必须由 defer 直接调用才能生效（recover 仅在被延迟函数中有效）。
func Recover(name string) {
	if r := recover(); r != nil {
		log.Printf("[safego] goroutine %q panic recovered: %v\n%s", name, r, debug.Stack())
	}
}

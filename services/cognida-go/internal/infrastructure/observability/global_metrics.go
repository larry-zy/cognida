// Package telemetry —— 全局 Metrics 单例的生命周期管理。
//
// 原先与 Agent 遥测装饰器(TelemetryMiddleware)同处 middleware.go，
// 该装饰器因依赖 service/agent/framework 造成 infra→service 反转，已迁出至
// service/agent/telemetry。Metrics 单例属于基础设施(可观测性)关注点，保留于此。
package telemetry

import "log"

// globalMetrics 全局指标实例(懒初始化)
var globalMetrics *Metrics

func init() {
	metrics, err := NewMetrics()
	if err != nil {
		// 遥测非致命：初始化失败仅告警，globalMetrics 保持 nil。
		// 所有 Metrics 方法对 nil 接收者安全(no-op)，调用方无需判空(〔M11〕)。
		log.Printf("⚠️ 全局 Metrics 初始化失败，遥测降级为 no-op: %v", err)
		return
	}
	globalMetrics = metrics
}

// GetMetrics 返回全局指标实例。
// 可能为 nil(初始化失败时)——但 *Metrics 所有方法对 nil 接收者安全，直接调用即可(〔M11〕)。
func GetMetrics() *Metrics {
	return globalMetrics
}

// Package telemetry —— 全局 Metrics 单例的生命周期管理。
//
// 原先与 Agent 遥测装饰器(TelemetryMiddleware)同处 middleware.go，
// 该装饰器因依赖 service/agent/framework 造成 infra→service 反转，已迁出至
// service/agent/telemetry。Metrics 单例属于基础设施(可观测性)关注点，保留于此。
package telemetry

// globalMetrics 全局指标实例(懒初始化)
var globalMetrics *Metrics

func init() {
	metrics, err := NewMetrics()
	if err != nil {
		// 记录错误但不致命
		return
	}
	globalMetrics = metrics
}

// GetMetrics 返回全局指标实例
func GetMetrics() *Metrics {
	return globalMetrics
}

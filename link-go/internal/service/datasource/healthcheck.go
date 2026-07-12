package datasource

import (
	"context"
	"time"

	model "link/internal/model/datasource"
)

// HealthReport 一次全量健康检查的结果概览。
type HealthReport struct {
	Checked int // 实际检查的数据源数（不含 need_credentials 跳过项）
	Healthy int // 其中连通正常的数量
	Skipped int // 因凭证失效被跳过的数量
}

// CheckHealthOnce 跨租户遍历全部数据源做一次连通性探测，更新每个数据源的
// status 与 last_health_check_at。凭证失效（need_credentials）的数据源需人工
// 重录密码，健康检查不覆盖其状态、只计入 Skipped。
//
// 该方法为幂等、可重复调用；调用方（如 cmd/server 后台任务）自行按周期驱动。
// 单个数据源检查失败只影响其自身状态，不中断整体遍历。
func (s *Service) CheckHealthOnce(ctx context.Context) (HealthReport, error) {
	list, err := s.repo.ListAll(ctx)
	if err != nil {
		return HealthReport{}, err
	}

	var report HealthReport
	for _, ds := range list {
		if ds.Status == model.StatusNeedCredentials {
			report.Skipped++
			continue
		}
		report.Checked++
		status := s.probe(ctx, ds)
		if status == model.StatusActive {
			report.Healthy++
		}
		// 更新失败不影响其它数据源，尽力而为
		_ = s.repo.UpdateStatus(ctx, ds.ID, ds.TenantID, status, time.Now())
	}
	return report, nil
}

// probe 对单个数据源做一次带超时的连通性探测，返回 active 或 error。
func (s *Service) probe(ctx context.Context, ds *model.DataSource) string {
	db, _, err := s.cm.Acquire(ctx, ds.TenantID, ds.ID)
	if err != nil {
		return model.StatusError
	}
	drv, err := DriverFor(ds.Type)
	if err != nil {
		return model.StatusError
	}
	pingCtx, cancel := context.WithTimeout(ctx, s.healthPingTimeout)
	defer cancel()
	if err := drv.Ping(pingCtx, db); err != nil {
		return model.StatusError
	}
	return model.StatusActive
}

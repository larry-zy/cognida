package datasource

import (
	"context"
	"testing"
	"time"

	model "link/internal/model/datasource"
)

// seedHealthDS 直接向仓储写入一条数据源（绕过 Service 校验，便于构造各状态）。
func seedHealthDS(t *testing.T, s *Service, repo *memRepo, id string, typ model.Type, status string) {
	t.Helper()
	enc, err := s.cipher.Encrypt("pw")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	ds := &model.DataSource{
		ID: id, TenantID: 1, Name: "hc-" + id, Type: typ,
		Host: "127.0.0.1", Port: 3306, DatabaseName: "demo", Username: "u",
		PasswordEncrypted: enc, Status: status,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := repo.Create(context.Background(), ds); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestCheckHealthOnce(t *testing.T) {
	s, repo := newTestService(t)
	ctx := context.Background()

	// 正常：fake driver Open 成功 → ping 成功 → active
	seedHealthDS(t, s, repo, "ok", model.TypeMySQL, model.StatusActive)
	// 凭证失效：应被跳过，状态不被覆盖
	seedHealthDS(t, s, repo, "cred", model.TypeMySQL, model.StatusNeedCredentials)
	// 打不开：未知类型 → DriverFor 失败 → error
	seedHealthDS(t, s, repo, "bad", model.Type("nosuchdb"), model.StatusActive)

	report, err := s.CheckHealthOnce(ctx)
	if err != nil {
		t.Fatalf("CheckHealthOnce: %v", err)
	}
	if report.Checked != 2 || report.Healthy != 1 || report.Skipped != 1 {
		t.Fatalf("概览不符: %+v", report)
	}

	// 状态与检查时间已回写
	okDS, _ := repo.Get(ctx, "ok", 1)
	if okDS.Status != model.StatusActive || okDS.LastHealthCheckAt == nil {
		t.Errorf("正常源应标 active 且记录检查时间: %+v", okDS)
	}
	badDS, _ := repo.Get(ctx, "bad", 1)
	if badDS.Status != model.StatusError {
		t.Errorf("不可连源应标 error, got %q", badDS.Status)
	}
	credDS, _ := repo.Get(ctx, "cred", 1)
	if credDS.Status != model.StatusNeedCredentials {
		t.Errorf("凭证失效源状态不应被覆盖, got %q", credDS.Status)
	}
	if credDS.LastHealthCheckAt != nil {
		t.Errorf("跳过的源不应更新检查时间")
	}
}

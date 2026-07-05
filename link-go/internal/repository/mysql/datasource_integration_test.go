//go:build integration
// +build integration

// Package mysql: 外部数据源全链路集成测试（task 2.5）。
//
// 把本地 MySQL（MYSQL_DSN 指向的库）当作一个"外部数据源"注册，验证
// 注册 → 测试连接 → schema 探查 → 密码留空不改 → 改错密码失效 → 重录密码重建 全链路。
//
//	MYSQL_DSN='root:password@tcp(localhost:3306)/link?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test -tags=integration ./internal/repository/mysql/ -run TestDataSourceLifecycle -v
package mysql

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	godriver "github.com/go-sql-driver/mysql"

	dssvc "link/internal/service/datasource"
)

const itDSTenant = int64(990002)

// itDSIDGen 测试用 ID 生成器：固定前缀，便于清理残留。
type itDSIDGen struct{ n int }

func (g *itDSIDGen) Generate() string {
	g.n++
	return fmt.Sprintf("it_ds_%d", g.n)
}

// TestDataSourceLifecycle 注册→测试连接→探查→密码变更失效重建全链路。
func TestDataSourceLifecycle(t *testing.T) {
	db := newIntegrationDB(t)
	if err := db.AutoMigrate(&DataSourceModel{}); err != nil {
		t.Fatalf("automigrate data_sources: %v", err)
	}
	// 清理上次残留（按测试 ID 前缀）
	cleanup := func() { db.Where("id LIKE ?", "it_ds_%").Delete(&DataSourceModel{}) }
	cleanup()
	t.Cleanup(cleanup)

	// 从 MYSQL_DSN 解析出连接参数，把本库当作"外部数据源"注册
	cfg, err := godriver.ParseDSN(os.Getenv("MYSQL_DSN")) // newIntegrationDB 已确保非空
	if err != nil {
		t.Fatalf("parse MYSQL_DSN: %v", err)
	}
	host, portStr, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		t.Fatalf("split addr %q: %v", cfg.Addr, err)
	}
	port, _ := strconv.Atoi(portStr)

	cipher, err := dssvc.NewCipher("it-datasource-secret")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	repo := NewDataSourceRepository(db)
	cm := dssvc.NewConnectionManager(repo, cipher, dssvc.DefaultPoolOptions())
	t.Cleanup(cm.Close)
	svc := dssvc.NewService(repo, cipher, cm, &itDSIDGen{})

	ctx := context.Background()
	input := &dssvc.SaveInput{
		Name:         "it-local-mysql",
		Type:         "mysql",
		Host:         host,
		Port:         port,
		DatabaseName: cfg.DBName,
		Username:     cfg.User,
		Password:     cfg.Passwd,
	}

	// 1. 注册
	created, err := svc.Create(ctx, itDSTenant, 1, input)
	if err != nil {
		t.Fatalf("create datasource: %v", err)
	}
	if created.Status != "active" {
		t.Errorf("expected status active, got %q", created.Status)
	}

	// 2. 测试连接（按已存 id，用落库密文凭证）
	if err := svc.TestConnection(ctx, itDSTenant, &dssvc.TestInput{DatasourceID: created.ID}); err != nil {
		t.Fatalf("test connection by id: %v", err)
	}

	// 3. 探查：表清单应包含本测试刚建的 data_sources 表
	tables, err := svc.ListTables(ctx, itDSTenant, created.ID)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	found := false
	for _, tb := range tables {
		if tb.Name == (DataSourceModel{}).TableName() {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected table %q in list, got %d tables", (DataSourceModel{}).TableName(), len(tables))
	}

	schema, err := svc.DescribeTable(ctx, itDSTenant, created.ID, (DataSourceModel{}).TableName())
	if err != nil {
		t.Fatalf("describe table: %v", err)
	}
	if len(schema.Columns) == 0 {
		t.Error("expected columns in table schema")
	}

	// 4. 经受管连接池执行真实查询
	pool, _, err := cm.Acquire(ctx, itDSTenant, created.ID)
	if err != nil {
		t.Fatalf("acquire pool: %v", err)
	}
	var one int
	if err := pool.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("query via pool: %v (got %d)", err, one)
	}

	// 5. 更新时密码留空 → 保留原密码，连接仍可用
	input.Password = ""
	input.Name = "it-local-mysql-renamed"
	if _, err := svc.Update(ctx, itDSTenant, created.ID, input); err != nil {
		t.Fatalf("update with empty password: %v", err)
	}
	if err := svc.TestConnection(ctx, itDSTenant, &dssvc.TestInput{DatasourceID: created.ID}); err != nil {
		t.Fatalf("connection should survive empty-password update: %v", err)
	}

	// 6. 改成错误密码 → 池失效，重建时 Ping 失败，且错误不泄漏密码
	input.Password = "definitely-wrong-password"
	if _, err := svc.Update(ctx, itDSTenant, created.ID, input); err != nil {
		t.Fatalf("update with wrong password: %v", err)
	}
	if _, _, err := cm.Acquire(ctx, itDSTenant, created.ID); err == nil {
		t.Fatal("acquire should fail after wrong-password update")
	} else if strings.Contains(err.Error(), "definitely-wrong-password") {
		t.Fatalf("error must not leak password: %v", err)
	}

	// 7. 重录正确密码 → 池重建，链路恢复
	input.Password = cfg.Passwd
	if _, err := svc.Update(ctx, itDSTenant, created.ID, input); err != nil {
		t.Fatalf("update back to correct password: %v", err)
	}
	pool2, _, err := cm.Acquire(ctx, itDSTenant, created.ID)
	if err != nil {
		t.Fatalf("acquire after password restore: %v", err)
	}
	if err := pool2.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("query after rebuild: %v", err)
	}

	// 8. 删除联动关池
	if err := svc.Delete(ctx, itDSTenant, created.ID); err != nil {
		t.Fatalf("delete datasource: %v", err)
	}
	if _, _, err := cm.Acquire(ctx, itDSTenant, created.ID); err == nil {
		t.Fatal("acquire should fail after delete")
	}
}

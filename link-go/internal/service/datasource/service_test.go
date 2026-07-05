package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type seqIDGen struct{ n int }

func (g *seqIDGen) Generate() string {
	g.n++
	return "ds-id-" + strings.Repeat("x", g.n)
}

func newTestService(t *testing.T) (*Service, *memRepo) {
	t.Helper()
	repo := newMemRepo()
	m := newTestManager(t, repo)
	return NewService(repo, m.cipher, m, &seqIDGen{}), repo
}

func validInput(name string) *SaveInput {
	return &SaveInput{
		Name: name, Type: "mysql", Host: "127.0.0.1", Port: 3306,
		DatabaseName: "demo", Username: "u", Password: "pw",
	}
}

func TestCreateAndNameConflict(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	created, err := s.Create(ctx, 1, 10, validInput("prod-db"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" || created.Status != "active" {
		t.Fatalf("创建结果异常: %+v", created)
	}

	if _, err := s.Create(ctx, 1, 10, validInput("prod-db")); !errors.Is(err, ErrNameConflict) {
		t.Fatalf("同名创建应返回 ErrNameConflict, got %v", err)
	}
	// 不同租户同名不冲突
	if _, err := s.Create(ctx, 2, 10, validInput("prod-db")); err != nil {
		t.Fatalf("跨租户同名应允许: %v", err)
	}
}

func TestCreateRequiresPassword(t *testing.T) {
	s, _ := newTestService(t)
	in := validInput("no-pass")
	in.Password = ""
	if _, err := s.Create(context.Background(), 1, 10, in); err == nil {
		t.Fatalf("创建缺密码必须报错")
	}
}

func TestUpdateEmptyPasswordKeepsOld(t *testing.T) {
	s, repo := newTestService(t)
	ctx := context.Background()

	created, err := s.Create(ctx, 1, 10, validInput("db1"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	before, _ := repo.Get(ctx, created.ID, 1)

	in := validInput("db1-renamed")
	in.Password = "" // 留空 → 不改密码
	if _, err := s.Update(ctx, 1, created.ID, in); err != nil {
		t.Fatalf("Update: %v", err)
	}
	after, _ := repo.Get(ctx, created.ID, 1)
	if after.PasswordEncrypted != before.PasswordEncrypted {
		t.Fatalf("密码留空更新后密文不应变化")
	}
	if after.Name != "db1-renamed" {
		t.Fatalf("其余字段应正常更新, got %q", after.Name)
	}

	// 提供新密码 → 密文变化
	in.Password = "new-pass"
	if _, err := s.Update(ctx, 1, created.ID, in); err != nil {
		t.Fatalf("Update with password: %v", err)
	}
	final, _ := repo.Get(ctx, created.ID, 1)
	if final.PasswordEncrypted == before.PasswordEncrypted {
		t.Fatalf("重录密码后密文应变化")
	}
}

func TestSanitizedResponseHasNoPassword(t *testing.T) {
	s, _ := newTestService(t)
	created, err := s.Create(context.Background(), 1, 10, validInput("db-json"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	raw, _ := json.Marshal(created)
	lower := strings.ToLower(string(raw))
	if strings.Contains(lower, "password") || strings.Contains(lower, "pw") {
		t.Fatalf("响应 JSON 不得含密码字段: %s", raw)
	}
}

func TestDeleteNotFound(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.Delete(context.Background(), 1, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除不存在数据源应返回 ErrNotFound, got %v", err)
	}
}

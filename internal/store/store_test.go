package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"workstation/internal/protocol"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "controller.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func registrationFixture() protocol.Registration {
	return protocol.Registration{ProtocolVersion: 1, ID: "service-a", Name: "并发测试服务", Version: "1.0.0", BaseURL: "http://127.0.0.1:8091", HealthPath: "/healthz", Definition: protocol.Definition{SchemaVersion: 1, Menus: []protocol.Menu{}, Pages: []protocol.Page{}}}
}

func TestConcurrentRegistrationPreservesActivation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := registrationFixture()
	if _, e := s.Register(ctx, r); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SetActivation(ctx, r.ID, "enabled", "admin"); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := s.Register(ctx, r)
			if e == nil && v.Activation != "enabled" {
				e = errors.New("重新注册改变管理员状态")
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	items, e := s.Services(ctx, "", "", "")
	if e != nil || len(items) != 1 || items[0].Activation != "enabled" {
		t.Fatal("并发注册不应创建重复记录或修改启用状态", e)
	}
}
func TestStaleChecksCannotResurrectOrOverwrite(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := registrationFixture()
	old, e := s.Register(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteService(ctx, r.ID, "admin"); e != nil {
		t.Fatal(e)
	}
	if e = s.SetHealth(ctx, old, "online", ""); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Service(ctx, r.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("过时检查不得复活删除记录", e)
	}
	newer, e := s.Register(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SetHealth(ctx, old, "offline", "过时结果"); e != nil {
		t.Fatal(e)
	}
	actual, e := s.Service(ctx, r.ID)
	if e != nil || actual.Online != "unknown" || actual.CheckedAt != nil || actual.RegisteredAt != newer.RegisteredAt {
		t.Fatal("过时检查覆盖了新登记", e)
	}
	r.BaseURL = "http://127.0.0.1:8092"
	if _, e = s.Register(ctx, r); e != nil {
		t.Fatal(e)
	}
	if e = s.SetHealth(ctx, newer, "online", ""); e != nil {
		t.Fatal(e)
	}
	actual, e = s.Service(ctx, r.ID)
	if e != nil || actual.Online != "unknown" {
		t.Fatal("旧地址检测影响新地址状态", e)
	}
}
func TestAuditAndMutationAreAtomic(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := registrationFixture()
	if _, e := s.Register(ctx, r); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(`CREATE TRIGGER reject_audit BEFORE INSERT ON audit_logs BEGIN SELECT RAISE(ABORT,'测试事务回滚'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SetActivation(ctx, r.ID, "enabled", "admin"); e == nil {
		t.Fatal("审计失败应导致事务失败")
	}
	v, e := s.Service(ctx, r.ID)
	if e != nil || v.Activation != "pending" {
		t.Fatal("启用状态未回滚", e)
	}
	if e = s.DeleteService(ctx, r.ID, "admin"); e == nil {
		t.Fatal("删除审计失败应回滚")
	}
	if _, e = s.Service(ctx, r.ID); e != nil {
		t.Fatal("删除记录未回滚", e)
	}
	r.Version = "2.0.0"
	if _, e = s.Register(ctx, r); e == nil {
		t.Fatal("注册审计失败应回滚")
	}
	v, e = s.Service(ctx, r.ID)
	if e != nil || v.Version != "1.0.0" {
		t.Fatal("注册信息未回滚", e)
	}
}
func TestMigrationAndDeletionPolicy(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if e := s.migrate(); e != nil {
		t.Fatal(e)
	}
	var migrations int
	if e := s.DB.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&migrations); e != nil || migrations != 1 {
		t.Fatal("迁移重复执行", e)
	}
	r := registrationFixture()
	if _, e := s.Register(ctx, r); e != nil {
		t.Fatal(e)
	}
	if e := s.RegistrationFailure(ctx, r.ID, "测试无效定义"); e != nil {
		t.Fatal(e)
	}
	if e := s.DeleteService(ctx, r.ID, "admin"); e != nil {
		t.Fatal(e)
	}
	events, e := s.Events(ctx, r.ID)
	if e != nil || len(events) != 0 {
		t.Fatal("关联注册事件应级联删除", e)
	}
	audits, total, e := s.Audits(ctx, 1, 100)
	if e != nil || total != 3 || len(audits) != 3 {
		t.Fatal("重要操作审计不应级联删除", e)
	}
}

func TestServiceSearchIncludesAddress(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	r := registrationFixture()
	if _, e := s.Register(ctx, r); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{"并发测试", "SERVICE-A", "127.0.0.1:8091"} {
		items, e := s.Services(ctx, q, "pending", "unknown")
		if e != nil || len(items) != 1 || items[0].ID != r.ID {
			t.Fatalf("应按名称、标识或地址匹配 q=%q: %v", q, e)
		}
	}
	for _, q := range []string{"127.0.0.1:9999", "' OR 1=1 --"} {
		items, e := s.Services(ctx, q, "", "")
		if e != nil || len(items) != 0 {
			t.Fatalf("搜索必须按字面量安全匹配 q=%q: %v", q, e)
		}
	}
}

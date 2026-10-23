package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workstation/internal/config"
	"workstation/internal/model"
	"workstation/internal/protocol"
	"workstation/internal/store"
)

type harness struct {
	t      *testing.T
	app    *Server
	server *httptest.Server
	client *http.Client
	csrf   string
	cfg    config.Config
	dbPath string
}

func newHarness(t *testing.T, origins []string) *harness {
	t.Helper()
	c := config.Default()
	c.BootstrapToken = strings.Repeat("b", 32)
	c.RegistrationKey = strings.Repeat("r", 32)
	c.AllowedServiceOrigins = origins
	c.DatabasePath = filepath.Join(t.TempDir(), "test.db")
	s, e := store.Open(c.DatabasePath)
	if e != nil {
		t.Fatal(e)
	}
	app := New(c, s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(app.Handler())
	jar, _ := cookiejar.New(nil)
	h := &harness{t: t, app: app, server: server, client: &http.Client{Jar: jar, Timeout: 20 * time.Second}, cfg: c, dbPath: c.DatabasePath}
	t.Cleanup(func() { server.Close(); s.Close() })
	return h
}
func (h *harness) request(method, path string, body any, headers map[string]string, want int) map[string]json.RawMessage {
	h.t.Helper()
	var b []byte
	if raw, ok := body.([]byte); ok {
		b = raw
	} else if body != nil {
		b, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, h.server.URL+"/api/v1"+path, bytes.NewReader(b))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if h.csrf != "" {
		req.Header.Set("X-CSRF-Token", h.csrf)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, e := h.client.Do(req)
	if e != nil {
		h.t.Fatal(e)
	}
	defer res.Body.Close()
	data, e := io.ReadAll(res.Body)
	if e != nil {
		h.t.Fatal(e)
	}
	if res.StatusCode != want {
		h.t.Fatalf("%s %s 状态=%d,期望=%d，响应=%s", method, path, res.StatusCode, want, data)
	}
	var result map[string]json.RawMessage
	if e = json.Unmarshal(data, &result); e != nil {
		h.t.Fatalf("非JSON响应 %s", data)
	}
	return result
}
func (h *harness) initialize() {
	h.request("POST", "/auth/setup", map[string]string{"username": "admin", "password": "correct-password-123", "bootstrap_token": h.cfg.BootstrapToken}, nil, 201)
	v := h.request("POST", "/auth/login", map[string]string{"username": "admin", "password": "correct-password-123"}, nil, 200)
	_ = json.Unmarshal(v["csrf_token"], &h.csrf)
}
func registration(base string) protocol.Registration {
	return protocol.Registration{ProtocolVersion: 1, ID: "demo-service", Name: "演示服务", Version: "1.0.0", Description: "独立服务", BaseURL: base, HealthPath: "/healthz", Definition: protocol.Definition{SchemaVersion: 1, Menus: []protocol.Menu{{ID: "menu", Label: "列表", PageID: "items"}}, Pages: []protocol.Page{{ID: "items", Title: "列表", Type: "table", Source: &protocol.Source{Path: "/api/items"}, Columns: []protocol.Column{{Key: "id", Label: "编号"}}}}}}
}
func serviceFrom(t *testing.T, v map[string]json.RawMessage) model.Service {
	t.Helper()
	var s model.Service
	if e := json.Unmarshal(v["service"], &s); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestFullLifecycleAndSecurity(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer remote.Close()
	h := newHarness(t, []string{remote.URL})
	h.request("GET", "/services", nil, nil, 401)
	h.request("POST", "/auth/setup", map[string]string{"username": "admin", "password": "correct-password-123", "bootstrap_token": "wrong"}, nil, 403)
	h.request("POST", "/auth/setup", map[string]string{"username": "admin", "password": "correct-password-123", "bootstrap_token": h.cfg.BootstrapToken}, map[string]string{"Origin": "https://attacker.example"}, 403)
	h.initialize()
	h.request("POST", "/auth/setup", map[string]string{"username": "admin", "password": "correct-password-123", "bootstrap_token": h.cfg.BootstrapToken}, nil, 409)
	r := registration(remote.URL)
	h.request("POST", "/registry/register", r, nil, 401)
	headers := map[string]string{"Authorization": "Bearer " + h.cfg.RegistrationKey}
	v := serviceFrom(t, h.request("POST", "/registry/register", r, headers, 200))
	if v.Activation != "pending" || v.Online != "unknown" {
		t.Fatalf("新服务状态错误 %+v", v)
	}
	h.request("PATCH", "/services/demo-service", map[string]string{"activation": "enabled"}, map[string]string{"X-CSRF-Token": ""}, 403)
	v = serviceFrom(t, h.request("PATCH", "/services/demo-service", map[string]string{"activation": "enabled"}, nil, 200))
	if v.Activation != "enabled" {
		t.Fatal("启用失败")
	}
	r.Version = "2.0.0"
	v = serviceFrom(t, h.request("POST", "/registry/register", r, headers, 200))
	if v.Activation != "enabled" || v.Version != "2.0.0" {
		t.Fatal("重复注册改变启用状态或未更新")
	}
	v = serviceFrom(t, h.request("POST", "/services/demo-service/check", nil, nil, 200))
	if v.Online != "online" || v.CheckedAt == nil {
		t.Fatal("服务应在线")
	}
	bad := r
	bad.Definition.SchemaVersion = 99
	h.request("POST", "/registry/register", bad, headers, 400)
	v = serviceFrom(t, h.request("GET", "/services/demo-service", nil, nil, 200))
	if v.LastRegistrationError == "" || !bytes.Contains(v.Definition, []byte(`"schema_version":1`)) {
		t.Fatal("非法注册必须保留旧定义并记录错误")
	}
	items := h.request("GET", "/services?q=demo&activation=enabled&online=online", nil, nil, 200)
	var list []model.Service
	_ = json.Unmarshal(items["items"], &list)
	if len(list) != 1 {
		t.Fatal("重复注册或筛选失效")
	}
	remote.Close()
	v = serviceFrom(t, h.request("POST", "/services/demo-service/check", nil, nil, 200))
	if v.Online != "offline" || v.Activation != "enabled" {
		t.Fatal("离线不得改变启用状态")
	}
	h.request("GET", "/dashboard", nil, nil, 200)
	h.request("PATCH", "/services/demo-service", map[string]string{"activation": "disabled"}, nil, 200)
	h.request("PATCH", "/settings", map[string]any{"site_name": "我的工作站", "check_interval_seconds": 60}, nil, 200)
	settings := h.request("GET", "/settings", nil, nil, 200)
	encoded, _ := json.Marshal(settings)
	if bytes.Contains(encoded, []byte(h.cfg.RegistrationKey)) || bytes.Contains(encoded, []byte(h.cfg.BootstrapToken)) {
		t.Fatal("设置泄漏密钥")
	}
	h.request("DELETE", "/services/demo-service", nil, nil, 200)
	h.request("GET", "/services/demo-service", nil, nil, 404)
	v = serviceFrom(t, h.request("POST", "/registry/register", r, headers, 200))
	if v.Activation != "pending" {
		t.Fatal("删除后的新登记应待启用")
	}
	h.request("GET", "/audit?page=1&page_size=100", nil, nil, 200)
	h.request("GET", "/proxy?url=http://example.com", nil, nil, 404)
	h.request("POST", "/auth/logout", nil, nil, 200)
	h.request("GET", "/services", nil, nil, 401)
	// 重新打开同一数据库，验证管理员、设置与登记均持久化。
	reopened, e := store.Open(h.dbPath)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if ok, e := reopened.Initialized(context.Background()); e != nil || !ok {
		t.Fatal("管理员未持久化", e)
	}
	name, interval, e := reopened.Settings(context.Background(), 30)
	if e != nil || name != "我的工作站" || interval != 60 {
		t.Fatal("设置未持久化", e)
	}
	if _, e = reopened.Service(context.Background(), "demo-service"); e != nil {
		t.Fatal("登记未持久化", e)
	}
}
func TestSessionExpiryAndAuthFailure(t *testing.T) {
	h := newHarness(t, nil)
	h.initialize()
	h.request("POST", "/auth/login", map[string]string{"username": "admin", "password": "wrong-password"}, nil, 401)
	if _, e := h.app.Store.DB.Exec("UPDATE sessions SET expires_at='2000-01-01T00:00:00Z'"); e != nil {
		t.Fatal(e)
	}
	h.request("GET", "/auth/me", nil, nil, 401)
}
func TestMalformedRegistrationAndLimits(t *testing.T) {
	h := newHarness(t, []string{"http://127.0.0.1:8091"})
	headers := map[string]string{"Authorization": "Bearer " + h.cfg.RegistrationKey}
	for _, body := range [][]byte{[]byte(`{"protocol_version":1,"id":"a","javascript":"alert(1)"}`), []byte(`{"id":"x","id":"y"}`), []byte(strings.Repeat(" ", protocol.MaxBody+1))} {
		h.request("POST", "/registry/register", body, headers, 400)
	}
	r := registration("http://169.254.169.254")
	h.request("POST", "/registry/register", r, headers, 400)
}
func TestLoginRateLimit(t *testing.T) {
	h := newHarness(t, nil)
	for i := 0; i < 10; i++ {
		h.request("POST", "/auth/login", map[string]string{"username": "unknown", "password": "wrong-password"}, nil, 401)
	}
	h.request("POST", "/auth/login", map[string]string{"username": "unknown", "password": "wrong-password"}, nil, 429)
}

func TestManagementRemainsAvailableDuringHealthCheck(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; w.WriteHeader(200) }))
	defer remote.Close()
	h := newHarness(t, []string{remote.URL})
	h.initialize()
	r := registration(remote.URL)
	h.request("POST", "/registry/register", r, map[string]string{"Authorization": "Bearer " + h.cfg.RegistrationKey}, 200)
	done := make(chan error, 1)
	go func() { _, e := h.app.Checker.Check(context.Background(), r.ID); done <- e }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("健康检查未开始")
	}
	start := time.Now()
	h.request("GET", "/dashboard", nil, nil, 200)
	h.request("DELETE", "/services/demo-service", nil, nil, 200)
	if time.Since(start) > time.Second {
		t.Error("健康检查期间管理操作不应被阻塞")
	}
	close(release)
	if e := <-done; !errors.Is(e, store.ErrNotFound) {
		t.Fatal("检查完成时记录已删除应返回不存在", e)
	}
	h.request("GET", "/services/demo-service", nil, nil, 404)
}

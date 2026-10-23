package registry

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"workstation/internal/model"
)

type resolverStub struct{ ips []net.IPAddr }

func (r resolverStub) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) { return r.ips, nil }
func TestUnsafeAddresses(t *testing.T) {
	for _, s := range []string{"169.254.169.254", "0.0.0.0", "224.0.0.1", "100.100.100.200", "fe80::1", "fd00:ec2::254", "::", "::ffff:169.254.169.254"} {
		if AllowedIP(net.ParseIP(s)) {
			t.Fatalf("错误地允许危险地址 %s", s)
		}
	}
	for _, s := range []string{"127.0.0.1", "192.168.1.2", "10.1.1.1", "::1", "1.1.1.1"} {
		if !AllowedIP(net.ParseIP(s)) {
			t.Fatalf("显式来源允许时应接受 %s", s)
		}
	}
}

func TestProbePinsDNSAndIgnoresProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "" {
			t.Error("保留服务Host头")
		}
		w.WriteHeader(204)
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("健康检测不得使用环境代理") }))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	base := "http://service.test:" + u.Port()
	c := New(nil, []string{base}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.Resolver = resolverStub{[]net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}}
	if got := c.probe(context.Background(), model.Service{BaseURL: base, HealthPath: "/healthz"}); got != "" {
		t.Fatal("应使用已校验IP而非重新解析或代理", got)
	}
}
func TestProbeBlocksRedirectsAndRebinding(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("不应跟随重定向") }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	c := New(nil, []string{redirect.URL}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got := c.probe(context.Background(), model.Service{BaseURL: redirect.URL, HealthPath: "/healthz"}); got == "" {
		t.Fatal("重定向不能视为健康")
	}
	c.Allowed = []string{"http://service.test"}
	c.Resolver = resolverStub{[]net.IPAddr{{IP: net.ParseIP("127.0.0.1")}, {IP: net.ParseIP("169.254.169.254")}}}
	if got := c.probe(context.Background(), model.Service{BaseURL: "http://service.test", HealthPath: "/healthz"}); got != "服务地址解析到禁止访问的网络" {
		t.Fatalf("必须拒绝混合DNS答案: %s", got)
	}
	c.Allowed = nil
	if got := c.probe(context.Background(), model.Service{BaseURL: redirect.URL, HealthPath: "/healthz"}); got != "服务地址已不在允许来源中" {
		t.Fatal(got)
	}
}

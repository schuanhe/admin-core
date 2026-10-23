package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigurationEnvironment(t *testing.T) {
	t.Setenv("CONTROLLER_BOOTSTRAP_TOKEN", strings.Repeat("a", 32))
	t.Setenv("CONTROLLER_REGISTRATION_KEY", strings.Repeat("b", 32))
	t.Setenv("CONTROLLER_CHECK_INTERVAL_SECONDS", "45")
	t.Setenv("CONTROLLER_ALLOWED_SERVICE_ORIGINS", "http://127.0.0.1:8091, https://service.example")
	c, e := Load("")
	if e != nil {
		t.Fatal(e)
	}
	if c.CheckIntervalSeconds != 45 || len(c.AllowedServiceOrigins) != 2 {
		t.Fatal("环境变量未生效")
	}
	t.Setenv("CONTROLLER_REGISTRATION_KEY", strings.Repeat("a", 32))
	if _, e = Load(""); e == nil {
		t.Fatal("不得复用初始化与注册密钥")
	}
}
func TestOriginValidation(t *testing.T) {
	for _, v := range []string{"http://127.0.0.1:8091", "https://example.com", "http://[::1]:80"} {
		if _, e := Origin(v); e != nil {
			t.Fatal(v, e)
		}
	}
	for _, v := range []string{"file:///etc/passwd", "http://admin:secret@example.com", "http://example.com/x", "http://example.com?x=1", "http://example.com#x", "http://example.com:99999", "http://example.com\\evil", "http://example.com;script-src", "http://example.com'unsafe-inline'", "http://-example.com", "http://example..com", "http://example.com?", "http://example.com#", "http://[fe80::1%25eth0]", "http://example.com.", "http://a_b.example"} {
		if _, e := Origin(v); e == nil {
			t.Fatalf("应拒绝 %s", v)
		}
	}
}

func TestConfigurationRejectsTrailingJSONAndPlaceholder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if e := os.WriteFile(path, []byte(`{} {}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(path); e == nil {
		t.Fatal("配置不能含尾随JSON")
	}
	c := Default()
	c.BootstrapToken = strings.Repeat("a", 32)
	c.RegistrationKey = "replace-with-a-secure-random-registration-key"
	if e := c.Validate(); e == nil {
		t.Fatal("应拒绝示例占位密钥")
	}
	c.RegistrationKey = strings.Repeat("b", 32)
	c.PublicOrigin = "https://station.example"
	if e := c.Validate(); e == nil {
		t.Fatal("HTTPS必须使用Secure Cookie")
	}
	c.SecureCookie = true
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	c.PublicOrigin = "http://station.example"
	if e := c.Validate(); e == nil {
		t.Fatal("HTTP配Secure Cookie会导致无法登录，应拒绝")
	}
	if got, e := Origin("https://EXAMPLE.com:443/"); e != nil || got != "https://example.com" {
		t.Fatal("应规范化默认端口与主机大小写", got, e)
	}
}

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	Listen                string                   `json:"listen"`
	DatabasePath          string                   `json:"database_path"`
	StaticDir             string                   `json:"static_dir"`
	PublicOrigin          string                   `json:"public_origin"`
	BootstrapToken        string                   `json:"bootstrap_token"`
	RegistrationKey       string                   `json:"registration_key"`
	AllowedServiceOrigins []string                 `json:"allowed_service_origins"`
	SessionHours          int                      `json:"session_hours"`
	SecureCookie          bool                     `json:"secure_cookie"`
	CheckIntervalSeconds  int                      `json:"check_interval_seconds"`
	ServiceAccessFile     string                   `json:"service_access_file,omitempty"`
	ServiceAccess         map[string]ServiceAccess `json:"-"`
}

func Default() Config {
	return Config{Listen: "127.0.0.1:8080", DatabasePath: "data/controller.db", StaticDir: "web/dist", PublicOrigin: "http://localhost:8080", AllowedServiceOrigins: []string{"http://127.0.0.1:8091"}, SessionHours: 12, CheckIntervalSeconds: 30}
}

func Load(path string) (Config, error) {
	c := Default()
	if path != "" {
		b, e := os.ReadFile(path)
		if e != nil {
			return c, e
		}
		d := json.NewDecoder(strings.NewReader(string(b)))
		d.DisallowUnknownFields()
		if e = d.Decode(&c); e != nil {
			return c, fmt.Errorf("配置文件无效: %w", e)
		}
		if e = d.Decode(new(any)); e != io.EOF {
			return c, errors.New("配置文件包含多余内容")
		}
	}
	for k, p := range map[string]*string{"LISTEN": &c.Listen, "DATABASE_PATH": &c.DatabasePath, "STATIC_DIR": &c.StaticDir, "PUBLIC_ORIGIN": &c.PublicOrigin, "BOOTSTRAP_TOKEN": &c.BootstrapToken, "REGISTRATION_KEY": &c.RegistrationKey, "SERVICE_ACCESS_FILE": &c.ServiceAccessFile} {
		if v, ok := os.LookupEnv("CONTROLLER_" + k); ok {
			*p = v
		}
	}
	for k, p := range map[string]*int{"SESSION_HOURS": &c.SessionHours, "CHECK_INTERVAL_SECONDS": &c.CheckIntervalSeconds} {
		if v, ok := os.LookupEnv("CONTROLLER_" + k); ok {
			n, e := strconv.Atoi(v)
			if e != nil {
				return c, fmt.Errorf("配置 %s 必须为整数", k)
			}
			*p = n
		}
	}
	if v, ok := os.LookupEnv("CONTROLLER_SECURE_COOKIE"); ok {
		n, e := strconv.ParseBool(v)
		if e != nil {
			return c, errors.New("SECURE_COOKIE 必须为布尔值")
		}
		c.SecureCookie = n
	}
	if v, ok := os.LookupEnv("CONTROLLER_ALLOWED_SERVICE_ORIGINS"); ok {
		c.AllowedServiceOrigins = []string{}
		for _, s := range strings.Split(v, ",") {
			if strings.TrimSpace(s) != "" {
				c.AllowedServiceOrigins = append(c.AllowedServiceOrigins, strings.TrimSpace(s))
			}
		}
	}
	if e := c.Validate(); e != nil {
		return c, e
	}
	if c.ServiceAccessFile != "" {
		policies, e := LoadServiceAccess(c.ServiceAccessFile, c)
		if e != nil {
			return c, e
		}
		c.ServiceAccess = policies
	}
	return c, nil
}

func (c Config) Validate() error {
	if len(c.BootstrapToken) < 32 || len(c.RegistrationKey) < 32 {
		return errors.New("必须配置至少32字节的独立初始化令牌和服务注册密钥")
	}
	if c.BootstrapToken == c.RegistrationKey {
		return errors.New("初始化令牌和服务注册密钥不得相同")
	}
	for _, secret := range []string{c.BootstrapToken, c.RegistrationKey} {
		if strings.TrimSpace(secret) != secret || strings.TrimSpace(secret) == "" {
			return errors.New("密钥不得为空白或包含首尾空格")
		}
		if strings.HasPrefix(strings.ToLower(secret), "replace-with-") || strings.HasPrefix(strings.ToLower(secret), "change-me") {
			return errors.New("请替换示例占位密钥，使用密码学安全的随机值")
		}
	}
	if c.Listen == "" || c.DatabasePath == "" || c.StaticDir == "" {
		return errors.New("监听地址、数据库路径和前端目录不能为空")
	}
	if c.SessionHours < 1 || c.SessionHours > 168 {
		return errors.New("会话有效期必须为1到168小时")
	}
	if c.CheckIntervalSeconds < 10 || c.CheckIntervalSeconds > 3600 {
		return errors.New("检查间隔必须为10到3600秒")
	}
	if _, e := Origin(c.PublicOrigin); e != nil {
		return fmt.Errorf("控制台公开地址无效: %w", e)
	}
	if strings.HasPrefix(c.PublicOrigin, "https://") != c.SecureCookie {
		return errors.New("公开地址使用HTTPS时必须启用安全Cookie，使用HTTP时必须关闭安全Cookie")
	}
	for _, v := range c.AllowedServiceOrigins {
		if _, e := Origin(v); e != nil {
			return fmt.Errorf("服务允许地址无效: %w", e)
		}
	}
	return nil
}

// Origin 只接受完整且无路径、凭据、查询参数的 HTTP(S) 来源。
var hostLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func Origin(raw string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u == nil {
		return "", errors.New("地址格式错误")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" || strings.ContainsAny(raw, "\\\r\n\t #") {
		return "", errors.New("仅允许无路径和凭据的HTTP(S)来源")
	}
	if u.Port() != "" {
		p, e := strconv.Atoi(u.Port())
		if e != nil || p < 1 || p > 65535 {
			return "", errors.New("端口无效")
		}
	}
	host := strings.ToLower(u.Hostname())
	if net.ParseIP(host) == nil {
		if len(host) > 253 {
			return "", errors.New("主机名过长")
		}
		for _, label := range strings.Split(host, ".") {
			if !hostLabel.MatchString(label) {
				return "", errors.New("主机名仅支持合法DNS名称或IP地址")
			}
		}
	}
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return u.Scheme + "://" + host, nil
}

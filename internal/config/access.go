package config

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// ServiceAccess 是部署者授予中心的最小业务权限，不接受服务注册请求修改。
// 该文件包含独立服务令牌，绝不能序列化到管理 API 或页面定义。
type ServiceAccess struct {
	Origin      string           `json:"origin"`
	BearerToken string           `json:"bearer_token"`
	Endpoints   []AccessEndpoint `json:"endpoints"`
}
type AccessEndpoint struct {
	Path    string   `json:"path"`
	Methods []string `json:"methods"`
}

func LoadServiceAccess(path string, c Config) (map[string]ServiceAccess, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, errors.New("无法读取服务访问策略文件")
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || info.Size() > 128*1024 {
		return nil, errors.New("服务访问策略文件超过128KiB或无法读取")
	}
	d := json.NewDecoder(io.LimitReader(f, 128*1024+1))
	d.DisallowUnknownFields()
	var policies map[string]ServiceAccess
	if d.Decode(&policies) != nil || d.Decode(new(any)) != io.EOF || len(policies) > 100 {
		return nil, errors.New("服务访问策略文件格式错误或超过限制")
	}
	ids := regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	paths := regexp.MustCompile(`^/[A-Za-z0-9_~./-]*$`)
	tokens := map[string]bool{}
	for id, p := range policies {
		origin, err := Origin(p.Origin)
		allowed := false
		for _, candidate := range c.AllowedServiceOrigins {
			v, _ := Origin(candidate)
			if v == origin {
				allowed = true
			}
		}
		if err != nil || !ids.MatchString(id) || !allowed {
			return nil, errors.New("业务策略的服务标识或来源未获部署白名单授权")
		}
		u, _ := url.Parse(origin)
		ip := net.ParseIP(u.Hostname())
		if u.Scheme == "http" && u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, errors.New("携带业务密钥的非回环连接必须使用HTTPS")
		}
		if len(p.BearerToken) < 32 || len(p.BearerToken) > 256 || strings.ContainsAny(p.BearerToken, " \t\r\n") || p.BearerToken == c.RegistrationKey || p.BearerToken == c.BootstrapToken || strings.HasPrefix(p.BearerToken, "replace-with-") || tokens[p.BearerToken] {
			return nil, errors.New("每个业务服务必须配置独立的32至256字节随机令牌，不能复用注册或初始化密钥")
		}
		for _, ch := range p.BearerToken {
			if ch < 33 || ch > 126 {
				return nil, errors.New("业务令牌必须为可打印ASCII字符")
			}
		}
		tokens[p.BearerToken] = true
		if len(p.Endpoints) == 0 || len(p.Endpoints) > 100 {
			return nil, errors.New("每个服务必须声明1至100个精确业务接口")
		}
		seen := map[string]bool{}
		for _, endpoint := range p.Endpoints {
			if len(endpoint.Path) > 512 || !paths.MatchString(endpoint.Path) || strings.Contains(endpoint.Path, "//") || len(endpoint.Methods) == 0 || len(endpoint.Methods) > 4 {
				return nil, errors.New("业务接口路径或方法白名单无效")
			}
			for _, segment := range strings.Split(endpoint.Path, "/") {
				if segment == "." || segment == ".." {
					return nil, errors.New("业务接口不允许路径穿越")
				}
			}
			for _, method := range endpoint.Methods {
				key := method + " " + endpoint.Path
				if (method != "GET" && method != "POST" && method != "PUT" && method != "DELETE") || seen[key] {
					return nil, errors.New("业务接口方法无效或重复")
				}
				seen[key] = true
			}
		}
		p.Origin = origin
		policies[id] = p
	}
	return policies, nil
}

func (p ServiceAccess) Allows(method, path string) bool {
	for _, endpoint := range p.Endpoints {
		if endpoint.Path != path {
			continue
		}
		for _, candidate := range endpoint.Methods {
			if candidate == method {
				return true
			}
		}
	}
	return false
}

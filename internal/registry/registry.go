package registry

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"workstation/internal/config"
	"workstation/internal/model"
	"workstation/internal/store"
)

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}
type Checker struct {
	Store    *store.Store
	Allowed  []string
	Resolver Resolver
	slots    chan struct{}
	Log      *slog.Logger
}

func New(s *store.Store, allowed []string, logger *slog.Logger) *Checker {
	return &Checker{Store: s, Allowed: allowed, Resolver: net.DefaultResolver, slots: make(chan struct{}, 4), Log: logger}
}
func AllowedIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	// 云元数据与保留地址不因为来源白名单而放行。
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "224.0.0.0/4", "240.0.0.0/4", "fe80::/10", "ff00::/8", "fd00:ec2::254/128"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}
func (c *Checker) probe(ctx context.Context, s model.Service) string {
	origin, e := config.Origin(s.BaseURL)
	if e != nil {
		return "服务地址配置无效"
	}
	allowed := false
	for _, a := range c.Allowed {
		v, _ := config.Origin(a)
		if v == origin {
			allowed = true
			break
		}
	}
	if !allowed {
		return "服务地址已不在允许来源中"
	}
	u, _ := url.Parse(origin)
	host := u.Hostname()
	addresses, e := c.Resolver.LookupIPAddr(ctx, host)
	if e != nil || len(addresses) == 0 {
		return "服务地址解析失败"
	}
	for _, a := range addresses {
		if !AllowedIP(a.IP) {
			return "服务地址解析到禁止访问的网络"
		}
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DisableKeepAlives: true, MaxResponseHeaderBytes: 16 * 1024, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		var last error
		for _, a := range addresses {
			d := net.Dialer{Timeout: 2 * time.Second}
			conn, e := d.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		return nil, last
	}}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, origin+s.HealthPath, nil)
	if e != nil {
		return "健康检查路径无效"
	}
	req.Header.Set("User-Agent", "Workstation-Controller/1")
	resp, e := client.Do(req)
	if e != nil {
		if errors.Is(e, context.DeadlineExceeded) {
			return "健康检查超时"
		}
		return "无法连接服务或TLS验证失败"
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "健康检查返回HTTP " + strconv.Itoa(resp.StatusCode)
	}
	return ""
}
func (c *Checker) Check(ctx context.Context, id string) (model.Service, error) {
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return model.Service{}, ctx.Err()
	}
	s, e := c.Store.Service(ctx, id)
	if e != nil {
		return s, e
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	message := c.probe(probeCtx, s)
	state := "online"
	if message != "" {
		state = "offline"
	}
	if e = c.Store.SetHealth(ctx, s, state, message); e != nil {
		return s, e
	}
	return c.Store.Service(ctx, id)
}
func (c *Checker) Run(ctx context.Context, defaultInterval int) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	last := time.Time{}
	lastPrune := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, interval, e := c.Store.Settings(ctx, defaultInterval)
			if e != nil {
				c.Log.Error("读取健康检查配置失败")
				continue
			}
			if time.Since(last) < time.Duration(interval)*time.Second {
				continue
			}
			last = time.Now()
			items, e := c.Store.Services(ctx, "", "", "")
			if e != nil {
				c.Log.Error("读取服务检查列表失败")
				continue
			}
			// 每轮最多四路并发，等待本轮完成避免积压后台任务。
			jobs := make(chan string)
			done := make(chan struct{})
			for i := 0; i < 4; i++ {
				go func() {
					defer func() { done <- struct{}{} }()
					for id := range jobs {
						if _, e := c.Check(ctx, id); e != nil && !errors.Is(e, store.ErrNotFound) && ctx.Err() == nil {
							c.Log.Warn("健康检查结果保存失败", "service_id", id)
						}
					}
				}()
			}
			for _, s := range items {
				select {
				case jobs <- s.ID:
				case <-ctx.Done():
				}
			}
			close(jobs)
			for i := 0; i < 4; i++ {
				<-done
			}
			if time.Since(lastPrune) > time.Hour {
				if e = c.Store.Prune(ctx); e != nil && ctx.Err() == nil {
					c.Log.Warn("历史记录清理失败")
				}
				lastPrune = time.Now()
			}
		}
	}
}

func ValidateOriginHosts(allowed []string) error {
	for _, a := range allowed {
		u, e := url.Parse(a)
		if e != nil {
			return e
		}
		if strings.Contains(u.Hostname(), "%") {
			return fmt.Errorf("不允许带作用域的IP地址")
		}
		if ip := net.ParseIP(u.Hostname()); ip != nil && !AllowedIP(ip) {
			return fmt.Errorf("不允许访问元数据、链路本地或保留网络地址")
		}
	}
	return nil
}

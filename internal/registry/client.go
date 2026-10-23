package registry

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"workstation/internal/config"
)

// RestrictedClient 固定本次DNS解析结果；不会读取环境代理、跟随重定向或连接其他主机。
func RestrictedClient(ctx context.Context, origin string, allowed []string, resolver Resolver, timeout time.Duration, loopbackHTTP bool) (*http.Client, func(), error) {
	normalized, e := config.Origin(origin)
	if e != nil {
		return nil, nil, errors.New("服务来源无效")
	}
	permitted := false
	for _, candidate := range allowed {
		v, _ := config.Origin(candidate)
		if v == normalized {
			permitted = true
		}
	}
	if !permitted {
		return nil, nil, errors.New("服务来源不在部署白名单中")
	}
	u, _ := url.Parse(normalized)
	addresses, e := resolver.LookupIPAddr(ctx, u.Hostname())
	if e != nil || len(addresses) == 0 {
		return nil, nil, errors.New("服务地址解析失败")
	}
	for _, a := range addresses {
		if !AllowedIP(a.IP) || (loopbackHTTP && u.Scheme == "http" && !a.IP.IsLoopback()) {
			return nil, nil, errors.New("服务地址解析到未获授权的网络")
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
	expected := net.JoinHostPort(u.Hostname(), port)
	transport := &http.Transport{
		Proxy: nil, DisableKeepAlives: true, DisableCompression: true,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: timeout,
		MaxResponseHeaderBytes: 16 * 1024,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != expected {
				return nil, errors.New("拒绝跨来源拨号")
			}
			var last error
			for _, a := range addresses {
				d := net.Dialer{Timeout: 2 * time.Second}
				conn, err := d.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			return nil, last
		},
	}
	client := &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client, transport.CloseIdleConnections, nil
}

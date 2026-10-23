package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"workstation/internal/config"
	"workstation/internal/httpapi"
	"workstation/internal/registry"
	"workstation/internal/store"
)

func main() {
	configFile := flag.String("config", "", "JSON配置文件路径（环境变量优先）")
	healthcheck := flag.Bool("healthcheck", false, "执行本地就绪检查后退出")
	flag.Parse()
	if *healthcheck {
		client := http.Client{Timeout: 3 * time.Second}
		resp, e := client.Get("http://127.0.0.1:8080/healthz")
		if e != nil {
			os.Exit(1)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	c, e := config.Load(*configFile)
	if e != nil {
		logger.Error("配置加载失败", "message", e.Error())
		os.Exit(1)
	}
	if e = registry.ValidateOriginHosts(c.AllowedServiceOrigins); e != nil {
		logger.Error("允许服务地址不安全", "message", e.Error())
		os.Exit(1)
	}
	if !c.SecureCookie && !strings.HasPrefix(c.Listen, "127.0.0.1:") && !strings.HasPrefix(c.Listen, "localhost:") {
		logger.Warn("正在使用非安全Cookie；生产环境请使用HTTPS并设置CONTROLLER_SECURE_COOKIE=true")
	}
	db, e := store.Open(c.DatabasePath)
	if e != nil {
		logger.Error("数据库初始化失败", "error_type", fmt.Sprintf("%T", e))
		os.Exit(1)
	}
	defer db.Close()
	app := httpapi.New(c, db, logger)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go app.Checker.Run(ctx, c.CheckIntervalSeconds)
	server := &http.Server{Addr: c.Listen, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	logger.Info("个人工作站控制器启动", "listen", c.Listen, "public_origin", c.PublicOrigin)
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		logger.Error("HTTP服务启动失败", "error_type", fmt.Sprintf("%T", e))
		os.Exit(1)
	}
}

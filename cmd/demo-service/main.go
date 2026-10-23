// 独立演示服务展示注册与双重鉴权 API；其业务数据不进入中心数据库。
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type task struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

type demo struct {
	mu           sync.Mutex
	tasks        []task
	preferences  map[string]any
	apiToken     string
	allowedPeers []netip.Prefix
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8091", "演示服务监听地址")
	controller := flag.String("controller", "http://localhost:8080", "中心控制器地址")
	baseURL := flag.String("base-url", "http://127.0.0.1:8091", "中心可访问的演示服务地址，不向浏览器开放")
	peers := flag.String("allowed-peers", "127.0.0.1/32,::1/128", "允许中心访问的真实TCP来源CIDR，逗号分隔；不信任转发头")
	accessConfig := flag.String("access-config", os.Getenv("CONTROLLER_SERVICE_ACCESS_FILE"), "本机演示可读取中心独立业务访问文件；生产应使用DEMO_API_TOKEN")
	config := flag.String("config", "", "可选中心本地配置，仅读取 registration_key")
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	key := os.Getenv("DEMO_REGISTRATION_KEY")
	if key == "" {
		key = os.Getenv("CONTROLLER_REGISTRATION_KEY")
	}
	if key == "" && *config != "" {
		b, err := os.ReadFile(*config)
		if err != nil {
			log.Error("读取配置失败")
			os.Exit(1)
		}
		var c struct {
			RegistrationKey string `json:"registration_key"`
		}
		if json.Unmarshal(b, &c) != nil {
			log.Error("配置格式不正确")
			os.Exit(1)
		}
		key = c.RegistrationKey
	}
	if len(key) < 32 {
		log.Error("请设置至少32字符的 DEMO_REGISTRATION_KEY 或使用 -config 指定本地配置")
		os.Exit(1)
	}
	apiToken := os.Getenv("DEMO_API_TOKEN")
	if apiToken == "" && *accessConfig != "" {
		data, err := os.ReadFile(*accessConfig)
		var policies map[string]struct {
			BearerToken string `json:"bearer_token"`
		}
		if err != nil || json.Unmarshal(data, &policies) != nil {
			log.Error("业务访问配置读取失败")
			os.Exit(1)
		}
		apiToken = policies["workstation-demo"].BearerToken
	}
	if len(apiToken) < 32 || len(apiToken) > 256 || strings.ContainsAny(apiToken, " \t\r\n") || apiToken == key || strings.HasPrefix(apiToken, "replace-with-") {
		log.Error("请配置独立于注册密钥的DEMO_API_TOKEN，或通过-access-config读取本机业务密钥")
		os.Exit(1)
	}
	d := newDemo(nil)
	d.apiToken = apiToken
	for _, raw := range strings.Split(*peers, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil || prefix.Bits() == 0 {
			log.Error("中心IP白名单无效，不允许空白名单或默认路由")
			os.Exit(1)
		}
		d.allowedPeers = append(d.allowedPeers, prefix.Masked())
	}
	server := &http.Server{Addr: *listen, Handler: d.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("演示服务启动失败", "error", err)
			stop()
		}
	}()
	log.Info("演示服务已启动，业务数据仅保存在本进程内存", "listen", *listen)
	go registerLoop(ctx, log, *controller, *baseURL, key)
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}

func newDemo(_ []string) *demo {
	d := &demo{tasks: []task{
		{1, "检查工作站备份", "待处理", "高"},
		{2, "整理研发资料", "进行中", "普通"},
		{3, "更新个人知识库", "待处理", "普通"},
	}, preferences: map[string]any{"owner": "工作站管理员", "refresh_seconds": 30, "theme": "system", "notifications": true, "note": "", "review_date": ""}}
	return d
}

func (d *demo) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]any{"status": "ok"}) })
	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if size < 1 || size > 100 {
			size = 10
		}
		start := len(d.tasks)
		if page <= len(d.tasks)/size+1 {
			start = (page - 1) * size
		}
		if start > len(d.tasks) {
			start = len(d.tasks)
		}
		end := start + size
		if end > len(d.tasks) {
			end = len(d.tasks)
		}
		respond(w, 200, map[string]any{"items": d.tasks[start:end], "total": len(d.tasks)})
	})
	mux.HandleFunc("POST /api/tasks/archive", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ID int `json:"id"`
		}
		if !decode(w, r, &input) {
			return
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		for i := range d.tasks {
			if d.tasks[i].ID == input.ID {
				d.tasks[i].Status = "已归档"
				respond(w, 200, map[string]any{"ok": true})
				return
			}
		}
		respond(w, 404, map[string]any{"error": map[string]string{"message": "任务不存在"}})
	})
	mux.HandleFunc("GET /api/preferences", func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		respond(w, 200, d.preferences)
	})
	mux.HandleFunc("PUT /api/preferences", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Owner          string `json:"owner"`
			RefreshSeconds int    `json:"refresh_seconds"`
			Theme          string `json:"theme"`
			Notifications  bool   `json:"notifications"`
			Note           string `json:"note"`
			ReviewDate     string `json:"review_date"`
		}
		if !decode(w, r, &input) {
			return
		}
		if len(strings.TrimSpace(input.Owner)) == 0 || len(input.Owner) > 120 || input.RefreshSeconds < 5 || input.RefreshSeconds > 3600 || (input.Theme != "light" && input.Theme != "dark" && input.Theme != "system") {
			respond(w, 422, map[string]any{"error": map[string]string{"message": "名称不能为空；刷新间隔须为5至3600秒；主题必须是支持的选项"}})
			return
		}
		if len(input.Note) > 2000 {
			respond(w, 422, map[string]any{"error": map[string]string{"message": "备注内容过长"}})
			return
		}
		if input.ReviewDate != "" {
			if _, err := time.Parse("2006-01-02", input.ReviewDate); err != nil {
				respond(w, 422, map[string]any{"error": map[string]string{"message": "复查日期无效"}})
				return
			}
		}
		d.mu.Lock()
		defer d.mu.Unlock()
		d.preferences = map[string]any{"owner": input.Owner, "refresh_seconds": input.RefreshSeconds, "theme": input.Theme, "notifications": input.Notifications, "note": input.Note, "review_date": input.ReviewDate}
		respond(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /api/overview", func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		archived := 0
		for _, task := range d.tasks {
			if task.Status == "已归档" {
				archived++
			}
		}
		completion := 0
		if len(d.tasks) > 0 {
			completion = archived * 100 / len(d.tasks)
		}
		respond(w, 200, map[string]any{"name": "独立工作站示例服务", "version": "1.0.0", "storage": "进程内存（重启重置）", "task_count": len(d.tasks), "archived_count": archived, "completion": completion, "owner": d.preferences["owner"], "remark": nil})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		address, parseError := netip.ParseAddr(host)
		allowed := false
		if err == nil && parseError == nil {
			for _, prefix := range d.allowedPeers {
				if prefix.Contains(address.Unmap()) {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			respond(w, 403, map[string]any{"error": map[string]string{"message": "来源地址未获授权"}})
			return
		}
		if r.URL.Path != "/healthz" {
			provided := sha256.Sum256([]byte(r.Header.Get("Authorization")))
			expected := sha256.Sum256([]byte("Bearer " + d.apiToken))
			if d.apiToken == "" || subtle.ConstantTimeCompare(provided[:], expected[:]) != 1 {
				respond(w, 401, map[string]any{"error": map[string]string{"message": "业务访问认证失败"}})
				return
			}
			// 业务API仅接受服务间调用，不为浏览器开放CORS，也不信任Origin作为身份。
			if r.Header.Get("Origin") != "" {
				respond(w, 403, map[string]any{"error": map[string]string{"message": "请通过中心受限网关访问"}})
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		respond(w, 400, map[string]any{"error": map[string]string{"message": "请求 JSON 无效"}})
		return false
	}
	return true
}

func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func registration(baseURL string) map[string]any {
	return map[string]any{
		"protocol_version": 1, "id": "workstation-demo", "name": "工作站示例服务", "version": "1.0.0", "description": "独立进程示例：任务表格、偏好设置与只读详情。数据仅在演示服务内存中。",
		"base_url": baseURL, "health_path": "/healthz",
		"definition": map[string]any{
			"schema_version": 1,
			"icon":           "workstation",
			"menus":          []any{map[string]any{"id": "workbench-menu", "label": "工作台", "page_id": "workbench", "icon": "overview"}, map[string]any{"id": "tasks-menu", "label": "任务列表", "page_id": "tasks", "icon": "table"}, map[string]any{"id": "preferences-menu", "label": "偏好设置", "page_id": "preferences", "icon": "settings"}, map[string]any{"id": "overview-menu", "label": "服务概览", "page_id": "overview", "icon": "detail"}},
			"pages": []any{
				workbenchPage(),
				map[string]any{"id": "tasks", "title": "任务列表", "breadcrumbs": []any{map[string]any{"label": "服务概览", "page_id": "overview"}}, "type": "table", "source": map[string]any{"path": "/api/tasks"}, "page_size": 10, "columns": []any{map[string]any{"key": "id", "label": "编号"}, map[string]any{"key": "title", "label": "任务"}, map[string]any{"key": "status", "label": "状态"}, map[string]any{"key": "priority", "label": "优先级"}}, "actions": []any{map[string]any{"id": "archive", "label": "归档", "path": "/api/tasks/archive", "method": "POST", "confirm": "确定归档这条任务吗？此操作只修改演示服务的数据。"}}},
				map[string]any{"id": "preferences", "title": "偏好设置", "type": "form", "source": map[string]any{"path": "/api/preferences"}, "submit": map[string]any{"path": "/api/preferences", "method": "PUT"}, "submit_label": "保存偏好", "fields": []any{
					map[string]any{"key": "owner", "label": "管理员名称", "type": "text", "required": true, "max_length": 40, "placeholder": "填写名称"},
					map[string]any{"key": "refresh_seconds", "label": "刷新间隔（秒）", "type": "number", "required": true, "min": 5, "max": 3600},
					map[string]any{"key": "theme", "label": "服务外观", "type": "radio", "required": true, "help": "仅保存此服务的偏好，不改变控制台主题。", "options": []any{map[string]any{"label": "跟随系统", "value": "system"}, map[string]any{"label": "浅色", "value": "light"}, map[string]any{"label": "深色", "value": "dark"}}},
					map[string]any{"key": "notifications", "label": "启用提醒", "type": "boolean"},
					map[string]any{"key": "review_date", "label": "复查日期", "type": "date"},
					map[string]any{"key": "note", "label": "备注", "type": "textarea", "max_length": 500, "rows": 3, "placeholder": "填写需要跟进的事项"},
				}},
				map[string]any{"id": "overview", "title": "服务概览", "type": "detail", "source": map[string]any{"path": "/api/overview"}, "fields": []any{map[string]any{"key": "name", "label": "服务名称"}, map[string]any{"key": "version", "label": "版本"}, map[string]any{"key": "storage", "label": "数据存储"}, map[string]any{"key": "task_count", "label": "任务数量"}, map[string]any{"key": "remark", "label": "备注"}}},
			},
		},
	}
}

func registerLoop(ctx context.Context, log *slog.Logger, controller, baseURL, key string) {
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	payload, _ := json.Marshal(registration(baseURL))
	send := func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(controller, "/")+"/api/v1/registry/register", bytes.NewReader(payload))
		if err != nil {
			log.Error("中心地址格式无效")
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+key)
		res, err := client.Do(req)
		if err != nil {
			if ctx.Err() == nil {
				log.Warn("暂时无法连接中心，下次自动重试")
			}
			return
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			log.Info("服务注册成功，管理员启用状态保持不变")
		} else {
			log.Warn("服务注册未被接受", "status", res.StatusCode, "提示", fmt.Sprintf("检查注册密钥、白名单与协议（HTTP %d）", res.StatusCode))
		}
	}
	send()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

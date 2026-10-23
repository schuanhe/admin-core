package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"workstation/internal/auth"
	"workstation/internal/config"
	"workstation/internal/model"
	"workstation/internal/protocol"
	"workstation/internal/registry"
	"workstation/internal/store"
)

const cookieName = "workstation_session"

type Server struct {
	Config       config.Config
	Store        *store.Store
	Auth         *auth.Manager
	Checker      *registry.Checker
	Log          *slog.Logger
	sequence     atomic.Uint64
	gatewaySlots chan struct{}
}
type principal struct{ username, csrf, hash string }
type principalKey struct{}

func New(c config.Config, s *store.Store, l *slog.Logger) *Server {
	return &Server{Config: c, Store: s, Auth: auth.New(s, c.SessionHours), Checker: registry.New(s, c.AllowedServiceOrigins, l), Log: l, gatewaySlots: make(chan struct{}, 8)}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/auth/status", s.status)
	mux.HandleFunc("POST /api/v1/auth/setup", s.setup)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/registry/register", s.register)
	mux.Handle("GET /api/v1/auth/me", s.authenticated(http.HandlerFunc(s.me)))
	mux.Handle("POST /api/v1/auth/logout", s.authenticated(http.HandlerFunc(s.logout)))
	mux.Handle("GET /api/v1/services", s.authenticated(http.HandlerFunc(s.listServices)))
	mux.Handle("PUT /api/v1/service-navigation", s.authenticated(http.HandlerFunc(s.updateNavigation)))
	mux.HandleFunc("GET /api/v1/integration/handbook", s.handbook)
	mux.Handle("GET /api/v1/services/{id}", s.authenticated(http.HandlerFunc(s.service)))
	mux.Handle("PATCH /api/v1/services/{id}", s.authenticated(http.HandlerFunc(s.activation)))
	mux.Handle("DELETE /api/v1/services/{id}", s.authenticated(http.HandlerFunc(s.deleteService)))
	mux.Handle("POST /api/v1/services/{id}/check", s.authenticated(http.HandlerFunc(s.check)))
	mux.Handle("POST /api/v1/services/{id}/pages/{pageID}/invoke", s.authenticated(http.HandlerFunc(s.invokePage)))
	mux.Handle("GET /api/v1/dashboard", s.authenticated(http.HandlerFunc(s.dashboard)))
	mux.Handle("GET /api/v1/settings", s.authenticated(http.HandlerFunc(s.settings)))
	mux.Handle("PATCH /api/v1/settings", s.authenticated(http.HandlerFunc(s.updateSettings)))
	mux.Handle("GET /api/v1/audit", s.authenticated(http.HandlerFunc(s.audits)))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "not_found", "接口不存在") })
	mux.HandleFunc("/", s.static)
	return s.middleware(http.TimeoutHandler(mux, 15*time.Second, `{"error":{"code":"timeout","message":"请求超时，请稍后重试"}}`))
}
func write(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	write(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func (s *Server) dbError(w http.ResponseWriter, e error) {
	if errors.Is(e, store.ErrNotFound) {
		fail(w, 404, "not_found", "登记记录不存在")
		return
	}
	s.Log.Error("数据库操作失败", "error_type", fmt.Sprintf("%T", e))
	fail(w, 500, "internal_error", "内部操作失败，请查看服务日志")
}
func decode(w http.ResponseWriter, r *http.Request, max int, dst any) ([]byte, error) {
	mediaType, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || mediaType != "application/json" {
		return nil, errors.New("请求必须使用 application/json")
	}
	b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(max)))
	if e != nil {
		return nil, errors.New("请求体超过大小限制或读取失败")
	}
	return b, protocol.Decode(b, dst)
}
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := fmt.Sprintf("%x-%x", started.UnixNano(), s.sequence.Add(1))
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		defer func() {
			if recover() != nil {
				s.Log.Error("请求处理异常", "request_id", requestID)
				fail(w, 500, "internal_error", "内部处理异常")
			}
			s.Log.Info("HTTP请求", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
		}()
		next.ServeHTTP(w, r)
	})
}
func (s *Server) originOK(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		a, e := config.Origin(origin)
		b, _ := config.Origin(s.Config.PublicOrigin)
		return e == nil && a == b
	}
	return true
}
func (s *Server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie(cookieName)
		if e != nil || len(c.Value) != 64 {
			fail(w, 401, "unauthorized", "请先登录")
			return
		}
		username, csrf, e := s.Store.Session(r.Context(), auth.Hash(c.Value))
		if e != nil {
			if !errors.Is(e, store.ErrNotFound) {
				s.dbError(w, e)
				return
			}
			fail(w, 401, "unauthorized", "会话已失效，请重新登录")
			return
		}
		// 读取接口也校验浏览器来源，不能让其他本地站点读取管理数据或CSRF令牌。
		if !s.originOK(r) {
			fail(w, 403, "origin_denied", "请求来源不受信任")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if !auth.Equal(r.Header.Get("X-CSRF-Token"), csrf) {
				fail(w, 403, "csrf_failed", "请求来源或CSRF令牌无效")
				return
			}
		}
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, principal{username: username, csrf: csrf, hash: auth.Hash(c.Value)}))
		next.ServeHTTP(w, r)
	})
}
func current(r *http.Request) principal { return r.Context().Value(principalKey{}).(principal) }
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if e := s.Store.DB.PingContext(ctx); e != nil {
		write(w, 503, map[string]string{"status": "unavailable"})
		return
	}
	write(w, 200, map[string]string{"status": "ok"})
}
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	v, e := s.Store.Initialized(r.Context())
	if e != nil {
		s.dbError(w, e)
		return
	}
	write(w, 200, map[string]bool{"initialized": v})
}
func (s *Server) authLimit(w http.ResponseWriter, r *http.Request) bool {
	if !s.originOK(r) {
		fail(w, 403, "origin_denied", "请求来源不受信任")
		return false
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !s.Auth.Allow(host) {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "rate_limited", "尝试过于频繁，请一分钟后重试")
		return false
	}
	return true
}
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if !s.authLimit(w, r) {
		return
	}
	var v struct {
		Username       string `json:"username"`
		Password       string `json:"password"`
		BootstrapToken string `json:"bootstrap_token"`
	}
	if _, e := decode(w, r, 4096, &v); e != nil {
		fail(w, 400, "invalid_input", e.Error())
		return
	}
	if !auth.Equal(v.BootstrapToken, s.Config.BootstrapToken) {
		_ = s.Store.Audit(r.Context(), "anonymous", "auth.setup_failed", "administrator", "初始化令牌校验失败")
		fail(w, 403, "invalid_token", "初始化令牌无效")
		return
	}
	e := s.Auth.Setup(r.Context(), v.Username, v.Password)
	if errors.Is(e, store.ErrInitialized) {
		fail(w, 409, "already_initialized", "管理员已经初始化")
		return
	}
	if e != nil {
		fail(w, 400, "invalid_input", e.Error())
		return
	}
	write(w, 201, map[string]bool{"ok": true})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.authLimit(w, r) {
		return
	}
	var v struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if _, e := decode(w, r, 4096, &v); e != nil {
		fail(w, 400, "invalid_input", e.Error())
		return
	}
	token, csrf, expires, e := s.Auth.Login(r.Context(), v.Username, v.Password)
	if e != nil {
		if errors.Is(e, auth.ErrCredentials) {
			_ = s.Store.Audit(r.Context(), "anonymous", "auth.login_failed", "administrator", "用户名或密码验证失败")
			fail(w, 401, "invalid_credentials", "用户名或密码错误")
			return
		}
		s.dbError(w, e)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.Config.SecureCookie, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: s.Config.SessionHours * 3600})
	write(w, 200, map[string]any{"user": map[string]string{"username": v.Username}, "csrf_token": csrf})
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	write(w, 200, map[string]any{"user": map[string]string{"username": p.username}, "csrf_token": p.csrf})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	if e := s.Store.DeleteSession(r.Context(), p.hash, p.username); e != nil {
		s.dbError(w, e)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.Config.SecureCookie, SameSite: http.SameSiteStrictMode, Expires: time.Unix(1, 0), MaxAge: -1})
	write(w, 200, map[string]bool{"ok": true})
}
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") || !auth.Equal(strings.TrimPrefix(h, "Bearer "), s.Config.RegistrationKey) {
		fail(w, 401, "invalid_registration_key", "服务注册认证失败")
		return
	}
	var v protocol.Registration
	b, e := decode(w, r, protocol.MaxBody, &v)
	if e == nil {
		e = v.Validate(s.Config.AllowedServiceOrigins)
	}
	if e != nil {
		var hint struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(b, &hint)
		if dbErr := s.Store.RegistrationFailure(r.Context(), hint.ID, e.Error()); dbErr != nil {
			s.Log.Warn("注册失败事件保存失败")
		}
		fail(w, 400, "invalid_registration", e.Error())
		return
	}
	service, e := s.Store.Register(r.Context(), v)
	if e != nil {
		s.dbError(w, e)
		return
	}
	write(w, 200, map[string]any{"service": service})
}
func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	a, o := q.Get("activation"), q.Get("online")
	if (a != "" && a != "pending" && a != "enabled" && a != "disabled") || (o != "" && o != "unknown" && o != "online" && o != "offline") || len(q.Get("q")) > 200 {
		fail(w, 400, "invalid_input", "筛选条件无效")
		return
	}
	items, e := s.Store.Services(r.Context(), q.Get("q"), a, o)
	if e != nil {
		s.dbError(w, e)
		return
	}
	write(w, 200, map[string]any{"items": items})
}
func (s *Server) service(w http.ResponseWriter, r *http.Request) {
	v, e := s.Store.Service(r.Context(), r.PathValue("id"))
	if e != nil {
		s.dbError(w, e)
		return
	}
	events, e := s.Store.Events(r.Context(), v.ID)
	if e != nil {
		s.dbError(w, e)
		return
	}
	write(w, 200, map[string]any{"service": v, "events": events})
}
func (s *Server) activation(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Activation string `json:"activation"`
	}
	if _, e := decode(w, r, 1024, &v); e != nil {
		fail(w, 400, "invalid_input", e.Error())
		return
	}
	if v.Activation != "enabled" && v.Activation != "disabled" {
		fail(w, 400, "invalid_input", "启用状态只能为enabled或disabled")
		return
	}
	service, e := s.Store.SetActivation(r.Context(), r.PathValue("id"), v.Activation, current(r).username)
	if e != nil {
		s.dbError(w, e)
		return
	}
	write(w, 200, map[string]any{"service": service})
}
func (s *Server) deleteService(w http.ResponseWriter, r *http.Request) {
	if e := s.Store.DeleteService(r.Context(), r.PathValue("id"), current(r).username); e != nil {
		s.dbError(w, e)
		return
	}
	write(w, 200, map[string]bool{"ok": true})
}
func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	v, e := s.Checker.Check(r.Context(), r.PathValue("id"))
	if e != nil {
		s.dbError(w, e)
		return
	}
	if e = s.Store.Audit(r.Context(), current(r).username, "service.check", v.ID, "手动检查状态: "+v.Online); e != nil {
		s.dbError(w, e)
		return
	}
	write(w, 200, map[string]any{"service": v})
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	items, e := s.Store.Services(r.Context(), "", "", "")
	if e != nil {
		s.dbError(w, e)
		return
	}
	enabled, offline, pending := 0, 0, 0
	for _, v := range items {
		if v.Activation == "enabled" {
			enabled++
		}
		if v.Activation == "pending" {
			pending++
		}
		if v.Online == "offline" {
			offline++
		}
	}
	recent := items
	if len(recent) > 5 {
		recent = recent[:5]
	}
	write(w, 200, map[string]any{"total": len(items), "enabled": enabled, "offline": offline, "pending": pending, "recent": recent})
}
func (s *Server) getSettings(ctx context.Context) (model.Settings, error) {
	name, interval, e := s.Store.Settings(ctx, s.Config.CheckIntervalSeconds)
	if e != nil {
		return model.Settings{}, e
	}
	enabled, e := s.Store.HandbookEnabled(ctx)
	return model.Settings{HandbookEnabled: enabled, SiteName: name, CheckIntervalSeconds: interval, AllowedServiceOrigins: s.Config.AllowedServiceOrigins, SessionHours: s.Config.SessionHours, GatewayServiceCount: len(s.Config.ServiceAccess)}, e
}
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	v, e := s.getSettings(r.Context())
	if e != nil {
		s.dbError(w, e)
		return
	}
	write(w, 200, v)
}
func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var v struct {
		SiteName             string `json:"site_name"`
		CheckIntervalSeconds int    `json:"check_interval_seconds"`
		HandbookEnabled      *bool  `json:"handbook_enabled"`
	}
	if _, e := decode(w, r, 4096, &v); e != nil {
		fail(w, 400, "invalid_input", e.Error())
		return
	}
	v.SiteName = strings.TrimSpace(v.SiteName)
	if v.SiteName == "" || utf8.RuneCountInString(v.SiteName) > 80 || v.CheckIntervalSeconds < 10 || v.CheckIntervalSeconds > 3600 {
		fail(w, 400, "invalid_input", "站点名称限1到80字，检查间隔限10到3600秒")
		return
	}
	var handbookEnabled []bool
	if v.HandbookEnabled != nil {
		handbookEnabled = append(handbookEnabled, *v.HandbookEnabled)
	}
	if e := s.Store.UpdateSettings(r.Context(), v.SiteName, v.CheckIntervalSeconds, current(r).username, handbookEnabled...); e != nil {
		s.dbError(w, e)
		return
	}
	s.settings(w, r)
}
func (s *Server) audits(w http.ResponseWriter, r *http.Request) {
	page, size := 1, 20
	var e error
	if q := r.URL.Query().Get("page"); q != "" {
		page, e = strconv.Atoi(q)
		if e != nil {
			page = 0
		}
	}
	if q := r.URL.Query().Get("page_size"); q != "" {
		size, e = strconv.Atoi(q)
		if e != nil {
			size = 0
		}
	}
	if page < 1 || page > 1000000 || size < 1 || size > 100 {
		fail(w, 400, "invalid_input", "分页参数无效")
		return
	}
	items, total, e := s.Store.Audits(r.Context(), page, size)
	if e != nil {
		s.dbError(w, e)
		return
	}
	write(w, 200, map[string]any{"items": items, "total": total, "page": page, "page_size": size})
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		fail(w, 405, "method_not_allowed", "请求方法不受支持")
		return
	}
	if strings.ContainsAny(r.URL.Path, "\\:") {
		fail(w, 404, "not_found", "资源不存在")
		return
	}
	clean := filepath.Clean(filepath.FromSlash("/" + r.URL.Path))
	relative := strings.TrimLeft(clean, "/\\")
	file := filepath.Join(s.Config.StaticDir, relative)
	root, e := filepath.Abs(s.Config.StaticDir)
	if e != nil {
		fail(w, 503, "frontend_not_built", "前端目录无效")
		return
	}
	if resolved, e := filepath.EvalSymlinks(file); e == nil {
		absolute, e := filepath.Abs(resolved)
		if e != nil {
			fail(w, 404, "not_found", "资源不存在")
			return
		}
		resolvedRoot, e := filepath.EvalSymlinks(root)
		if e != nil {
			fail(w, 404, "not_found", "资源不存在")
			return
		}
		rel, e := filepath.Rel(resolvedRoot, absolute)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			fail(w, 404, "not_found", "资源不存在")
			return
		}
	}
	if fi, e := os.Stat(file); e == nil && !fi.IsDir() {
		http.ServeFile(w, r, file)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api") || filepath.Ext(r.URL.Path) != "" {
		fail(w, 404, "not_found", "资源不存在")
		return
	}
	index := filepath.Join(s.Config.StaticDir, "index.html")
	if _, e := os.Stat(index); e != nil {
		fail(w, 503, "frontend_not_built", "前端尚未构建，请在web目录执行npm run build")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, index)
}

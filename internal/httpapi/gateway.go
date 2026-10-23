package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"

	"workstation/internal/config"
	"workstation/internal/protocol"
	"workstation/internal/registry"
)

type pageInvocation struct {
	Operation string         `json:"operation"`
	ActionID  string         `json:"action_id,omitempty"`
	Query     map[string]int `json:"query,omitempty"`
	Body      map[string]any `json:"body,omitempty"`
}

// 浏览器没有URL、路径、方法或凭据控制权；实际接口取页面定义与部署策略的交集。
func (s *Server) invokePage(w http.ResponseWriter, r *http.Request) {
	if !protocol.ValidServiceID(r.PathValue("id")) || !protocol.ValidID(r.PathValue("pageID")) {
		fail(w, 400, "invalid_input", "服务或页面标识无效")
		return
	}
	service, e := s.Store.Service(r.Context(), r.PathValue("id"))
	if e != nil {
		s.dbError(w, e)
		return
	}
	if service.Activation != "enabled" {
		fail(w, 403, "service_disabled", "服务尚未启用或已被禁用")
		return
	}
	policy, ok := s.Config.ServiceAccess[service.ID]
	origin, originError := config.Origin(service.BaseURL)
	if !ok || originError != nil || origin != policy.Origin {
		fail(w, 403, "service_access_denied", "该服务未配置受信任的业务访问策略，或登记地址与策略不一致")
		return
	}
	var definition protocol.Definition
	if protocol.Decode(service.Definition, &definition) != nil || definition.Validate() != nil {
		fail(w, 422, "invalid_definition", "此服务页面定义无效，其他管理功能不受影响")
		return
	}
	var page *protocol.Page
	for i := range definition.Pages {
		if definition.Pages[i].ID == r.PathValue("pageID") {
			page = &definition.Pages[i]
			break
		}
	}
	if page == nil {
		fail(w, 404, "page_not_found", "服务未提供该页面")
		return
	}
	var input pageInvocation
	if _, e = decode(w, r, 64*1024, &input); e != nil {
		fail(w, 400, "invalid_input", e.Error())
		return
	}
	method, path := "", ""
	switch input.Operation {
	case "source":
		if page.Source != nil && input.Body == nil && input.ActionID == "" {
			method, path = "GET", page.Source.Path
		}
	case "submit":
		if page.Type == "form" && page.Submit != nil && len(input.Query) == 0 && input.ActionID == "" && validFormBody(*page, input.Body) {
			method, path = page.Submit.Method, page.Submit.Path
		}
	case "action":
		if page.Type == "table" && len(input.Query) == 0 && len(input.Body) == 1 {
			id, exists := input.Body["id"]
			_, textID := id.(string)
			_, numberID := id.(float64)
			if exists && (textID || numberID) {
				for _, action := range page.Actions {
					if action.ID == input.ActionID {
						method, path = action.Method, action.Path
						break
					}
				}
			}
		}
	}
	if method == "" || !protocol.ValidPath(path) {
		fail(w, 400, "invalid_operation", "页面操作、分页或提交字段不符合定义")
		return
	}
	if !policy.Allows(method, path) {
		fail(w, 403, "endpoint_denied", "该业务接口未获部署者的路径与方法白名单授权")
		return
	}
	query := url.Values{}
	if input.Operation == "source" && page.Type == "table" {
		if len(input.Query) > 2 {
			fail(w, 400, "invalid_query", "仅支持page和page_size分页参数")
			return
		}
		for key := range input.Query {
			if key != "page" && key != "page_size" {
				fail(w, 400, "invalid_query", "分页参数不受支持")
				return
			}
		}
		number, size := input.Query["page"], input.Query["page_size"]
		if number == 0 {
			number = 1
		}
		if size == 0 {
			size = page.PageSize
			if size == 0 {
				size = 20
			}
		}
		if number < 1 || number > 1000000 || size < 1 || size > 100 {
			fail(w, 400, "invalid_query", "分页参数超出限制")
			return
		}
		query.Set("page", strconv.Itoa(number))
		query.Set("page_size", strconv.Itoa(size))
	} else if len(input.Query) != 0 {
		fail(w, 400, "invalid_query", "该页面操作不接受查询参数")
		return
	}
	select {
	case s.gatewaySlots <- struct{}{}:
		defer func() { <-s.gatewaySlots }()
	default:
		fail(w, 429, "gateway_busy", "业务请求繁忙，请稍后重试")
		return
	}
	// 写操作先登记意图；审计不可用时不发出业务写请求。
	if method != "GET" {
		if e = s.Store.Audit(r.Context(), current(r).username, "service.api_attempt", service.ID, method+" "+path); e != nil {
			s.dbError(w, e)
			return
		}
	}
	status := "failed"
	defer func() {
		if method != "GET" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if s.Store.Audit(ctx, current(r).username, "service.api_result", service.ID, method+" "+path+" "+status) != nil {
				s.Log.Warn("业务操作结果审计保存失败", "service_id", service.ID)
			}
		}
	}()
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	client, closeClient, e := registry.RestrictedClient(ctx, origin, s.Config.AllowedServiceOrigins, s.Checker.Resolver, 6*time.Second, true)
	if e != nil {
		fail(w, 502, "service_unavailable", "服务地址解析或网络策略检查失败")
		return
	}
	defer closeClient()
	target := origin + path
	if len(query) != 0 {
		target += "?" + query.Encode()
	}
	var body io.Reader
	if input.Body != nil {
		payload, _ := json.Marshal(input.Body)
		body = bytes.NewReader(payload)
	}
	request, e := http.NewRequestWithContext(ctx, method, target, body)
	if e != nil {
		fail(w, 400, "invalid_operation", "业务操作无效")
		return
	}
	// 使用新请求构建固定头集合，不透传浏览器Cookie、认证头、Origin或转发头。
	request.Header.Set("Authorization", "Bearer "+policy.BearerToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Workstation-Controller/1")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, e := client.Do(request)
	if e != nil {
		fail(w, 502, "service_unavailable", "无法连接业务服务或请求已超时，控制中心仍可正常使用")
		return
	}
	defer response.Body.Close()
	status = fmt.Sprintf("HTTP_%d", response.StatusCode)
	if response.StatusCode == 401 || response.StatusCode == 403 {
		fail(w, 502, "service_auth_failed", "业务服务拒绝中心访问，请检查独立业务令牌与服务端IP白名单")
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		code := http.StatusBadGateway
		if response.StatusCode == 400 || response.StatusCode == 422 || response.StatusCode == 409 {
			code = response.StatusCode
		}
		fail(w, code, "service_rejected", "业务服务未接受本次操作，请检查提交内容或服务状态")
		return
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType != "application/json" {
		fail(w, 502, "invalid_service_response", "业务接口必须返回JSON数据")
		return
	}
	payload, e := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	if e != nil || len(payload) > 1024*1024 || !utf8.Valid(payload) || !json.Valid(payload) || bytes.Contains(payload, []byte(policy.BearerToken)) || bytes.Contains(payload, []byte(s.Config.RegistrationKey)) || bytes.Contains(payload, []byte(s.Config.BootstrapToken)) {
		fail(w, 502, "invalid_service_response", "业务响应无效、超出1MiB或包含受保护的凭据")
		return
	}
	status = "succeeded"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func validFormBody(page protocol.Page, body map[string]any) bool {
	if body == nil || len(body) > len(page.Fields) {
		return false
	}
	fields := map[string]protocol.Field{}
	for _, field := range page.Fields {
		fields[field.Key] = field
	}
	for key := range body {
		if _, exists := fields[key]; !exists {
			return false
		}
	}
	for _, field := range page.Fields {
		value := body[field.Key]
		if value == nil {
			if field.Required {
				return false
			}
			continue
		}
		switch field.Type {
		case "text", "textarea", "date":
			text, ok := value.(string)
			limit := field.MaxLength
			if limit == 0 {
				limit = 2000
			}
			if !ok || utf8.RuneCountInString(text) > limit || (field.Required && len(bytes.TrimSpace([]byte(text))) == 0) {
				return false
			}
			if field.Type == "date" && text != "" {
				if _, err := time.Parse("2006-01-02", text); err != nil {
					return false
				}
			}
		case "number":
			number, ok := value.(float64)
			if !ok || math.IsInf(number, 0) || math.IsNaN(number) {
				return false
			}
			if (field.Min != nil && number < *field.Min) || (field.Max != nil && number > *field.Max) {
				return false
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return false
			}
		case "select", "radio":
			found := false
			for _, option := range field.Options {
				if value == option.Value {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		default:
			return false
		}
	}
	return true
}

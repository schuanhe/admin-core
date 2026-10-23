package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestDemoBusinessAPI(t *testing.T) {
	d := newDemo([]string{"http://localhost:8080"})
	d.apiToken = strings.Repeat("a", 48)
	d.allowedPeers = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	h := d.handler()
	request := func(method, path string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.RemoteAddr = "127.0.0.1:32100"
		r.Header.Set("Authorization", "Bearer "+d.apiToken)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := request(http.MethodGet, "/api/tasks?page=1&page_size=2", nil)
	var list struct {
		Items []task `json:"items"`
		Total int    `json:"total"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Items) != 2 || list.Total != 3 {
		t.Fatalf("分页异常：%s", w.Body.String())
	}
	if request(http.MethodPost, "/api/tasks/archive", []byte(`{"id":1}`)).Code != 200 {
		t.Fatal("归档失败")
	}
	if request(http.MethodPut, "/api/preferences", []byte(`{"owner":"本地管理员","refresh_seconds":20,"theme":"dark","notifications":false}`)).Code != 200 {
		t.Fatal("保存失败")
	}
	if request(http.MethodPut, "/api/preferences", []byte(`{"owner":"","refresh_seconds":0,"theme":"bad"}`)).Code != 422 {
		t.Fatal("必须拒绝非法业务输入")
	}
	r := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	r.RemoteAddr = "127.0.0.1:32100"
	r.Header.Set("Authorization", "Bearer "+d.apiToken)
	r.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("必须拒绝未允许来源")
	}
	if request(http.MethodOptions, "/api/preferences", nil).Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("业务接口不得向浏览器开放CORS")
	}
	if request(http.MethodGet, "/api/tasks?page=9223372036854775807&page_size=100", nil).Code != 200 {
		t.Fatal("超大分页必须安全返回")
	}
}

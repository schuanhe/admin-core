package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func validRegistration() Registration {
	return Registration{ProtocolVersion: 1, ID: "demo-service", Name: "演示服务", Version: "1.0.0", BaseURL: "http://127.0.0.1:8091", HealthPath: "/healthz", Definition: Definition{SchemaVersion: 1, Menus: []Menu{{ID: "menu", Label: "测试", PageID: "table"}}, Pages: []Page{{ID: "table", Title: "表格", Type: "table", Source: &Source{Path: "/api/items"}, Columns: []Column{{Key: "id", Label: "编号"}}}}}}
}
func TestRegistrationValidation(t *testing.T) {
	good := validRegistration()
	if e := good.Validate([]string{good.BaseURL}); e != nil {
		t.Fatal(e)
	}
	tests := []struct {
		name   string
		mutate func(*Registration)
	}{
		{"协议版本", func(r *Registration) { r.ProtocolVersion = 2 }},
		{"危险服务来源", func(r *Registration) { r.BaseURL = "http://169.254.169.254" }},
		{"地址含凭据", func(r *Registration) { r.BaseURL = "http://secret@127.0.0.1:8091" }},
		{"错误页面版本", func(r *Registration) { r.Definition.SchemaVersion = 2 }},
		{"任意组件", func(r *Registration) { r.Definition.Pages[0].Type = "script" }},
		{"跨来源路径", func(r *Registration) { r.Definition.Pages[0].Source.Path = "//example.com/api" }},
		{"不存在页面引用", func(r *Registration) { r.Definition.Menus[0].PageID = "missing" }},
		{"原型字段", func(r *Registration) { r.Definition.Pages[0].Columns[0].Key = "constructor" }},
		{"重复列", func(r *Registration) {
			r.Definition.Pages[0].Columns = append(r.Definition.Pages[0].Columns, r.Definition.Pages[0].Columns[0])
		}},
		{"过多字段", func(r *Registration) { r.Definition.Pages[0].Columns = make([]Column, 31) }},
		{"跨类型字段", func(r *Registration) { r.Definition.Pages[0].Fields = []Field{{Key: "x", Label: "x"}} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := validRegistration()
			tc.mutate(&r)
			if e := r.Validate([]string{"http://127.0.0.1:8091"}); e == nil {
				t.Fatal("应当拒绝无效注册")
			}
		})
	}
}
func TestDecodeLimits(t *testing.T) {
	for _, raw := range []string{`{"x":1,"x":2}`, `{"__proto__":{}}`, `{"x":"` + strings.Repeat("a", 2001) + `"}`, strings.Repeat("[", 14) + "0" + strings.Repeat("]", 14), `{"x":1} {}`, `{"x":` + strings.Repeat(" ", MaxBody) + `1}`, `{"unknown":true}`} {
		var r Registration
		if e := Decode([]byte(raw), &r); e == nil {
			t.Fatalf("应拒绝无效输入: %.80s", raw)
		}
	}
	r := validRegistration()
	b, _ := json.Marshal(r)
	var decoded Registration
	if e := Decode(b, &decoded); e != nil {
		t.Fatal(e)
	}
}
func TestValidPaths(t *testing.T) {
	for _, p := range []string{"/healthz", "/api/items", "/"} {
		if !ValidPath(p) {
			t.Errorf("应接受 %s", p)
		}
	}
	for _, p := range []string{"//evil", "/../secret", "/api/./x", "/%2f%2fevil", "/a\\b", "/a?token=123", "/a#x", "https://evil/x", "/a:b", "/a b", "/api//x"} {
		if ValidPath(p) {
			t.Errorf("应拒绝 %s", p)
		}
	}
}
func TestAllPageTypes(t *testing.T) {
	d := Definition{SchemaVersion: 1, Menus: []Menu{}, Pages: []Page{{ID: "form", Title: "表单", Type: "form", Fields: []Field{{Key: "name", Label: "名称", Type: "text", Required: true}, {Key: "count", Label: "数量", Type: "number"}, {Key: "active", Label: "启用", Type: "boolean"}, {Key: "kind", Label: "类型", Type: "select", Options: []Option{{Label: "A", Value: "a"}}}}, Submit: &Endpoint{Path: "/api/config", Method: "PUT"}}, {ID: "detail", Title: "详情", Type: "detail", Fields: []Field{{Key: "name", Label: "名称"}}, Source: &Source{Path: "/api/config"}}}}
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
}

func TestDefinitionRequiresExplicitArrays(t *testing.T) {
	for _, raw := range []string{`{"schema_version":1}`, `{"schema_version":1,"menus":null,"pages":[]}`, `{"schema_version":1,"menus":[],"pages":null}`} {
		var d Definition
		if e := Decode([]byte(raw), &d); e != nil {
			t.Fatal(e)
		}
		if e := d.Validate(); e == nil {
			t.Fatal("菜单与页面必须明确是数组")
		}
	}
	d := Definition{SchemaVersion: 1, Menus: []Menu{}, Pages: []Page{}}
	if e := d.Validate(); e != nil {
		t.Fatal("无界面服务允许显式空数组", e)
	}
}

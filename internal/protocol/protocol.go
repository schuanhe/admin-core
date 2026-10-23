package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"workstation/internal/config"
	webcontract "workstation/web"
)

var MaxBody = webcontract.Catalog.Limits["registration_bytes"]

var serviceID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var fieldID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var pathPattern = regexp.MustCompile(`^/[A-Za-z0-9_~./-]*$`)

type Registration struct {
	ProtocolVersion int        `json:"protocol_version"`
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Version         string     `json:"version"`
	Description     string     `json:"description"`
	BaseURL         string     `json:"base_url"`
	HealthPath      string     `json:"health_path"`
	Definition      Definition `json:"definition"`
}
type Definition struct {
	SchemaVersion int    `json:"schema_version"`
	Icon          string `json:"icon,omitempty"`
	Menus         []Menu `json:"menus"`
	Pages         []Page `json:"pages"`
}
type Menu struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	PageID string `json:"page_id"`
	Icon   string `json:"icon,omitempty"`
}
type Breadcrumb struct {
	Label  string `json:"label"`
	PageID string `json:"page_id,omitempty"`
}
type Source struct {
	Path string `json:"path"`
}
type Endpoint struct {
	Path   string `json:"path"`
	Method string `json:"method"`
}
type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}
type Option struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}
type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Options     []Option `json:"options,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Help        string   `json:"help,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	MaxLength   int      `json:"max_length,omitempty"`
	Rows        int      `json:"rows,omitempty"`
}
type Action struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Path    string `json:"path"`
	Method  string `json:"method"`
	Confirm string `json:"confirm,omitempty"`
}
type Page struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Type        string            `json:"type"`
	Description string            `json:"description,omitempty"`
	Source      *Source           `json:"source,omitempty"`
	Columns     []Column          `json:"columns,omitempty"`
	Actions     []Action          `json:"actions,omitempty"`
	PageSize    int               `json:"page_size,omitempty"`
	Fields      []Field           `json:"fields,omitempty"`
	Submit      *Endpoint         `json:"submit,omitempty"`
	SubmitLabel string            `json:"submit_label,omitempty"`
	Breadcrumbs []Breadcrumb      `json:"breadcrumbs,omitempty"`
	ClassName   string            `json:"class_name,omitempty"`
	Style       map[string]string `json:"style,omitempty"`
	Stylesheet  string            `json:"stylesheet,omitempty"`
	Blocks      []Block           `json:"blocks,omitempty"`
}

// 图标名只选择中心内置资源，不接受路径、SVG、HTML或外部URL。
func validIcon(name string) bool {
	if name == "" {
		return true
	}
	_, exists := webcontract.Catalog.Icons[name]
	return exists
}

func ValidID(s string) bool {
	return fieldID.MatchString(s) && s != "constructor" && s != "prototype" && s != "__proto__"
}
func ValidServiceID(s string) bool { return serviceID.MatchString(s) }
func text(s string, max int, required bool) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && (!required || strings.TrimSpace(s) != "")
}

// Decode 在类型解码前约束大小、结构深度、字段总量和重复键。
func Decode(data []byte, dst any) error {
	if len(data) > MaxBody {
		return errors.New("JSON 超过256 KiB限制")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	count := 0
	if err := walk(dec, 0, &count, 2000); err != nil {
		return err
	}
	if _, e := dec.Token(); e != io.EOF {
		return errors.New("JSON 包含多余内容")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if e := d.Decode(dst); e != nil {
		return errors.New("JSON 字段或类型无效")
	}
	return nil
}
func walk(d *json.Decoder, depth int, count *int, maxText int) error {
	if depth > webcontract.Catalog.Limits["json_depth"] {
		return errors.New("JSON 嵌套超过12层")
	}
	t, e := d.Token()
	if e != nil {
		return errors.New("JSON 格式无效")
	}
	if s, ok := t.(string); ok && utf8.RuneCountInString(s) > maxText {
		return errors.New("JSON 字符串超过字段长度限制")
	}
	if v, ok := t.(json.Delim); ok {
		switch v {
		case '{':
			keys := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return errors.New("JSON 字段无效")
				}
				key, ok := k.(string)
				if !ok || len(key) > 128 || keys[key] || key == "__proto__" || key == "constructor" || key == "prototype" {
					return errors.New("JSON 包含重复、保留或过长字段")
				}
				keys[key] = true
				*count++
				if *count > webcontract.Catalog.Limits["json_fields"] {
					return errors.New("JSON 字段超过3000项")
				}
				limit := 2000
				if key == "stylesheet" {
					limit = webcontract.Catalog.Limits["stylesheet_bytes"]
				}
				if e = walk(d, depth+1, count, limit); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		case '[':
			for d.More() {
				*count++
				if *count > webcontract.Catalog.Limits["json_fields"] {
					return errors.New("JSON 元素超过3000项")
				}
				if e = walk(d, depth+1, count, 2000); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		default:
			return errors.New("JSON 结构无效")
		}
	}
	return nil
}

func ValidPath(s string) bool {
	if len(s) == 0 || len(s) > 512 || !pathPattern.MatchString(s) || !strings.HasPrefix(s, "/") || strings.Contains(s, "//") || strings.ContainsAny(s, "\\?#\r\n\t ") {
		return false
	}
	u, e := url.Parse(s)
	if e != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || u.Path != s {
		return false
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == "." || seg == ".." {
			return false
		}
	}
	return true
}
func (r *Registration) Validate(allowed []string) error {
	if r.ProtocolVersion != 1 {
		return errors.New("不支持该注册协议版本，仅支持1")
	}
	if !ValidServiceID(r.ID) || !text(r.Name, 120, true) || !text(r.Version, 64, true) || !text(r.Description, 2000, false) {
		return errors.New("服务标识、名称、版本或描述无效")
	}
	origin, e := config.Origin(r.BaseURL)
	if e != nil {
		return errors.New("服务地址必须是完整HTTP(S)来源")
	}
	ok := false
	for _, a := range allowed {
		v, _ := config.Origin(a)
		if origin == v {
			ok = true
			break
		}
	}
	if !ok {
		return errors.New("服务地址不在管理员配置的允许来源中")
	}
	r.BaseURL = origin
	if r.HealthPath == "" {
		r.HealthPath = "/healthz"
	}
	if !ValidPath(r.HealthPath) {
		return errors.New("健康检查路径无效")
	}
	return r.Definition.Validate()
}

func (d Definition) Validate() error {
	if !validIcon(d.Icon) {
		return errors.New("服务图标不在内置白名单中")
	}
	if d.Menus == nil || d.Pages == nil {
		return errors.New("menus与pages必须是JSON数组，可以为空数组")
	}
	if d.SchemaVersion != 1 {
		return errors.New("不支持该页面定义版本，仅支持1")
	}
	if len(d.Menus) > webcontract.Catalog.Limits["menus"] || len(d.Pages) > webcontract.Catalog.Limits["pages"] {
		return errors.New("菜单和页面分别最多20项")
	}
	pages := map[string]bool{}
	for _, p := range d.Pages {
		if !ValidID(p.ID) || pages[p.ID] || !text(p.Title, 120, true) || !text(p.Description, 2000, false) {
			return errors.New("页面标识或标题无效或重复")
		}
		pages[p.ID] = true
		if e := p.Validate(); e != nil {
			return fmt.Errorf("页面 %s: %w", p.ID, e)
		}
	}
	menus := map[string]bool{}
	for _, p := range d.Pages {
		if e := validateBlockReferences(p.Blocks, d.Pages); e != nil {
			return e
		}
		if len(p.Breadcrumbs) > 4 {
			return errors.New("页面面包屑最多4层")
		}
		seen := map[string]bool{}
		for _, crumb := range p.Breadcrumbs {
			if !text(crumb.Label, 120, true) || (crumb.PageID != "" && (!pages[crumb.PageID] || crumb.PageID == p.ID || seen[crumb.PageID])) {
				return errors.New("面包屑名称或本服务页面引用无效或重复")
			}
			if crumb.PageID != "" {
				seen[crumb.PageID] = true
			}
		}
	}
	for _, m := range d.Menus {
		if !ValidID(m.ID) || menus[m.ID] || !text(m.Label, 120, true) || !pages[m.PageID] || !validIcon(m.Icon) {
			return errors.New("菜单标识、标题或页面引用无效")
		}
		menus[m.ID] = true
	}
	return nil
}
func (p Page) Validate() error {
	if err := validatePresentation(p.ClassName, p.Style); err != nil {
		return err
	}
	if !utf8.ValidString(p.Stylesheet) || len(p.Stylesheet) > webcontract.Catalog.Limits["stylesheet_bytes"] {
		return errors.New("页面CSS最多16KiB UTF-8文本")
	}
	if p.Type != "compose" && len(p.Blocks) > 0 {
		return errors.New("只有组合页面可以包含blocks")
	}
	if p.Source != nil && !ValidPath(p.Source.Path) {
		return errors.New("数据来源路径无效")
	}
	switch p.Type {
	case "compose":
		if len(p.Columns) > 0 || len(p.Fields) > 0 || len(p.Actions) > 0 || p.PageSize != 0 || p.Submit != nil || p.SubmitLabel != "" {
			return errors.New("组合页面不能配置表格或表单专用字段")
		}
		if blocksNeedSource(p.Blocks) && p.Source == nil {
			return errors.New("包含数据绑定或描述列表的组合页面需要source")
		}
		return validateBlocks(p.Blocks)
	case "table":
		if p.Source == nil || len(p.Columns) < 1 || len(p.Columns) > 30 || len(p.Actions) > 8 || p.PageSize < 0 || p.PageSize > 100 || len(p.Fields) > 0 || p.Submit != nil || p.SubmitLabel != "" {
			return errors.New("表格字段或分页配置无效")
		}
		keys := map[string]bool{}
		for _, c := range p.Columns {
			if !ValidID(c.Key) || keys[c.Key] || !text(c.Label, 120, true) {
				return errors.New("表格列无效或重复")
			}
			keys[c.Key] = true
		}
		ids := map[string]bool{}
		for _, a := range p.Actions {
			if !ValidID(a.ID) || ids[a.ID] || !text(a.Label, 120, true) || !text(a.Confirm, 2000, false) || !ValidPath(a.Path) || (a.Method != "POST" && a.Method != "DELETE") {
				return errors.New("表格操作无效")
			}
			ids[a.ID] = true
		}
	case "form", "detail":
		if len(p.Fields) < 1 || len(p.Fields) > 40 || len(p.Columns) > 0 || len(p.Actions) > 0 || p.PageSize != 0 {
			return errors.New("表单或详情字段配置无效")
		}
		if p.Type == "detail" && (p.Source == nil || p.Submit != nil || p.SubmitLabel != "") {
			return errors.New("详情配置无效")
		}
		if p.Type == "form" && (p.Submit == nil || !ValidPath(p.Submit.Path) || (p.Submit.Method != "POST" && p.Submit.Method != "PUT") || !text(p.SubmitLabel, 120, false)) {
			return errors.New("表单提交配置无效")
		}
		keys := map[string]bool{}
		for _, f := range p.Fields {
			if !ValidID(f.Key) || keys[f.Key] || !text(f.Label, 120, true) {
				return errors.New("字段标识或标签无效或重复")
			}
			keys[f.Key] = true
			if p.Type == "detail" {
				if f.Type != "" || f.Required || len(f.Options) > 0 || f.Placeholder != "" || f.Help != "" || f.Min != nil || f.Max != nil || f.MaxLength != 0 || f.Rows != 0 {
					return errors.New("详情字段仅支持key和label")
				}
				continue
			}
			if f.Type != "text" && f.Type != "textarea" && f.Type != "date" && f.Type != "radio" && f.Type != "number" && f.Type != "select" && f.Type != "boolean" {
				return errors.New("表单字段类型不受支持")
			}
			if !text(f.Placeholder, 120, false) || !text(f.Help, 300, false) {
				return errors.New("输入提示或帮助说明过长")
			}
			if (f.Min != nil || f.Max != nil) && f.Type != "number" {
				return errors.New("min/max仅用于数字字段")
			}
			for _, value := range []*float64{f.Min, f.Max} {
				if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
					return errors.New("数字范围必须有限")
				}
			}
			if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
				return errors.New("min不能大于max")
			}
			if f.MaxLength < 0 || f.MaxLength > 2000 || (f.MaxLength != 0 && f.Type != "text" && f.Type != "textarea") {
				return errors.New("max_length仅用于文本字段，范围1至2000")
			}
			if f.Rows != 0 && (f.Type != "textarea" || f.Rows < 2 || f.Rows > 8) {
				return errors.New("rows仅用于多行文本，范围2至8")
			}
			if f.Type == "select" || f.Type == "radio" {
				if len(f.Options) < 1 || len(f.Options) > 100 {
					return errors.New("下拉选项必须为1到100项")
				}
				if f.Type == "radio" && len(f.Options) > 10 {
					return errors.New("单选选项最多10项")
				}
				seen := map[string]bool{}
				for _, o := range f.Options {
					if !text(o.Label, 120, true) {
						return errors.New("选项标签无效")
					}
					switch v := o.Value.(type) {
					case string:
						if !text(v, 2000, false) {
							return errors.New("选项值过长")
						}
					case float64:
					default:
						return errors.New("选项值仅支持字符串或数字")
					}
					b, _ := json.Marshal(o.Value)
					if seen[string(b)] {
						return errors.New("选项值重复")
					}
					seen[string(b)] = true
				}
			} else if len(f.Options) > 0 {
				return errors.New("只有下拉字段可以设置选项")
			}
		}
	default:
		return errors.New("页面类型不在组件白名单中")
	}
	return nil
}

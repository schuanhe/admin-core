package protocol

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	webcontract "workstation/web"
)

// Block只描述中心内置组件，没有模板、脚本或任意组件加载入口。
type Block struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	ClassName string            `json:"class_name,omitempty"`
	Style     map[string]string `json:"style,omitempty"`
	Title     string            `json:"title,omitempty"`
	Text      string            `json:"text,omitempty"`
	Value     any               `json:"value,omitempty"`
	ValueKey  string            `json:"value_key,omitempty"`
	Unit      string            `json:"unit,omitempty"`
	Tone      string            `json:"tone,omitempty"`
	Columns   int               `json:"columns,omitempty"`
	Gap       *int              `json:"gap,omitempty"`
	Children  []Block           `json:"children,omitempty"`
	Fields    []Column          `json:"fields,omitempty"`
	Items     []string          `json:"items,omitempty"`
	PageID    string            `json:"page_id,omitempty"`
}

var blockFields = func() map[string]string {
	result := map[string]string{}
	for kind, entry := range webcontract.Catalog.Components {
		keys := []string{}
		for key := range entry.Fields {
			keys = append(keys, key)
		}
		result[kind] = strings.Join(keys, " ")
	}
	return result
}()

func (b Block) MarshalJSON() ([]byte, error) {
	type plain Block
	data, err := json.Marshal(plain(b))
	if err != nil || b.Type != "list" {
		return data, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	fields["items"], err = json.Marshal(b.Items)
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func (b *Block) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return errors.New("组件必须是对象")
	}
	var kind string
	if json.Unmarshal(raw["type"], &kind) != nil {
		return errors.New("组件类型无效")
	}
	fields, exists := blockFields[kind]
	if !exists {
		return errors.New("组件不在白名单中")
	}
	allowed := " " + fields + " "
	for key := range webcontract.Catalog.BlockCommon {
		allowed += key + " "
	}
	for key := range raw {
		if !strings.Contains(allowed, " "+key+" ") {
			return errors.New("组件包含不支持的字段")
		}
	}
	type plain Block
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*b = Block(decoded)
	return nil
}

var classToken = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)
var cssProperty = regexp.MustCompile(`^(?:[a-z][a-z-]{0,47}|--service-[a-z][a-z0-9-]{0,31})$`)

// 后端只持久化惰性CSS数据；浏览器在挂载前执行AST规则校验和作用域转换。
func validatePresentation(classes string, style map[string]string) error {
	if len(classes) > 128 || len(strings.Fields(classes)) > 4 || len(style) > webcontract.Catalog.Limits["style_properties"] {
		return errors.New("类名或声明式样式超过限制")
	}
	for _, name := range strings.Fields(classes) {
		if !classToken.MatchString(name) {
			return errors.New("类名只允许ASCII字母、数字、横线与下划线")
		}
	}
	for key, value := range style {
		if !cssProperty.MatchString(key) || !text(value, 256, true) {
			return errors.New("样式属性名或值无效")
		}
	}
	return nil
}

func validateBlocks(blocks []Block) error {
	count := 0
	seen := map[string]bool{}
	var visit func([]Block, int, string) error
	visit = func(items []Block, depth int, parent string) error {
		if len(items) > webcontract.Catalog.Limits["block_children"] || depth > webcontract.Catalog.Limits["block_depth"] {
			return errors.New("每组组件最多20项，组件最多嵌套4层")
		}
		for _, b := range items {
			count++
			if count > webcontract.Catalog.Limits["blocks"] || !ValidID(b.ID) || seen[b.ID] {
				return errors.New("页面最多80组件，组件id必须有效且唯一")
			}
			seen[b.ID] = true
			if _, ok := blockFields[b.Type]; !ok {
				return errors.New("组件类型不受支持")
			}
			if err := validatePresentation(b.ClassName, b.Style); err != nil {
				return err
			}
			if !text(b.Title, 120, false) || !text(b.Text, 2000, false) || !text(b.Unit, 20, false) || (b.ValueKey != "" && !ValidID(b.ValueKey)) {
				return errors.New("组件文本或数据绑定无效")
			}
			if b.Gap != nil && (*b.Gap < 0 || *b.Gap > webcontract.Catalog.Limits["gap"]) {
				return errors.New("组件间距范围0至48像素")
			}
			switch b.Type {
			case "stack", "grid", "card", "tab", "tabs":
				if len(b.Children) == 0 {
					return errors.New("容器组件需要children")
				}
				if b.Type == "grid" && (b.Columns < 0 || b.Columns > webcontract.Catalog.Limits["columns"]) {
					return errors.New("栅格列数范围1至4")
				}
				if b.Type == "tab" && (parent != "tabs" || !text(b.Title, 120, true)) {
					return errors.New("tab必须放在tabs内并提供标题")
				}
				if b.Type == "tabs" {
					for _, child := range b.Children {
						if child.Type != "tab" {
							return errors.New("tabs只接受tab子项")
						}
					}
					if len(b.Children) > webcontract.Catalog.Limits["tabs"] {
						return errors.New("页签最多8项")
					}
				}
				if err := visit(b.Children, depth+1, b.Type); err != nil {
					return err
				}
			case "text":
				if b.Text == "" && b.ValueKey == "" {
					return errors.New("文本组件需要text或value_key")
				}
			case "alert":
				if !text(b.Text, 2000, true) || (b.Tone != "" && b.Tone != "info" && b.Tone != "success" && b.Tone != "warning" && b.Tone != "error") {
					return errors.New("提示组件文本或tone无效")
				}
			case "statistic", "progress":
				if b.Type == "statistic" && !text(b.Title, 120, true) {
					return errors.New("统计组件需要title")
				}
				if b.Value == nil && b.ValueKey == "" {
					return errors.New("组件需要value或value_key")
				}
				if b.Value != nil {
					switch value := b.Value.(type) {
					case string:
						if b.Type == "progress" || !text(value, 120, false) {
							return errors.New("统计值无效")
						}
					case float64:
						if math.IsNaN(value) || math.IsInf(value, 0) || (b.Type == "progress" && (value < 0 || value > 100)) {
							return errors.New("进度须为0至100的有限数字")
						}
					default:
						return errors.New("统计值必须为文本或数字")
					}
				}
			case "descriptions":
				if len(b.Fields) < 1 || len(b.Fields) > 30 || b.Columns < 0 || b.Columns > webcontract.Catalog.Limits["columns"] {
					return errors.New("描述列表字段或列数无效")
				}
				keys := map[string]bool{}
				for _, f := range b.Fields {
					if !ValidID(f.Key) || keys[f.Key] || !text(f.Label, 120, true) {
						return errors.New("描述列表字段无效或重复")
					}
					keys[f.Key] = true
				}
			case "list":
				if b.Items == nil || len(b.Items) > 30 {
					return errors.New("列表items必须为至多30项的数组")
				}
				for _, item := range b.Items {
					if !text(item, 2000, true) {
						return errors.New("列表项文本无效")
					}
				}
			case "page", "page-link":
				if !ValidID(b.PageID) || (b.Type == "page-link" && !text(b.Title, 120, true)) {
					return errors.New("页面引用或链接标题无效")
				}
			}
		}
		return nil
	}
	if len(blocks) == 0 {
		return errors.New("组合页面需要blocks")
	}
	return visit(blocks, 1, "")
}

func validateBlockReferences(blocks []Block, pages []Page) error {
	for _, block := range blocks {
		if block.Type == "page" || block.Type == "page-link" {
			found := false
			for _, page := range pages {
				if page.ID == block.PageID {
					found = true
					if block.Type == "page" && page.Type == "compose" {
						return errors.New("嵌入页面只允许table/form/detail，避免递归引用")
					}
				}
			}
			if !found {
				return errors.New("组件引用的本服务页面不存在")
			}
		}
		if err := validateBlockReferences(block.Children, pages); err != nil {
			return err
		}
	}
	return nil
}

func blocksNeedSource(blocks []Block) bool {
	for _, b := range blocks {
		if b.ValueKey != "" || b.Type == "descriptions" || blocksNeedSource(b.Children) {
			return true
		}
	}
	return false
}

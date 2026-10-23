// Package webcontract 提供Go与Vue共用的组件注册表，不依赖前端构建产物。
package webcontract

import (
	_ "embed"
	"encoding/json"
)

//go:embed src/dynamic/catalog.json
var CatalogJSON []byte

//go:embed src/dynamic/example.json
var ExampleDefinition json.RawMessage

type Entry struct {
	Summary string            `json:"summary"`
	Fields  map[string]string `json:"fields"`
}
type UICatalog struct {
	Revision    int                        `json:"revision"`
	Limits      map[string]int             `json:"limits"`
	PageCommon  map[string]string          `json:"page_common"`
	PageTypes   map[string]Entry           `json:"page_types"`
	BlockCommon map[string]string          `json:"block_common"`
	Components  map[string]Entry           `json:"components"`
	FormFields  map[string]string          `json:"form_fields"`
	FormTypes   map[string]string          `json:"form_types"`
	CSS         map[string]json.RawMessage `json:"css"`
	Icons       map[string]string          `json:"icons"`
}

// 注册表是校验器与CSS编译器的输入，也是手册的数据源，不再维护平行的文档名单。
var Catalog = func() UICatalog {
	var value UICatalog
	if err := json.Unmarshal(CatalogJSON, &value); err != nil {
		panic("内置界面协议注册表无效")
	}
	return value
}()

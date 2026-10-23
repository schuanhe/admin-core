package httpapi

import (
	"encoding/json"
	"net/http"
	"workstation/internal/protocol"
)

// 启动时生成只读通用资料；每次请求实时检查开关，不缓存授权结果。
var handbookMarkdown = protocol.HandbookMarkdown()
var handbookJSON = func() []byte {
	contract := protocol.HandbookContract()
	contract["manual_markdown"] = handbookMarkdown
	data, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		panic("内置接入手册生成失败")
	}
	return data
}()

func (s *Server) handbook(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.Store.HandbookEnabled(r.Context())
	if err != nil {
		s.dbError(w, err)
		return
	}
	if !enabled {
		fail(w, 404, "not_found", "接入手册接口未开放")
		return
	}
	format := r.URL.Query().Get("format")
	if (format != "" && format != "markdown" && format != "json") || len(r.URL.Query()) > 1 || (len(r.URL.Query()) == 1 && len(r.URL.Query()["format"]) != 1) {
		fail(w, 400, "invalid_input", "format仅支持markdown或json")
		return
	}
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	if format == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(handbookJSON)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(handbookMarkdown))
}

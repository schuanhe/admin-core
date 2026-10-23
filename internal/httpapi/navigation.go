package httpapi

import (
	"net/http"
	"workstation/internal/protocol"
	"workstation/internal/store"
)

func (s *Server) updateNavigation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Items []store.NavigationItem `json:"items"`
	}
	if _, err := decode(w, r, 64*1024, &input); err != nil {
		fail(w, 400, "invalid_input", err.Error())
		return
	}
	if len(input.Items) == 0 || len(input.Items) > 500 {
		fail(w, 400, "invalid_input", "一次可排序1至500个服务")
		return
	}
	seen := map[string]bool{}
	for _, item := range input.Items {
		if !protocol.ValidServiceID(item.ServiceID) || seen[item.ServiceID] {
			fail(w, 400, "invalid_input", "服务标识无效或重复")
			return
		}
		seen[item.ServiceID] = true
	}
	if err := s.Store.UpdateNavigation(r.Context(), input.Items, current(r).username); err != nil {
		s.dbError(w, err)
		return
	}
	write(w, 200, map[string]bool{"ok": true})
}

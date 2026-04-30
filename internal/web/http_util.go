package web

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// APIError 是 API 错误响应体。
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIErrorResp 是统一错误外层结构：{"error": {...}}。
type APIErrorResp struct {
	Error APIError `json:"error"`
}

// writeJSON 写入 JSON 响应，内部错误仅记日志。
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[web] 写入 JSON 响应失败: %v", err)
	}
}

// writeError 写入统一错误响应。
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, APIErrorResp{Error: APIError{Code: code, Message: message}})
}

// readJSON 解析请求体到 v。
func readJSON(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// methodNotAllowed 返回 405。
func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "不支持的请求方法")
}

// wsFromPath 解析 /api/workspaces/{id}/xxx 形式中的 id 与剩余路径。
// 返回 (id, rest, ok)。
//
// 例：/api/workspaces/abc/status  -> ("abc", "status", true)
// 例：/api/workspaces/abc/branches/switch -> ("abc", "branches/switch", true)
func wsFromPath(path string) (id, rest string, ok bool) {
	const prefix = "/api/workspaces/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	tail := strings.TrimPrefix(path, prefix)
	if tail == "" {
		return "", "", false
	}
	slash := strings.Index(tail, "/")
	if slash < 0 {
		return tail, "", true
	}
	return tail[:slash], tail[slash+1:], true
}

// resolveWorkspace 根据请求路径解析 workspace，若不存在则直接写 404 并返回 nil。
func (s *Server) resolveWorkspace(w http.ResponseWriter, r *http.Request) *Workspace {
	id, _, ok := wsFromPath(r.URL.Path)
	if !ok || id == "" {
		writeError(w, http.StatusBadRequest, "invalid_path", "URL 中缺少 workspace id")
		return nil
	}
	if s.workspaces == nil {
		writeError(w, http.StatusInternalServerError, "no_manager", "Workspace 管理器未初始化")
		return nil
	}
	ws := s.workspaces.Get(id)
	if ws == nil {
		writeError(w, http.StatusNotFound, "workspace_not_found", "workspace 不存在或已关闭: "+id)
		return nil
	}
	return ws
}

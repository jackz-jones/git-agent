package web

import (
	"net/http"
)

// handleListWorkspaces 对应 GET /api/workspaces。
func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	if s.workspaces == nil {
		writeJSON(w, http.StatusOK, []WorkspaceInfo{})
		return
	}
	writeJSON(w, http.StatusOK, s.workspaces.List())
}

// handleOpenWorkspace 对应 POST /api/workspaces，body: {"path": "..."}。
func (s *Server) handleOpenWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "请求体解析失败: "+err.Error())
		return
	}
	if s.workspaces == nil {
		writeError(w, http.StatusInternalServerError, "no_manager", "Workspace 管理器未初始化")
		return
	}

	ws, reused, err := s.workspaces.Open(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "open_failed", err.Error())
		return
	}

	resp := struct {
		Workspace WorkspaceInfo `json:"workspace"`
		Reused    bool          `json:"reused"`
	}{
		Workspace: toInfo(ws),
		Reused:    reused,
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetWorkspace 对应 GET /api/workspaces/{id}。
func (s *Server) handleGetWorkspace(w http.ResponseWriter, r *http.Request, id string) {
	if s.workspaces == nil {
		writeError(w, http.StatusInternalServerError, "no_manager", "Workspace 管理器未初始化")
		return
	}
	ws := s.workspaces.Get(id)
	if ws == nil {
		writeError(w, http.StatusNotFound, "workspace_not_found", "workspace 不存在: "+id)
		return
	}
	writeJSON(w, http.StatusOK, toInfo(ws))
}

// handleCloseWorkspace 对应 DELETE /api/workspaces/{id}。
func (s *Server) handleCloseWorkspace(w http.ResponseWriter, r *http.Request, id string) {
	if s.workspaces == nil {
		writeError(w, http.StatusInternalServerError, "no_manager", "Workspace 管理器未初始化")
		return
	}
	if err := s.workspaces.Close(id); err != nil {
		writeError(w, http.StatusNotFound, "workspace_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRecent 对应 GET /api/recent。
func (s *Server) handleRecent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.recent == nil {
		writeJSON(w, http.StatusOK, []RecentEntry{})
		return
	}
	writeJSON(w, http.StatusOK, s.recent.List())
}

package web

import (
	"net/http"
	"strings"
)

// registerRoutesExtra 注册所有 /api/* 的具体路由。
// 由 registerRoutes() 调用；之所以单独拆分，是为了按 handler 文件分组。
func (s *Server) registerRoutesExtra() {
	// 列表 & 打开 workspace
	s.mux.HandleFunc("/api/workspaces", s.handleWorkspacesRoot)
	// 所有 /api/workspaces/{id}/... 共用一个分发入口
	s.mux.HandleFunc("/api/workspaces/", s.handleWorkspacesSub)

	// 目录浏览（用于目录选择器）
	s.mux.HandleFunc("/api/browse", s.handleBrowse)

	// 最近打开
	s.mux.HandleFunc("/api/recent", s.handleRecent)

	// 设置
	s.mux.HandleFunc("/api/settings", s.handleSettings)
	s.mux.HandleFunc("/api/settings/test-llm", s.handleTestLLM)
}

// handleWorkspacesRoot 对应 /api/workspaces（无 id）。
// GET  -> 列出已打开 workspace
// POST -> 打开/创建 workspace
func (s *Server) handleWorkspacesRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListWorkspaces(w, r)
	case http.MethodPost:
		s.handleOpenWorkspace(w, r)
	default:
		methodNotAllowed(w)
	}
}

// handleWorkspacesSub 分发 /api/workspaces/{id}/{sub...}。
// 根据尾部子路径路由到各具体 handler。
func (s *Server) handleWorkspacesSub(w http.ResponseWriter, r *http.Request) {
	id, rest, ok := wsFromPath(r.URL.Path)
	if !ok || id == "" {
		writeError(w, http.StatusBadRequest, "invalid_path", "URL 中缺少 workspace id")
		return
	}

	// 无子路径：DELETE 关闭 workspace
	if rest == "" {
		switch r.Method {
		case http.MethodDelete:
			s.handleCloseWorkspace(w, r, id)
		case http.MethodGet:
			s.handleGetWorkspace(w, r, id)
		default:
			methodNotAllowed(w)
		}
		return
	}

	// 规整子路径（去掉可能的尾部斜杠）
	rest = strings.TrimSuffix(rest, "/")

	switch rest {
	case "status":
		s.handleStatus(w, r)
	case "tree":
		s.handleTree(w, r)
	case "file":
		s.handleFile(w, r)
	case "log":
		s.handleLog(w, r)
	case "diff":
		s.handleDiff(w, r)
	case "branches":
		s.handleBranches(w, r)
	case "branches/switch":
		s.handleBranchSwitch(w, r)
	case "commit":
		s.handleCommit(w, r)
	case "restore":
		s.handleRestore(w, r)
	case "push":
		s.handlePush(w, r)
	case "init":
		s.handleInitRepo(w, r)
	case "agent":
		s.handleAgentWS(w, r)
	default:
		writeError(w, http.StatusNotFound, "not_found", "未知的子路径: "+rest)
	}
}

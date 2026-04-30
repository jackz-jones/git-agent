package web

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// handleStatus 对应 GET /api/workspaces/{id}/status。
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	ws := s.resolveWorkspace(w, r)
	if ws == nil {
		return
	}
	gw := ws.Agent.GetGitWrapper()
	if gw == nil {
		writeError(w, http.StatusInternalServerError, "no_git", "GitWrapper 未初始化")
		return
	}
	if !gw.IsInitialized() {
		// 未初始化时不报错，返回显式标记
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"initialized": false,
		})
		return
	}
	info, err := gw.Status()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "status_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"initialized": true,
		"status":      info,
	})
}

// TreeNode 是文件树节点。
type TreeNode struct {
	Name    string `json:"name"`
	Path    string `json:"path"` // 相对 workspace 根的 / 分隔路径
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size,omitempty"`
	HasMore bool   `json:"hasMore,omitempty"` // 是否还有未列出的子节点（目录懒加载截断）
}

// handleTree 对应 GET /api/workspaces/{id}/tree?path=...&depth=1。
// 懒加载：默认仅返回指定目录的直接子项，避免一次性遍历超大目录。
func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	ws := s.resolveWorkspace(w, r)
	if ws == nil {
		return
	}

	rel := r.URL.Query().Get("path")
	abs, err := safeJoin(ws.Path, rel)
	if err != nil {
		writeError(w, http.StatusForbidden, "path_denied", err.Error())
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, "path_not_found", err.Error())
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "not_dir", "path 必须指向目录")
		return
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_dir_failed", err.Error())
		return
	}

	const maxEntries = 500 // 单目录下最多返回 500 个节点，防御超大目录
	nodes := make([]TreeNode, 0, len(entries))
	hasMore := false
	for i, e := range entries {
		name := e.Name()
		// 跳过 .git 目录（用户不关心）
		if name == ".git" {
			continue
		}
		if i >= maxEntries {
			hasMore = true
			break
		}
		child := filepath.Join(abs, name)
		ci, err := os.Stat(child)
		if err != nil {
			continue
		}
		n := TreeNode{
			Name:  name,
			Path:  filepath.ToSlash(filepath.Join(rel, name)),
			IsDir: ci.IsDir(),
		}
		if !ci.IsDir() {
			n.Size = ci.Size()
		}
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":    filepath.ToSlash(rel),
		"nodes":   nodes,
		"hasMore": hasMore,
	})
}

// handleFile 对应 GET /api/workspaces/{id}/file?path=...
// 返回文件的当前内容（文本）。超大文件或二进制文件会被截断并标记。
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	ws := s.resolveWorkspace(w, r)
	if ws == nil {
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "missing_path", "缺少 path 查询参数")
		return
	}
	abs, err := safeJoin(ws.Path, rel)
	if err != nil {
		writeError(w, http.StatusForbidden, "path_denied", err.Error())
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		writeError(w, http.StatusNotFound, "file_not_found", err.Error())
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "is_dir", "path 指向目录而不是文件")
		return
	}

	const maxBytes = 1024 * 1024 // 1MB
	f, err := os.Open(abs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "open_failed", err.Error())
		return
	}
	defer f.Close()

	limited := io.LimitReader(f, maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}
	truncated := int64(len(raw)) > maxBytes
	if truncated {
		raw = raw[:maxBytes]
	}
	// 粗略判断二进制：前 512B 出现 NUL
	isBinary := false
	probe := raw
	if len(probe) > 512 {
		probe = probe[:512]
	}
	for _, b := range probe {
		if b == 0 {
			isBinary = true
			break
		}
	}

	if isBinary {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"path":     filepath.ToSlash(rel),
			"size":     info.Size(),
			"binary":   true,
			"content":  "",
			"truncate": truncated,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":     filepath.ToSlash(rel),
		"size":     info.Size(),
		"binary":   false,
		"content":  string(raw),
		"truncate": truncated,
	})
}

// handleLog 对应 GET /api/workspaces/{id}/log?limit=50&author=...
func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	ws := s.resolveWorkspace(w, r)
	if ws == nil {
		return
	}
	gw := ws.Agent.GetGitWrapper()
	if gw == nil || !gw.IsInitialized() {
		writeError(w, http.StatusBadRequest, "not_initialized", "仓库未初始化")
		return
	}

	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	author := q.Get("author")
	filePath := q.Get("path")

	var versions []interface{}
	if filePath != "" {
		list, err := gw.GetHistory(filePath, limit, author)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "log_failed", err.Error())
			return
		}
		for _, v := range list {
			versions = append(versions, v)
		}
	} else {
		list, err := gw.Log(limit, author)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "log_failed", err.Error())
			return
		}
		for _, v := range list {
			versions = append(versions, v)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"versions": versions,
	})
}

// handleDiff 对应 GET /api/workspaces/{id}/diff?path=...&commit=...
// - commit 不为空：返回该 commit 的全量 diff（CommitDiff）
// - 否则返回指定文件相对 HEAD 的 diff
func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	ws := s.resolveWorkspace(w, r)
	if ws == nil {
		return
	}
	gw := ws.Agent.GetGitWrapper()
	if gw == nil || !gw.IsInitialized() {
		writeError(w, http.StatusBadRequest, "not_initialized", "仓库未初始化")
		return
	}

	q := r.URL.Query()
	commit := q.Get("commit")
	if commit != "" {
		diff, err := gw.CommitDiff(commit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "diff_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"diff": diff})
		return
	}

	filePath := q.Get("path")
	diff, err := gw.Diff(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "diff_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"diff": diff})
}

// handleBranches 对应 GET /api/workspaces/{id}/branches 与 POST（新建）
func (s *Server) handleBranches(w http.ResponseWriter, r *http.Request) {
	ws := s.resolveWorkspace(w, r)
	if ws == nil {
		return
	}
	gw := ws.Agent.GetGitWrapper()
	if gw == nil || !gw.IsInitialized() {
		writeError(w, http.StatusBadRequest, "not_initialized", "仓库未初始化")
		return
	}

	switch r.Method {
	case http.MethodGet:
		list, err := gw.ListBranches()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "branches_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"branches": list})
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "missing_name", "缺少分支名")
			return
		}
		if err := gw.CreateBranch(req.Name); err != nil {
			writeError(w, http.StatusInternalServerError, "create_branch_failed", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		methodNotAllowed(w)
	}
}

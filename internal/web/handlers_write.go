package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// defaultGitignore 是自动生成的最小 .gitignore 模板。
const defaultGitignore = `# Git Agent default .gitignore
# 你可以按需增删此文件

.DS_Store
Thumbs.db
*.log
`

// handleInitRepo 对应 POST /api/workspaces/{id}/init。
// 执行 git init，并生成默认 .gitignore（不自动提交）。
func (s *Server) handleInitRepo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
	if gw.IsInitialized() {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      true,
			"message": "仓库已初始化",
		})
		return
	}

	// 读取用户信息：有配置才会创建初始 commit，否则仅 git init
	uc := ws.Agent.GetUserConfig()
	var authorName, authorEmail string
	if uc != nil {
		authorName = uc.Name
		authorEmail = uc.Email
	}

	if err := gw.InitRepo(authorName, authorEmail); err != nil {
		s.audit.Log("init", ws, false, nil, err)
		writeError(w, http.StatusInternalServerError, "init_failed", err.Error())
		return
	}

	// 生成默认 .gitignore（仅当不存在时）
	gitignorePath := filepath.Join(ws.Path, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		_ = os.WriteFile(gitignorePath, []byte(defaultGitignore), 0o644)
	}

	// 如果没配置用户信息，提示一下
	missingUser := authorName == "" || authorEmail == ""

	s.audit.Log("init", ws, true, map[string]interface{}{
		"gitignore": true,
	}, nil)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":             true,
		"gitignoreAdded": true,
		"needUserConfig": missingUser,
	})
}

// commitReq 是保存版本请求体。
type commitReq struct {
	Message string   `json:"message"`         // 必填，提交描述
	Files   []string `json:"files,omitempty"` // 空数组表示全部变更
}

// handleCommit 对应 POST /api/workspaces/{id}/commit。
func (s *Server) handleCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
	var req commitReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "missing_message", "提交描述不能为空")
		return
	}

	uc := ws.Agent.GetUserConfig()
	if uc == nil || uc.Name == "" || uc.Email == "" {
		writeError(w, http.StatusBadRequest, "user_not_configured",
			"请先在设置中填写姓名与邮箱，否则无法保存版本")
		return
	}

	hash, err := gw.SaveVersion(req.Message, req.Files, uc.Name, uc.Email)
	if err != nil {
		s.audit.Log("commit", ws, false, nil, err)
		writeError(w, http.StatusInternalServerError, "commit_failed", err.Error())
		return
	}
	s.audit.Log("commit", ws, true, map[string]interface{}{
		"hash":  hash,
		"files": req.Files,
	}, nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"hash":      hash,
		"shortHash": shortHash(hash),
	})
}

// restoreReq 是回滚请求体。
type restoreReq struct {
	Version string `json:"version"`        // commit hash
	Path    string `json:"path,omitempty"` // 指定文件回滚；为空则整仓库回滚
	Force   bool   `json:"force"`          // 若存在未提交修改，需要 true 才执行
}

// handleRestore 对应 POST /api/workspaces/{id}/restore。
func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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

	var req restoreReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if strings.TrimSpace(req.Version) == "" {
		writeError(w, http.StatusBadRequest, "missing_version", "缺少要回滚的版本号")
		return
	}

	// 若未传 force，且有未提交修改 -> 要求二次确认
	if !req.Force {
		if st, err := gw.Status(); err == nil && !st.IsClean {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error": APIError{
					Code:    "dirty_working_tree",
					Message: "当前存在未提交修改，继续回滚将丢失这些修改；确认请传 force=true",
				},
				"requiresConfirm": true,
			})
			return
		}
	}

	var err error
	if req.Path != "" {
		err = gw.RestoreFile(req.Path, req.Version)
	} else {
		err = gw.RestoreVersion(req.Version)
	}
	if err != nil {
		s.audit.Log("restore", ws, false, map[string]interface{}{
			"version": req.Version,
			"path":    req.Path,
		}, err)
		writeError(w, http.StatusInternalServerError, "restore_failed", err.Error())
		return
	}
	s.audit.Log("restore", ws, true, map[string]interface{}{
		"version": req.Version,
		"path":    req.Path,
	}, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// pushReq 是推送请求体。
type pushReq struct {
	Remote   string `json:"remote,omitempty"`   // 默认 origin
	Username string `json:"username,omitempty"` // 可选，覆盖配置
	Password string `json:"password,omitempty"` // 可选，覆盖配置
}

// handlePush 对应 POST /api/workspaces/{id}/push。
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
	var req pushReq
	_ = readJSON(r, &req) // 允许空 body
	if req.Remote == "" {
		req.Remote = "origin"
	}

	// 合并认证来源：请求体 > 配置文件 > 环境变量
	username, password := req.Username, req.Password
	if s.config != nil {
		snap := s.config.Snapshot()
		if username == "" {
			username = snap.HTTP.Username
		}
		if password == "" {
			password = snap.HTTP.Password
		}
	}
	if username == "" {
		username = os.Getenv("GIT_HTTP_USERNAME")
	}
	if password == "" {
		password = os.Getenv("GIT_HTTP_PASSWORD")
	}

	var err error
	if username != "" || password != "" {
		err = gw.PushWithAuth(req.Remote, username, password)
	} else {
		err = gw.Push(req.Remote)
	}

	if err != nil {
		needSync := needsSync(err.Error())
		s.audit.Log("push", ws, false, map[string]interface{}{
			"remote":   req.Remote,
			"needSync": needSync,
		}, err)
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error": APIError{
				Code:    "push_failed",
				Message: err.Error(),
			},
			"needSync": needSync,
		})
		return
	}
	s.audit.Log("push", ws, true, map[string]interface{}{
		"remote": req.Remote,
	}, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// needsSync 判断错误信息是否表示"需要先 pull 再 push"。
func needsSync(msg string) bool {
	lower := strings.ToLower(msg)
	keys := []string{
		"non-fast-forward", "non fast forward",
		"fetch first", "rejected",
		"updates were rejected",
	}
	for _, k := range keys {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

// shortHash 返回 commit 短哈希（前 7 位）。
func shortHash(h string) string {
	if len(h) <= 7 {
		return h
	}
	return h[:7]
}

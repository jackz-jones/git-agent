package web

import (
	"net/http"
)

// switchBranchReq 是分支切换请求体。
type switchBranchReq struct {
	Name  string `json:"name"`  // 目标分支
	Force bool   `json:"force"` // 若存在未提交修改，需要 true 才执行
}

// handleBranchSwitch 对应 POST /api/workspaces/{id}/branches/switch。
func (s *Server) handleBranchSwitch(w http.ResponseWriter, r *http.Request) {
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

	var req switchBranchReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "缺少目标分支名")
		return
	}

	// 若未传 force，且有未提交修改 -> 要求二次确认
	if !req.Force {
		if st, err := gw.Status(); err == nil && !st.IsClean {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error": APIError{
					Code:    "dirty_working_tree",
					Message: "当前存在未提交修改，继续切换分支可能丢失或冲突；确认请传 force=true",
				},
				"requiresConfirm": true,
			})
			return
		}
	}

	if err := gw.SwitchBranch(req.Name); err != nil {
		s.audit.Log("branch_switch", ws, false, map[string]interface{}{
			"target": req.Name,
		}, err)
		writeError(w, http.StatusInternalServerError, "switch_failed", err.Error())
		return
	}
	s.audit.Log("branch_switch", ws, true, map[string]interface{}{
		"target": req.Name,
	}, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

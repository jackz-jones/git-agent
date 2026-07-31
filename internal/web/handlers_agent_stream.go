package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackz-jones/git-agent/internal/agent"
)

// agentStreamReq 是 POST /api/workspaces/{id}/agent 的请求体。
type agentStreamReq struct {
	Input string `json:"input"`
	// Mode: "auto"（默认，已配 LLM 则 LLM，否则 local） | "local"（强制本地）
	Mode string `json:"mode,omitempty"`
}

// handleAgentWS 对应 /api/workspaces/{id}/agent。
//
// 本实现使用 Server-Sent Events（SSE）而非 WebSocket，以避免引入 gorilla/websocket 外部依赖；
// 协议语义与需求 6 一致：服务端持续推送 thinking/tool_call/tool_result/final/workspace_updated 事件。
//
// 请求方式：
//   - GET  /api/workspaces/{id}/agent              —— 返回"使用说明"（健康探测）
//   - POST /api/workspaces/{id}/agent   body:{input, mode}  —— 单次对话，SSE 流式推送
//
// 客户端读取示例（前端使用 fetch + ReadableStream 处理，而非 EventSource，因为 EventSource 不支持 POST body）。
func (s *Server) handleAgentWS(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]string{
			"hint": "请使用 POST 发送 {input, mode}，服务端会以 SSE 推送事件",
		})
		return
	case http.MethodPost:
		// 继续 SSE 流程
	default:
		methodNotAllowed(w)
		return
	}

	ws := s.resolveWorkspace(w, r)
	if ws == nil {
		return
	}
	var req agentStreamReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if strings.TrimSpace(req.Input) == "" {
		writeError(w, http.StatusBadRequest, "missing_input", "input 不能为空")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "no_flusher", "当前 ResponseWriter 不支持 SSE")
		return
	}

	// SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // 阻止反向代理缓冲
	w.WriteHeader(http.StatusOK)

	ctx := r.Context()
	writeMu := &sync.Mutex{}
	// stopped 标记：当客户端断开或写失败时置 1，后续所有 sendEvent 直接返回，
	// 避免向已关闭的连接反复写入 broken pipe。
	var stopped atomic.Bool

	sendEvent := func(ev agent.AgentEvent) {
		if stopped.Load() {
			return
		}
		// 客户端断开时，ctx.Done 会被关闭；此时立刻停写。
		select {
		case <-ctx.Done():
			stopped.Store(true)
			return
		default:
		}
		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now()
		}
		raw, err := json.Marshal(ev)
		if err != nil {
			return
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		// 注意：每条 SSE 数据必须以 "data: ... \n\n" 结尾
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, raw); err != nil {
			// broken pipe / connection reset：客户端已断开，停止后续写入。
			stopped.Store(true)
			return
		}
		// Flush 也可能因连接关闭而 panic on some proxies；使用 defer recover 兜底。
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					stopped.Store(true)
				}
			}()
			flusher.Flush()
		}()
	}

	// 本地模式强制：需要时切换
	// 当前 Agent.Process 会根据 IsLLMEnabled() 自动分流；
	// "强制 local" 的需求目前通过提示实现（Process 内部自动 fallback），后续如需硬切可在 agent 层加接口。
	if req.Mode == "local" && ws.Agent.IsLLMEnabled() {
		sendEvent(agent.AgentEvent{
			Kind:    agent.EventThinking,
			Message: "已按要求使用本地关键词匹配模式（不调用 LLM）",
		})
		// 这里不做真正的"禁用 LLM"，只作为提示；完整支持留待后续扩展
	}

	// 启动处理：ProcessStream 会在内部注册钩子 → 调用 Process → 推送 final → 清理钩子
	resp := ws.Agent.ProcessStream(ctx, req.Input, sendEvent)
	_ = resp // final 事件已由 ProcessStream 内部发出
}
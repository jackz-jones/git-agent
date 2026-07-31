package agent

import (
	"context"
	"sync"
	"time"
)

// AgentEventKind 标识事件类别，对应需求 6.2 中的 thinking / tool_call / tool_result / final / workspace_updated。
type AgentEventKind string

const (
	EventThinking         AgentEventKind = "thinking"
	EventToolCall         AgentEventKind = "tool_call"
	EventToolResult       AgentEventKind = "tool_result"
	EventStateChanged     AgentEventKind = "state_changed"
	EventFinal            AgentEventKind = "final"
	EventWorkspaceUpdated AgentEventKind = "workspace_updated"
	EventError            AgentEventKind = "error"
)

// AgentEvent 是 Agent 向外界广播的一次事件快照。
type AgentEvent struct {
	Kind      AgentEventKind `json:"kind"`
	Timestamp time.Time      `json:"timestamp"`

	// 通用字段
	Message string `json:"message,omitempty"`

	// 工具调用相关字段
	ToolName string                 `json:"toolName,omitempty"`
	Args     string                 `json:"args,omitempty"`   // JSON 字符串（由 LLM 生成）
	Result   string                 `json:"result,omitempty"` // 工具输出
	Extra    map[string]interface{} `json:"extra,omitempty"`

	// 状态变更事件
	State AgentState `json:"state,omitempty"`

	// final 事件携带完整响应
	Response *AgentResponse `json:"response,omitempty"`
}

// EventHook 是事件回调函数签名。
// 为避免 UI 推送阻塞 Agent，hook 内部应当快速返回（推荐异步/缓冲）。
type EventHook func(ev AgentEvent)

// eventHookHolder 存放 Agent 的事件钩子。
// 独立结构体封装是为了让锁更细粒度，不干扰主 Agent.mu。
type eventHookHolder struct {
	mu   sync.RWMutex
	hook EventHook
}

// set 设置钩子（nil 表示清除）。
func (h *eventHookHolder) set(fn EventHook) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hook = fn
}

// emit 触发钩子；hook 为 nil 或 panic 时静默。
func (h *eventHookHolder) emit(ev AgentEvent) {
	if h == nil {
		return
	}
	h.mu.RLock()
	fn := h.hook
	h.mu.RUnlock()
	if fn == nil {
		return
	}
	defer func() {
		// 钩子内部出错不应影响 Agent 主流程
		_ = recover()
	}()
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	fn(ev)
}

// SetEventHook 注册事件钩子。
// 传入 nil 可清除已注册的钩子。
// 本方法是为 Web 层"流式观察 Agent 执行过程"而新增，不影响 CLI 使用。
func (a *Agent) SetEventHook(hook EventHook) {
	a.ensureEvents()
	a.events.set(hook)
}

// ensureEvents 懒初始化事件钩子持有器。
// 之所以不放在 New() 里，是为了让该字段对 CLI 完全透明。
func (a *Agent) ensureEvents() {
	a.mu.Lock()
	if a.events == nil {
		a.events = &eventHookHolder{}
	}
	a.mu.Unlock()
}

// emit 是 agent 内部工具方法，用于广播事件；从 setState 等处调用。
func (a *Agent) emit(ev AgentEvent) {
	a.mu.RLock()
	h := a.events
	a.mu.RUnlock()
	if h == nil {
		return
	}
	h.emit(ev)
}

// emitToolCall 工具调用开始前触发。
// 需求 13.2：args 是 LLM 生成的 JSON 字符串，可能包含 password/token 等凭据，
// 在广播到 SSE 与日志前必须做字段级脱敏。
func (a *Agent) emitToolCall(name, args string) {
	a.emit(AgentEvent{
		Kind:     EventToolCall,
		ToolName: name,
		Args:     redactJSONArgs(args),
	})
}

// emitToolResult 工具调用返回后触发。
func (a *Agent) emitToolResult(name, result string) {
	a.emit(AgentEvent{
		Kind:     EventToolResult,
		ToolName: name,
		Result:   result,
	})
}

// EmitStateChanged 广播状态变更事件。
// 用于 Web 层在外部触发（如 ReloadLLMConfig）后，通知已建立的 SSE 客户端刷新状态。
// message 可用于携带触发原因（如 "reload"）。
func (a *Agent) EmitStateChanged(state AgentState, message string) {
	a.emit(AgentEvent{
		Kind:    EventStateChanged,
		State:   state,
		Message: message,
	})
}

// ProcessStream 以流式方式处理用户输入。
// 内部等价于：注册 hook → Process → 推送 final 事件 → 清理 hook。
// 调用方应确保 hook 快速返回（内部可做缓冲 channel 适配）。
func (a *Agent) ProcessStream(ctx context.Context, input string, hook EventHook) *AgentResponse {
	if hook != nil {
		a.SetEventHook(hook)
		defer a.SetEventHook(nil)
	}
	// 先广播一个 thinking 起点事件，便于 UI 立刻渲染"正在思考"
	a.emit(AgentEvent{Kind: EventThinking, Message: "开始处理请求"})

	resp := a.Process(ctx, input)

	// 最终响应通过 final 事件对外发出
	a.emit(AgentEvent{Kind: EventFinal, Response: resp})
	return resp
}

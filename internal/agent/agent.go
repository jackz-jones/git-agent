package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackz-jones/git-agent/internal/conflict"
	"github.com/jackz-jones/git-agent/internal/gitwrapper"
	"github.com/jackz-jones/git-agent/internal/interpreter"
	"github.com/jackz-jones/git-agent/internal/llm"
	"github.com/jackz-jones/git-agent/internal/planner"
	"github.com/jackz-jones/git-agent/internal/repository"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// AgentState 表示 Agent 的当前状态
type AgentState string

const (
	StateIdle        AgentState = "idle"        // 空闲，等待用户输入
	StateThinking    AgentState = "thinking"    // 正在解析用户意图（LLM 推理中）
	StatePlanning    AgentState = "planning"    // 正在规划执行步骤
	StateExecuting   AgentState = "executing"   // 正在执行操作
	StateConflicting AgentState = "conflicting" // 遇到冲突，等待用户决策
	StateError       AgentState = "error"       // 执行出错
)

// AgentResponse 是 Agent 返回给用户的响应
type AgentResponse struct {
	Success     bool        `json:"success"`
	Message     string      `json:"message"`           // 面向用户的友好消息（办公语言）
	Details     string      `json:"details,omitempty"` // 技术细节（可选）
	State       AgentState  `json:"state"`
	Data        interface{} `json:"data,omitempty"`
	Suggestions []string    `json:"suggestions,omitempty"` // 后续操作建议
	Timestamp   time.Time   `json:"timestamp"`
	// LLM 相关信息
	UsedLLM    bool       `json:"used_llm"`              // 本次响应是否使用了 LLM
	TokenUsage *llm.Usage `json:"token_usage,omitempty"` // LLM token 使用量
}

// UserConfig 用户配置
type UserConfig struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`     // admin, editor, viewer
	Language string `json:"language"` // zh, en
}

// LLMConfig LLM 配置
type LLMConfig struct {
	Enabled   bool   `json:"enabled"` // 是否启用 LLM
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url"` // OpenAI 兼容 API 地址
	Model     string `json:"model"`    // 模型名称
	MaxTokens int    `json:"max_tokens"`
}

// Agent 是智能 Git Agent 的核心
// 支持两种模式：
// 1. LLM 模式：通过 LangChain Go 框架驱动大模型理解用户意图、规划操作、翻译结果
// 2. 本地模式（fallback）：关键词匹配 + 硬编码规划
type Agent struct {
	mu               sync.RWMutex
	repoPath         string
	state            AgentState
	userConfig       *UserConfig
	llmConfig        *LLMConfig
	gitWrapper       *gitwrapper.GitWrapper
	interpreter      *interpreter.Interpreter // 本地 fallback 解析器
	planner          *planner.Planner         // 本地 fallback 规划器
	conflictDetector *conflict.Detector
	repoManager      *repository.Manager

	// LangChain Go 组件
	langchainLLM llms.Model            // LangChain LLM 实例
	chatHistory  []llms.MessageContent // 对话上下文（LangChain MessageContent）
	toolDefs     []llms.Tool           // LangChain 工具定义（用于 function calling）
	currentTools []llms.Tool           // 当前对话轮次使用的工具子集（智能筛选后）
	toolRegistry *llm.GitToolRegistry  // 工具注册中心

	// 事件通道
	inputChan  chan string
	outputChan chan *AgentResponse
}

// New 创建新的 Agent 实例（本地模式）
func New(repoPath string, userConfig *UserConfig) (*Agent, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("获取绝对路径失败: %w", err)
	}

	gitW, err := gitwrapper.New(absPath)
	if err != nil {
		return nil, fmt.Errorf("初始化 Git 封装层失败: %w", err)
	}

	if userConfig == nil {
		userConfig = &UserConfig{
			Name:     "默认用户",
			Email:    "user@git-agent.dev",
			Role:     "editor",
			Language: "zh",
		}
	}

	interp := interpreter.New(userConfig.Language)
	plan := planner.New()
	conflictDet := conflict.NewDetector(absPath)
	repoMgr, err := repository.NewManager(absPath)
	if err != nil {
		return nil, fmt.Errorf("初始化仓库管理器失败: %w", err)
	}

	agent := &Agent{
		repoPath:         absPath,
		state:            StateIdle,
		userConfig:       userConfig,
		llmConfig:        &LLMConfig{Enabled: false},
		gitWrapper:       gitW,
		interpreter:      interp,
		planner:          plan,
		conflictDetector: conflictDet,
		repoManager:      repoMgr,
		chatHistory:      make([]llms.MessageContent, 0),
		inputChan:        make(chan string, 100),
		outputChan:       make(chan *AgentResponse, 100),
	}

	return agent, nil
}

// NewWithLLM 创建带 LLM 的 Agent 实例（使用 LangChain Go 框架）
func NewWithLLM(repoPath string, userConfig *UserConfig, llmConfig *LLMConfig) (*Agent, error) {
	a, err := New(repoPath, userConfig)
	if err != nil {
		return nil, err
	}

	if llmConfig == nil || !llmConfig.Enabled {
		return a, nil
	}

	a.llmConfig = llmConfig

	// 使用 LangChain Go 创建 LLM 实例
	lcLLM, err := llm.NewLangChainLLM(llm.OpenAIConfig{
		APIKey:  llmConfig.APIKey,
		BaseURL: llmConfig.BaseURL,
		Model:   llmConfig.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 LangChain LLM 失败: %w", err)
	}
	a.langchainLLM = lcLLM

	// 初始化工具注册中心
	a.toolRegistry = llm.NewGitToolRegistry()
	a.registerToolExecutors()

	// 构建工具定义
	a.toolDefs = a.toolRegistry.BuildToolDefinitions()
	if len(a.toolDefs) == 0 {
		return nil, fmt.Errorf("没有注册任何工具定义，LLM 模式需要至少一个工具")
	}

	// 初始化对话上下文，加入系统提示词
	a.chatHistory = append(a.chatHistory,
		llms.TextParts(llms.ChatMessageTypeSystem, llm.SystemPrompt),
	)

	return a, nil
}

// registerToolExecutors 注册所有 Git 工具的执行器
func (a *Agent) registerToolExecutors() {
	// save_version
	a.toolRegistry.Register("save_version", func(ctx context.Context, params map[string]interface{}) (string, error) {
		message := toString(params["message"], "chore: save changes")
		files := toString(params["files"], "")
		if files != "" {
			result, err := a.gitWrapper.SaveVersion(message, strings.Split(files, ","), a.userConfig.Name, a.userConfig.Email)
			return toJSONResult(result, err)
		}
		result, err := a.gitWrapper.SaveVersion(message, nil, a.userConfig.Name, a.userConfig.Email)
		return toJSONResult(result, err)
	})

	// view_history
	a.toolRegistry.Register("view_history", func(ctx context.Context, params map[string]interface{}) (string, error) {
		limit := toInt(params["limit"], 10)
		author := toString(params["author"], "")
		file := toString(params["file"], "")
		result, err := a.gitWrapper.GetHistory(file, limit, author)
		return toJSONResult(result, err)
	})

	// restore_version
	a.toolRegistry.Register("restore_version", func(ctx context.Context, params map[string]interface{}) (string, error) {
		versionID := toString(params["version_id"], "")
		file := toString(params["file"], "")
		if file != "" {
			err := a.gitWrapper.RestoreFile(file, versionID)
			return toJSONResult(nil, err)
		}
		err := a.gitWrapper.RestoreVersion(versionID)
		return toJSONResult(nil, err)
	})

	// view_diff
	a.toolRegistry.Register("view_diff", func(ctx context.Context, params map[string]interface{}) (string, error) {
		file := toString(params["file"], "")
		result, err := a.gitWrapper.Diff(file)
		return toJSONResult(result, err)
	})

	// view_status
	a.toolRegistry.Register("view_status", func(ctx context.Context, params map[string]interface{}) (string, error) {
		result, err := a.gitWrapper.Status()
		return toJSONResult(result, err)
	})

	// submit_change
	a.toolRegistry.Register("submit_change", func(ctx context.Context, params map[string]interface{}) (string, error) {
		message := toString(params["message"], "chore: submit changes to team")
		if err := a.gitWrapper.AddAll(); err != nil {
			return "", err
		}
		hash, err := a.gitWrapper.Commit(message, a.userConfig.Name, a.userConfig.Email)
		if err != nil {
			return "", err
		}
		remote := "origin"
		if err := a.gitWrapper.Push(remote); err != nil {
			// 推送失败但本地保存成功，返回部分成功信息
			var authErr *gitwrapper.AuthError
			if errors.As(err, &authErr) {
				return toJSONResult(map[string]string{
					"hash":          hash,
					"push_error":    "auth_failed",
					"error_message": "远程仓库连接认证失败",
				}, nil)
			}
			return toJSONResult(map[string]string{
				"hash":          hash,
				"push_error":    err.Error(),
				"error_message": translateError(err),
			}, nil)
		}
		return toJSONResult(map[string]string{"hash": hash}, nil)
	})

	// view_team_change
	a.toolRegistry.Register("view_team_change", func(ctx context.Context, params map[string]interface{}) (string, error) {
		// 先拉取远程最新内容（与本地模式 planViewTeamChange 行为一致）
		remote := "origin"
		_ = a.gitWrapper.Pull(remote) // 拉取失败不影响查看本地历史
		limit := toInt(params["limit"], 10)
		author := toString(params["author"], "")
		result, err := a.gitWrapper.Log(limit, author)
		return toJSONResult(result, err)
	})

	// merge_branch
	a.toolRegistry.Register("merge_branch", func(ctx context.Context, params map[string]interface{}) (string, error) {
		branch := toString(params["branch"], "")
		err := a.gitWrapper.Merge(branch)
		return toJSONResult(nil, err)
	})

	// init_repo
	a.toolRegistry.Register("init_repo", func(ctx context.Context, params map[string]interface{}) (string, error) {
		err := a.gitWrapper.InitRepo()
		return toJSONResult(nil, err)
	})

	// create_branch
	a.toolRegistry.Register("create_branch", func(ctx context.Context, params map[string]interface{}) (string, error) {
		name := toString(params["name"], "")
		err := a.gitWrapper.CreateBranch(name)
		return toJSONResult(nil, err)
	})

	// switch_branch
	a.toolRegistry.Register("switch_branch", func(ctx context.Context, params map[string]interface{}) (string, error) {
		name := toString(params["name"], "")
		err := a.gitWrapper.SwitchBranch(name)
		return toJSONResult(nil, err)
	})

	// list_branches
	a.toolRegistry.Register("list_branches", func(ctx context.Context, params map[string]interface{}) (string, error) {
		result, err := a.gitWrapper.ListBranches()
		return toJSONResult(result, err)
	})

	// create_tag
	a.toolRegistry.Register("create_tag", func(ctx context.Context, params map[string]interface{}) (string, error) {
		name := toString(params["name"], "")
		err := a.gitWrapper.CreateTag(name)
		return toJSONResult(nil, err)
	})

	// push_to_remote
	a.toolRegistry.Register("push_to_remote", func(ctx context.Context, params map[string]interface{}) (string, error) {
		remote := toString(params["remote"], "origin")
		username := toString(params["username"], "")
		password := toString(params["password"], "")

		// 如果提供了认证信息，使用 PushWithAuth
		if username != "" && password != "" {
			err := a.gitWrapper.PushWithAuth(remote, username, password)
			if err != nil {
				return "", err
			}
			return toJSONResult(nil, nil)
		}

		err := a.gitWrapper.Push(remote)
		if err != nil {
			return "", err
		}
		return toJSONResult(nil, nil)
	})

	// pull_from_remote
	a.toolRegistry.Register("pull_from_remote", func(ctx context.Context, params map[string]interface{}) (string, error) {
		remote := toString(params["remote"], "origin")
		err := a.gitWrapper.Pull(remote)
		return toJSONResult(nil, err)
	})

	// detect_conflict
	a.toolRegistry.Register("detect_conflict", func(ctx context.Context, params map[string]interface{}) (string, error) {
		result, err := a.conflictDetector.Scan()
		return toJSONResult(result, err)
	})

	// resolve_conflict
	a.toolRegistry.Register("resolve_conflict", func(ctx context.Context, params map[string]interface{}) (string, error) {
		filePath := toString(params["file_path"], "")
		strategy := toString(params["strategy"], "")
		result, err := a.conflictDetector.Resolve(filePath, strategy)
		return toJSONResult(result, err)
	})
}

// maxReActIterations ReAct 循环最大迭代次数
// 防止 LLM 反复调用同一工具导致无限循环
const maxReActIterations = 5

// intentToolMapping 意图到工具名称的映射
// 用于智能筛选发送给 LLM 的工具，避免小模型被过多工具干扰
var intentToolMapping = map[string][]string{
	"save_version":    {"save_version", "view_status", "view_diff"},
	"view_history":    {"view_history", "view_status"},
	"restore_version": {"restore_version", "view_history", "view_diff"},
	"view_diff":       {"view_diff", "view_status"},
	"view_status":     {"view_status", "view_diff", "view_history"},
	"submit_change":   {"submit_change", "view_status", "push_to_remote"},
	"view_team_change": {"view_team_change", "view_history", "view_diff"},
	"approve_merge":   {"merge_branch", "view_status", "view_diff"},
	"init_repo":       {"init_repo"},
	"create_branch":   {"create_branch", "switch_branch", "list_branches"},
	"switch_branch":   {"switch_branch", "list_branches", "create_branch"},
	"list_branches":   {"list_branches", "switch_branch"},
	"create_tag":      {"create_tag", "view_history"},
	"push":            {"push_to_remote", "view_status"},
	"pull":            {"pull_from_remote", "view_status", "detect_conflict"},
	"detect_conflict": {"detect_conflict", "resolve_conflict", "view_status"},
	"help":            {"view_status", "view_history"},
}

// selectRelevantTools 根据用户输入智能筛选相关工具
// 先用本地 interpreter 预判意图，再只发送相关工具给 LLM
// 对于小模型（如 7B），工具太多会导致模型不调用任何工具
func (a *Agent) selectRelevantTools(input string) []llms.Tool {
	// 用本地 interpreter 预判意图
	intent, err := a.interpreter.Parse(input)
	if err != nil || intent == nil {
		// 解析失败，发送所有工具
		return a.toolDefs
	}
	
	// 获取与意图相关的工具名称
	relevantNames := intentToolMapping[string(intent.Type)]
	if len(relevantNames) == 0 {
		// 未知意图，发送所有工具
		return a.toolDefs
	}
	
	// 始终包含 view_status（最常用的辅助工具）
	hasViewStatus := false
	for _, name := range relevantNames {
		if name == "view_status" {
			hasViewStatus = true
			break
		}
	}
	if !hasViewStatus && intent.Type != "init_repo" {
		relevantNames = append(relevantNames, "view_status")
	}
	
	// 根据名称筛选工具
	nameSet := make(map[string]bool, len(relevantNames))
	for _, name := range relevantNames {
		nameSet[name] = true
	}
	
	var result []llms.Tool
	for _, tool := range a.toolDefs {
		if nameSet[tool.Function.Name] {
			result = append(result, tool)
		}
	}
	
	// 如果筛选结果为空，回退到全量工具
	if len(result) == 0 {
		return a.toolDefs
	}
	
	return result
}

// IsLLMEnabled 检查 LLM 是否已启用
func (a *Agent) IsLLMEnabled() bool {
	return a.langchainLLM != nil && a.llmConfig.Enabled
}

// Process 处理用户自然语言输入，这是 Agent 的主入口
// 完整的 Agent Loop：感知 → 推理 → 行动 → 反馈
// 如果 LLM 可用，优先使用 LangChain Go 驱动的 LLM Agent；否则回退到本地关键词匹配
func (a *Agent) Process(ctx context.Context, input string) *AgentResponse {
	if a.IsLLMEnabled() {
		return a.processWithLLM(ctx, input)
	}
	return a.processLocal(ctx, input)
}

// processWithLLM 使用 LangChain Go 驱动的 LLM Agent Loop
// 核心流程：用户输入 → LLM 推理（可能调用工具） → 执行工具 → 结果回传 LLM → 生成回复
func (a *Agent) processWithLLM(ctx context.Context, input string) *AgentResponse {
	a.setState(StateThinking)

	// 将用户输入加入对话上下文
	a.chatHistory = append(a.chatHistory,
		llms.TextParts(llms.ChatMessageTypeHuman, input),
	)

	// 智能工具筛选：先用本地 interpreter 预判意图，只发送相关工具给 LLM
	// 这对于小模型（如 7B）特别重要，工具太多会导致模型不调用任何工具
	a.currentTools = a.selectRelevantTools(input)

	// 调用 LLM，携带筛选后的工具定义（function calling）
	resp, err := a.langchainLLM.GenerateContent(
		ctx,
		a.chatHistory,
		llms.WithTools(a.currentTools),
		llms.WithToolChoice("auto"), // 显式指定 auto，确保 Ollama 等兼容 API 启用工具调用
		llms.WithTemperature(0.1),   // Agent 场景使用低温度，减少幻觉
		llms.WithMaxTokens(a.llmConfig.MaxTokens),
		openai.WithLegacyMaxTokensField(), // 兼容 Ollama、DeepSeek 等非 OpenAI API
	)
	if err != nil {
		// LLM 调用失败，回退到本地模式
		a.chatHistory = a.chatHistory[:len(a.chatHistory)-1]
		return a.fallbackToLocal(ctx, input, err)
	}

	// 处理 LLM 响应，从第 1 次迭代开始
	return a.handleLLMResponse(ctx, resp, 1)
}

// handleLLMResponse 处理 LangChain LLM 响应
// 支持 ReAct 多轮：LLM 可能返回工具调用 → 执行工具 → 结果回传 → LLM 继续推理
// iteration: 当前迭代次数，用于防止无限循环
func (a *Agent) handleLLMResponse(ctx context.Context, resp *llms.ContentResponse, iteration int) *AgentResponse {
	if len(resp.Choices) == 0 {
		a.setState(StateIdle)
		return &AgentResponse{
			Success:   true,
			Message:   "LLM 未返回任何内容，请重试。",
			State:     StateIdle,
			UsedLLM:   true,
			Timestamp: time.Now(),
		}
	}

	choice := resp.Choices[0]
	var totalUsage llm.Usage

	// 从 LLM 响应的 GenerationInfo 中提取 Token 用量
	// langchaingo 将 usage 信息放在 ContentChoice.GenerationInfo 中
	if choice.GenerationInfo != nil {
		if v, ok := choice.GenerationInfo["PromptTokens"]; ok {
			if n, ok := v.(int); ok {
				totalUsage.PromptTokens = n
			}
		}
		if v, ok := choice.GenerationInfo["CompletionTokens"]; ok {
			if n, ok := v.(int); ok {
				totalUsage.CompletionTokens = n
			}
		}
		if v, ok := choice.GenerationInfo["TotalTokens"]; ok {
			if n, ok := v.(int); ok {
				totalUsage.TotalTokens = n
			}
		}
		// 如果 TotalTokens 为 0，自动计算
		if totalUsage.TotalTokens == 0 && (totalUsage.PromptTokens > 0 || totalUsage.CompletionTokens > 0) {
			totalUsage.TotalTokens = totalUsage.PromptTokens + totalUsage.CompletionTokens
		}
	}

	// 从 ContentChoice 中获取文本内容和工具调用
	assistantContent := choice.Content
	toolCalls := choice.ToolCalls

	// 将 assistant 消息加入对话历史（使用 MessageContent 格式）
	aiParts := []llms.ContentPart{}
	if assistantContent != "" {
		aiParts = append(aiParts, llms.TextContent{Text: assistantContent})
	}
	for _, tc := range toolCalls {
		aiParts = append(aiParts, tc)
	}
	if len(aiParts) > 0 {
		a.chatHistory = append(a.chatHistory, llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: aiParts,
		})
	}

	// 如果有工具调用，执行工具并将结果回传 LLM
	if len(toolCalls) > 0 {
		return a.handleLangChainToolCalls(ctx, toolCalls, &totalUsage, iteration)
	}

	// LLM 直接回复（普通对话或帮助类问题）
	a.setState(StateIdle)
	return &AgentResponse{
		Success:    true,
		Message:    assistantContent,
		State:      StateIdle,
		UsedLLM:    true,
		TokenUsage: &totalUsage,
		Timestamp:  time.Now(),
	}
}

// handleLangChainToolCalls 处理 LangChain 格式的工具调用
// 这是 ReAct 模式的核心：LLM 决定调用什么工具 → 执行工具 → 结果回传 LLM → LLM 生成最终回复
// iteration: 当前迭代次数，用于防止无限循环
func (a *Agent) handleLangChainToolCalls(ctx context.Context, toolCalls []llms.ToolCall, totalUsage *llm.Usage, iteration int) *AgentResponse {
	// 重复工具调用检测：追踪本次迭代中调用的工具名称
	seenTools := make(map[string]int) // toolName → 连续调用次数

	for _, tc := range toolCalls {
		a.setState(StateExecuting)

		// 获取工具名称和参数（FunctionCall 是指针类型）
		toolName := ""
		toolArgs := ""
		if tc.FunctionCall != nil {
			toolName = tc.FunctionCall.Name
			toolArgs = tc.FunctionCall.Arguments
		}

		// 检测重复工具调用：同一工具在同一轮中被连续调用
		seenTools[toolName]++
		if seenTools[toolName] > 2 {
			// 同一工具被重复调用过多，跳出循环防止无限循环
			a.setState(StateError)
			errMsg := fmt.Sprintf("工具 %s 被重复调用多次，可能当前操作已完成但系统未正确识别。", toolName)
			return &AgentResponse{
				Success: false,
				Message: errMsg + "建议先查看当前状态，确认操作是否已经成功。",
				State:   StateError,
				Suggestions: []string{
					"查看当前状态",
					"查看修改内容",
				},
				Timestamp: time.Now(),
			}
		}

		// 通过工具注册中心执行工具
		tool, ok := a.toolRegistry.GetTool(toolName)
		if !ok {
			// 工具不存在，将错误信息回传 LLM
			a.chatHistory = append(a.chatHistory, llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						ToolCallID: tc.ID,
						Name:       toolName,
						Content:    fmt.Sprintf("错误：未知工具 %s", toolName),
					},
				},
			})
			continue
		}

		// 执行工具
		result, err := tool.Call(ctx, toolArgs)
		if err != nil {
			// 对 AuthError 提供友好提示，引导 LLM 给出非技术性的建议
			var authErr *gitwrapper.AuthError
			if errors.As(err, &authErr) {
				result = "推送失败：远程仓库需要身份验证，但当前没有配置有效的认证方式。请用通俗语言告诉用户推送失败了，建议他们联系技术同事帮忙配置访问权限，或者提供用户名和访问令牌来重试。不要提及 SSH、密钥、公钥等技术术语，不要建议执行任何命令行操作。"
			} else {
				result = fmt.Sprintf("操作失败：%s", translateError(err))
			}
		}

		// 将工具执行结果加入对话历史
		a.chatHistory = append(a.chatHistory, llms.MessageContent{
			Role: llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{
				llms.ToolCallResponse{
					ToolCallID: tc.ID,
					Name:       toolName,
					Content:    result,
				},
			},
		})
	}

	// 将工具执行结果回传 LLM，让它生成最终回复
	a.setState(StateThinking)
	resp, err := a.langchainLLM.GenerateContent(
		ctx,
		a.chatHistory,
		llms.WithTools(a.currentTools), // 使用当前轮次筛选后的工具
		llms.WithToolChoice("auto"), // 显式指定 auto，确保 Ollama 等兼容 API 启用工具调用
		llms.WithTemperature(0.1),
		llms.WithMaxTokens(a.llmConfig.MaxTokens),
		openai.WithLegacyMaxTokensField(), // 兼容 Ollama、DeepSeek 等非 OpenAI API
	)
	if err != nil {
		a.setState(StateIdle)
		return &AgentResponse{
			Success:    true,
			Message:    "操作已完成，但无法生成详细说明。",
			State:      StateIdle,
			UsedLLM:    true,
			TokenUsage: totalUsage,
			Timestamp:  time.Now(),
		}
	}

	// 递归处理 LLM 的后续响应（可能还有工具调用 = 多轮 ReAct）
	// 检查迭代次数上限，防止无限循环
	nextIteration := iteration + 1
	if nextIteration > maxReActIterations {
		a.setState(StateError)
		return &AgentResponse{
			Success: false,
			Message: "操作陷入循环，已自动终止。这可能是因为当前修改状态异常，建议先查看当前状态，确认修改是否已成功保存。",
			State:   StateError,
			Suggestions: []string{
				"查看当前状态",
				"查看修改内容",
			},
			Timestamp: time.Now(),
		}
	}
	return a.handleLLMResponse(ctx, resp, nextIteration)
}

// processLocal 本地 fallback 处理（不使用 LLM）
func (a *Agent) processLocal(ctx context.Context, input string) *AgentResponse {
	a.setState(StateThinking)

	// 第一步：感知 - 解析用户意图
	intent, err := a.interpreter.Parse(input)
	if err != nil {
		a.setState(StateError)
		return &AgentResponse{
			Success: false,
			Message: fmt.Sprintf("抱歉，我没能理解您的意思：「%s」。您可以换个说法试试，或者输入「帮助」查看我能做什么。", input),
			Details: err.Error(),
			State:   StateError,
			Suggestions: []string{
				"保存当前修改",
				"查看修改历史",
				"查看当前状态",
			},
			Timestamp: time.Now(),
		}
	}

	// 第二步：推理 - 根据意图规划执行步骤
	a.setState(StatePlanning)
	plan, err := a.planner.CreatePlan(intent)
	if err != nil {
		a.setState(StateError)
		return &AgentResponse{
			Success:   false,
			Message:   fmt.Sprintf("抱歉，我暂时无法完成这个操作：「%s」。", intent.Description()),
			Details:   err.Error(),
			State:     StateError,
			Timestamp: time.Now(),
		}
	}

	// 第三步：行动 - 执行计划中的每一步
	a.setState(StateExecuting)
	result, err := a.executePlan(ctx, plan)
	if err != nil {
		if isConflictError(err) {
			a.setState(StateConflicting)
			return a.handleConflict(result, err)
		}

		// 认证错误：提供更友好的建议
		var authErr *gitwrapper.AuthError
		if errors.As(err, &authErr) {
			a.setState(StateError)
			return &AgentResponse{
				Success: false,
				Message: "抱歉，推送失败了！远程仓库需要身份验证，但当前没有配置有效的认证方式。",
				Details: err.Error(),
				State:   StateError,
				Suggestions: []string{
					"请联系您的技术同事，帮忙配置 SSH 密钥或访问令牌",
					"如果您有 GitHub 账号，可以告诉同事您需要：1）在电脑上生成 SSH 密钥；2）把公钥添加到 GitHub",
					"也可以尝试提供用户名和访问令牌来推送",
				},
				Timestamp: time.Now(),
			}
		}

		a.setState(StateError)
		return &AgentResponse{
			Success: false,
			Message: fmt.Sprintf("操作执行失败：%s", translateError(err)),
			Details: err.Error(),
			State:   StateError,
			Suggestions: []string{
				"查看当前状态",
				"查看修改内容",
			},
			Timestamp: time.Now(),
		}
	}

	// 第四步：反馈 - 将结果翻译为用户友好的语言
	a.setState(StateIdle)
	return a.formatResponse(intent, result)
}

// fallbackToLocal LLM 失败时回退到本地处理
func (a *Agent) fallbackToLocal(ctx context.Context, input string, llmErr error) *AgentResponse {
	localResp := a.processLocal(ctx, input)
	localResp.Details = fmt.Sprintf("LLM 不可用（%s），已回退到本地模式", llmErr.Error())
	return localResp
}

// ProcessSimple 简化版处理接口，不需要 context
func (a *Agent) ProcessSimple(input string) *AgentResponse {
	return a.Process(context.Background(), input)
}

// GetConversationHistory 获取对话历史
func (a *Agent) GetConversationHistory() []llms.MessageContent {
	a.mu.RLock()
	defer a.mu.RUnlock()
	history := make([]llms.MessageContent, len(a.chatHistory))
	copy(history, a.chatHistory)
	return history
}

// ClearConversation 清空对话历史（保留 system prompt）
func (a *Agent) ClearConversation() {
	a.mu.Lock()
	defer a.mu.Unlock()
	// 保留第一条 system prompt
	for i, msg := range a.chatHistory {
		if msg.Role == llms.ChatMessageTypeSystem {
			a.chatHistory = a.chatHistory[:i+1]
			return
		}
	}
	a.chatHistory = make([]llms.MessageContent, 0)
}

// executePlan 执行执行计划中的每一步
func (a *Agent) executePlan(ctx context.Context, plan *planner.Plan) (*planner.ExecutionResult, error) {
	result := planner.NewExecutionResult()

	for i, step := range plan.Steps {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		stepResult, err := a.executeStep(step)
		if err != nil {
			result.AddFailedStep(i, step, err)
			if step.Required {
				return result, err
			}
			continue
		}
		result.AddCompletedStep(i, step, stepResult)
	}

	return result, nil
}

// executeStep 执行单个步骤
func (a *Agent) executeStep(step *planner.Step) (interface{}, error) {
	switch step.Type {
	case planner.StepGitInit:
		return nil, a.gitWrapper.InitRepo()

	case planner.StepGitAdd:
		files := step.Params["files"]
		if files == "" {
			return nil, a.gitWrapper.AddAll()
		}
		return nil, a.gitWrapper.AddFiles(splitFiles(files))

	case planner.StepGitCommit:
		message := step.Params["message"]
		if message == "" {
			message = "chore: save changes"
		}
		hash, err := a.gitWrapper.Commit(message, a.userConfig.Name, a.userConfig.Email)
		if err != nil {
			return nil, err
		}
		return map[string]string{"hash": hash}, nil

	case planner.StepGitPush:
		remote := step.Params["remote"]
		if remote == "" {
			remote = "origin"
		}
		err := a.gitWrapper.Push(remote)
		if err != nil {
			// 对 AuthError 不直接返回，包装为友好错误
			var authErr *gitwrapper.AuthError
			if errors.As(err, &authErr) {
				return nil, fmt.Errorf("远程仓库连接认证失败")
			}
			return nil, err
		}
		return nil, nil

	case planner.StepGitPull:
		remote := step.Params["remote"]
		if remote == "" {
			remote = "origin"
		}
		return nil, a.gitWrapper.Pull(remote)

	case planner.StepGitLog:
		limit := 10
		if l, ok := step.Params["limit"]; ok {
			fmt.Sscanf(l, "%d", &limit)
		}
		author := step.Params["author"]
		filePath := step.Params["file"]
		return a.gitWrapper.GetHistory(filePath, limit, author)

	case planner.StepGitDiff:
		filePath := step.Params["file"]
		return a.gitWrapper.Diff(filePath)

	case planner.StepGitStatus:
		return a.gitWrapper.Status()

	case planner.StepGitBranch:
		action := step.Params["action"]
		name := step.Params["name"]
		switch action {
		case "create":
			return nil, a.gitWrapper.CreateBranch(name)
		case "list":
			return a.gitWrapper.ListBranches()
		case "switch":
			return nil, a.gitWrapper.SwitchBranch(name)
		default:
			return a.gitWrapper.ListBranches()
		}

	case planner.StepGitMerge:
		branch := step.Params["branch"]
		return nil, a.gitWrapper.Merge(branch)

	case planner.StepGitTag:
		name := step.Params["name"]
		return nil, a.gitWrapper.CreateTag(name)

	case planner.StepGitRestore:
		versionID := step.Params["version_id"]
		filePath := step.Params["file"]
		if filePath != "" {
			return nil, a.gitWrapper.RestoreFile(filePath, versionID)
		}
		return nil, a.gitWrapper.RestoreVersion(versionID)

	case planner.StepConflictDetect:
		return a.conflictDetector.Scan()

	case planner.StepConflictResolve:
		strategy := step.Params["strategy"]
		conflictID := step.Params["conflict_id"]
		return a.conflictDetector.Resolve(conflictID, strategy)

	case planner.StepRepoCreate:
		name := step.Params["name"]
		return a.repoManager.Create(name)

	case planner.StepRepoClone:
		url := step.Params["url"]
		return a.repoManager.Clone(url)

	default:
		return nil, fmt.Errorf("未知的步骤类型: %s", step.Type)
	}
}

// handleConflict 处理冲突情况
func (a *Agent) handleConflict(result *planner.ExecutionResult, err error) *AgentResponse {
	conflicts, scanErr := a.conflictDetector.Scan()
	if scanErr != nil {
		return &AgentResponse{
			Success:   false,
			Message:   "操作过程中发现了冲突，但无法获取冲突详情。请联系管理员处理。",
			Details:   scanErr.Error(),
			State:     StateError,
			Timestamp: time.Now(),
		}
	}

	var conflictDesc []string
	for _, c := range conflicts {
		conflictDesc = append(conflictDesc, c.Description())
	}

	suggestions := []string{}
	for _, c := range conflicts {
		suggestions = append(suggestions, c.Suggestions()...)
	}

	return &AgentResponse{
		Success:     false,
		Message:     fmt.Sprintf("发现 %d 处冲突需要您确认：\n%s", len(conflicts), joinStrings(conflictDesc)),
		Details:     err.Error(),
		State:       StateConflicting,
		Data:        conflicts,
		Suggestions: suggestions,
		Timestamp:   time.Now(),
	}
}

// formatResponse 将执行结果格式化为用户友好的响应
func (a *Agent) formatResponse(intent *interpreter.UserIntent, result *planner.ExecutionResult) *AgentResponse {
	msg := a.interpreter.TranslateResult(intent, result)
	suggestions := a.interpreter.SuggestNext(intent)

	return &AgentResponse{
		Success:     true,
		Message:     msg,
		State:       StateIdle,
		Suggestions: suggestions,
		Timestamp:   time.Now(),
	}
}

// GetState 获取 Agent 当前状态
func (a *Agent) GetState() AgentState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

// GetUserConfig 获取用户配置
func (a *Agent) GetUserConfig() *UserConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.userConfig
}

// GetRepoPath 获取仓库路径
func (a *Agent) GetRepoPath() string {
	return a.repoPath
}

// GetGitWrapper 获取 Git 操作封装（供高级用户或 API 直接调用）
func (a *Agent) GetGitWrapper() *gitwrapper.GitWrapper {
	return a.gitWrapper
}

// Close 关闭 Agent，释放资源
func (a *Agent) Close() {
	close(a.inputChan)
	close(a.outputChan)
}

// setState 设置 Agent 状态
func (a *Agent) setState(state AgentState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = state
}

// ==================== 辅助函数 ====================

// toString 从 map 中获取字符串值，支持默认值
func toString(v interface{}, defaultVal string) string {
	if v == nil {
		return defaultVal
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return defaultVal
		}
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// toInt 从 map 中获取整数值，支持默认值
func toInt(v interface{}, defaultVal int) int {
	if v == nil {
		return defaultVal
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case string:
		var i int
		fmt.Sscanf(val, "%d", &i)
		if i == 0 {
			return defaultVal
		}
		return i
	default:
		return defaultVal
	}
}

// toJSONResult 将结果序列化为 JSON 字符串
func toJSONResult(result interface{}, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if result == nil {
		return "操作成功", nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%v", result), nil
	}
	return string(data), nil
}

func isConflictError(err error) bool {
	if err == nil {
		return false
	}
	return containsAny(err.Error(), "conflict", "冲突", "CONFLICT")
}

func translateError(err error) string {
	msg := err.Error()

	// 优先检查 AuthError 类型
	var authErr *gitwrapper.AuthError
	if errors.As(err, &authErr) {
		return "远程仓库连接认证失败"
	}

	translations := map[string]string{
		"repository not initialized": "仓库尚未初始化",
		"no changes to commit":       "没有需要保存的修改",
		"branch not found":           "找不到该工作分支",
		"merge conflict":             "合并时发现冲突",
		"remote not found":           "找不到远程仓库",
		"permission denied":          "权限不足",
		"file not found":             "文件不存在",
		"无法连接到远程仓库":            "无法连接到远程仓库，请检查网络",
		"远程仓库有更新":              "远程仓库有更新的内容，请先获取最新修改",
		"远程仓库不存在":              "远程仓库不存在，请确认仓库地址",
		"认证失败":                    "远程仓库连接认证失败",
	}
	for eng, zh := range translations {
		if contains(msg, eng) {
			return zh
		}
	}
	return msg
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += "\n"
		}
		result += s
	}
	return result
}

func splitFiles(s string) []string {
	if s == "" {
		return nil
	}
	var files []string
	for _, f := range splitByComma(s) {
		f = trimSpace(f)
		if f != "" {
			files = append(files, f)
		}
	}
	return files
}

func splitByComma(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

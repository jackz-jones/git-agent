package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackz-jones/git-agent/internal/conflict"
	"github.com/jackz-jones/git-agent/internal/gitwrapper"
	"github.com/jackz-jones/git-agent/internal/interpreter"
	"github.com/jackz-jones/git-agent/internal/llm"
	"github.com/jackz-jones/git-agent/internal/planner"
	"github.com/jackz-jones/git-agent/internal/promptkit"
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
	promptKit    *promptkit.Kit        // Skills/Rules 按需加载器

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

	// 初始化 Skills/Rules 按需加载器
	userConfigDir := promptkit.GetUserConfigDir()
	projectConfigDir := promptkit.GetProjectConfigDir(repoPath)
	a.promptKit = promptkit.NewKit(promptkit.ResourcesFS, promptkit.Config{
		UserDir:    userConfigDir,
		ProjectDir: projectConfigDir,
	})
	if err := a.promptKit.Load(); err != nil {
		// 加载失败不阻塞，只记录日志
		_ = err
	}

	// 启动热加载：监听用户级和项目级目录的文件变更
	if err := a.promptKit.WatchAndHotReload(); err != nil {
		// 热加载启动失败不影响正常使用
		log.Printf("[agent] 热加载监听启动失败: %v", err)
	}

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

	// 初始化对话上下文，加入系统提示词（骨架 + 动态注入的全局规则）
	systemPrompt := a.buildSystemPrompt("")
	a.chatHistory = append(a.chatHistory,
		llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
	)

	return a, nil
}

// registerToolExecutors 注册所有 Git 工具的执行器
func (a *Agent) registerToolExecutors() {
	// save_version
	a.toolRegistry.Register("save_version", func(ctx context.Context, params map[string]interface{}) (string, error) {
		if err := a.ensureUserConfig(); err != nil {
			return "", err
		}
		message := toString(params["message"], "chore: save pending changes (auto-generated, please specify)")
		// 校验 message 质量：拒绝过于笼统的描述
		if isVagueCommitMessage(message) {
			// 自动获取 diff 摘要，帮助 LLM 重写具体的 commit message
			diffHint := ""
			if diffResult, diffErr := a.gitWrapper.Diff(""); diffErr == nil && diffResult != "" {
				// 截取前 500 字符作为提示
				runes := []rune(diffResult)
				if len(runes) > 500 {
					diffHint = string(runes[:500]) + "..."
				} else {
					diffHint = diffResult
				}
			}
			errMsg := fmt.Sprintf("commit message '%s' is too vague. Please write a specific message that describes WHAT was changed.", message)
			if diffHint != "" {
				errMsg += fmt.Sprintf(" Here is a summary of the current changes:\n%s\nPlease rewrite the commit message based on the changes above.", diffHint)
			} else {
				errMsg += " Examples: 'feat: add commit_hash param to view_diff', 'fix: resolve nil pointer in Diff method'. Do NOT use generic phrases like 'update files' or 'change code'."
			}
			return "", fmt.Errorf("%s", errMsg)
		}
		files := toString(params["files"], "")
		var result interface{}
		var err error
		if files != "" {
			result, err = a.gitWrapper.SaveVersion(message, strings.Split(files, ","), a.userConfig.Name, a.userConfig.Email)
		} else {
			result, err = a.gitWrapper.SaveVersion(message, nil, a.userConfig.Name, a.userConfig.Email)
		}
		if err != nil {
			return "", err
		}
		// 成功保存后，给 LLM 明确信号：操作已完成，不要再查看 diff 或 status
		jsonResult, _ := toJSONResult(result, nil)
		return jsonResult + "\n[Version saved successfully. Do NOT call view_diff or view_status to verify - just inform the user the save was successful.]", nil
	})

	// view_history — 硬编码表格格式，不依赖 LLM 或 SKILL 来格式化
	a.toolRegistry.Register("view_history", func(ctx context.Context, params map[string]interface{}) (string, error) {
		limit := toInt(params["limit"], 10)
		author := toString(params["author"], "")
		file := toString(params["file"], "")
		result, err := a.gitWrapper.GetHistory(file, limit, author)
		if err != nil {
			return "", err
		}
		return formatVersionTable(result), nil
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
		commitHash := toString(params["commit_hash"], "")
		if commitHash != "" {
			result, err := a.gitWrapper.CommitDiff(commitHash)
			if err != nil {
				return "", err
			}
			return result, nil
		}
		result, err := a.gitWrapper.Diff(file)
		if err != nil {
			return "", err
		}
		// 当没有修改时，给 LLM 更明确的指示，避免它反复调用 view_diff
		if result == "没有发现修改" {
			return "没有发现修改。当前工作区与已保存版本完全一致，没有任何差异。请直接基于此信息回复用户，不要再次调用 view_diff 或 view_status。", nil
		}
		return result, nil
	})

	// view_status — LatestCommit 部分硬编码表格格式
	a.toolRegistry.Register("view_status", func(ctx context.Context, params map[string]interface{}) (string, error) {
		result, err := a.gitWrapper.Status()
		if err != nil {
			return "", err
		}
		// 将 LatestCommit 硬编码格式化为表格，其余字段保持 JSON
		data, err := json.Marshal(result)
		if err != nil {
			return fmt.Sprintf("%v", result), nil
		}
		// 构建 JSON 结果，附加格式化的最新提交表格
		jsonResult := string(data)
		if result.LatestCommit != nil {
			commitTable := formatVersionTable([]gitwrapper.VersionInfo{*result.LatestCommit})
			jsonResult = jsonResult + "\n\n最新提交：" + commitTable
		}
		return jsonResult, nil
	})

	// submit_change
	a.toolRegistry.Register("submit_change", func(ctx context.Context, params map[string]interface{}) (string, error) {
		if err := a.ensureUserConfig(); err != nil {
			return "", err
		}
		message := toString(params["message"], "chore: submit changes to team")
		// 校验 message 质量：拒绝过于笼统的描述
		if isVagueCommitMessage(message) {
			// 自动获取 diff 摘要，帮助 LLM 重写具体的 commit message
			diffHint := ""
			if diffResult, diffErr := a.gitWrapper.Diff(""); diffErr == nil && diffResult != "" {
				runes := []rune(diffResult)
				if len(runes) > 500 {
					diffHint = string(runes[:500]) + "..."
				} else {
					diffHint = diffResult
				}
			}
			errMsg := fmt.Sprintf("commit message '%s' is too vague. Please write a specific message that describes WHAT was changed.", message)
			if diffHint != "" {
				errMsg += fmt.Sprintf(" Here is a summary of the current changes:\n%s\nPlease rewrite the commit message based on the changes above.", diffHint)
			} else {
				errMsg += " Examples: 'feat: add HTTPS auth support for push', 'fix: resolve merge conflict detection edge case'. Do NOT use generic phrases like 'update files' or 'change code'."
			}
			return "", fmt.Errorf("%s", errMsg)
		}
		if err := a.gitWrapper.AddAll(); err != nil {
			return "", err
		}
		hash, err := a.gitWrapper.Commit(message, a.userConfig.Name, a.userConfig.Email)
		if err != nil {
			return "", err
		}
		remote := "origin"
		pushErr := a.gitWrapper.Push(remote)
		if pushErr != nil {
			// 推送失败但本地保存成功，返回部分成功信息
			var authErr *gitwrapper.AuthError
			if errors.As(pushErr, &authErr) {
				return toJSONResult(map[string]string{
					"hash":          hash,
					"push_error":    "auth_failed",
					"error_message": "远程仓库连接认证失败",
				}, nil)
			}
			return toJSONResult(map[string]string{
				"hash":          hash,
				"push_error":    pushErr.Error(),
				"error_message": translateError(pushErr),
			}, nil)
		}
		// 成功提交并推送后，给 LLM 明确信号
		return toJSONResult(map[string]string{"hash": hash, "status": "submitted_and_pushed"}, nil)
	})

	// view_team_change — 硬编码表格格式，不依赖 LLM 或 SKILL 来格式化
	a.toolRegistry.Register("view_team_change", func(ctx context.Context, params map[string]interface{}) (string, error) {
		// 先拉取远程最新内容（与本地模式 planViewTeamChange 行为一致）
		remote := "origin"
		_ = a.gitWrapper.Pull(remote) // 拉取失败不影响查看本地历史
		limit := toInt(params["limit"], 10)
		author := toString(params["author"], "")
		result, err := a.gitWrapper.Log(limit, author)
		if err != nil {
			return "", err
		}
		return formatVersionTable(result), nil
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

	// update_user_info
	a.toolRegistry.Register("update_user_info", func(ctx context.Context, params map[string]interface{}) (string, error) {
		name := toString(params["name"], "")
		email := toString(params["email"], "")
		if name == "" && email == "" {
			return "", fmt.Errorf("请提供至少一项用户信息（名字或邮箱）")
		}
		a.UpdateUserInfo(name, email)
		return toJSONResult(map[string]string{
			"name":  a.userConfig.Name,
			"email": a.userConfig.Email,
		}, nil)
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
		remoteURL := toString(params["remote_url"], "")

		// 先检查 ahead/behind 状态
		aheadBehind, _ := a.gitWrapper.GetAheadBehind()
		if aheadBehind != nil {
			if aheadBehind.Ahead == 0 && aheadBehind.Behind == 0 {
				return toJSONResult(map[string]string{
					"status":  "up_to_date",
					"message": "本地和远程已完全同步，无需推送",
				}, nil)
			}
		}

		// 如果提供了新的远程 URL（通常是 HTTPS 地址），先切换远程地址
		if remoteURL != "" {
			if err := a.gitWrapper.SetRemoteURL(remote, remoteURL); err != nil {
				return "", fmt.Errorf("切换远程仓库地址失败: %w", err)
			}
		}

		// 如果提供了认证信息，先检测远程 URL 协议
		if username != "" && password != "" {
			// 检测远程 URL 是否为 SSH 协议
			if a.gitWrapper.IsRemoteSSH(remote) {
				currentURL, _ := a.gitWrapper.GetRemoteURL(remote)
				// SSH 远程仓库不支持用户名/令牌认证
				return "", &gitwrapper.AuthError{
					RemoteURL: currentURL,
					AuthType:  "ssh",
					Cause:     fmt.Errorf("当前远程仓库使用 SSH 协议，不支持用户名/令牌认证"),
				}
			}
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
	"save_version":     {"save_version", "view_status", "update_user_info"},
	"view_history":     {"view_history", "view_status"},
	"restore_version":  {"restore_version", "view_history", "view_diff"},
	"view_diff":        {"view_diff", "view_status"},
	"view_status":      {"view_status", "view_diff", "view_history"},
	"submit_change":    {"submit_change", "view_status", "push_to_remote", "update_user_info"},
	"view_team_change": {"view_team_change", "view_history", "view_diff"},
	"approve_merge":    {"merge_branch", "view_status", "view_diff"},
	"init_repo":        {"init_repo"},
	"create_branch":    {"create_branch", "switch_branch", "list_branches"},
	"switch_branch":    {"switch_branch", "list_branches", "create_branch"},
	"list_branches":    {"list_branches", "switch_branch"},
	"create_tag":       {"create_tag", "view_history"},
	"push":             {"push_to_remote", "view_status"},
	"pull":             {"pull_from_remote", "view_status", "detect_conflict"},
	"detect_conflict":  {"detect_conflict", "resolve_conflict", "view_status"},
	"help":             {"view_status", "view_history"},
	"update_user_info": {"update_user_info"},
}

// buildSystemPrompt 构建系统提示词
// basePrompt: 基础骨架提示词（不含意图相关内容）
// intent: 当前意图（用于按需加载关联的 Skills/Rules），空字符串表示只加载全局规则
func (a *Agent) buildSystemPrompt(intent string) string {
	var sb strings.Builder

	// 1. 角色骨架（始终包含，保持精简）
	sb.WriteString(`你是一个文件版本管理助手（Git Agent），专门帮助不懂技术的办公人员管理文件版本。

## 你的角色
- 你是用户的文件管家，帮助用户保存、查看、恢复文件版本
- 你用简单易懂的办公语言与用户交流，绝不使用 git 术语
- 你将用户的自然语言需求转化为具体的版本管理操作

## 你能做的事情
1. 保存文件版本 — 用户编辑完文件后，帮他们保存一个新版本
2. 查看修改历史 — 查看某个文件或整个仓库的修改记录
3. 恢复旧版本 — 把文件恢复到之前的某个版本
4. 查看修改内容 — 对比不同版本之间的差异
5. 查看当前状态 — 看看有哪些文件被修改了，以及本地是否领先远程
6. 团队协作 — 提交修改给团队、查看同事的修改、合并他人的修改
7. 分支管理 — 创建和切换工作副本（分支）
8. 冲突处理 — 检测和解决多人同时编辑产生的冲突
9. 同步仓库 — 推送修改到远程、拉取最新内容
10. 仓库管理 — 初始化和创建文档仓库

## 注意事项
- 操作前先确认用户意图，如果模糊可以追问
- 涉及恢复/合并等不可逆操作时，先提醒用户确认
- 出现冲突时，用通俗语言解释冲突原因，并给出建议
- 每次操作完成后，可以建议用户接下来可能想做的事情
- 绝对不要向用户暴露任何 git 命令或技术术语

## 输出要求
当你需要执行操作时，请调用对应的工具函数。系统会自动执行并将结果返回给你。
然后你需要把执行结果用用户友好的语言转述给用户。
`)

	// 2. 动态注入 Skills/Rules
	if a.promptKit != nil {
		intents := []string{}
		if intent != "" {
			intents = append(intents, intent)
		}
		dynamicPrompt := a.promptKit.GetPromptForIntents(intents)
		if dynamicPrompt != "" {
			sb.WriteString("\n")
			sb.WriteString(dynamicPrompt)
		}
	}

	return sb.String()
}

// detectIntentType 检测用户输入的意图类型
// 复用 interpreter 的解析结果，返回意图名称字符串
func (a *Agent) detectIntentType(input string) string {
	intent, err := a.interpreter.Parse(input)
	if err != nil || intent == nil {
		return ""
	}
	return string(intent.Type)
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
	
	// 上下文感知优化：如果 chat history 中已有 view_diff 的结果，
	// 从工具列表中移除 view_diff，避免 LLM 重复查看而不执行操作
	if a.hasToolResultInHistory("view_diff") {
		filtered := make([]string, 0, len(relevantNames))
		for _, name := range relevantNames {
			if name != "view_diff" && name != "detect_conflict" {
				filtered = append(filtered, name)
			}
		}
		relevantNames = filtered
	}
	
	// 根据名称筛选工具，并将操作类工具排在前面
	// 这样 LLM 更倾向于调用操作类工具而非查看类工具
	nameSet := make(map[string]bool, len(relevantNames))
	for _, name := range relevantNames {
		nameSet[name] = true
	}
	
	var actionTools []llms.Tool
	var viewTools []llms.Tool
	for _, tool := range a.toolDefs {
		if nameSet[tool.Function.Name] {
			if strings.HasPrefix(tool.Function.Name, "view_") || tool.Function.Name == "detect_conflict" {
				viewTools = append(viewTools, tool)
			} else {
				actionTools = append(actionTools, tool)
			}
		}
	}
	
	// 操作类工具排在前面，查看类工具排在后面
	result := append(actionTools, viewTools...)
	
	// 如果筛选结果为空，回退到全量工具
	if len(result) == 0 {
		return a.toolDefs
	}
	
	return result
}

// hasToolResultInHistory 检查 chat history 中是否已有某个工具的执行结果
// 用于避免 LLM 重复调用查看类工具
func (a *Agent) hasToolResultInHistory(toolName string) bool {
	for _, msg := range a.chatHistory {
		if msg.Role == llms.ChatMessageTypeTool {
			for _, part := range msg.Parts {
				if resp, ok := part.(llms.ToolCallResponse); ok {
					if resp.Name == toolName {
						return true
					}
				}
			}
		}
	}
	return false
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

	// 智能工具筛选：先用本地 interpreter 预判意图，只发送相关工具给 LLM
	// 这对于小模型（如 7B）特别重要，工具太多会导致模型不调用任何工具
	a.currentTools = a.selectRelevantTools(input)

	// 按需注入意图相关的 Skills/Rules
	// 在用户输入前插入一条系统提示，告知 LLM 当前操作应遵循的规则
	if a.promptKit != nil {
		intent := a.detectIntentType(input)
		intentPrompt := a.promptKit.GetPromptForIntents([]string{intent})
		if intentPrompt != "" {
			a.chatHistory = append(a.chatHistory,
				llms.TextParts(llms.ChatMessageTypeSystem,
					"[当前操作的适用规则]\n"+intentPrompt),
			)
		}
	}

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
	return a.handleLLMResponseWithUsage(ctx, resp, iteration, nil)
}

// handleLLMResponseWithUsage 处理 LLM 响应，支持跨多轮 ReAct 累加 token 用量
func (a *Agent) handleLLMResponseWithUsage(ctx context.Context, resp *llms.ContentResponse, iteration int, accumUsage *llm.Usage, callHistory ...map[string]int) *AgentResponse {
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

	// 如果有传入的累积用量，先复制
	if accumUsage != nil {
		totalUsage = *accumUsage
	}

	// 从 LLM 响应的 GenerationInfo 中提取 Token 用量
	// langchaingo 将 usage 信息放在 ContentChoice.GenerationInfo 中
	if choice.GenerationInfo != nil {
		if v, ok := choice.GenerationInfo["PromptTokens"]; ok {
			if n, ok := v.(int); ok {
				totalUsage.PromptTokens += n
			}
		}
		if v, ok := choice.GenerationInfo["CompletionTokens"]; ok {
			if n, ok := v.(int); ok {
				totalUsage.CompletionTokens += n
			}
		}
		if v, ok := choice.GenerationInfo["TotalTokens"]; ok {
			if n, ok := v.(int); ok {
				totalUsage.TotalTokens += n
			}
		}
	}
	// 如果 TotalTokens 为 0，用 PromptTokens + CompletionTokens 计算
	if totalUsage.TotalTokens == 0 && (totalUsage.PromptTokens > 0 || totalUsage.CompletionTokens > 0) {
		totalUsage.TotalTokens = totalUsage.PromptTokens + totalUsage.CompletionTokens
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
		history := make(map[string]int)
		if len(callHistory) > 0 && callHistory[0] != nil {
			for k, v := range callHistory[0] {
				history[k] = v
			}
		}
		return a.handleLangChainToolCalls(ctx, toolCalls, &totalUsage, iteration, history)
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
// 这是 ReAct 模式的核心：LLM 决定调用什么工具 → 执行工具 → 结果回传 LLM → LLM 生成最终回复
// iteration: 当前迭代次数，用于防止无限循环
// toolCallHistory: 跨轮次的工具调用历史（toolName → 调用次数），用于检测跨轮次重复调用
func (a *Agent) handleLangChainToolCalls(ctx context.Context, toolCalls []llms.ToolCall, totalUsage *llm.Usage, iteration int, toolCallHistory ...map[string]int) *AgentResponse {
	// 合并跨轮次的工具调用历史
	callHistory := make(map[string]int)
	if len(toolCallHistory) > 0 && toolCallHistory[0] != nil {
		for k, v := range toolCallHistory[0] {
			callHistory[k] = v
		}
	}

	// 检测是否有"终止性工具"成功执行
	// save_version 和 submit_change 成功后，操作已完成，LLM 不需要再调用任何工具
	terminalToolCalled := false
	terminalTools := map[string]bool{
		"save_version":   true,
		"submit_change":  true,
		"push_to_remote": true,
		"restore_version": true,
	}

	for _, tc := range toolCalls {
		a.setState(StateExecuting)

		// 获取工具名称和参数（FunctionCall 是指针类型）
		toolName := ""
		toolArgs := ""
		if tc.FunctionCall != nil {
			toolName = tc.FunctionCall.Name
			toolArgs = tc.FunctionCall.Arguments
		}

		// 记录工具调用（跨轮次累计）
		callHistory[toolName]++

		// 检测跨轮次重复调用：区分查看类工具和操作类工具
		// 查看类工具（view_*）是幂等的，重复调用毫无意义，2次即终止
		// 操作类工具允许稍多重试，3次终止
		maxCallsForTool := 3
		if strings.HasPrefix(toolName, "view_") || toolName == "detect_conflict" {
			maxCallsForTool = 2
		}
		if callHistory[toolName] > maxCallsForTool {
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
				if authErr.AuthType == "ssh" {
					result = "推送失败：当前远程仓库使用 SSH 协议连接，但 SSH 密钥认证未成功。请用通俗语言告诉用户推送失败了，并提供以下两种解决方案（让用户选择）：方案一（推荐，更简单）：将远程仓库地址改为 HTTPS 格式，然后使用访问令牌认证。步骤：1) 说明需要改为 HTTPS 地址；2) 以 GitHub 为例，HTTPS 地址格式为 https://github.com/用户名/仓库名.git；3) 引导用户获取访问令牌（登录 GitHub → 右上角头像 → Settings → Developer settings → Personal access tokens → Tokens (classic) → Generate new token，勾选 repo 权限）；4) 提示用户告诉我 HTTPS 地址、用户名和令牌，我会重新推送。方案二：配置 SSH 密钥。建议联系技术同事帮忙配置。注意：不要建议执行命令行操作，不要提及技术术语。"
				} else {
					result = "推送失败：远程仓库需要身份验证，但当前没有配置有效的认证方式。请用通俗语言告诉用户推送失败了，然后引导用户获取访问令牌来重试，步骤如下：1) 说明需要身份验证；2) 简要说明获取访问令牌的步骤（以 GitHub 为例：登录 GitHub → 右上角头像 → Settings → Developer settings → Personal access tokens → Tokens (classic) → Generate new token，勾选 repo 权限，生成并复制令牌）；3) 提示用户告诉我用户名和令牌，我会用它们重新推送；4) 如果用户不想自己操作，建议联系技术同事帮忙。注意：不要提及 SSH、密钥等技术术语，不要建议执行命令行操作。"
				}
			} else {
				result = fmt.Sprintf("操作失败：%s", translateError(err))
			}
		}

		// 检测终止性工具：成功执行后标记操作已完成
		if terminalTools[toolName] && err == nil {
			terminalToolCalled = true
		}

		// 将工具执行结果加入对话历史
		// 对重复调用的查看类工具，在结果中附加终止提示，引导 LLM 直接生成回复
		resultContent := result
		if callHistory[toolName] >= 2 && (strings.HasPrefix(toolName, "view_") || toolName == "detect_conflict") {
			resultContent = result + "\n\n[SYSTEM NOTICE: You have already called " + toolName + " " + fmt.Sprintf("%d", callHistory[toolName]) + " times. Do NOT call this tool again. Use the information above to generate your final response to the user NOW.]"
		}
		a.chatHistory = append(a.chatHistory, llms.MessageContent{
			Role: llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{
				llms.ToolCallResponse{
					ToolCallID: tc.ID,
					Name:       toolName,
					Content:    resultContent,
				},
			},
		})
	}

	// 将工具执行结果回传 LLM，让它生成最终回复
	a.setState(StateThinking)

	// 关键优化：如果终止性工具已成功执行，从工具列表中移除查看类工具
	// 这样 LLM 仍能进行第二次 save_version（分批提交场景），但不会反复 view_diff 确认
	toolsForLLM := a.currentTools
	if terminalToolCalled {
		// 过滤掉查看类工具，只保留操作类工具
		var filteredTools []llms.Tool
		for _, tool := range a.currentTools {
			name := tool.Function.Name
			if !strings.HasPrefix(name, "view_") && name != "detect_conflict" {
				filteredTools = append(filteredTools, tool)
			}
		}
		toolsForLLM = filteredTools
		// 如果过滤后没有工具了，设为 nil 让 LLM 生成纯文本
		if len(toolsForLLM) == 0 {
			toolsForLLM = nil
		}
	}

	// 构建 LLM 调用选项
	opts := []llms.CallOption{
		llms.WithTemperature(0.1),
		llms.WithMaxTokens(a.llmConfig.MaxTokens),
		openai.WithLegacyMaxTokensField(), // 兼容 Ollama、DeepSeek 等非 OpenAI API
	}
	if len(toolsForLLM) > 0 {
		opts = append(opts, llms.WithTools(toolsForLLM), llms.WithToolChoice("auto"))
	}

	resp, err := a.langchainLLM.GenerateContent(
		ctx,
		a.chatHistory,
		opts...,
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
	return a.handleLLMResponseWithUsage(ctx, resp, nextIteration, totalUsage, callHistory)
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
				Message: "抱歉，推送失败了！远程仓库需要身份验证，但当前还没有配置有效的认证方式。",
				Details: err.Error(),
				State:   StateError,
				Suggestions: []string{
					"获取 GitHub 访问令牌：登录 GitHub → 右上角头像 → Settings → Developer settings → Personal access tokens → Generate new token（勾选 repo 权限）",
					"获取令牌后，告诉我用户名和令牌，我就可以帮您重新推送",
					"如果不想自己操作，可以联系技术同事帮忙配置访问权限",
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
		if err := a.ensureUserConfig(); err != nil {
			return nil, err
		}
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

		// 检查 ahead/behind 状态
		aheadBehind, _ := a.gitWrapper.GetAheadBehind()
		if aheadBehind != nil && aheadBehind.Ahead == 0 && aheadBehind.Behind == 0 {
			return map[string]string{
				"status":  "up_to_date",
				"message": "本地和远程已完全同步，无需推送",
			}, nil
		}
		if aheadBehind != nil && aheadBehind.Ahead > 0 {
			// 有领先提交，执行推送
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
		commitHash := step.Params["commit_hash"]
		if commitHash != "" {
			return a.gitWrapper.CommitDiff(commitHash)
		}
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

	case planner.StepUpdateUserInfo:
		name := step.Params["name"]
		email := step.Params["email"]
		if name == "" && email == "" {
			return nil, fmt.Errorf("请提供用户名或邮箱信息")
		}
		a.UpdateUserInfo(name, email)
		result := map[string]string{}
		if name != "" {
			result["name"] = name
		}
		if email != "" {
			result["email"] = email
		}
		return result, nil

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

// IsUserConfigured 检查用户信息是否已配置（名字和邮箱都不为空）
func (a *Agent) IsUserConfigured() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.userConfig.Name != "" && a.userConfig.Email != ""
}

// UpdateUserInfo 更新用户信息（名字和/或邮箱）
func (a *Agent) UpdateUserInfo(name, email string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if name != "" {
		a.userConfig.Name = name
	}
	if email != "" {
		a.userConfig.Email = email
	}
	// 持久化用户信息到 .git/config
	if a.gitWrapper != nil {
		_ = a.gitWrapper.SetLocalUserConfig(a.userConfig.Name, a.userConfig.Email)
	}
}

// LoadLocalUserConfig 从 .git/config 加载已保存的用户信息
// 如果本地仓库配置中有 user.name 和 user.email，则覆盖内存中的配置
func (a *Agent) LoadLocalUserConfig() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.gitWrapper == nil {
		return
	}
	name, email, err := a.gitWrapper.GetLocalUserConfig()
	if err != nil {
		return
	}
	// 仅当内存中的配置为空时，才使用 .git/config 中的值
	if a.userConfig.Name == "" && name != "" {
		a.userConfig.Name = name
	}
	if a.userConfig.Email == "" && email != "" {
		a.userConfig.Email = email
	}
}

// ensureUserConfig 检查用户信息是否已配置，如果未配置返回错误提示
func (a *Agent) ensureUserConfig() error {
	if a.userConfig.Name == "" || a.userConfig.Email == "" {
		missing := []string{}
		if a.userConfig.Name == "" {
			missing = append(missing, "名字")
		}
		if a.userConfig.Email == "" {
			missing = append(missing, "邮箱")
		}
		return fmt.Errorf("请先配置您的用户信息（缺少：%s），这样团队成员才能知道是谁提交的修改。您可以说「我的名字是张三，邮箱是 zhangsan@example.com」来设置", strings.Join(missing, "和"))
	}
	return nil
}

// Close 关闭 Agent，释放资源
func (a *Agent) Close() {
	// 关闭 Skills/Rules 热加载监听器
	if a.promptKit != nil {
		a.promptKit.Close()
	}
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

// formatVersionTable 将 []gitwrapper.VersionInfo 硬编码格式化为 Markdown 表格
// 这是项目的基本功能，不依赖 SKILL/RULE，确保提交记录展示格式统一
func formatVersionTable(versions []gitwrapper.VersionInfo) string {
	if len(versions) == 0 {
		return "暂无历史记录"
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("| ID | 提交 Hash | 提交人 | 时间 | 修改内容 |\n")
	sb.WriteString("|-----|----------|--------|------|----------|\n")

	for idx, v := range versions {
		// 时间精确到秒
		dateStr := v.Date.Format("2006-01-02 15:04:05")
		// 清理 message 中的换行
		message := strings.TrimSpace(strings.ReplaceAll(v.Message, "\n", " "))
		// 截断过长的 message
		if len([]rune(message)) > 50 {
			message = string([]rune(message)[:50]) + "..."
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s |\n", idx+1, v.ShortHash, v.Author, dateStr, message))
	}

	sb.WriteString(fmt.Sprintf("\n共 %d 条提交记录。", len(versions)))
	return sb.String()
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

// isVagueCommitMessage 检查 commit message 是否过于笼统
// 返回 true 表示 message 太笼统，应该拒绝
func isVagueCommitMessage(message string) bool {
	if message == "" {
		return true
	}

	// 提取 conventional commit 的 summary 部分（去掉 type: 前缀）
	summary := message
	if idx := strings.Index(message, ":"); idx >= 0 && idx < 20 {
		summary = strings.TrimSpace(message[idx+1:])
	}

	// 笼统模式列表：这些短语出现在 summary 中则判定为笼统
	vaguePatterns := []string{
		"update files",
		"update code",
		"update source code",
		"update source code files",
		"update documentation files",
		"update doc files",
		"save changes",
		"save pending changes",
		"submit changes",
		"commit changes",
		"add changes",
		"make changes",
		"modify files",
		"modify code",
		"change files",
		"change code",
	}

	lowerSummary := strings.ToLower(summary)
	for _, pattern := range vaguePatterns {
		if lowerSummary == pattern || strings.Contains(lowerSummary, pattern) {
			return true
		}
	}

	// 如果 summary 太短（少于 10 个字符），也可能太笼统
	if len(summary) < 10 {
		return true
	}

	return false
}

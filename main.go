package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/chzyer/readline"
	"github.com/jackz-jones/git-agent/internal"
	"github.com/jackz-jones/git-agent/internal/agent"
)

// completer 提供简单的命令自动补全
var completer = readline.NewPrefixCompleter(
	readline.PcItem("/mode"),
	readline.PcItem("/mode", readline.PcItem("local"), readline.PcItem("llm")),
	readline.PcItem("/clear"),
	readline.PcItem("/reset"),
	readline.PcItem("exit"),
	readline.PcItem("quit"),
)

// ANSI 颜色常量
const (
	colorReset = "\033[0m"
	colorGray  = "\033[90m"
	colorCyan  = "\033[36m"

	// 组合样式
	styleSuccess = "\033[1;32m" // 加粗绿色
	styleError   = "\033[1;31m" // 加粗红色
	styleWarn    = "\033[1;33m" // 加粗黄色
	styleInfo    = "\033[1;36m" // 加粗青色
	stylePrompt  = "\033[1;34m" // 加粗蓝色（输入提示符）
	styleDim     = "\033[2m"    // 暗淡
	styleBold    = "\033[1m"    // 加粗
)

func main() {
	// 命令行参数
	llmAPIKey := flag.String("api-key", "", "LLM API Key（也可通过 GIT_AGENT_API_KEY 环境变量设置）")
	llmBaseURL := flag.String("base-url", "", "LLM API Base URL（也可通过 GIT_AGENT_BASE_URL 环境变量设置）")
	llmModel := flag.String("model", "", "LLM 模型名称（也可通过 GIT_AGENT_MODEL 环境变量设置）")
	repoPath := flag.String("repo", ".", "仓库路径")
	showHelp := flag.Bool("help", false, "显示帮助")
	showVersion := flag.Bool("version", false, "显示版本")

	flag.Parse()

	if *showHelp {
		printHelp()
		return
	}

	if *showVersion {
		fmt.Println(internal.VersionInfo())
		return
	}

	// 如果有子命令
	if flag.NArg() > 0 {
		cmd := flag.Arg(0)
		switch cmd {
		case "help":
			printHelp()
		case "version":
			fmt.Println(internal.VersionInfo())
		default:
			fmt.Printf("%s未知命令: %s%s\n", styleError, cmd, colorReset)
			fmt.Println("直接运行 git-agent 进入交互模式，或使用 git-agent --help 查看帮助")
			os.Exit(1)
		}
		return
	}

	runInteractive(*repoPath, *llmAPIKey, *llmBaseURL, *llmModel)
}

// runInteractive 启动交互式 Agent 模式
func runInteractive(repoPath, apiKey, baseURL, model string) {
	// 简洁启动横幅
	fmt.Printf("\n%s%sGit Agent%s  ", styleBold, colorCyan, colorReset)
	if internal.Version != "" {
		fmt.Printf("%sv%s", colorGray, internal.Version)
		if internal.CommitID != "" {
			fmt.Printf("(%s)", internal.ShortCommitID())
		}
	}
	fmt.Println()

	// 构建 LLM 配置
	llmConfig := buildLLMConfig(apiKey, baseURL, model)

	// 模式状态
	if llmConfig.Enabled {
		fmt.Printf("  %s🧠 LLM %s@ %s%s\n", styleInfo, llmConfig.Model, maskURL(llmConfig.BaseURL), colorReset)
	} else {
		fmt.Printf("  %s📝 本地模式%s  %s(配置 LLM 可获得更智能的体验: git-agent --help)%s\n", styleWarn, colorReset, colorGray, colorReset)
	}

	// 用户配置
	userConfig := &agent.UserConfig{
		Name:     os.Getenv("GIT_AGENT_USER"),
		Email:    os.Getenv("GIT_AGENT_EMAIL"),
		Role:     "editor",
		Language: "zh",
	}

	// 创建 Agent
	var a *agent.Agent
	var err error

	if llmConfig.Enabled {
		a, err = agent.NewWithLLM(repoPath, userConfig, llmConfig)
	} else {
		a, err = agent.New(repoPath, userConfig)
	}

	if err != nil {
		fmt.Printf("  %s初始化失败：%v%s\n", styleError, err, colorReset)
		fmt.Printf("  %s提示：输入「初始化仓库」创建新仓库%s\n", colorGray, colorReset)
	}

	// 从 .git/config 加载已保存的用户信息（优先级：环境变量 > .git/config）
	if a != nil {
		a.LoadLocalUserConfig()
	}

	// 检查用户信息是否已配置（此时已合并环境变量和 .git/config）
	if a != nil && (a.GetUserConfig().Name == "" || a.GetUserConfig().Email == "") {
		fmt.Println()
		fmt.Printf("  %s⚠ 用户信息未配置%s\n", styleWarn, colorReset)
		if a.GetUserConfig().Name == "" {
			fmt.Printf("  %sexport GIT_AGENT_USER=你的名字%s\n", colorGray, colorReset)
		}
		if a.GetUserConfig().Email == "" {
			fmt.Printf("  %sexport GIT_AGENT_EMAIL=你的邮箱%s\n", colorGray, colorReset)
		}
		fmt.Printf("  %s或在交互中告诉我您的名字和邮箱%s\n", colorGray, colorReset)
	}

	// 快捷提示
	fmt.Println()
	fmt.Printf("  %s输入「帮助」查看所有操作  输入「退出」结束会话%s\n", colorGray, colorReset)
	fmt.Println()

	reader, err := readline.NewEx(&readline.Config{
		Prompt:          fmt.Sprintf("%s>%s ", stylePrompt, colorReset),
		HistoryFile:     "/tmp/git-agent-history.tmp",
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		HistoryLimit:    1000,
		Stdout:          os.Stdout,
	})
	if err != nil {
		fmt.Printf("%s初始化输入失败：%v%s\n", styleError, err, colorReset)
		return
	}
	defer reader.Close()

	ctx := context.Background()

	for {
		// 根据模式动态更新提示符（不含 \n 和 emoji，避免 readline 宽度计算错乱）
		modeIndicator := "local"
		if a != nil && a.IsLLMEnabled() {
			modeIndicator = "llm"
		}
		reader.SetPrompt(fmt.Sprintf("%s[%s] >%s ", stylePrompt, modeIndicator, colorReset))

		line, err := reader.Readline()
		if err != nil {
			// readline.ErrInterrupt 表示 Ctrl+C，io.EOF 表示 Ctrl+D
			break
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		// 退出命令
		if input == "exit" || input == "quit" || input == "退出" {
			fmt.Printf("  %s再见！您的文件版本都安全保存着。%s\n", styleDim, colorReset)
			break
		}

		// 切换模式命令
		if strings.HasPrefix(input, "/mode") {
			handleModeSwitch(a, input)
			continue
		}

		// 清空对话
		if input == "/clear" || input == "/reset" {
			if a != nil {
				a.ClearConversation()
				fmt.Printf("  %s对话历史已清空%s\n", styleInfo, colorReset)
			}
			continue
		}

		// 通过 Agent 处理
		if a != nil {
			response := a.Process(ctx, input)
			printResponse(response)
		} else {
			fmt.Printf("  %sAgent 未初始化，请先输入「初始化仓库」%s\n", styleError, colorReset)
		}
	}

	if a != nil {
		a.Close()
	}
}

// printResponse 打印 Agent 响应 - 使用颜色区分消息类型
func printResponse(resp *agent.AgentResponse) {
	if resp == nil {
		return
	}

	fmt.Println()

	// 根据 Success 字段选择颜色
	if resp.Success {
		fmt.Printf("  %s%s%s\n", styleSuccess, resp.Message, colorReset)
	} else {
		fmt.Printf("  %s%s%s\n", styleError, resp.Message, colorReset)
	}

	if resp.Details != "" {
		fmt.Printf("  %s详情：%s%s\n", colorGray, resp.Details, colorReset)
	}

	if resp.UsedLLM && resp.TokenUsage != nil {
		fmt.Printf("  %sToken 用量：%d（输入: %d, 输出: %d）%s\n",
			colorGray,
			resp.TokenUsage.TotalTokens,
			resp.TokenUsage.PromptTokens,
			resp.TokenUsage.CompletionTokens,
			colorReset,
		)
	}

	if len(resp.Suggestions) > 0 {
		fmt.Printf("  %s💡%s ", styleBold, colorReset)
		for i, s := range resp.Suggestions {
			if i > 0 {
				fmt.Printf("%s | %s", colorGray, colorReset)
			}
			fmt.Printf("%s%s%s", styleDim, s, colorReset)
		}
		fmt.Println()
	}
}

// handleModeSwitch 处理模式切换
func handleModeSwitch(a *agent.Agent, input string) {
	parts := strings.Fields(input)
	if len(parts) < 2 {
		fmt.Printf("  %s用法：%s/mode local | /mode llm\n", colorGray, colorReset)
		return
	}

	mode := parts[1]
	switch mode {
	case "local":
		fmt.Printf("  %s已切换到本地模式%s\n", styleWarn, colorReset)
	case "llm":
		if a != nil && a.IsLLMEnabled() {
			fmt.Printf("  %s当前已是 LLM 模式%s\n", styleInfo, colorReset)
		} else {
			fmt.Printf("  %sLLM 未配置%s\n", styleWarn, colorReset)
			fmt.Printf("  %sgit-agent --api-key YOUR_KEY --base-url YOUR_URL --model MODEL_NAME%s\n", colorGray, colorReset)
		}
	default:
		fmt.Printf("  %s未知模式：%s%s（可选：local, llm）\n", styleError, mode, colorReset)
	}
}

// buildLLMConfig 构建 LLM 配置
// 优先级：命令行参数 > 环境变量 > 默认值
func buildLLMConfig(apiKey, baseURL, model string) *agent.LLMConfig {
	// 优先使用命令行参数，然后是环境变量
	if apiKey == "" {
		apiKey = os.Getenv("GIT_AGENT_API_KEY")
	}
	if baseURL == "" {
		baseURL = os.Getenv("GIT_AGENT_BASE_URL")
	}
	if model == "" {
		model = os.Getenv("GIT_AGENT_MODEL")
	}

	enabled := apiKey != ""
	maxTokens := 4096
	if mt := os.Getenv("GIT_AGENT_MAX_TOKENS"); mt != "" {
		if v, err := strconv.Atoi(mt); err == nil {
			maxTokens = v
		}
	}

	return &agent.LLMConfig{
		Enabled:   enabled,
		APIKey:    apiKey,
		BaseURL:   baseURL,
		Model:     model,
		MaxTokens: maxTokens,
	}
}

// maskURL 隐藏 URL 中的敏感信息
func maskURL(url string) string {
	if url == "" {
		return "https://api.openai.com/v1"
	}
	// 只显示域名部分
	parts := strings.Split(url, "//")
	if len(parts) >= 2 {
		domainParts := strings.Split(parts[1], "/")
		return domainParts[0]
	}
	return "***"
}

// printHelp 打印帮助信息
func printHelp() {
	fmt.Printf("\n%sGit Agent%s  %s文件版本管理助手%s\n\n", styleBold, colorReset, colorCyan, colorReset)

	fmt.Printf("%s用法%s\n", styleBold, colorReset)
	fmt.Println("  git-agent                              启动交互模式（本地模式）")
	fmt.Println("  git-agent --api-key KEY                启动交互模式（LLM 模式）")
	fmt.Println("  git-agent --api-key KEY --base-url URL  指定 LLM API 地址")
	fmt.Println("  git-agent --api-key KEY --model MODEL   指定 LLM 模型")

	fmt.Printf("\n%s参数%s\n", styleBold, colorReset)
	fmt.Println("  --api-key    LLM API Key")
	fmt.Println("  --base-url   LLM API Base URL（默认 https://api.openai.com/v1）")
	fmt.Println("  --model      LLM 模型名称（默认 gpt-4o）")
	fmt.Println("  --repo       仓库路径（默认当前目录）")

	fmt.Printf("\n%s环境变量%s\n", styleBold, colorReset)
	fmt.Printf("  GIT_AGENT_API_KEY       LLM API Key\n")
	fmt.Printf("  GIT_AGENT_BASE_URL      LLM API Base URL\n")
	fmt.Printf("  GIT_AGENT_MODEL         LLM 模型名称\n")
	fmt.Printf("  GIT_AGENT_MAX_TOKENS    最大 token 数（默认 4096）\n")
	fmt.Printf("  %sGIT_AGENT_USER         用户名（必填）%s\n", styleWarn, colorReset)
	fmt.Printf("  %sGIT_AGENT_EMAIL        用户邮箱（必填）%s\n", styleWarn, colorReset)
	fmt.Printf("  GIT_HTTP_USERNAME      HTTPS 认证用户名（推送时使用）\n")
	fmt.Printf("  GIT_HTTP_PASSWORD      HTTPS 认证密码/令牌（推送时使用）\n")

	fmt.Printf("\n%s交互命令%s\n", styleBold, colorReset)
	fmt.Println("  /mode local   切换到本地模式")
	fmt.Println("  /mode llm     切换到 LLM 模式")
	fmt.Println("  /clear        清空对话历史")
	fmt.Println("  exit/quit     退出")

	fmt.Printf("\n%s自然语言示例%s\n", styleBold, colorReset)
	fmt.Println("  保存修改          保存当前文件为新版本")
	fmt.Println("  查看历史          查看修改记录")
	fmt.Println("  恢复版本          回到之前的版本")
	fmt.Println("  查看状态          查看当前修改情况")
	fmt.Println("  看看小李改了什么   查看他人的修改")
	fmt.Println("  提交给团队        提交修改供团队审核")

	fmt.Printf("\n%s启用 LLM 模式%s\n", styleBold, colorReset)
	fmt.Println("  # OpenAI")
	fmt.Println("  git-agent --api-key sk-xxx --model gpt-4o")
	fmt.Println()
	fmt.Println("  # 国产模型（如 DeepSeek）")
	fmt.Println("  git-agent --api-key sk-xxx --base-url https://api.deepseek.com/v1 --model deepseek-chat")
	fmt.Println()
	fmt.Println("  # Azure OpenAI")
	fmt.Println("  git-agent --api-key YOUR_KEY --base-url https://YOUR.openai.azure.com/openai/deployments/YOUR_MODEL --model gpt-4o")
	fmt.Println()
}

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

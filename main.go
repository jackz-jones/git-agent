package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jackz-jones/git-agent/internal"
	"github.com/jackz-jones/git-agent/internal/agent"
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
			fmt.Printf("未知命令: %s\n", cmd)
			fmt.Println("直接运行 git-agent 进入交互模式，或使用 git-agent --help 查看帮助")
			os.Exit(1)
		}
		return
	}

	runInteractive(*repoPath, *llmAPIKey, *llmBaseURL, *llmModel)
}

// runInteractive 启动交互式 Agent 模式
func runInteractive(repoPath, apiKey, baseURL, model string) {
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Printf("║     🤖 Git Agent - 文件版本管理助手      ║\n")
	fmt.Printf("║         让版本控制像保存一样简单          ║\n")
	fmt.Println("╚══════════════════════════════════════════╝")

	// 显示版本信息
	if internal.Version != "" {
		fmt.Printf("📌 版本：%s", internal.Version)
		if internal.CommitID != "" {
			fmt.Printf("（%s）", internal.ShortCommitID())
		}
		fmt.Println()
	}
	fmt.Println()

	// 构建 LLM 配置
	llmConfig := buildLLMConfig(apiKey, baseURL, model)

	if llmConfig.Enabled {
		fmt.Printf("🧠 LLM 模式已启用（%s @ %s）\n", llmConfig.Model, maskURL(llmConfig.BaseURL))
		fmt.Println("   您可以直接用自然语言对话，Agent 会智能理解您的需求。")
	} else {
		fmt.Println("📝 本地模式（关键词匹配）")
		fmt.Println("   提示：配置 LLM 可获得更智能的交互体验。运行 git-agent --help 查看配置方式。")
	}
	fmt.Println()

	fmt.Println("💡 用自然语言告诉我你想做什么，例如：")
	fmt.Println("   • 保存当前修改")
	fmt.Println("   • 查看修改历史")
	fmt.Println("   • 看看小李改了什么")
	fmt.Println("   • 恢复昨天的版本")
	fmt.Println("   • 输入「帮助」查看更多操作")
	fmt.Println()

	// 用户配置
	userConfig := &agent.UserConfig{
		Name:     os.Getenv("GIT_AGENT_USER"),
		Email:    os.Getenv("GIT_AGENT_EMAIL"),
		Role:     "editor",
		Language: "zh",
	}

	// 检查用户信息是否已配置
	if userConfig.Name == "" || userConfig.Email == "" {
		fmt.Println("⚠️  您还未配置用户信息，提交记录将无法标识您的身份。")
		fmt.Println("   请通过环境变量设置：")
		if userConfig.Name == "" {
			fmt.Println("   export GIT_AGENT_USER=你的名字")
		}
		if userConfig.Email == "" {
			fmt.Println("   export GIT_AGENT_EMAIL=你的邮箱")
		}
		fmt.Println("   或者在交互中告诉我您的名字和邮箱，我会帮您设置。")
		fmt.Println()
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
		fmt.Printf("❌ 初始化失败：%v\n", err)
		fmt.Println("💡 提示：如果您想创建新仓库，请输入「初始化仓库」")
	}

	reader := bufio.NewReader(os.Stdin)
	ctx := context.Background()

	for {
		// 状态指示器
		modeIndicator := "📝"
		if a != nil && a.IsLLMEnabled() {
			modeIndicator = "🧠"
		}
		fmt.Printf("\n%s 您想做什么？ ", modeIndicator)

		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("读取输入失败：%v\n", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// 退出命令
		if input == "exit" || input == "quit" || input == "退出" {
			fmt.Println("👋 再见！您的文件版本都安全保存着。")
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
				fmt.Println("🧹 对话历史已清空")
			}
			continue
		}

		// 通过 Agent 处理
		if a != nil {
			response := a.Process(ctx, input)
			printResponse(response)
		} else {
			fmt.Println("❌ Agent 未初始化，请先输入「初始化仓库」")
		}
	}

	if a != nil {
		a.Close()
	}
}

// printResponse 打印 Agent 响应
func printResponse(resp *agent.AgentResponse) {
	if resp == nil {
		return
	}

	fmt.Println()
	fmt.Println(resp.Message)

	if resp.Details != "" {
		fmt.Printf("  📝 详情：%s\n", resp.Details)
	}

	if resp.UsedLLM && resp.TokenUsage != nil {
		fmt.Printf("  🔋 Token 用量：%d (prompt: %d, completion: %d)\n",
			resp.TokenUsage.TotalTokens,
			resp.TokenUsage.PromptTokens,
			resp.TokenUsage.CompletionTokens,
		)
	}

	if len(resp.Suggestions) > 0 {
		fmt.Println("  💡 您可能还想：")
		for _, s := range resp.Suggestions {
			fmt.Printf("     • %s\n", s)
		}
	}
}

// handleModeSwitch 处理模式切换
func handleModeSwitch(a *agent.Agent, input string) {
	parts := strings.Fields(input)
	if len(parts) < 2 {
		fmt.Println("用法：/mode local | /mode llm")
		return
	}

	mode := parts[1]
	switch mode {
	case "local":
		fmt.Println("📝 已切换到本地模式（关键词匹配）")
		// 注：切换模式需要重建 Agent，这里仅做提示
	case "llm":
		if a != nil && a.IsLLMEnabled() {
			fmt.Println("🧠 当前已是 LLM 模式")
		} else {
			fmt.Println("⚠️ LLM 未配置。请使用以下方式启动 LLM 模式：")
			fmt.Println("   git-agent --api-key YOUR_KEY --base-url YOUR_URL --model MODEL_NAME")
		}
	default:
		fmt.Printf("未知模式：%s（可选：local, llm）\n", mode)
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
	fmt.Println("Git Agent - 文件版本管理助手")
	fmt.Println()
	fmt.Println("用法：")
	fmt.Println("  git-agent                              启动交互模式（本地模式）")
	fmt.Println("  git-agent --api-key KEY                启动交互模式（LLM 模式）")
	fmt.Println("  git-agent --api-key KEY --base-url URL  指定 LLM API 地址")
	fmt.Println("  git-agent --api-key KEY --model MODEL   指定 LLM 模型")
	fmt.Println()
	fmt.Println("参数：")
	fmt.Println("  --api-key    LLM API Key")
	fmt.Println("  --base-url   LLM API Base URL（默认 https://api.openai.com/v1）")
	fmt.Println("  --model      LLM 模型名称（默认 gpt-4o）")
	fmt.Println("  --repo       仓库路径（默认当前目录）")
	fmt.Println()
	fmt.Println("环境变量：")
	fmt.Println("  GIT_AGENT_API_KEY       LLM API Key")
	fmt.Println("  GIT_AGENT_BASE_URL      LLM API Base URL")
	fmt.Println("  GIT_AGENT_MODEL         LLM 模型名称")
	fmt.Println("  GIT_AGENT_MAX_TOKENS    最大 token 数（默认 4096）")
	fmt.Println("  GIT_AGENT_USER          用户名（必填）")
	fmt.Println("  GIT_AGENT_EMAIL         用户邮箱（必填）")
	fmt.Println()
	fmt.Println("交互模式命令：")
	fmt.Println("  /mode local   切换到本地模式")
	fmt.Println("  /mode llm     切换到 LLM 模式")
	fmt.Println("  /clear        清空对话历史")
	fmt.Println("  exit/quit     退出")
	fmt.Println()
	fmt.Println("交互模式中，您可以直接用自然语言操作，例如：")
	fmt.Println("  保存修改          保存当前文件为新版本")
	fmt.Println("  查看历史          查看修改记录")
	fmt.Println("  恢复版本          回到之前的版本")
	fmt.Println("  查看状态          查看当前修改情况")
	fmt.Println("  看看小李改了什么   查看他人的修改")
	fmt.Println("  提交给团队        提交修改供团队审核")
	fmt.Println()
	fmt.Println("示例 - 启用 LLM 模式：")
	fmt.Println("  # OpenAI")
	fmt.Println("  git-agent --api-key sk-xxx --model gpt-4o")
	fmt.Println()
	fmt.Println("  # 国产模型（如 DeepSeek）")
	fmt.Println("  git-agent --api-key sk-xxx --base-url https://api.deepseek.com/v1 --model deepseek-chat")
	fmt.Println()
	fmt.Println("  # Azure OpenAI")
	fmt.Println("  git-agent --api-key YOUR_KEY --base-url https://YOUR.openai.azure.com/openai/deployments/YOUR_MODEL --model gpt-4o")
}

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

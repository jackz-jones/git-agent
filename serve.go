package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackz-jones/git-agent/internal/agent"
	"github.com/jackz-jones/git-agent/internal/web"
)

// runServe 解析 serve 子命令参数并启动 Web 服务。
// 参数格式：git-agent serve [--port N] [--host H] [--open=false]
func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", 8088, "Web 服务监听端口（默认 8088）")
	host := fs.String("host", "127.0.0.1", "Web 服务监听地址（默认 127.0.0.1，仅本机可访问）")
	openBrowser := fs.Bool("open", true, "启动后是否自动打开浏览器")
	allowLAN := fs.Bool("i-know-what-i-do", false, "允许监听非 127.0.0.1 地址（开放到局域网，需自行承担风险）")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// 安全校验：非 127.0.0.1/localhost 需要显式放行标志
	if *host != "127.0.0.1" && *host != "localhost" && !*allowLAN {
		return fmt.Errorf("拒绝监听非本机地址 %q；如确需对局域网开放，请追加 --i-know-what-i-do 参数", *host)
	}

	opts := web.Options{
		Host:     *host,
		Port:     *port,
		Open:     *openBrowser,
		AllowLAN: *allowLAN,
	}

	// 加载持久化配置（~/.git-agent/config.json 与 recent.json）
	configDir := web.DefaultConfigDir()
	cfgStore, err := web.NewConfigStore(configDir)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	recentStore, err := web.NewRecentStore(configDir)
	if err != nil {
		return fmt.Errorf("加载最近打开列表失败: %w", err)
	}

	// 构建默认用户/LLM 配置：环境变量 > 配置文件 > 默认值
	userCfg, llmCfg := mergeDefaultConfigs(cfgStore.Snapshot())

	manager := web.NewManager(userCfg, llmCfg)
	// 打开成功时写入最近打开列表
	manager.SetOnOpen(func(ws *web.Workspace) {
		if err := recentStore.Add(ws.Path); err != nil {
			fmt.Printf("  %s记录最近打开列表失败：%v%s\n", colorGray, err, colorReset)
		}
	})
	srv := web.NewServer(opts, manager).
		WithConfigStore(cfgStore).
		WithRecentStore(recentStore).
		WithAuditLogger(web.NewAuditLogger(configDir))

	// 捕获 Ctrl+C / SIGTERM 用于优雅退出
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 就绪后可选地打开浏览器
	if *openBrowser {
		go func() {
			select {
			case <-srv.Ready():
				// 延迟一点点，确保 Serve() 已真正开始接受连接
				time.Sleep(200 * time.Millisecond)
				openURL := srv.URL()
				if opts.AllowLAN && srv.Token() != "" {
					openURL = fmt.Sprintf("%s/?token=%s", srv.URL(), srv.Token())
				}
				if err := web.OpenBrowser(openURL); err != nil {
					fmt.Printf("  %s提示：自动打开浏览器失败（%v），请手动访问 %s%s\n",
						colorGray, err, openURL, colorReset)
				}
			case <-ctx.Done():
				return
			}
		}()
	}

	printServeBanner(srv.URL(), srv.Token(), opts.AllowLAN)

	if err := srv.Serve(ctx); err != nil {
		return err
	}
	fmt.Printf("  %s服务已停止%s\n", styleDim, colorReset)
	return nil
}

// printServeBanner 打印 serve 启动横幅。
func printServeBanner(url, token string, allowLAN bool) {
	fmt.Printf("\n%s%sGit Agent Web%s\n", styleBold, colorCyan, colorReset)
	if allowLAN && token != "" {
		// LAN 模式：将 Token 拼接到 URL 注量到剪贴板友好的链接
		fullURL := fmt.Sprintf("%s/?token=%s", url, token)
		fmt.Printf("  %s🌐 %s%s\n", styleInfo, fullURL, colorReset)
		fmt.Printf("  %s🔐 鉴权 Token：%s%s\n", styleInfo, token, colorReset)
		fmt.Printf("  %s❗️ LAN 模式已开启，请将链接仅分享给可信任的使用者%s\n", colorGray, colorReset)
	} else {
		fmt.Printf("  %s🌐 %s%s\n", styleInfo, url, colorReset)
	}
	fmt.Printf("  %s按 Ctrl+C 退出%s\n\n", colorGray, colorReset)
}

// mergeDefaultConfigs 按照"环境变量 > 配置文件 > 默认值"的优先级合并用户与 LLM 配置。
// 命令行参数目前不暴露 api-key 等（serve 模式鼓励走设置页持久化），
// 如果未来需要也可在 runServe 中再覆盖一层。
func mergeDefaultConfigs(cfg web.AppConfig) (*agent.UserConfig, *agent.LLMConfig) {
	userCfg := &agent.UserConfig{
		Name:     firstNonEmpty(os.Getenv("GIT_AGENT_USER"), cfg.User.Name),
		Email:    firstNonEmpty(os.Getenv("GIT_AGENT_EMAIL"), cfg.User.Email),
		Role:     "editor",
		Language: "zh",
	}

	apiKey := firstNonEmpty(os.Getenv("GIT_AGENT_API_KEY"), cfg.LLM.APIKey)
	baseURL := firstNonEmpty(os.Getenv("GIT_AGENT_BASE_URL"), cfg.LLM.BaseURL)
	model := firstNonEmpty(os.Getenv("GIT_AGENT_MODEL"), cfg.LLM.Model)

	maxTokens := cfg.LLM.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	if mt := os.Getenv("GIT_AGENT_MAX_TOKENS"); mt != "" {
		if v, err := strconv.Atoi(mt); err == nil {
			maxTokens = v
		}
	}

	// Enabled 判断：配置文件中显式启用 且 apiKey 非空 → 启用
	// 环境变量有 apiKey 时也视为启用（兼容 CLI 行为）
	enabled := cfg.LLM.Enabled && apiKey != "" || os.Getenv("GIT_AGENT_API_KEY") != ""

	llmCfg := &agent.LLMConfig{
		Enabled:   enabled,
		APIKey:    apiKey,
		BaseURL:   baseURL,
		Model:     model,
		MaxTokens: maxTokens,
	}
	return userCfg, llmCfg
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

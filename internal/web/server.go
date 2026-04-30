package web

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Options 是 Web 服务的启动参数。
type Options struct {
	Host string // 监听地址，默认 127.0.0.1
	Port int    // 监听端口，默认 8088
	Open bool   // 启动后是否自动打开浏览器
	// AllowLAN 允许非 127.0.0.1 监听（配合 Host=0.0.0.0 使用）。
	// 当为 true 且 Host 非 localhost 时，会启用 Token 鉴权（后续任务实现）。
	AllowLAN bool
}

// DefaultOptions 返回默认选项。
func DefaultOptions() Options {
	return Options{
		Host: "127.0.0.1",
		Port: 8088,
		Open: true,
	}
}

// Server 封装 HTTP 服务与生命周期管理。
// 后续任务会在此结构体上挂载 Workspace Manager、Config Store 等组件。
type Server struct {
	opts       Options
	mux        *http.ServeMux
	server     *http.Server
	workspaces *Manager
	config     *ConfigStore
	recent     *RecentStore
	audit      *AuditLogger

	// 就绪信号：Serve() 监听成功后关闭此通道，便于测试与 --open 时序控制。
	readyOnce sync.Once
	readyCh   chan struct{}
}

// NewServer 构造 Server。
// manager 允许为 nil（未来任务会填充），此时 /api/workspaces 端点未注册或返回空。
func NewServer(opts Options, manager *Manager) *Server {
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	if opts.Port == 0 {
		opts.Port = 8088
	}
	s := &Server{
		opts:       opts,
		mux:        http.NewServeMux(),
		workspaces: manager,
		readyCh:    make(chan struct{}),
	}
	s.registerRoutes()
	return s
}

// Workspaces 返回 Workspace 管理器。
func (s *Server) Workspaces() *Manager { return s.workspaces }

// WithConfigStore 注入全局配置存储。
func (s *Server) WithConfigStore(c *ConfigStore) *Server {
	s.config = c
	return s
}

// WithRecentStore 注入最近打开列表存储。
func (s *Server) WithRecentStore(r *RecentStore) *Server {
	s.recent = r
	return s
}

// ConfigStore 返回全局配置存储。
func (s *Server) ConfigStore() *ConfigStore { return s.config }

// RecentStore 返回最近打开列表。
func (s *Server) RecentStore() *RecentStore { return s.recent }

// WithAuditLogger 注入审计日志器。
func (s *Server) WithAuditLogger(a *AuditLogger) *Server {
	s.audit = a
	return s
}

// Audit 返回审计日志器。
func (s *Server) Audit() *AuditLogger { return s.audit }

// registerRoutes 注册全部路由。
// 后续任务会在此处挂载 /api/* 的具体 handler，这里先占位基础端点。
func (s *Server) registerRoutes() {
	// 健康检查
	s.mux.HandleFunc("/api/health", s.handleHealth)

	// /api/* 业务路由
	s.registerRoutesExtra()

	// 静态资源（前端产物）—— 必须最后注册，作为兜底
	s.mux.HandleFunc("/", s.handleStatic)
}

// handleHealth 返回服务基本信息。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = fmt.Fprintf(w, `{"status":"ok","service":"git-agent","time":"%s"}`, time.Now().Format(time.RFC3339))
}

// handleStatic 提供前端静态资源。
// 若 dist 下无真实前端产物，则返回占位 HTML。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	distSub, err := DistFS()
	if err != nil {
		s.servePlaceholder(w)
		return
	}

	// 判断 dist 是否有真实的 index.html
	if _, err := fs.Stat(distSub, "index.html"); err != nil {
		s.servePlaceholder(w)
		return
	}

	// SPA 路由回退：非存在的文件路径统一回退到 index.html
	reqPath := strings.TrimPrefix(r.URL.Path, "/")
	if reqPath == "" {
		reqPath = "index.html"
	}
	if _, err := fs.Stat(distSub, reqPath); err != nil {
		// 不存在 -> 回退 index.html（SPA 场景）
		reqPath = "index.html"
	}

	data, err := fs.ReadFile(distSub, reqPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentTypeByExt(reqPath))
	_, _ = w.Write(data)
}

func (s *Server) servePlaceholder(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(placeholderHTML))
}

// contentTypeByExt 根据扩展名推断 Content-Type（覆盖 embed 场景常见类型）。
func contentTypeByExt(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(name, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(name, ".woff"):
		return "font/woff"
	case strings.HasSuffix(name, ".map"):
		return "application/json; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// Addr 返回当前配置的 host:port 字符串。
func (s *Server) Addr() string {
	return net.JoinHostPort(s.opts.Host, fmt.Sprintf("%d", s.opts.Port))
}

// URL 返回浏览器可访问的 URL。
func (s *Server) URL() string {
	host := s.opts.Host
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d", host, s.opts.Port)
}

// Ready 返回一个 chan，监听就绪后关闭。
func (s *Server) Ready() <-chan struct{} {
	return s.readyCh
}

// Serve 启动 HTTP 服务并阻塞，直到 ctx 取消或发生错误。
// ctx 取消时会执行优雅关停。
func (s *Server) Serve(ctx context.Context) error {
	addr := s.Addr()

	// 先监听端口，便于区分 "端口占用" 与 "服务错误"。
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败（端口可能被占用）: %w", addr, err)
	}

	s.server = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 通知就绪
	s.readyOnce.Do(func() { close(s.readyCh) })

	// 监听 ctx 取消 -> 优雅 shutdown
	shutdownErrCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErrCh <- s.server.Shutdown(shutdownCtx)
	}()

	log.Printf("[web] listening on %s", s.URL())
	if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	// 等待 shutdown 完成
	shutdownErr := <-shutdownErrCh

	// 关闭全部 workspace，释放 Agent 资源
	if s.workspaces != nil {
		s.workspaces.CloseAll()
	}

	if shutdownErr != nil {
		return fmt.Errorf("优雅关停失败: %w", shutdownErr)
	}
	return nil
}

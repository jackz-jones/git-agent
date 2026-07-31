package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// AuthConfig 是 Web 服务鉴权配置。
//
// 设计目标（对应需求 8、18）：
//   - 仅 127.0.0.1/localhost 监听时：默认不启用 Token，但仍启用 Origin/Host 校验，防御 DNS Rebinding。
//   - AllowLAN=true（例如监听 0.0.0.0）时：必须携带 Bearer Token 或 ?token=xxx；
//     未通过时返回 401，且响应体不包含 workspace、config 等信息。
type AuthConfig struct {
	// Token 为一次性/单会话令牌；空字符串表示不启用 Token 校验。
	Token string
	// AllowLAN 标记当前实例是否面向局域网开放。
	AllowLAN bool
	// AllowedHosts 是 Host header 白名单（不含端口）；空表示允许任意。
	// 仅本机模式下用于防御 DNS Rebinding：只放行 127.0.0.1/localhost/[::1]。
	AllowedHosts []string
}

// GenerateToken 生成 32 字节的随机十六进制 token。
func GenerateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// authMiddleware 构造 HTTP middleware，实施 Token 鉴权 + Host/Origin 校验。
// 免鉴权路径：健康检查、静态资源（非 /api/ 前缀）。
func authMiddleware(cfg AuthConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 静态资源与健康检查放行（前端页面本身允许匿名加载）
		if r.URL.Path == "/api/health" || !strings.HasPrefix(r.URL.Path, "/api/") {
			// 但仍需 Host 校验以防 DNS Rebinding
			if !hostAllowed(r, cfg) {
				writeAuthError(w, http.StatusForbidden, "host_denied", "拒绝的 Host header")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Host / Origin 校验
		if !hostAllowed(r, cfg) {
			writeAuthError(w, http.StatusForbidden, "host_denied", "拒绝的 Host header")
			return
		}
		if !originAllowed(r, cfg) {
			writeAuthError(w, http.StatusForbidden, "origin_denied", "拒绝的 Origin/Referer")
			return
		}

		// Token 校验（仅在配置了 Token 时启用）
		if cfg.Token != "" {
			provided := extractToken(r)
			if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.Token)) != 1 {
				writeAuthError(w, http.StatusUnauthorized, "unauthorized", "缺少或无效的访问令牌")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// extractToken 从请求中提取 Token：
//  1. Authorization: Bearer <token>
//  2. ?token=xxx （用于 SSE / EventSource 无法自定义 header 的场景）
//  3. Cookie: git_agent_token=xxx
func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if strings.HasPrefix(strings.ToLower(h), "bearer ") {
			return strings.TrimSpace(h[7:])
		}
	}
	if v := r.URL.Query().Get("token"); v != "" {
		return v
	}
	if c, err := r.Cookie("git_agent_token"); err == nil {
		return c.Value
	}
	return ""
}

// hostAllowed 校验请求 Host header 是否在白名单内；仅当 AllowedHosts 非空时启用。
func hostAllowed(r *http.Request, cfg AuthConfig) bool {
	if len(cfg.AllowedHosts) == 0 {
		return true
	}
	hostOnly := r.Host
	if h, _, err := net.SplitHostPort(hostOnly); err == nil {
		hostOnly = h
	}
	hostOnly = strings.ToLower(hostOnly)
	for _, allowed := range cfg.AllowedHosts {
		if strings.EqualFold(hostOnly, allowed) {
			return true
		}
	}
	return false
}

// originAllowed 校验 Origin/Referer 与当前 Host 是否同源（防御 CSRF + DNS Rebinding）。
// 无 Origin 且无 Referer 的请求（如非浏览器直连）放行——已经过 Token 校验。
func originAllowed(r *http.Request, cfg AuthConfig) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	originHost := u.Hostname()
	if originHost == "" {
		return true
	}
	// 与请求自身 Host 同源即可
	reqHost := r.Host
	if h, _, err := net.SplitHostPort(reqHost); err == nil {
		reqHost = h
	}
	if strings.EqualFold(reqHost, originHost) {
		return true
	}
	// 若配置了 AllowedHosts，同样允许命中白名单
	for _, allowed := range cfg.AllowedHosts {
		if strings.EqualFold(allowed, originHost) {
			return true
		}
	}
	return false
}

// writeAuthError 输出鉴权错误响应，故意不泄漏内部细节。
func writeAuthError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": msg,
	})
}

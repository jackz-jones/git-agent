// Package web 提供 git-agent 的本地 Web UI 服务。
//
// 该包通过 HTTP + WebSocket 暴露已有的 Agent/GitWrapper 能力，
// 前端产物（web/dist）通过 go:embed 打包进二进制。
package web

import (
	"embed"
	"io/fs"
)

// distFS 嵌入前端构建产物。
// 注意：首次克隆仓库时 web/dist 可能不存在，使用 all: 前缀允许目录为空；
// 同时通过 //go:embed 指令的 "web/dist" 需要仓库中至少存在一个占位文件，
// 这里通过 placeholder.html 保证编译不失败。
//
//go:embed all:dist
var distFS embed.FS

// DistFS 返回以 "dist" 为根的子文件系统。
// 若目录下无任何文件（只有 placeholder），Server 会自动降级到内置首页。
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// placeholderHTML 是前端未构建时返回的简易首页，
// 用于让用户知晓服务已启动、但前端尚未准备就绪。
const placeholderHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<title>Git Agent Web UI</title>
<style>
  body { font-family: -apple-system, "Segoe UI", Roboto, sans-serif; margin: 40px; color: #222; }
  h1 { color: #2c7; }
  code { background: #f4f4f4; padding: 2px 6px; border-radius: 4px; }
  .tip { color: #888; margin-top: 24px; }
</style>
</head>
<body>
  <h1>Git Agent 服务已启动</h1>
  <p>前端尚未构建，当前为占位页面。</p>
  <p>API 健康检查：<a href="/api/health">/api/health</a></p>
  <p class="tip">要使用完整 UI，请在项目根目录执行：<br>
    <code>cd web &amp;&amp; npm install &amp;&amp; npm run build</code><br>
    或直接运行 <code>make build-web</code>（后续计划提供）。
  </p>
</body>
</html>`

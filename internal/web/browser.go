package web

import (
	"os/exec"
	"runtime"
)

// OpenBrowser 尽力而为地在系统默认浏览器中打开指定 URL。
// 失败时返回 error 供调用方记录日志，不影响主流程。
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, *bsd 等
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

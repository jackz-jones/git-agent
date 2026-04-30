package web

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ValidateWorkspacePath 校验用户传入的目录路径是否可作为 Workspace 根。
// 返回规整后的绝对路径；失败时返回 error 并带用户可读的原因。
//
// 校验规则：
//  1. 必须是非空字符串
//  2. 必须存在且是目录（不接受文件）
//  3. 不能是系统保留目录（根目录、家目录本身、/etc、/System、C:\、C:\Windows 等）
//  4. 必须可读
func ValidateWorkspacePath(input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("路径不能为空")
	}

	// 展开 ~
	if strings.HasPrefix(input, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			input = filepath.Join(home, strings.TrimPrefix(input, "~"))
		}
	}

	abs, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("无法获取绝对路径: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("路径不存在: %s", abs)
		}
		return "", fmt.Errorf("访问路径失败: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("路径不是目录: %s", abs)
	}

	if err := ensureNotReservedDir(abs); err != nil {
		return "", err
	}
	return abs, nil
}

// ensureNotReservedDir 拒绝系统保留目录与用户家目录本身。
func ensureNotReservedDir(abs string) error {
	clean := filepath.Clean(abs)

	// 用户家目录本身不允许（作为 Workspace 颗粒度太大）
	if home, err := os.UserHomeDir(); err == nil {
		if strings.EqualFold(clean, filepath.Clean(home)) {
			return fmt.Errorf("不建议直接对用户家目录进行版本管理，请选择具体子目录")
		}
	}

	reserved := reservedDirs()
	for _, r := range reserved {
		if strings.EqualFold(clean, r) {
			return fmt.Errorf("系统保留目录不允许作为工作区：%s", clean)
		}
	}
	return nil
}

// reservedDirs 返回平台相关的系统保留目录列表。
func reservedDirs() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			`C:\`, `C:\Windows`, `C:\Program Files`, `C:\Program Files (x86)`,
			`D:\`, `E:\`, // 盘符根目录保护
		}
	case "darwin":
		return []string{"/", "/System", "/Library", "/Applications", "/bin", "/sbin", "/usr", "/etc", "/var", "/private"}
	default: // linux / *bsd
		return []string{"/", "/etc", "/bin", "/sbin", "/usr", "/var", "/lib", "/boot", "/proc", "/sys", "/dev"}
	}
}

package web

import (
	"fmt"
	"path/filepath"
	"strings"
)

// safeJoin 将 root 与用户传入的 relPath 拼接，校验结果不越出 root。
// 返回绝对路径；越界时返回错误。
//
// 同时兼容传入绝对路径的场景：只要绝对路径位于 root 下即可。
func safeJoin(root, relPath string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("root 不能为空")
	}
	rootClean, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	target := relPath
	if target == "" {
		return rootClean, nil
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(rootClean, target)
	}
	target = filepath.Clean(target)

	// 校验前缀
	rootWithSep := rootClean
	if !strings.HasSuffix(rootWithSep, string(filepath.Separator)) {
		rootWithSep += string(filepath.Separator)
	}
	if target != rootClean && !strings.HasPrefix(target, rootWithSep) {
		return "", fmt.Errorf("路径越出工作区范围: %s", relPath)
	}
	return target, nil
}

// relToRoot 将绝对路径转换为相对 root 的形式（用于 API 返回）。
func relToRoot(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

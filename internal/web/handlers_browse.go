package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// BrowseDirEntry 是目录浏览返回的单条记录。
type BrowseDirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"` // 绝对路径
}

// handleBrowse 对应 GET /api/browse?path=...
// 浏览本地文件系统，仅返回子目录列表（不返回文件），用于目录选择器。
// 当 path 为空时返回系统根目录列表（macOS/Linux 返回 /，Windows 返回盘符列表）。
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	reqPath := r.URL.Query().Get("path")

	// 空路径：返回根目录
	if reqPath == "" {
		roots := listRoots()
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"current": "",
			"parent":  "",
			"dirs":    roots,
		})
		return
	}

	// 规范化路径
	absPath, err := filepath.Abs(reqPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_path", "无效路径: "+err.Error())
		return
	}

	// 安全：拒绝进入系统保留目录及其子路径，避免通过目录选择器窥探/操作敏感位置。
	if err := ensureBrowseAllowed(absPath); err != nil {
		writeError(w, http.StatusForbidden, "path_denied", err.Error())
		return
	}

	// 检查路径是否存在
	info, err := os.Stat(absPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "path_not_found", "路径不存在: "+absPath)
		return
	}

	// 如果指向的是文件而非目录，返回错误提示
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "not_directory", "请选择目录而非文件")
		return
	}

	// 读取子目录
	entries, err := os.ReadDir(absPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_dir_failed", "读取目录失败: "+err.Error())
		return
	}

	const maxEntries = 200
	dirs := make([]BrowseDirEntry, 0)
	for _, e := range entries {
		// 只返回目录，跳过隐藏目录（以 . 开头）与 .git
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == ".git" {
			continue
		}
		if len(dirs) >= maxEntries {
			break
		}
		dirs = append(dirs, BrowseDirEntry{
			Name: name,
			Path: filepath.Join(absPath, name),
		})
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	// 计算父目录
	parent := filepath.Dir(absPath)
	if parent == absPath {
		// 已经是根目录
		parent = ""
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current": absPath,
		"parent":  parent,
		"dirs":    dirs,
	})
}

// listRoots 返回系统根目录列表。
func listRoots() []BrowseDirEntry {
	if runtime.GOOS == "windows" {
		// Windows：列出可用盘符
		roots := make([]BrowseDirEntry, 0)
		for c := 'A'; c <= 'Z'; c++ {
			drive := string(c) + ":\\"
			if _, err := os.Stat(drive); err == nil {
				roots = append(roots, BrowseDirEntry{
					Name: string(c) + ":",
					Path: drive,
				})
			}
		}
		return roots
	}
	// Unix 系统：根目录就是 /
	return []BrowseDirEntry{
		{Name: "/", Path: "/"},
	}
}

// ensureBrowseAllowed 拒绝浏览系统保留目录本身及其内部；
// 允许浏览到保留目录之外的所有位置。仅目录选择器使用。
func ensureBrowseAllowed(abs string) error {
	clean := filepath.Clean(abs)
	// 明确拒绝 .git 内部窥探
	if strings.Contains(filepath.ToSlash(clean), "/.git/") ||
		strings.HasSuffix(filepath.ToSlash(clean), "/.git") ||
		filepath.Base(clean) == ".git" {
		return fmt.Errorf("拒绝浏览 .git 内部目录")
	}
	for _, r := range reservedDirs() {
		rc := filepath.Clean(r)
		// 保留目录本身：拒绝
		if strings.EqualFold(clean, rc) {
			return fmt.Errorf("系统保留目录不允许浏览: %s", clean)
		}
		// 保留目录的子路径：也拒绝，避免通过 /etc/xxx 探测配置
		sep := string(filepath.Separator)
		if !strings.HasSuffix(rc, sep) {
			rc += sep
		}
		if strings.HasPrefix(clean, rc) {
			return fmt.Errorf("系统保留目录不允许浏览: %s", clean)
		}
	}
	return nil
}
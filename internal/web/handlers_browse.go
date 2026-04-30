package web

import (
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
		// 只返回目录，跳过隐藏目录（以 . 开头）
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
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

package web

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// RecentMaxEntries 是"最近打开"列表的最大长度。
const RecentMaxEntries = 20

// RecentEntry 是单条最近打开记录。
type RecentEntry struct {
	Path     string    `json:"path"`
	OpenedAt time.Time `json:"openedAt"`
}

// RecentStore 管理 ~/.git-agent/recent.json，提供 Add / List 能力。
type RecentStore struct {
	mu      sync.RWMutex
	path    string
	entries []RecentEntry
}

// NewRecentStore 根据配置目录打开或创建 recent.json。
func NewRecentStore(dir string) (*RecentStore, error) {
	if dir == "" {
		dir = DefaultConfigDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	rs := &RecentStore{path: filepath.Join(dir, "recent.json")}
	if err := rs.load(); err != nil {
		return nil, err
	}
	return rs, nil
}

func (r *RecentStore) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取最近打开列表失败: %w", err)
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, &r.entries)
}

// Add 追加或更新一条记录，按 path 去重，超出上限按时间裁剪。
func (r *RecentStore) Add(path string) error {
	if path == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	// 去重
	filtered := r.entries[:0]
	for _, e := range r.entries {
		if e.Path != path {
			filtered = append(filtered, e)
		}
	}
	filtered = append(filtered, RecentEntry{Path: path, OpenedAt: now})
	r.entries = filtered

	// 排序并裁剪
	sort.SliceStable(r.entries, func(i, j int) bool {
		return r.entries[i].OpenedAt.After(r.entries[j].OpenedAt)
	})
	if len(r.entries) > RecentMaxEntries {
		r.entries = r.entries[:RecentMaxEntries]
	}
	return r.saveLocked()
}

// Remove 移除一条记录（例如用户显式删除或目录已消失）。
func (r *RecentStore) Remove(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := r.entries[:0]
	for _, e := range r.entries {
		if e.Path != path {
			out = append(out, e)
		}
	}
	r.entries = out
	return r.saveLocked()
}

// List 返回最近打开记录的快照（按时间倒序）。
func (r *RecentStore) List() []RecentEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RecentEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

// saveLocked 写盘（调用方必须持有写锁）。
func (r *RecentStore) saveLocked() error {
	raw, err := json.MarshalIndent(r.entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

// Path 返回 recent.json 路径。
func (r *RecentStore) Path() string { return r.path }

package web

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// AppConfig 是写入 ~/.git-agent/config.json 的顶层结构。
// 所有字段 omitempty，方便渐进式保存。
type AppConfig struct {
	User struct {
		Name  string `json:"name,omitempty"`
		Email string `json:"email,omitempty"`
	} `json:"user"`

	LLM struct {
		Enabled   bool   `json:"enabled,omitempty"`
		APIKey    string `json:"apiKey,omitempty"`
		BaseURL   string `json:"baseUrl,omitempty"`
		Model     string `json:"model,omitempty"`
		MaxTokens int    `json:"maxTokens,omitempty"`
	} `json:"llm"`

	HTTP struct {
		Username string `json:"username,omitempty"`
		Password string `json:"password,omitempty"`
	} `json:"http"`
}

// ConfigStore 负责线程安全地读写 AppConfig。
type ConfigStore struct {
	mu   sync.RWMutex
	path string
	data AppConfig
}

// DefaultConfigDir 返回配置与审计日志的根目录 ~/.git-agent。
// 失败时回退到当前工作目录下的 .git-agent。
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".git-agent")
	}
	return filepath.Join(home, ".git-agent")
}

// NewConfigStore 根据配置目录打开或创建 config.json。
func NewConfigStore(dir string) (*ConfigStore, error) {
	if dir == "" {
		dir = DefaultConfigDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	cs := &ConfigStore{path: filepath.Join(dir, "config.json")}
	if err := cs.load(); err != nil {
		return nil, err
	}
	return cs, nil
}

// load 从磁盘读取配置；文件不存在时使用零值。
func (c *ConfigStore) load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	f, err := os.Open(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取配置失败: %w", err)
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("读取配置内容失败: %w", err)
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &c.data); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}
	return nil
}

// Snapshot 返回当前配置的浅拷贝。
func (c *ConfigStore) Snapshot() AppConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

// Update 以函数方式修改配置并原子保存。
//
//	cs.Update(func(cfg *AppConfig) { cfg.User.Name = "alice" })
func (c *ConfigStore) Update(fn func(cfg *AppConfig)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if fn != nil {
		fn(&c.data)
	}
	return c.saveLocked()
}

// Replace 使用传入的完整 AppConfig 覆盖后保存。
func (c *ConfigStore) Replace(cfg AppConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = cfg
	return c.saveLocked()
}

// saveLocked 将当前内存中的配置原子写回文件（0600 权限）。
// 调用方必须持有写锁。
func (c *ConfigStore) saveLocked() error {
	raw, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("写入临时配置文件失败: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("原子重命名配置文件失败: %w", err)
	}
	return nil
}

// Path 返回配置文件路径（供测试/诊断使用）。
func (c *ConfigStore) Path() string { return c.path }

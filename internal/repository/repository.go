package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// RepoInfo 仓库信息
type RepoInfo struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	IsBare    bool      `json:"is_bare"`
}

// Manager 仓库管理器
type Manager struct {
	basePath string
}

// NewManager 创建仓库管理器
func NewManager(basePath string) (*Manager, error) {
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("获取绝对路径失败: %w", err)
	}

	return &Manager{
		basePath: absPath,
	}, nil
}

// Create 创建新仓库
func (m *Manager) Create(name string) (*RepoInfo, error) {
	repoPath := filepath.Join(m.basePath, name)

	// 检查是否已存在
	if _, err := os.Stat(repoPath); err == nil {
		return nil, fmt.Errorf("仓库 %s 已存在", name)
	}

	// 创建目录
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return nil, fmt.Errorf("创建目录失败: %w", err)
	}

	// 初始化 git 仓库
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		_ = os.RemoveAll(repoPath)
		return nil, fmt.Errorf("初始化仓库失败: %w", err)
	}

	// 创建初始提交
	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("获取工作区失败: %w", err)
	}

	// 创建 README
	readmeContent := fmt.Sprintf("# %s\n\n文档仓库", name)
	f, err := wt.Filesystem.Create("README.md")
	if err == nil {
		_, _ = f.Write([]byte(readmeContent))
		f.Close()
	}
	_, _ = wt.Add("README.md")

	_, err = wt.Commit(fmt.Sprintf("初始化文档仓库「%s」", name), &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Git Agent",
			Email: "agent@git-agent.dev",
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("创建初始提交失败: %w", err)
	}

	info := &RepoInfo{
		Name:      name,
		Path:      repoPath,
		CreatedAt: time.Now(),
		IsBare:    false,
	}

	return info, nil
}

// Clone 克隆远程仓库
func (m *Manager) Clone(url string) (*RepoInfo, error) {
	// 从 URL 中提取仓库名
	name := extractRepoName(url)
	if name == "" {
		name = fmt.Sprintf("repo-%d", time.Now().Unix())
	}

	repoPath := filepath.Join(m.basePath, name)

	_, err := git.PlainClone(repoPath, false, &git.CloneOptions{
		URL: url,
	})
	if err != nil {
		return nil, fmt.Errorf("克隆仓库失败: %w", err)
	}

	info := &RepoInfo{
		Name:      name,
		Path:      repoPath,
		CreatedAt: time.Now(),
		IsBare:    false,
	}

	return info, nil
}

// List 列出所有仓库
func (m *Manager) List() ([]RepoInfo, error) {
	var repos []RepoInfo

	entries, err := os.ReadDir(m.basePath)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		gitPath := filepath.Join(m.basePath, entry.Name(), ".git")
		if _, err := os.Stat(gitPath); err == nil {
			info, err := entry.Info()
			if err != nil {
				continue
			}

			repos = append(repos, RepoInfo{
				Name:      entry.Name(),
				Path:      filepath.Join(m.basePath, entry.Name()),
				CreatedAt: info.ModTime(),
				IsBare:    false,
			})
		}
	}

	return repos, nil
}

// extractRepoName 从 URL 提取仓库名
func extractRepoName(url string) string {
	// 去掉末尾的 .git
	cleanURL := url
	if len(cleanURL) > 4 && cleanURL[len(cleanURL)-4:] == ".git" {
		cleanURL = cleanURL[:len(cleanURL)-4]
	}

	// 取最后一段路径
	for i := len(cleanURL) - 1; i >= 0; i-- {
		if cleanURL[i] == '/' {
			return cleanURL[i+1:]
		}
	}

	return cleanURL
}

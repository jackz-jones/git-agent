package gitwrapper

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

// VersionInfo 表示一个版本（commit）的信息
type VersionInfo struct {
	Hash      string    `json:"hash"`
	ShortHash string    `json:"short_hash"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Date      time.Time `json:"date"`
	Message   string    `json:"message"`
}

// FileChange 表示文件变更信息
type FileChange struct {
	Path      string `json:"path"`
	Status    string `json:"status"`    // added, modified, deleted, renamed
	Insertions int   `json:"insertions"`
	Deletions  int   `json:"deletions"`
}

// StatusInfo 表示仓库状态信息
type StatusInfo struct {
	Staged       []FileChange `json:"staged"`
	Unstaged     []FileChange `json:"unstaged"`
	Untracked    []string     `json:"untracked"`
	IsClean      bool         `json:"is_clean"`
	LatestCommit *VersionInfo `json:"latest_commit,omitempty"`    // 最新一次提交的信息
	AheadBehind  *AheadBehind `json:"ahead_behind,omitempty"`     // 本地与远程的领先/落后情况
}

// AheadBehind 表示本地分支相对远程分支的领先/落后提交数
type AheadBehind struct {
	Ahead  int    `json:"ahead"`   // 本地领先远程的提交数
	Behind int    `json:"behind"`  // 本地落后远程的提交数
	Remote string `json:"remote"`  // 远程仓库名称
	Branch string `json:"branch"`  // 当前分支名
}

// BranchInfo 表示分支信息
type BranchInfo struct {
	Name      string    `json:"name"`
	IsCurrent bool      `json:"is_current"`
	LastCommit *VersionInfo `json:"last_commit,omitempty"`
}

// GitWrapper 封装 go-git 操作，提供面向办公场景的高层接口
// 所有 git 概念都被翻译为用户友好的办公语言
type GitWrapper struct {
	repo *git.Repository
	path string
}

// New 创建 GitWrapper，打开或初始化仓库
func New(repoPath string) (*GitWrapper, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("获取绝对路径失败: %w", err)
	}

	repo, err := git.PlainOpen(absPath)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			// 仓库不存在，返回空 wrapper，需要先 InitRepo
			return &GitWrapper{
				repo: nil,
				path: absPath,
			}, nil
		}
		return nil, fmt.Errorf("打开仓库失败: %w", err)
	}

	return &GitWrapper{
		repo: repo,
		path: absPath,
	}, nil
}

// InitRepo 初始化新的 Git 仓库
func (g *GitWrapper) InitRepo() error {
	if g.repo != nil {
		return nil // 已初始化
	}

	repo, err := git.PlainInit(g.path, false)
	if err != nil {
		return fmt.Errorf("初始化仓库失败: %w", err)
	}
	g.repo = repo

	// 创建初始提交，确保 main 分支存在
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("获取工作区失败: %w", err)
	}

	// 创建 .gitignore 如果不存在
	f, err := wt.Filesystem.Create(".gitignore")
	if err == nil {
		_, _ = f.Write([]byte(""))
		f.Close()
	}
	_, _ = wt.Add(".gitignore")

	_, err = wt.Commit("初始化文档仓库", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Git Agent",
			Email: "agent@git-agent.dev",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("创建初始提交失败: %w", err)
	}

	return nil
}

// IsInitialized 检查仓库是否已初始化
func (g *GitWrapper) IsInitialized() bool {
	return g.repo != nil
}

// SaveVersion 保存当前文件为新版本（git add + git commit）
// 这是用户最常用的操作
func (g *GitWrapper) SaveVersion(description string, files []string, authorName, authorEmail string) (string, error) {
	if err := g.ensureRepo(); err != nil {
		return "", err
	}

	wt, err := g.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("获取工作区失败: %w", err)
	}

	// 添加文件
	if len(files) == 0 {
		_, err = wt.Add(".")
	} else {
		for _, f := range files {
			_, err = wt.Add(f)
			if err != nil {
				return "", fmt.Errorf("添加文件 %s 失败: %w", f, err)
			}
		}
	}
	if err != nil {
		return "", fmt.Errorf("添加文件到暂存区失败: %w", err)
	}

	// 提交
	if authorName == "" || authorEmail == "" {
		return "", fmt.Errorf("用户信息未配置，无法提交。请设置 GIT_AGENT_USER 和 GIT_AGENT_EMAIL 环境变量，或在交互中告诉我您的名字和邮箱")
	}

	hash, err := wt.Commit(description, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		if err.Error() == "no changes to commit" {
			return "", fmt.Errorf("没有需要保存的修改")
		}
		return "", fmt.Errorf("保存版本失败: %w", err)
	}

	return hash.String()[:8], nil
}

// AddFiles 添加指定文件到暂存区
func (g *GitWrapper) AddFiles(files []string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	wt, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("获取工作区失败: %w", err)
	}

	for _, f := range files {
		_, err = wt.Add(f)
		if err != nil {
			return fmt.Errorf("添加文件 %s 失败: %w", f, err)
		}
	}
	return nil
}

// AddAll 添加所有变更文件到暂存区
func (g *GitWrapper) AddAll() error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	wt, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("获取工作区失败: %w", err)
	}

	_, err = wt.Add(".")
	if err != nil {
		return fmt.Errorf("添加所有文件失败: %w", err)
	}
	return nil
}

// Commit 提交暂存区的更改
func (g *GitWrapper) Commit(message, authorName, authorEmail string) (string, error) {
	if err := g.ensureRepo(); err != nil {
		return "", err
	}

	wt, err := g.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("获取工作区失败: %w", err)
	}

	if authorName == "" || authorEmail == "" {
		return "", fmt.Errorf("用户信息未配置，无法提交。请设置 GIT_AGENT_USER 和 GIT_AGENT_EMAIL 环境变量，或在交互中告诉我您的名字和邮箱")
	}

	hash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("提交失败: %w", err)
	}

	return hash.String()[:8], nil
}

// GetHistory 获取文件/仓库的版本历史（git log）
func (g *GitWrapper) GetHistory(filePath string, limit int, author string) ([]VersionInfo, error) {
	if err := g.ensureRepo(); err != nil {
		return nil, err
	}

	opts := &git.LogOptions{}
	if filePath != "" {
		opts.FileName = &filePath
	}

	iter, err := g.repo.Log(opts)
	if err != nil {
		return nil, fmt.Errorf("获取历史记录失败: %w", err)
	}

	var versions []VersionInfo
	count := 0
	err = iter.ForEach(func(c *object.Commit) error {
		// 按作者过滤
		if author != "" && c.Author.Name != author {
			return nil
		}
		versions = append(versions, VersionInfo{
			Hash:      c.Hash.String(),
			ShortHash: c.Hash.String()[:8],
			Author:    c.Author.Name,
			Email:     c.Author.Email,
			Date:      c.Author.When,
			Message:   c.Message,
		})
		count++
		if limit > 0 && count >= limit {
			return fmt.Errorf("limit reached")
		}
		return nil
	})

	// "limit reached" 不是真正的错误，忽略
	if err != nil && err.Error() != "limit reached" {
		return nil, err
	}

	return versions, nil
}

// Log 获取提交日志
func (g *GitWrapper) Log(limit int, author string) ([]VersionInfo, error) {
	return g.GetHistory("", limit, author)
}

// RestoreVersion 恢复到指定版本（整个仓库）
func (g *GitWrapper) RestoreVersion(versionID string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	wt, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("获取工作区失败: %w", err)
	}

	hash := plumbing.NewHash(versionID)
	err = wt.Reset(&git.ResetOptions{
		Commit: hash,
		Mode:   git.HardReset,
	})
	if err != nil {
		return fmt.Errorf("恢复版本失败: %w", err)
	}

	return nil
}

// RestoreFile 恢复指定文件到某个版本
func (g *GitWrapper) RestoreFile(filePath, versionID string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	wt, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("获取工作区失败: %w", err)
	}

	if versionID == "" {
		// 恢复到最近一次提交的版本
		head, err := g.repo.Head()
		if err != nil {
			return fmt.Errorf("获取 HEAD 失败: %w", err)
		}
		versionID = head.Hash().String()
	}

	hash := plumbing.NewHash(versionID)
	commit, err := g.repo.CommitObject(hash)
	if err != nil {
		return fmt.Errorf("获取版本信息失败: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("获取文件树失败: %w", err)
	}

	entry, err := tree.FindEntry(filePath)
	if err != nil {
		return fmt.Errorf("在版本 %s 中找不到文件 %s: %w", versionID[:8], filePath, err)
	}

	blob, err := g.repo.BlobObject(entry.Hash)
	if err != nil {
		return fmt.Errorf("读取文件内容失败: %w", err)
	}

	r, err := blob.Reader()
	if err != nil {
		return fmt.Errorf("读取文件内容失败: %w", err)
	}
	defer r.Close()

	// 写入文件
	f, err := wt.Filesystem.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				return fmt.Errorf("写入文件失败: %w", wErr)
			}
		}
		if err != nil {
			break
		}
	}

	_, _ = wt.Add(filePath)
	return nil
}

// Status 获取仓库当前状态
func (g *GitWrapper) Status() (*StatusInfo, error) {
	if err := g.ensureRepo(); err != nil {
		return nil, err
	}

	wt, err := g.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("获取工作区失败: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("获取状态失败: %w", err)
	}

	info := &StatusInfo{
		IsClean: status.IsClean(),
	}

	for file, s := range status {
		change := FileChange{Path: file}

		// 暂存区状态
		switch s.Staging {
		case git.Added:
			change.Status = "added"
			info.Staged = append(info.Staged, change)
		case git.Modified:
			change.Status = "modified"
			info.Staged = append(info.Staged, change)
		case git.Deleted:
			change.Status = "deleted"
			info.Staged = append(info.Staged, change)
		}

		// 工作区状态
		switch s.Worktree {
		case git.Modified:
			change.Status = "modified"
			info.Unstaged = append(info.Unstaged, change)
		case git.Deleted:
			change.Status = "deleted"
			info.Unstaged = append(info.Unstaged, change)
		case git.Untracked:
			info.Untracked = append(info.Untracked, file)
		}
	}

	// 获取最新提交信息
	headRef, err := g.repo.Head()
	if err == nil {
		headCommit, err := g.repo.CommitObject(headRef.Hash())
		if err == nil {
			info.LatestCommit = &VersionInfo{
				Hash:      headCommit.Hash.String(),
				ShortHash: headCommit.Hash.String()[:8],
				Author:    headCommit.Author.Name,
				Email:     headCommit.Author.Email,
				Date:      headCommit.Author.When,
				Message:   headCommit.Message,
			}
		}
	}

	// 获取 ahead/behind 信息
	aheadBehind, _ := g.GetAheadBehind()
	if aheadBehind != nil {
		info.AheadBehind = aheadBehind
	}

	return info, nil
}

// Diff 获取文件差异，对比工作区与 HEAD 提交之间的变更
// 等价于 git diff [filePath]
func (g *GitWrapper) Diff(filePath string) (string, error) {
	if err := g.ensureRepo(); err != nil {
		return "", err
	}

	// 获取 HEAD commit
	head, err := g.repo.Head()
	if err != nil {
		return "", fmt.Errorf("获取 HEAD 失败: %w", err)
	}

	commit, err := g.repo.CommitObject(head.Hash())
	if err != nil {
		return "", fmt.Errorf("获取提交信息失败: %w", err)
	}

	commitTree, err := commit.Tree()
	if err != nil {
		return "", fmt.Errorf("获取文件树失败: %w", err)
	}

	// 获取工作区状态
	wt, err := g.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("获取工作区失败: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return "", fmt.Errorf("获取状态失败: %w", err)
	}

	// 筛选出有变更的文件
	type fileChange struct {
		path    string
		status  git.StatusCode // Worktree 状态
		staging git.StatusCode // Staging 状态
	}
	var changes []fileChange
	for file, s := range status {
		if filePath != "" && file != filePath {
			continue
		}
		// 只关注工作区有变更的文件（Untracked 也算新增）
		if s.Worktree == git.Modified || s.Worktree == git.Deleted || s.Worktree == git.Untracked ||
			s.Staging == git.Modified || s.Staging == git.Added || s.Staging == git.Deleted {
			changes = append(changes, fileChange{path: file, status: s.Worktree, staging: s.Staging})
		}
	}

	if len(changes) == 0 {
		return "没有发现修改", nil
	}

	var result strings.Builder

	for _, change := range changes {
		// 添加文件头
		result.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", change.path, change.path))

		switch {
		// 新增的未跟踪文件
		case change.status == git.Untracked:
			diffContent, err := g.diffNewFile(wt, change.path)
			if err != nil {
				result.WriteString(fmt.Sprintf("（无法读取新文件内容: %s）\n", err))
				continue
			}
			result.WriteString(diffContent)

		// 已删除的文件
		case change.status == git.Deleted:
			diffContent, err := g.diffDeletedFile(commitTree, change.path)
			if err != nil {
				result.WriteString(fmt.Sprintf("（无法读取已删除文件内容: %s）\n", err))
				continue
			}
			result.WriteString(diffContent)

		// 修改的文件：对比 HEAD 版本和工作区版本
		case change.status == git.Modified || change.staging == git.Modified:
			diffContent, err := g.diffModifiedFile(wt, commitTree, change.path)
			if err != nil {
				result.WriteString(fmt.Sprintf("（无法计算差异: %s）\n", err))
				continue
			}
			result.WriteString(diffContent)

		// 已暂存的新增文件
		case change.staging == git.Added:
			diffContent, err := g.diffNewFile(wt, change.path)
			if err != nil {
				result.WriteString(fmt.Sprintf("（无法读取新文件内容: %s）\n", err))
				continue
			}
			result.WriteString(diffContent)

		default:
			result.WriteString(fmt.Sprintf("文件: %s | 状态: %s\n", change.path, translateStatusCode(change.status, change.staging)))
		}

		result.WriteString("\n")
	}

	return strings.TrimSuffix(result.String(), "\n"), nil
}

// diffNewFile 生成新增文件的差异（所有行都是新增）
func (g *GitWrapper) diffNewFile(wt *git.Worktree, filePath string) (string, error) {
	f, err := wt.Filesystem.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	var sb strings.Builder
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		sb.WriteString("+")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// CommitDiff 查看某个 commit 相对其父 commit 的差异，等价于 git diff <commit>^..<commit>
func (g *GitWrapper) CommitDiff(commitHash string) (string, error) {
	if err := g.ensureRepo(); err != nil {
		return "", err
	}

	hash := plumbing.NewHash(commitHash)
	commit, err := g.repo.CommitObject(hash)
	if err != nil {
		return "", fmt.Errorf("找不到提交 %s: %w", commitHash, err)
	}

	commitTree, err := commit.Tree()
	if err != nil {
		return "", fmt.Errorf("获取提交文件树失败: %w", err)
	}

	// 获取父 commit
	if len(commit.ParentHashes) == 0 {
		// 初始提交：与空树对比，所有文件都是新增
		return g.diffTreeAgainstEmpty(commitTree)
	}

	parentHash := commit.ParentHashes[0]
	parentCommit, err := g.repo.CommitObject(parentHash)
	if err != nil {
		return "", fmt.Errorf("获取父提交失败: %w", err)
	}

	parentTree, err := parentCommit.Tree()
	if err != nil {
		return "", fmt.Errorf("获取父提交文件树失败: %w", err)
	}

	// 对比两棵文件树
	patch, err := parentTree.Patch(commitTree)
	if err != nil {
		return "", fmt.Errorf("计算差异失败: %w", err)
	}

	if len(patch.FilePatches()) == 0 {
		return "该提交没有文件变更", nil
	}

	var result strings.Builder
	for _, filePatch := range patch.FilePatches() {
		from, to := filePatch.Files()
		if from != nil && to != nil {
			result.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", from.Path(), to.Path()))
		} else if from != nil {
			result.WriteString(fmt.Sprintf("--- a/%s\n+++ /dev/null\n", from.Path()))
		} else if to != nil {
			result.WriteString(fmt.Sprintf("--- /dev/null\n+++ b/%s\n", to.Path()))
		}

		chunks := filePatch.Chunks()
		for _, chunk := range chunks {
			content := chunk.Content()
			switch chunk.Type() {
			case fdiff.Add:
				for _, line := range strings.Split(content, "\n") {
					if line != "" {
						result.WriteString("+")
						result.WriteString(line)
						result.WriteString("\n")
					}
				}
			case fdiff.Delete:
				for _, line := range strings.Split(content, "\n") {
					if line != "" {
						result.WriteString("-")
						result.WriteString(line)
						result.WriteString("\n")
					}
				}
		case fdiff.Equal:
				for _, line := range strings.Split(content, "\n") {
					if line != "" {
						result.WriteString(" ")
						result.WriteString(line)
						result.WriteString("\n")
					}
				}
			}
		}
		result.WriteString("\n")
	}

	return strings.TrimSuffix(result.String(), "\n"), nil
}

// diffTreeAgainstEmpty 将文件树与空树对比，用于初始提交的 diff
func (g *GitWrapper) diffTreeAgainstEmpty(tree *object.Tree) (string, error) {
	var result strings.Builder

	err := tree.Files().ForEach(func(file *object.File) error {
		result.WriteString(fmt.Sprintf("--- /dev/null\n+++ b/%s\n", file.Name))

		content, err := file.Contents()
		if err != nil {
			result.WriteString(fmt.Sprintf("（无法读取文件内容: %s）\n", err))
			return nil
		}

		for _, line := range strings.Split(content, "\n") {
			result.WriteString("+")
			result.WriteString(line)
			result.WriteString("\n")
		}
		result.WriteString("\n")
		return nil
	})

	return strings.TrimSuffix(result.String(), "\n"), err
}

// diffDeletedFile 生成已删除文件的差异（所有行都是删除）
func (g *GitWrapper) diffDeletedFile(commitTree *object.Tree, filePath string) (string, error) {
	entry, err := commitTree.FindEntry(filePath)
	if err != nil {
		return "", fmt.Errorf("在 HEAD 中找不到文件 %s: %w", filePath, err)
	}

	blob, err := g.repo.BlobObject(entry.Hash)
	if err != nil {
		return "", fmt.Errorf("读取文件内容失败: %w", err)
	}

	r, err := blob.Reader()
	if err != nil {
		return "", fmt.Errorf("读取文件内容失败: %w", err)
	}
	defer r.Close()

	content, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("读取文件内容失败: %w", err)
	}

	var sb strings.Builder
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		sb.WriteString("-")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// diffModifiedFile 生成修改文件的差异，对比 HEAD 版本和工作区版本
func (g *GitWrapper) diffModifiedFile(wt *git.Worktree, commitTree *object.Tree, filePath string) (string, error) {
	// 读取 HEAD 版本的内容
	var oldContent string
	entry, err := commitTree.FindEntry(filePath)
	if err == nil {
		blob, err := g.repo.BlobObject(entry.Hash)
		if err == nil {
			r, err := blob.Reader()
			if err == nil {
				content, err := io.ReadAll(r)
				if err == nil {
					oldContent = string(content)
				}
				r.Close()
			}
		}
	}

	// 读取工作区版本的内容
	f, err := wt.Filesystem.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开工作区文件失败: %w", err)
	}
	defer f.Close()

	newContentBytes, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("读取工作区文件失败: %w", err)
	}
	newContent := string(newContentBytes)

	// 使用简单的行级 diff 算法
	return unifiedDiff(oldContent, newContent), nil
}

// unifiedDiff 生成简单的统一差异格式输出
// 对比旧内容和新内容，生成类似 git diff 的输出
func unifiedDiff(oldContent, newContent string) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	var sb strings.Builder

	// 使用 LCS (最长公共子序列) 算法计算差异
	ops := computeLCSDiff(oldLines, newLines)

	for _, op := range ops {
		switch op.kind {
		case opEqual:
			sb.WriteString(" ")
			sb.WriteString(op.line)
			sb.WriteString("\n")
		case opDelete:
			sb.WriteString("-")
			sb.WriteString(op.line)
			sb.WriteString("\n")
		case opInsert:
			sb.WriteString("+")
			sb.WriteString(op.line)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// diffOp 表示一个 diff 操作
type diffOp struct {
	kind int    // opEqual, opDelete, opInsert
	line string
}

const (
	opEqual  = 0
	opDelete = 1
	opInsert = 2
)

// computeLCSDiff 使用 LCS 算法计算两个行序列的差异
func computeLCSDiff(oldLines, newLines []string) []diffOp {
	m, n := len(oldLines), len(newLines)

	// 构建 DP 表
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if oldLines[i-1] == newLines[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// 回溯得到差异操作序列
	var ops []diffOp
	i, j := m, n
	for i > 0 && j > 0 {
		if oldLines[i-1] == newLines[j-1] {
			ops = append(ops, diffOp{opEqual, oldLines[i-1]})
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			ops = append(ops, diffOp{opDelete, oldLines[i-1]})
			i--
		} else {
			ops = append(ops, diffOp{opInsert, newLines[j-1]})
			j--
		}
	}

	for i > 0 {
		ops = append(ops, diffOp{opDelete, oldLines[i-1]})
		i--
	}
	for j > 0 {
		ops = append(ops, diffOp{opInsert, newLines[j-1]})
		j--
	}

	// 反转操作序列（因为回溯是逆序的）
	for left, right := 0, len(ops)-1; left < right; left, right = left+1, right-1 {
		ops[left], ops[right] = ops[right], ops[left]
	}

	return ops
}

// CreateBranch 创建分支（用户语言：创建工作副本）
func (g *GitWrapper) CreateBranch(name string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	head, err := g.repo.Head()
	if err != nil {
		return fmt.Errorf("获取 HEAD 失败: %w", err)
	}

	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(name), head.Hash())
	err = g.repo.Storer.SetReference(ref)
	if err != nil {
		return fmt.Errorf("创建分支失败: %w", err)
	}

	return nil
}

// SwitchBranch 切换分支
func (g *GitWrapper) SwitchBranch(name string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	wt, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("获取工作区失败: %w", err)
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
	})
	if err != nil {
		return fmt.Errorf("切换分支失败: %w", err)
	}

	return nil
}

// ListBranches 列出所有分支
func (g *GitWrapper) ListBranches() ([]BranchInfo, error) {
	if err := g.ensureRepo(); err != nil {
		return nil, err
	}

	head, err := g.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("获取 HEAD 失败: %w", err)
	}

	var branches []BranchInfo

	iter, err := g.repo.Branches()
	if err != nil {
		return nil, fmt.Errorf("获取分支列表失败: %w", err)
	}

	err = iter.ForEach(func(ref *plumbing.Reference) error {
		isCurrent := ref.Name().Short() == head.Name().Short()

		info := BranchInfo{
			Name:      ref.Name().Short(),
			IsCurrent: isCurrent,
		}

		// 获取最后提交信息
		commit, err := g.repo.CommitObject(ref.Hash())
		if err == nil {
			info.LastCommit = &VersionInfo{
				Hash:      commit.Hash.String(),
				ShortHash: commit.Hash.String()[:8],
				Author:    commit.Author.Name,
				Date:      commit.Author.When,
				Message:   commit.Message,
			}
		}

		branches = append(branches, info)
		return nil
	})

	return branches, err
}

// Merge 合并分支到当前分支
// 等价于 git merge <branch>
func (g *GitWrapper) Merge(branch string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	// 获取目标分支的引用
	branchRef, err := g.repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return fmt.Errorf("找不到分支 %s: %w", branch, err)
	}

	// 获取当前 HEAD
	head, err := g.repo.Head()
	if err != nil {
		return fmt.Errorf("获取 HEAD 失败: %w", err)
	}

	// 如果合并的是自身，无需操作
	if head.Hash() == branchRef.Hash() {
		return fmt.Errorf("分支 %s 与当前分支指向同一提交，无需合并", branch)
	}

	// 获取两个 commit 对象
	headCommit, err := g.repo.CommitObject(head.Hash())
	if err != nil {
		return fmt.Errorf("获取当前提交失败: %w", err)
	}

	branchCommit, err := g.repo.CommitObject(branchRef.Hash())
	if err != nil {
		return fmt.Errorf("获取分支 %s 的提交失败: %w", branch, err)
	}

	// 检查是否可以快进合并（当前 HEAD 是分支的祖先）
	isAncestor, err := isAncestor(headCommit, branchCommit)
	if err != nil {
		return fmt.Errorf("检查合并关系失败: %w", err)
	}

	wt, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("获取工作区失败: %w", err)
	}

	if isAncestor {
		// 快进合并：直接将 HEAD 移动到目标分支
		err = wt.Reset(&git.ResetOptions{
			Commit: branchRef.Hash(),
			Mode:   git.MergeReset,
		})
		if err != nil {
			return fmt.Errorf("快进合并失败: %w", err)
		}
		return nil
	}

	// 非快进合并：执行三方合并
	// 找到两个分支的共同祖先
	ancestor, err := headCommit.MergeBase(branchCommit)
	if err != nil {
		return fmt.Errorf("查找共同祖先失败: %w", err)
	}
	if len(ancestor) == 0 {
		return fmt.Errorf("无法找到共同祖先，合并不可能")
	}

	// 获取共同祖先的 tree、HEAD 的 tree 和分支的 tree
	ancestorTree, err := ancestor[0].Tree()
	if err != nil {
		return fmt.Errorf("获取共同祖先文件树失败: %w", err)
	}

	headTree, err := headCommit.Tree()
	if err != nil {
		return fmt.Errorf("获取当前分支文件树失败: %w", err)
	}

	branchTree, err := branchCommit.Tree()
	if err != nil {
		return fmt.Errorf("获取目标分支文件树失败: %w", err)
	}

	// 检测冲突：比较两个分支相对于共同祖先的修改
	conflicts, err := detectMergeConflicts(ancestorTree, headTree, branchTree)
	if err != nil {
		return fmt.Errorf("检测合并冲突失败: %w", err)
	}

	if len(conflicts) > 0 {
		// 有冲突，报告冲突文件
		var conflictFiles []string
		for _, c := range conflicts {
			conflictFiles = append(conflictFiles, c)
		}
		return fmt.Errorf("合并冲突：以下文件存在冲突，请先解决：\n%s", strings.Join(conflictFiles, "\n"))
	}

	// 无冲突，将分支的修改应用到工作区
	changes, err := object.DiffTree(ancestorTree, branchTree)
	if err != nil {
		return fmt.Errorf("计算差异失败: %w", err)
	}

	patch, err := changes.Patch()
	if err != nil {
		return fmt.Errorf("生成补丁失败: %w", err)
	}

	// 应用修改到工作区
	for _, filePatch := range patch.FilePatches() {
		if filePatch.IsBinary() {
			continue
		}

		from, to := filePatch.Files()
		if to != nil {
			// 文件被修改或新增
			var content strings.Builder
			for _, chunk := range filePatch.Chunks() {
				opType := chunk.Type()
				if opType == fdiff.Add || opType == fdiff.Equal {
					content.WriteString(chunk.Content())
				}
			}
			path := to.Path()
			f, err := wt.Filesystem.Create(path)
			if err != nil {
				return fmt.Errorf("创建文件 %s 失败: %w", path, err)
			}
			_, err = f.Write([]byte(content.String()))
			f.Close()
			if err != nil {
				return fmt.Errorf("写入文件 %s 失败: %w", path, err)
			}
			_, _ = wt.Add(path)
		} else if from != nil && to == nil {
			// 文件被删除
			_ = wt.Filesystem.Remove(from.Path())
		}
	}

	// 创建合并提交
	_, err = wt.Commit(fmt.Sprintf("合并分支 %s", branch), &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Git Agent",
			Email: "agent@git-agent.dev",
			When:  time.Now(),
		},
		Parents: []plumbing.Hash{head.Hash(), branchRef.Hash()},
	})
	if err != nil {
		return fmt.Errorf("创建合并提交失败: %w", err)
	}

	return nil
}

// isAncestor 检查 ancestor 是否是 descendant 的祖先
func isAncestor(ancestor, descendant *object.Commit) (bool, error) {
	iter := descendant.Parents()
	for {
		parent, err := iter.Next()
		if err != nil {
			break
		}
		if parent.Hash == ancestor.Hash {
			return true, nil
		}
		// 递归检查
		found, err := isAncestor(ancestor, parent)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

// detectMergeConflicts 检测两个分支相对于共同祖先的修改是否有冲突
func detectMergeConflicts(ancestorTree, ourTree, theirTree *object.Tree) ([]string, error) {
	// 获取我们和对方各自的修改
	ourChanges, err := object.DiffTree(ancestorTree, ourTree)
	if err != nil {
		return nil, err
	}
	theirChanges, err := object.DiffTree(ancestorTree, theirTree)
	if err != nil {
		return nil, err
	}

	// 构建我们修改过的文件集合
	ourModifiedFiles := make(map[string]bool)
	for _, change := range ourChanges {
		action, err := change.Action()
		if err != nil {
			continue
		}
		if action == merkletrie.Modify || action == merkletrie.Insert {
			name := change.To.Name
			if name == "" {
				name = change.From.Name
			}
			if name != "" {
				ourModifiedFiles[name] = true
			}
		}
	}

	// 检查对方的修改是否与我们的修改有冲突（同一文件双方都修改了）
	var conflicts []string
	for _, change := range theirChanges {
		action, err := change.Action()
		if err != nil {
			continue
		}
		if action == merkletrie.Modify || action == merkletrie.Insert {
			name := change.To.Name
			if name == "" {
				name = change.From.Name
			}
			if name != "" && ourModifiedFiles[name] {
				conflicts = append(conflicts, name)
			}
		}
	}

	return conflicts, nil
}

// Push 推送到远程仓库
func (g *GitWrapper) Push(remote string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	remoteObj, err := g.repo.Remote(remote)
	if err != nil {
		return fmt.Errorf("找不到远程仓库 %s: %w", remote, err)
	}

	pushOpts := &git.PushOptions{
		RemoteName: remote,
	}

	// 自动检测远程 URL 类型并配置认证
	auth, err := g.getAuthForRemote(remoteObj)
	if err != nil {
		return fmt.Errorf("推送失败，远程仓库连接有问题: %w", err)
	}
	if auth != nil {
		pushOpts.Auth = auth
	}

	err = remoteObj.Push(pushOpts)
	if err != nil {
		return classifyPushError(err, remote)
	}

	return nil
}

// PushWithAuth 使用认证推送（仅限 HTTPS 方式，需要用户名和密码/令牌）
// 如果远程仓库是 SSH 协议，将返回友好错误提示用户改用 HTTPS 或配置 SSH 密钥
func (g *GitWrapper) PushWithAuth(remote, username, password string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	remoteObj, err := g.repo.Remote(remote)
	if err != nil {
		return fmt.Errorf("找不到远程仓库 %s: %w", remote, err)
	}

	// 检测远程 URL 协议：SSH URL 不支持 HTTP Basic Auth
	config := remoteObj.Config()
	if config != nil && len(config.URLs) > 0 && isSSHURL(config.URLs[0]) {
		return &AuthError{
			RemoteURL: config.URLs[0],
			AuthType:  "ssh",
			Cause:     fmt.Errorf("当前远程仓库使用 SSH 协议，不支持用户名/令牌认证。请改用 HTTPS 地址或配置 SSH 密钥"),
		}
	}

	err = remoteObj.Push(&git.PushOptions{
		RemoteName: remote,
		Auth: &http.BasicAuth{
			Username: username,
			Password: password,
		},
	})
	if err != nil {
		return classifyPushError(err, remote)
	}

	return nil
}

// GetRemoteURL 获取远程仓库的 URL
func (g *GitWrapper) GetRemoteURL(remote string) (string, error) {
	if err := g.ensureRepo(); err != nil {
		return "", err
	}

	remoteObj, err := g.repo.Remote(remote)
	if err != nil {
		return "", fmt.Errorf("找不到远程仓库 %s: %w", remote, err)
	}

	config := remoteObj.Config()
	if config == nil || len(config.URLs) == 0 {
		return "", fmt.Errorf("远程仓库 %s 没有配置地址", remote)
	}

	return config.URLs[0], nil
}

// SetRemoteURL 设置远程仓库的 URL（可用于将 SSH 地址切换为 HTTPS 地址）
func (g *GitWrapper) SetRemoteURL(remote, url string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	_, err := g.repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: remote,
		URLs: []string{url},
	})
	if err != nil {
		// 远程已存在，删除后重新创建
		if strings.Contains(err.Error(), "already exists") {
			_ = g.repo.DeleteRemote(remote)
			_, err = g.repo.CreateRemote(&gitconfig.RemoteConfig{
				Name: remote,
				URLs: []string{url},
			})
		}
		if err != nil {
			return fmt.Errorf("设置远程仓库地址失败: %w", err)
		}
	}

	return nil
}

// IsRemoteSSH 检查指定远程仓库是否使用 SSH 协议
func (g *GitWrapper) IsRemoteSSH(remote string) bool {
	remoteObj, err := g.repo.Remote(remote)
	if err != nil {
		return false
	}
	config := remoteObj.Config()
	if config == nil || len(config.URLs) == 0 {
		return false
	}
	return isSSHURL(config.URLs[0])
}

// isSSHURL 判断 URL 是否为 SSH 格式
func isSSHURL(url string) bool {
	return strings.HasPrefix(url, "git@") ||
		strings.HasPrefix(url, "ssh://") ||
		strings.HasPrefix(url, "git://")
}

// isHTTPSURL 判断 URL 是否为 HTTPS 格式
func isHTTPSURL(url string) bool {
	return strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")
}

// getAuthForRemote 根据远程 URL 自动选择认证方式
func (g *GitWrapper) getAuthForRemote(remoteObj *git.Remote) (transport.AuthMethod, error) {
	config := remoteObj.Config()
	if config == nil || len(config.URLs) == 0 {
		return nil, nil
	}

	url := config.URLs[0]

	// HTTPS 远程仓库：尝试从环境变量获取凭据
	if isHTTPSURL(url) {
		username := os.Getenv("GIT_HTTP_USERNAME")
		password := os.Getenv("GIT_HTTP_PASSWORD")
		if username != "" && password != "" {
			return &http.BasicAuth{
				Username: username,
				Password: password,
			}, nil
		}
		// 没有配置凭据，尝试无认证推送（公开仓库可能不需要）
		return nil, nil
	}

	// SSH 远程仓库：尝试多种认证方式
	if isSSHURL(url) {
		// 提取主机名，用于匹配 ~/.ssh/config
		host := extractSSHHost(url)

		// 1. 优先尝试 ~/.ssh/config 中配置的 IdentityFile
		if identityFile := getIdentityFileFromSSHConfig(host); identityFile != "" {
			auth, err := ssh.NewPublicKeysFromFile("git", identityFile, "")
			if err == nil {
				return auth, nil
			}
		}

		// 2. 尝试 SSH Agent（用户已加载密钥到 agent）
		auth, err := ssh.NewSSHAgentAuth("git")
		if err == nil {
			return auth, nil
		}

		// 3. 尝试默认的 SSH 密钥文件
		homeDir, err := os.UserHomeDir()
		if err == nil {
			for _, keyFile := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
				keyPath := filepath.Join(homeDir, ".ssh", keyFile)
				if _, statErr := os.Stat(keyPath); statErr == nil {
					auth, err := ssh.NewPublicKeysFromFile("git", keyPath, "")
					if err == nil {
						return auth, nil
					}
				}
			}
		}

		// SSH 认证都失败了，返回友好的错误信息
		return nil, &AuthError{
			RemoteURL: url,
			AuthType:  "ssh",
			Cause:     fmt.Errorf("SSH 密钥未配置或无法加载"),
		}
	}

	return nil, nil
}

// extractSSHHost 从 SSH URL 中提取主机名
// 例如 git@github.com:user/repo.git -> github.com
// ssh://git@github.com/user/repo.git -> github.com
func extractSSHHost(url string) string {
	if strings.HasPrefix(url, "ssh://") {
		// ssh://git@github.com/user/repo.git
		rest := strings.TrimPrefix(url, "ssh://")
		if atIdx := strings.Index(rest, "@"); atIdx >= 0 {
			rest = rest[atIdx+1:]
		}
		if slashIdx := strings.Index(rest, "/"); slashIdx >= 0 {
			return rest[:slashIdx]
		}
		// 可能包含端口
		if colonIdx := strings.Index(rest, ":"); colonIdx >= 0 {
			return rest[:colonIdx]
		}
		return rest
	}
	// git@github.com:user/repo.git
	if atIdx := strings.Index(url, "@"); atIdx >= 0 {
		rest := url[atIdx+1:]
		if colonIdx := strings.Index(rest, ":"); colonIdx >= 0 {
			return rest[:colonIdx]
		}
	}
	return ""
}

// getIdentityFileFromSSHConfig 从 ~/.ssh/config 中查找指定主机的 IdentityFile
func getIdentityFileFromSSHConfig(host string) string {
	if host == "" {
		return ""
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	configPath := filepath.Join(homeDir, ".ssh", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	return parseSSHConfigIdentityFile(string(data), host)
}

// parseSSHConfigIdentityFile 解析 ~/.ssh/config 内容，查找匹配主机的 IdentityFile
// 支持多 Host 块匹配，支持 Host 别名（如 github.com-xxx）映射到实际 Hostname
func parseSSHConfigIdentityFile(content, targetHost string) string {
	lines := strings.Split(content, "\n")

	type hostBlock struct {
		patterns      []string // Host 指令的模式列表
		hostname      string
		identityFile  string
	}

	var blocks []*hostBlock
	var current *hostBlock

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := strings.ToLower(parts[0])
		value := strings.Join(parts[1:], " ")

		if key == "host" {
			current = &hostBlock{
				patterns: strings.Fields(value),
			}
			blocks = append(blocks, current)
			continue
		}

		if current == nil {
			continue
		}

		switch key {
		case "hostname":
			current.hostname = value
		case "identityfile":
			// 展开 ~ 为 home 目录
			if strings.HasPrefix(value, "~") {
				homeDir, _ := os.UserHomeDir()
				value = filepath.Join(homeDir, value[1:])
			}
			current.identityFile = value
		}
	}

	// 两轮匹配：
	// 1. 先按 Host 别名匹配（如 Host github.com-jackz-jones）
	// 2. 再按 Hostname 匹配（如 Hostname github.com）
	// 如果 Host 别名匹配到了且有 IdentityFile，优先使用

	// 第一轮：Host 模式直接匹配目标主机名
	for _, block := range blocks {
		for _, pattern := range block.patterns {
			if matchSSHHostPattern(pattern, targetHost) {
				// 检查这个 block 的 Hostname 是否也匹配（如果有 Hostname 配置）
				if block.hostname != "" && block.hostname != targetHost {
					// Host 别名匹配但 Hostname 指向不同主机，跳过
					continue
				}
				if block.identityFile != "" {
					return block.identityFile
				}
			}
		}
	}

	// 第二轮：按 Hostname 字段匹配目标主机名
	for _, block := range blocks {
		if block.hostname == targetHost {
			if block.identityFile != "" {
				return block.identityFile
			}
		}
	}

	return ""
}

// matchSSHHostPattern 匹配 SSH config 的 Host 模式
// 支持 * 通配符，如 github.com-* 匹配 github.com-anything
func matchSSHHostPattern(pattern, host string) bool {
	if pattern == host {
		return true
	}
	if strings.Contains(pattern, "*") {
		// 简单的通配符匹配
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(host, parts[0]) && strings.HasSuffix(host, parts[1])
		}
	}
	return false
}

// AuthError 认证错误（用于提供友好的错误信息）
type AuthError struct {
	RemoteURL string
	AuthType  string // "ssh" 或 "https"
	Cause     error
}

func (e *AuthError) Error() string {
	return e.Cause.Error()
}

// classifyPushError 对推送错误进行分类，返回用户友好的错误信息
func classifyPushError(err error, remote string) error {
	msg := err.Error()

	// SSH 认证失败
	if containsAnyLower(msg,
		"permission denied",
		"authentication failed",
		"unable to authenticate",
		"ssh: handshake failed",
		"host key",
		"knownhosts",
		"no supported methods remain",
	) {
		return &AuthError{
			RemoteURL: remote,
			AuthType:  "ssh",
			Cause:     fmt.Errorf("远程仓库连接认证失败"),
		}
	}

	// HTTPS 认证失败 / 认证方式不匹配
	if containsAnyLower(msg,
		"authorization failed",
		"access denied",
		"403",
		"credentials",
		"authentication required",
		"invalid auth method",
	) {
		return &AuthError{
			RemoteURL: remote,
			AuthType:  "https",
			Cause:     fmt.Errorf("远程仓库访问凭据无效"),
		}
	}

	// 网络连接问题
	if containsAnyLower(msg,
		"connection refused",
		"connection timed out",
		"no such host",
		"network is unreachable",
		"i/o timeout",
		"dial tcp",
	) {
		return fmt.Errorf("无法连接到远程仓库，请检查网络连接")
	}

	// 远程仓库拒绝推送（非快进）
	if containsAnyLower(msg,
		"non-fast-forward",
		"would clobber",
		"updates were rejected",
	) {
		return fmt.Errorf("远程仓库有更新的内容，请先拉取最新修改再推送")
	}

	// 仓库不存在
	if containsAnyLower(msg, "repository not found", "not found") {
		return fmt.Errorf("远程仓库不存在，请确认仓库地址是否正确")
	}

	return fmt.Errorf("推送到 %s 失败: %w", remote, err)
}

// containsAnyLower 不区分大小写地检查字符串是否包含任一子串
func containsAnyLower(s string, substrs ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}

// Pull 从远程仓库拉取
func (g *GitWrapper) Pull(remote string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	wt, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("获取工作区失败: %w", err)
	}

	pullOpts := &git.PullOptions{
		RemoteName: remote,
	}

	// 自动检测远程 URL 类型并配置认证
	remoteObj, err := g.repo.Remote(remote)
	if err == nil {
		auth, authErr := g.getAuthForRemote(remoteObj)
		if authErr != nil {
			return fmt.Errorf("拉取失败，远程仓库连接有问题: %w", authErr)
		}
		if auth != nil {
			pullOpts.Auth = auth
		}
	}

	err = wt.Pull(pullOpts)
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return classifyPullError(err, remote)
	}

	return nil
}

// classifyPullError 对拉取错误进行分类，返回用户友好的错误信息
func classifyPullError(err error, remote string) error {
	msg := err.Error()

	// 认证失败
	if containsAnyLower(msg,
		"permission denied",
		"authentication failed",
		"unable to authenticate",
		"authorization failed",
		"access denied",
		"403",
	) {
		return &AuthError{
			RemoteURL: remote,
			AuthType:  "ssh",
			Cause:     fmt.Errorf("远程仓库连接认证失败"),
		}
	}

	// 网络连接问题
	if containsAnyLower(msg,
		"connection refused",
		"connection timed out",
		"no such host",
		"network is unreachable",
		"i/o timeout",
		"dial tcp",
	) {
		return fmt.Errorf("无法连接到远程仓库，请检查网络连接")
	}

	return fmt.Errorf("从 %s 拉取失败: %w", remote, err)
}

// CreateTag 创建标签
func (g *GitWrapper) CreateTag(name string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	head, err := g.repo.Head()
	if err != nil {
		return fmt.Errorf("获取 HEAD 失败: %w", err)
	}

	_, err = g.repo.CreateTag(name, head.Hash(), &git.CreateTagOptions{
		Tagger: &object.Signature{
			Name:  "Git Agent",
			Email: "agent@git-agent.dev",
			When:  time.Now(),
		},
		Message: name,
	})
	if err != nil {
		return fmt.Errorf("创建标签失败: %w", err)
	}

	return nil
}

// GetRepo 获取底层 go-git Repository 对象（高级用途）
func (g *GitWrapper) GetRepo() *git.Repository {
	return g.repo
}

// ensureRepo 确保仓库已初始化
// GetLocalUserConfig 从 .git/config 中读取本地仓库的用户配置（user.name 和 user.email）
func (g *GitWrapper) GetLocalUserConfig() (name, email string, err error) {
	if err := g.ensureRepo(); err != nil {
		return "", "", err
	}

	cfg, err := g.repo.Config()
	if err != nil {
		return "", "", fmt.Errorf("读取仓库配置失败: %w", err)
	}

	return cfg.User.Name, cfg.User.Email, nil
}

// SetLocalUserConfig 将用户信息保存到 .git/config 中（持久化）
func (g *GitWrapper) SetLocalUserConfig(name, email string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	cfg, err := g.repo.Config()
	if err != nil {
		return fmt.Errorf("读取仓库配置失败: %w", err)
	}

	if name != "" {
		cfg.User.Name = name
	}
	if email != "" {
		cfg.User.Email = email
	}

	err = g.repo.SetConfig(cfg)
	if err != nil {
		return fmt.Errorf("保存仓库配置失败: %w", err)
	}

	return nil
}

// GetAheadBehind 获取当前分支相对远程跟踪分支的领先/落后提交数
func (g *GitWrapper) GetAheadBehind() (*AheadBehind, error) {
	if err := g.ensureRepo(); err != nil {
		return nil, err
	}

	// 获取当前分支的 HEAD 引用
	headRef, err := g.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("获取 HEAD 引用失败: %w", err)
	}

	// 获取当前分支名
	branchName := headRef.Name().Short()

	// 查找对应的远程跟踪分支引用
	remoteRefName := plumbing.ReferenceName("refs/remotes/origin/" + branchName)
	remoteRef, err := g.repo.Reference(remoteRefName, false)
	if err != nil {
		// 没有远程跟踪分支，说明可能还没有推送过或者没有远程仓库
		return nil, nil
	}

	localHash := headRef.Hash()
	remoteHash := remoteRef.Hash()

	// 如果本地和远程指向同一个 commit，则既不领先也不落后
	if localHash == remoteHash {
		return &AheadBehind{
			Ahead:  0,
			Behind: 0,
			Remote: "origin",
			Branch: branchName,
		}, nil
	}

	// 通过遍历 commit 链来计算 ahead/behind
	ahead := 0
	behind := 0

	// 计算 ahead：从 local HEAD 到 remote HEAD 有多少个 commit
	localCommit, err := g.repo.CommitObject(localHash)
	if err != nil {
		return nil, fmt.Errorf("获取本地 HEAD 提交失败: %w", err)
	}

	// 从 local HEAD 往回遍历，直到找到 remote HEAD
	iter, err := g.repo.Log(&git.LogOptions{From: localHash})
	if err != nil {
		return nil, fmt.Errorf("获取本地提交历史失败: %w", err)
	}

	foundRemote := false
	err = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == remoteHash {
			foundRemote = true
			return fmt.Errorf("stop")
		}
		ahead++
		return nil
	})
	if err != nil && err.Error() != "stop" {
		return nil, err
	}

	if !foundRemote {
		// remote HEAD 不在 local 的历史中，说明分叉了
		// 尝试从 remote HEAD 往回遍历
		ahead = 0
		remoteIter, err := g.repo.Log(&git.LogOptions{From: remoteHash})
		if err != nil {
			return nil, nil // 无法确定，返回 nil
		}
		foundLocal := false
		err = remoteIter.ForEach(func(c *object.Commit) error {
			if c.Hash == localHash {
				foundLocal = true
				return fmt.Errorf("stop")
			}
			behind++
			return nil
		})
		if err != nil && err.Error() != "stop" {
			return nil, nil
		}

		if !foundLocal {
			// 真正的分叉情况，需要更复杂的 merge-base 计算
			// 简化处理：只报告分叉
			return &AheadBehind{
				Ahead:  -1, // -1 表示分叉
				Behind: -1,
				Remote: "origin",
				Branch: branchName,
			}, nil
		}
	} else {
		// remote HEAD 在 local 的历史中，计算 behind
		behind = 0
		remoteIter, err := g.repo.Log(&git.LogOptions{From: remoteHash})
		if err != nil {
			return &AheadBehind{
				Ahead:  ahead,
				Behind: 0,
				Remote: "origin",
				Branch: branchName,
			}, nil
		}
		err = remoteIter.ForEach(func(c *object.Commit) error {
			if c.Hash == localHash {
				return fmt.Errorf("stop")
			}
			behind++
			return nil
		})
		if err != nil && err.Error() != "stop" {
			return &AheadBehind{
				Ahead:  ahead,
				Behind: 0,
				Remote: "origin",
				Branch: branchName,
			}, nil
		}
	}

	// 如果不使用 localCommit 变量，避免编译错误
	_ = localCommit

	return &AheadBehind{
		Ahead:  ahead,
		Behind: behind,
		Remote: "origin",
		Branch: branchName,
	}, nil
}

func (g *GitWrapper) ensureRepo() error {
	if g.repo == nil {
		return fmt.Errorf("仓库尚未初始化，请先初始化仓库")
	}
	return nil
}

// translateStatusCode 将 git 状态码翻译为可读文字
func translateStatusCode(worktree, staging git.StatusCode) string {
	switch {
	case staging == git.Added:
		return "已暂存（新增）"
	case staging == git.Modified:
		return "已暂存（修改）"
	case staging == git.Deleted:
		return "已暂存（删除）"
	case worktree == git.Modified:
		return "已修改（未暂存）"
	case worktree == git.Deleted:
		return "已删除（未暂存）"
	case worktree == git.Untracked:
		return "未跟踪"
	default:
		return "未知"
	}
}

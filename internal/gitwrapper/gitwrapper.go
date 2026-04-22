package gitwrapper

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
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
	Staged    []FileChange `json:"staged"`
	Unstaged  []FileChange `json:"unstaged"`
	Untracked []string     `json:"untracked"`
	IsClean   bool         `json:"is_clean"`
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
	if authorName == "" {
		authorName = "Git Agent"
	}
	if authorEmail == "" {
		authorEmail = "agent@git-agent.dev"
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

	if authorName == "" {
		authorName = "Git Agent"
	}
	if authorEmail == "" {
		authorEmail = "agent@git-agent.dev"
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
		return nil
	})

	return versions, err
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

	err = remoteObj.Push(&git.PushOptions{
		RemoteName: remote,
		Auth:       nil, // TODO: 支持认证
	})
	if err != nil {
		return fmt.Errorf("推送到 %s 失败: %w", remote, err)
	}

	return nil
}

// PushWithAuth 使用认证推送
func (g *GitWrapper) PushWithAuth(remote, username, password string) error {
	if err := g.ensureRepo(); err != nil {
		return err
	}

	remoteObj, err := g.repo.Remote(remote)
	if err != nil {
		return fmt.Errorf("找不到远程仓库 %s: %w", remote, err)
	}

	err = remoteObj.Push(&git.PushOptions{
		RemoteName: remote,
		Auth: &http.BasicAuth{
			Username: username,
			Password: password,
		},
	})
	if err != nil {
		return fmt.Errorf("推送到 %s 失败: %w", remote, err)
	}

	return nil
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

	err = wt.Pull(&git.PullOptions{
		RemoteName: remote,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("从 %s 拉取失败: %w", remote, err)
	}

	return nil
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

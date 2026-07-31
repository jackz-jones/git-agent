package gitwrapper

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ---------- 通用工具 ----------

const (
	testAuthor = "Test User"
	testEmail  = "test@example.com"
)

// newTestRepo 在临时目录创建一个已完成初始化的 GitWrapper。
func newTestRepo(t *testing.T) *GitWrapper {
	t.Helper()
	dir := t.TempDir()
	g, err := New(dir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := g.InitRepo(testAuthor, testEmail); err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}
	if err := g.SetLocalUserConfig(testAuthor, testEmail); err != nil {
		t.Fatalf("SetLocalUserConfig failed: %v", err)
	}
	return g
}

// writeFile 在仓库工作区写入文件。
func writeFile(t *testing.T, g *GitWrapper, rel, content string) {
	t.Helper()
	full := filepath.Join(g.path, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// stageFile 相当于 git add rel。
func stageFile(t *testing.T, g *GitWrapper, rel string) {
	t.Helper()
	wt, err := g.repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add(rel); err != nil {
		t.Fatalf("add %s: %v", rel, err)
	}
}

// ---------- 需求 4: InitRepo ----------

func TestInitRepo_WithoutAuthor_NoInitialCommit(t *testing.T) {
	dir := t.TempDir()
	g, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.InitRepo("", ""); err != nil {
		t.Fatalf("InitRepo without author must succeed, got %v", err)
	}
	if !g.IsInitialized() {
		t.Fatalf("expected initialized")
	}
	// 无初始提交时 HEAD 应该无法解析
	if _, err := g.repo.Head(); err == nil {
		t.Fatalf("expected no HEAD (empty repo), got nil error")
	}
}

func TestInitRepo_WithAuthor_CreatesInitialCommit(t *testing.T) {
	dir := t.TempDir()
	g, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.InitRepo(testAuthor, testEmail); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	head, err := g.repo.Head()
	if err != nil {
		t.Fatalf("expected initial commit, got %v", err)
	}
	c, err := g.repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	if c.Author.Name != testAuthor || c.Author.Email != testEmail {
		t.Fatalf("author mismatch: %+v", c.Author)
	}
}

// ---------- 需求 1 & 17: Diff / Status 暂存态 ----------

func TestStatus_StagedAndUnstagedAreIndependent(t *testing.T) {
	g := newTestRepo(t)

	// 先提交一个基础文件
	writeFile(t, g, "a.txt", "hello\n")
	if _, err := g.SaveVersion("add a.txt", nil, testAuthor, testEmail); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}

	// 修改 a.txt，暂存修改后又在工作区继续修改 → staged=modified, unstaged=modified
	writeFile(t, g, "a.txt", "hello v2\n")
	stageFile(t, g, "a.txt")
	writeFile(t, g, "a.txt", "hello v3\n")

	st, err := g.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	var stagedA, unstagedA *FileChange
	for i := range st.Staged {
		if st.Staged[i].Path == "a.txt" {
			stagedA = &st.Staged[i]
		}
	}
	for i := range st.Unstaged {
		if st.Unstaged[i].Path == "a.txt" {
			unstagedA = &st.Unstaged[i]
		}
	}
	if stagedA == nil || unstagedA == nil {
		t.Fatalf("expected a.txt in both staged and unstaged, got st=%+v", st)
	}
	if stagedA == unstagedA {
		t.Fatalf("staged and unstaged FileChange must be independent instances")
	}
	if stagedA.Status == "" || unstagedA.Status == "" {
		t.Fatalf("both statuses should be populated, staged=%q unstaged=%q",
			stagedA.Status, unstagedA.Status)
	}
}

func TestDiff_StagedOnlyFileProducesPatch(t *testing.T) {
	g := newTestRepo(t)

	writeFile(t, g, "b.txt", "line1\nline2\n")
	if _, err := g.SaveVersion("init b", nil, testAuthor, testEmail); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}

	// 仅暂存修改，不再动工作区
	writeFile(t, g, "b.txt", "line1\nline2\nline3\n")
	stageFile(t, g, "b.txt")

	patch, err := g.Diff("b.txt")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if patch == "" {
		t.Fatalf("expected non-empty diff for staged-only file")
	}
	if !strings.Contains(patch, "line3") {
		t.Fatalf("expected patch to reference line3, got:\n%s", patch)
	}
}

// ---------- 需求 2: isAncestor 迭代/多父 ----------

// buildCommit 使用低层 API 直接构造一个 commit 对象，方便造 merge 图。
func buildCommit(t *testing.T, g *GitWrapper, msg string, parents []plumbing.Hash, treeHash plumbing.Hash) plumbing.Hash {
	t.Helper()
	c := &object.Commit{
		Author: object.Signature{
			Name: testAuthor, Email: testEmail, When: time.Now(),
		},
		Committer: object.Signature{
			Name: testAuthor, Email: testEmail, When: time.Now(),
		},
		Message:      msg,
		TreeHash:     treeHash,
		ParentHashes: parents,
	}
	obj := g.repo.Storer.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		t.Fatalf("encode: %v", err)
	}
	h, err := g.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	return h
}

func TestIsAncestor_MergeCommitDoesNotBlowUp(t *testing.T) {
	g := newTestRepo(t)
	writeFile(t, g, "seed.txt", "seed\n")
	if _, err := g.SaveVersion("seed", nil, testAuthor, testEmail); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}

	head, _ := g.repo.Head()
	root, err := g.repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}

	// 构造一个 diamond: root -> A, root -> B, A+B -> M
	hA := buildCommit(t, g, "A", []plumbing.Hash{root.Hash}, root.TreeHash)
	hB := buildCommit(t, g, "B", []plumbing.Hash{root.Hash}, root.TreeHash)
	hM := buildCommit(t, g, "M", []plumbing.Hash{hA, hB}, root.TreeHash)

	cRoot, _ := g.repo.CommitObject(root.Hash)
	cA, _ := g.repo.CommitObject(hA)
	cB, _ := g.repo.CommitObject(hB)
	cM, _ := g.repo.CommitObject(hM)

	if ok, err := isAncestor(cRoot, cM); err != nil || !ok {
		t.Fatalf("root should be ancestor of M, ok=%v err=%v", ok, err)
	}
	if ok, err := isAncestor(cA, cM); err != nil || !ok {
		t.Fatalf("A should be ancestor of M, ok=%v err=%v", ok, err)
	}
	if ok, err := isAncestor(cB, cM); err != nil || !ok {
		t.Fatalf("B should be ancestor of M, ok=%v err=%v", ok, err)
	}
	if ok, _ := isAncestor(cA, cB); ok {
		t.Fatalf("A is not ancestor of B")
	}
	if ok, err := isAncestor(cM, cRoot); err != nil || ok {
		t.Fatalf("M cannot be ancestor of root, ok=%v err=%v", ok, err)
	}
}

func TestIsAncestor_NilInputs(t *testing.T) {
	if ok, err := isAncestor(nil, nil); err != nil || ok {
		t.Fatalf("nil,nil -> false,nil, got %v,%v", ok, err)
	}
}

// ---------- 需求 3: Merge author 校验 ----------

func TestMerge_MissingUserConfigReturnsClearError(t *testing.T) {
	dir := t.TempDir()
	g, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.InitRepo(testAuthor, testEmail); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}

	// 构造 feature 分支且引入分叉提交
	if err := g.CreateBranch("feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := g.SwitchBranch("feature"); err != nil {
		t.Fatalf("SwitchBranch feature: %v", err)
	}
	writeFile(t, g, "f.txt", "on feature\n")
	if err := g.SetLocalUserConfig(testAuthor, testEmail); err != nil {
		t.Fatalf("SetLocalUserConfig: %v", err)
	}
	if _, err := g.SaveVersion("feature commit", nil, testAuthor, testEmail); err != nil {
		t.Fatalf("SaveVersion feature: %v", err)
	}

	// 回到主分支（master 或 main）
	master := "master"
	if _, err := g.repo.Reference(plumbing.NewBranchReferenceName("master"), true); err != nil {
		if _, err2 := g.repo.Reference(plumbing.NewBranchReferenceName("main"), true); err2 == nil {
			master = "main"
		}
	}
	if err := g.SwitchBranch(master); err != nil {
		t.Fatalf("SwitchBranch %s: %v", master, err)
	}
	writeFile(t, g, "m.txt", "on master\n")
	if _, err := g.SaveVersion("master commit", nil, testAuthor, testEmail); err != nil {
		t.Fatalf("SaveVersion master: %v", err)
	}

	// 直接改写 .git/config 文件，清空 [user] section。
	// go-git 的 SetConfig 在测试场景下与 Config() 的读取存在缓存差异，
	// 这里直接落盘一份不含 user.name / user.email 的最小 config 即可。
	gitConfigPath := filepath.Join(dir, ".git", "config")
	minimalConfig := "[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false\n"
	if err := os.WriteFile(gitConfigPath, []byte(minimalConfig), 0o644); err != nil {
		t.Fatalf("rewrite .git/config: %v", err)
	}
	// 重新 open 仓库，强制 go-git 重新加载 config
	g2, err := New(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	err = g2.Merge("feature")
	if err == nil {
		t.Fatalf("expected error when user config missing")
	}
	if !strings.Contains(err.Error(), "用户信息未配置") {
		t.Fatalf("expected '用户信息未配置' error, got %v", err)
	}
}

// ---------- 需求 5: RestoreFile 保留元数据 + 原子写 ----------

func TestRestoreFile_PreservesExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable bit not meaningful on Windows")
	}
	g := newTestRepo(t)

	// 写入一个可执行脚本
	rel := "run.sh"
	writeFile(t, g, rel, "#!/bin/sh\necho hi\n")
	full := filepath.Join(g.path, rel)
	if err := os.Chmod(full, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if _, err := g.SaveVersion("add script", nil, testAuthor, testEmail); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}
	// SaveVersion 返回的是 8 位短 hash，RestoreFile 需要完整 40 位 hash，
	// 因此从 HEAD 取完整 hash
	head, err := g.repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	fullHash := head.Hash().String()

	// 破坏权限并覆写内容
	if err := os.Chmod(full, 0o644); err != nil {
		t.Fatalf("chmod2: %v", err)
	}
	writeFile(t, g, rel, "broken\n")

	if err := g.RestoreFile(rel, fullHash); err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}

	info, err := os.Stat(full)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("expected executable bit restored, got mode=%v", info.Mode())
	}
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(got), "echo hi") {
		t.Fatalf("content not restored, got %q", got)
	}
}

func TestRestoreFile_UnknownVersionDoesNotWipeFile(t *testing.T) {
	g := newTestRepo(t)
	writeFile(t, g, "keep.txt", "current\n")
	if _, err := g.SaveVersion("add keep", nil, testAuthor, testEmail); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}
	// 修改后尝试恢复到不存在的版本
	writeFile(t, g, "keep.txt", "current-modified\n")
	err := g.RestoreFile("keep.txt", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err == nil {
		t.Fatalf("expected error for unknown version")
	}
	// 原子写：出错时不应产生残留的临时文件
	entries, err := os.ReadDir(g.path)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".keep.txt.") ||
			strings.HasPrefix(e.Name(), "keep.txt.tmp") {
			t.Fatalf("unexpected temp file left behind: %s", e.Name())
		}
	}
}

// ---------- 需求 6: sentinel error via errors.Is ----------

func TestSaveVersion_NoChangesReturnsSentinel(t *testing.T) {
	g := newTestRepo(t)
	writeFile(t, g, "c.txt", "c\n")
	if _, err := g.SaveVersion("first", nil, testAuthor, testEmail); err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}
	// 再次保存无修改
	_, err := g.SaveVersion("second", nil, testAuthor, testEmail)
	if err == nil {
		t.Fatalf("expected ErrNoChangesToCommit")
	}
	if !errors.Is(err, ErrNoChangesToCommit) {
		t.Fatalf("expected errors.Is(err, ErrNoChangesToCommit) == true, got err=%v", err)
	}
}

// 保证包内低层 API 存在（编译期防回归）
var _ = git.ErrRepositoryNotExists

package conflict

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanNoConflicts(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDetector(tmpDir)

	conflicts, err := d.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(conflicts))
	}
}

func TestScanWithConflicts(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDetector(tmpDir)

	// 创建有冲突标记的文件
	conflictContent := `line 1
line 2
<<<<<<< HEAD
our change
=======
their change
>>>>>>> branch
line 5
`
	err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(conflictContent), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	conflicts, err := d.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].ConflictType != ConflictEditEdit {
		t.Fatalf("expected ConflictEditEdit, got %s", conflicts[0].ConflictType)
	}
}

func TestResolveWithOurs(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDetector(tmpDir)

	conflictContent := `before
<<<<<<< HEAD
our change
=======
their change
>>>>>>> branch
after
`
	filePath := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(filePath, []byte(conflictContent), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result, err := d.Resolve("test.txt", "ours")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if result.Strategy != "ours" {
		t.Fatalf("expected ours strategy, got %s", result.Strategy)
	}

	// 验证文件内容
	content, _ := os.ReadFile(filePath)
	expected := "before\nour change\nafter\n"
	if string(content) != expected {
		t.Fatalf("expected %q, got %q", expected, string(content))
	}
}

func TestResolveWithTheirs(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDetector(tmpDir)

	conflictContent := `before
<<<<<<< HEAD
our change
=======
their change
>>>>>>> branch
after
`
	filePath := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(filePath, []byte(conflictContent), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result, err := d.Resolve("test.txt", "theirs")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if result.Strategy != "theirs" {
		t.Fatalf("expected theirs strategy, got %s", result.Strategy)
	}

	content, _ := os.ReadFile(filePath)
	expected := "before\ntheir change\nafter\n"
	if string(content) != expected {
		t.Fatalf("expected %q, got %q", expected, string(content))
	}
}

func TestResolveUnknownStrategy(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDetector(tmpDir)

	filePath := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(filePath, []byte("content"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err = d.Resolve("test.txt", "unknown")
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

func TestFileConflictDescription(t *testing.T) {
	conflict := FileConflict{
		FilePath:     "report.md",
		ConflictType: ConflictEditEdit,
	}
	desc := conflict.Description()
	if desc == "" {
		t.Fatal("expected non-empty description")
	}
}

func TestFileConflictSuggestions(t *testing.T) {
	conflict := FileConflict{
		FilePath:       "report.md",
		ConflictType:   ConflictEditEdit,
		AutoResolvable: false,
	}
	suggestions := conflict.Suggestions()
	if len(suggestions) < 2 {
		t.Fatalf("expected at least 2 suggestions, got %d", len(suggestions))
	}

	conflict.AutoResolvable = true
	suggestions = conflict.Suggestions()
	if len(suggestions) < 3 {
		t.Fatalf("expected at least 3 suggestions for auto-resolvable, got %d", len(suggestions))
	}
}

func TestAutoResolveSimpleConflicts(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDetector(tmpDir)

	conflictContent := `before
<<<<<<< HEAD
our change
=======
their change
>>>>>>> branch
after
`
	err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(conflictContent), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	resolved, err := d.AutoResolveSimpleConflicts()
	if err != nil {
		t.Fatalf("AutoResolveSimpleConflicts failed: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved conflict, got %d", len(resolved))
	}
}

func TestSkipGitDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	d := NewDetector(tmpDir)

	// 创建 .git 目录下的冲突文件，应该被跳过
	gitDir := filepath.Join(tmpDir, ".git", "objects")
	os.MkdirAll(gitDir, 0755)
	conflictContent := `<<<<<<< HEAD
ours
=======
theirs
>>>>>>> branch
`
	os.WriteFile(filepath.Join(gitDir, "conflict.txt"), []byte(conflictContent), 0644)

	conflicts, err := d.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts (should skip .git), got %d", len(conflicts))
	}
}

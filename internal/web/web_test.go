package web

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidateWorkspacePath 覆盖路径校验的主要场景。
func TestValidateWorkspacePath(t *testing.T) {
	tmp := t.TempDir()

	// 正常目录
	if got, err := ValidateWorkspacePath(tmp); err != nil || got == "" {
		t.Fatalf("正常目录校验失败: got=%q err=%v", got, err)
	}

	// 空字符串
	if _, err := ValidateWorkspacePath("   "); err == nil {
		t.Fatal("空字符串应返回错误")
	}

	// 不存在的路径
	missing := filepath.Join(tmp, "not-exist")
	if _, err := ValidateWorkspacePath(missing); err == nil {
		t.Fatal("不存在路径应返回错误")
	}

	// 文件（不是目录）
	file := filepath.Join(tmp, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateWorkspacePath(file); err == nil {
		t.Fatal("文件路径应返回错误")
	}

	// 系统保留目录（以 "/" 为例，Windows 场景跳过即可）
	if _, err := ValidateWorkspacePath("/"); err == nil {
		t.Fatal("根目录应被拒绝")
	}

	// 家目录本身
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := ValidateWorkspacePath(home); err == nil {
			t.Fatal("家目录本身应被拒绝")
		}
	}
}

// TestSafeJoin 检查路径穿越防护。
func TestSafeJoin(t *testing.T) {
	root := t.TempDir()

	// 空 rel 应返回 root 本身
	p, err := safeJoin(root, "")
	if err != nil || p != root {
		t.Fatalf("空 rel 应返回 root，got=%q err=%v", p, err)
	}

	// 子目录
	if _, err := safeJoin(root, "sub/a.txt"); err != nil {
		t.Fatalf("合法子路径应通过: %v", err)
	}

	// 越界
	if _, err := safeJoin(root, "../outside"); err == nil {
		t.Fatal("越界路径应返回错误")
	}
	if _, err := safeJoin(root, "/etc/passwd"); err == nil {
		t.Fatal("绝对路径越界应返回错误")
	}
}

// TestRecentStore 验证持久化行为：去重、倒序、上限裁剪。
func TestRecentStore(t *testing.T) {
	dir := t.TempDir()
	rs, err := NewRecentStore(dir)
	if err != nil {
		t.Fatalf("NewRecentStore: %v", err)
	}

	// 插入多个 + 去重
	_ = rs.Add("/a")
	_ = rs.Add("/b")
	_ = rs.Add("/a") // 更新 /a 时间，应在最前

	list := rs.List()
	if len(list) != 2 {
		t.Fatalf("去重失败: %+v", list)
	}
	if list[0].Path != "/a" {
		t.Fatalf("最新项应排第一，得到: %+v", list)
	}

	// 上限裁剪
	for i := 0; i < RecentMaxEntries+5; i++ {
		_ = rs.Add(filepath.Join("/p", string(rune('a'+i))))
	}
	if got := len(rs.List()); got != RecentMaxEntries {
		t.Fatalf("上限裁剪失败: %d", got)
	}

	// 重新加载应能读到数据
	rs2, err := NewRecentStore(dir)
	if err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if len(rs2.List()) != RecentMaxEntries {
		t.Fatalf("重新加载数据不一致")
	}
}

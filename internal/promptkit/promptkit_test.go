package promptkit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKitLoad(t *testing.T) {
	kit := NewKit(ResourcesFS, Config{})
	if err := kit.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 验证内置 Skills 已加载
	skills := kit.ListAll()
	if len(skills) == 0 {
		t.Fatal("Expected at least some skills/rules to be loaded")
	}

	// 验证 commit-message skill 存在
	found := false
	for _, s := range skills {
		if s.Name == "commit-message" && s.Type == SkillTypeSkill {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Expected commit-message skill to be loaded")
	}

	// 验证 no-git-terms rule 存在
	found = false
	for _, s := range skills {
		if s.Name == "no-git-terms" && s.Type == SkillTypeRule {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Expected no-git-terms rule to be loaded")
	}
}

func TestKitGetPromptForIntents(t *testing.T) {
	kit := NewKit(ResourcesFS, Config{})
	if err := kit.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 测试 save_version 意图应该加载 commit-message skill 和 always-execute rule
	prompt := kit.GetPromptForIntents([]string{"save_version"})
	if prompt == "" {
		t.Fatal("Expected non-empty prompt for save_version intent")
	}

	// 验证包含 commit-message 内容
	if !containsSubstring(prompt, "commit-message") {
		t.Error("Expected prompt to contain commit-message skill")
	}

	// 验证包含全局规则 no-git-terms
	if !containsSubstring(prompt, "no-git-terms") {
		t.Error("Expected prompt to contain no-git-terms global rule")
	}
}

func TestKitUserOverride(t *testing.T) {
	// 创建临时用户目录
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 写入自定义 commit-message.md（覆盖内置）
	customContent := `<!-- meta: {"intents":["save_version"],"priority":10,"description":"自定义 commit 规范"} -->

# 自定义 Commit 规范
使用中文写 commit message。
`
	if err := os.WriteFile(filepath.Join(skillsDir, "commit-message.md"), []byte(customContent), 0644); err != nil {
		t.Fatal(err)
	}

	kit := NewKit(ResourcesFS, Config{UserDir: tmpDir})
	if err := kit.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	prompt := kit.GetPromptForIntents([]string{"save_version"})
	if !containsSubstring(prompt, "自定义 Commit 规范") {
		t.Error("Expected user override to take effect")
	}
	if containsSubstring(prompt, "必须使用英文") {
		t.Error("Expected builtin content to be overridden by user content")
	}
}

func TestKitHotReload(t *testing.T) {
	// 创建临时用户目录
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	rulesDir := filepath.Join(tmpDir, "rules")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}

	kit := NewKit(ResourcesFS, Config{UserDir: tmpDir})
	if err := kit.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 启动热加载
	if err := kit.WatchAndHotReload(); err != nil {
		t.Fatalf("WatchAndHotReload failed: %v", err)
	}
	defer kit.Close()

	// 初始 prompt 不包含自定义内容
	prompt := kit.GetPromptForIntents([]string{"save_version"})
	if containsSubstring(prompt, "热加载测试规则") {
		t.Fatal("Should not contain hot-reload test content before file creation")
	}

	// 写入新 rule 文件
	newRule := `<!-- meta: {"intents":["save_version"],"priority":1,"description":"热加载测试规则"} -->

# 热加载测试规则
这是热加载测试内容。
`
	if err := os.WriteFile(filepath.Join(rulesDir, "hot-reload-test.md"), []byte(newRule), 0644); err != nil {
		t.Fatal(err)
	}

	// 等待热加载生效（防抖 300ms + 文件系统延迟）
	time.Sleep(800 * time.Millisecond)

	// 验证新 rule 已加载
	prompt = kit.GetPromptForIntents([]string{"save_version"})
	if !containsSubstring(prompt, "热加载测试规则") {
		t.Error("Expected hot-reloaded rule to appear in prompt")
	}

	// 验证重载时间已更新
	if kit.LastReloadTime().IsZero() {
		t.Error("Expected LastReloadTime to be set after hot reload")
	}
}

func TestKitDisabled(t *testing.T) {
	kit := NewKit(ResourcesFS, Config{Disabled: []string{"display-format"}})
	if err := kit.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	skills := kit.ListAll()
	for _, s := range skills {
		if s.Name == "display-format" && s.Enabled {
			t.Error("Expected display-format to be disabled")
		}
	}

	// display-format 不应出现在 prompt 中
	prompt := kit.GetPromptForIntents([]string{"view_history"})
	if containsSubstring(prompt, "display-format") {
		t.Error("Expected disabled skill not to appear in prompt")
	}
}

func TestKitProjectOverride(t *testing.T) {
	// 创建临时目录
	userDir := t.TempDir()
	projectDir := t.TempDir()

	userSkillsDir := filepath.Join(userDir, "skills")
	projectSkillsDir := filepath.Join(projectDir, "skills")
	os.MkdirAll(userSkillsDir, 0755)
	os.MkdirAll(projectSkillsDir, 0755)

	// 用户级覆盖
	userContent := `<!-- meta: {"intents":["save_version"],"priority":10,"description":"用户级覆盖"} -->

# 用户级 Commit 规范
用户级内容。
`
	os.WriteFile(filepath.Join(userSkillsDir, "commit-message.md"), []byte(userContent), 0644)

	// 项目级覆盖（应覆盖用户级）
	projectContent := `<!-- meta: {"intents":["save_version"],"priority":10,"description":"项目级覆盖"} -->

# 项目级 Commit 规范
项目级内容。
`
	os.WriteFile(filepath.Join(projectSkillsDir, "commit-message.md"), []byte(projectContent), 0644)

	kit := NewKit(ResourcesFS, Config{UserDir: userDir, ProjectDir: projectDir})
	if err := kit.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	prompt := kit.GetPromptForIntents([]string{"save_version"})
	if !containsSubstring(prompt, "项目级 Commit 规范") {
		t.Error("Expected project-level override to take precedence")
	}
	if containsSubstring(prompt, "用户级 Commit 规范") {
		t.Error("Expected user-level content to be overridden by project-level")
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

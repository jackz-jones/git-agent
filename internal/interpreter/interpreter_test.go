package interpreter

import (
	"testing"
)

func TestParse(t *testing.T) {
	p := New(".")

	t.Run("parse save version intent", func(t *testing.T) {
		intent, err := p.Parse("保存修改")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if intent.Type != IntentSaveVersion {
			t.Fatalf("expected IntentSaveVersion, got %s", intent.Type)
		}
	})

	t.Run("parse view history intent", func(t *testing.T) {
		intent, err := p.Parse("查看历史")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if intent.Type != IntentViewHistory {
			t.Fatalf("expected IntentViewHistory, got %s", intent.Type)
		}
	})

	t.Run("parse view status intent", func(t *testing.T) {
		intent, err := p.Parse("查看状态")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if intent.Type != IntentViewStatus {
			t.Fatalf("expected IntentViewStatus, got %s", intent.Type)
		}
	})

	t.Run("parse init repo intent", func(t *testing.T) {
		intent, err := p.Parse("初始化仓库")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if intent.Type != IntentInitRepo {
			t.Fatalf("expected IntentInitRepo, got %s", intent.Type)
		}
	})

	t.Run("parse help intent", func(t *testing.T) {
		intent, err := p.Parse("帮助")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if intent.Type != IntentHelp {
			t.Fatalf("expected IntentHelp, got %s", intent.Type)
		}
	})

	t.Run("parse view diff intent", func(t *testing.T) {
		intent, err := p.Parse("看看改了什么")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if intent.Type != IntentViewDiff {
			t.Fatalf("expected IntentViewDiff, got %s", intent.Type)
		}
	})

	t.Run("parse restore version intent", func(t *testing.T) {
		intent, err := p.Parse("恢复之前的版本")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if intent.Type != IntentRestoreVersion {
			t.Fatalf("expected IntentRestoreVersion, got %s", intent.Type)
		}
	})

	t.Run("parse submit change intent", func(t *testing.T) {
		intent, err := p.Parse("提交给团队")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if intent.Type != IntentSubmitChange {
			t.Fatalf("expected IntentSubmitChange, got %s", intent.Type)
		}
	})

	t.Run("parse view team change intent", func(t *testing.T) {
		intent, err := p.Parse("看看小李改了什么")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if intent.Type != IntentViewTeamChange {
			t.Fatalf("expected IntentViewTeamChange, got %s", intent.Type)
		}
	})

	t.Run("parse unknown intent", func(t *testing.T) {
		_, err := p.Parse("xyzrandomstring999")
		if err == nil {
			t.Fatal("expected error for unknown intent")
		}
	})

	t.Run("parse with message parameter", func(t *testing.T) {
		intent, err := p.Parse("保存修改：更新了报告数据")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if intent.Type != IntentSaveVersion {
			t.Fatalf("expected IntentSaveVersion, got %s", intent.Type)
		}
		if msg, ok := intent.Params["message"]; ok {
			if msg == "" {
				t.Fatal("expected non-empty message")
			}
		}
	})
}

func TestSuggestNext(t *testing.T) {
	p := New(".")

	t.Run("suggest next after save version", func(t *testing.T) {
		intent := &UserIntent{Type: IntentSaveVersion, UserInput: "保存修改"}
		suggestions := p.SuggestNext(intent)
		if len(suggestions) == 0 {
			t.Fatal("expected suggestions for save version")
		}
	})

	t.Run("suggest next after view history", func(t *testing.T) {
		intent := &UserIntent{Type: IntentViewHistory, UserInput: "查看历史"}
		suggestions := p.SuggestNext(intent)
		if len(suggestions) == 0 {
			t.Fatal("expected suggestions for view history")
		}
	})
}

func TestTranslateResult(t *testing.T) {
	p := New(".")

	t.Run("translate save version result", func(t *testing.T) {
		intent := &UserIntent{Type: IntentSaveVersion, UserInput: "保存修改"}
		msg := p.TranslateResult(intent, "abc123")
		if msg == "" {
			t.Fatal("expected non-empty translation")
		}
	})

	t.Run("translate view history result", func(t *testing.T) {
		intent := &UserIntent{Type: IntentViewHistory, UserInput: "查看历史"}
		msg := p.TranslateResult(intent, "3 versions")
		if msg == "" {
			t.Fatal("expected non-empty translation")
		}
	})
}

func TestUncommittedQuery(t *testing.T) {
	p := New("zh")
	testCases := []struct {
		input    string
		expected IntentType
	}{
		{"还有哪些修改没提交的", IntentViewStatus},
		{"哪些修改没提交", IntentViewStatus},
		{"没提交的修改", IntentViewStatus},  // view_status 也是合理的，显示哪些文件有未提交修改
		{"还有哪些没提交", IntentViewStatus},
		{"哪些文件没提交", IntentViewStatus},
		{"看看有什么没提交的", IntentViewStatus},
		{"有什么修改还没提交", IntentViewStatus},
		{"修改了什么还没提交", IntentViewDiff}, // view_diff 也合理，用户想知道具体修改内容
	}
	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			intent, err := p.Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(\"%s\") failed: %v", tc.input, err)
			}
			if intent.Type != tc.expected {
				t.Errorf("Parse(\"%s\") = %s (confidence=%.2f), want %s", tc.input, intent.Type, intent.Confidence, tc.expected)
			}
		})
	}
}

func TestDailyCommit(t *testing.T) {
	p := New("zh")
	testCases := []struct {
		input    string
		expected IntentType
	}{
		// 日常提交操作应被识别为 save_version
		{"提交", IntentSaveVersion},
		{"提交修改", IntentSaveVersion},
		{"提交一下", IntentSaveVersion},
		{"保存", IntentSaveVersion},
		{"保存修改", IntentSaveVersion},
		{"暂存", IntentSaveVersion},
		{"存一下", IntentSaveVersion},
		{"commit", IntentSaveVersion},
		{"提交代码", IntentSaveVersion},
		{"提交文档修改", IntentSaveVersion},
		{"保存当前修改", IntentSaveVersion},
		// 带否定词的应识别为 view_status/view_diff
		{"还有哪些修改没提交的", IntentViewStatus},
		{"没提交的修改", IntentViewStatus},
		{"修改了什么还没提交", IntentViewDiff}, // view_diff 也合理，用户想知道具体修改内容
	}
	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			intent, err := p.Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(\"%s\") failed: %v", tc.input, err)
			}
			if intent.Type != tc.expected {
				t.Errorf("Parse(\"%s\") = %s (confidence=%.2f), want %s", tc.input, intent.Type, intent.Confidence, tc.expected)
			}
		})
	}
}

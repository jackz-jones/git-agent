package interpreter

import (
	"testing"
)

func TestNaturalLanguageUnderstanding(t *testing.T) {
	p := New("zh")

	testCases := []struct {
		input    string
		expected IntentType
	}{
		// 截图中的失败场景
		{"查看修改详情", IntentViewDiff},
		{"查看当前状态", IntentViewStatus},
		{"保存当前修改", IntentSaveVersion},

		// 常见自然语言表达 - 查看差异
		{"看看改了什么", IntentViewDiff},
		{"改了啥", IntentViewDiff},
		{"有什么不同", IntentViewDiff},
		{"对比一下", IntentViewDiff},
		{"修改了什么", IntentViewDiff},
		{"修改内容", IntentViewDiff},
		{"改动详情", IntentViewDiff},
		{"变更内容", IntentViewDiff},
		{"查看修改", IntentViewDiff},

		// 保存版本
		{"存一下", IntentSaveVersion},
		{"保存", IntentSaveVersion},
		{"提交", IntentSaveVersion},
		{"保存一下", IntentSaveVersion},
		{"存档", IntentSaveVersion},
		{"保存修改", IntentSaveVersion},

		// 查看历史
		{"历史", IntentViewHistory},
		{"查看记录", IntentViewHistory},
		{"谁改的", IntentViewHistory},
		{"提交记录", IntentViewHistory},

		// 恢复版本
		{"恢复到之前的版本", IntentRestoreVersion},
		{"回滚", IntentRestoreVersion},
		{"撤销修改", IntentRestoreVersion},

		// 查看状态
		{"状态", IntentViewStatus},
		{"当前状态", IntentViewStatus},
		{"什么情况", IntentViewStatus},
		{"有哪些修改", IntentViewStatus},

		// 帮助
		{"帮助", IntentHelp},
		{"怎么用", IntentHelp},
		{"能做什么", IntentHelp},

		// 团队
		{"看看小李改了什么", IntentViewTeamChange},

		// 推送/拉取
		{"推到远程", IntentPush},
		{"拉取最新", IntentPull},

		// 分支
		{"有哪些分支", IntentListBranches},

		// 冲突
		{"冲突", IntentDetectConflict},
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

package planner

import (
	"testing"

	"github.com/jackz-jones/git-agent/internal/interpreter"
)

func TestNew(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("Planner should not be nil")
	}
}

func TestCreatePlan(t *testing.T) {
	p := New()

	t.Run("save version plan", func(t *testing.T) {
		intent := &interpreter.UserIntent{
			Type:      interpreter.IntentSaveVersion,
			UserInput: "保存修改",
			Params:    map[string]string{"message": "保存修改"},
		}
		plan, err := p.CreatePlan(intent)
		if err != nil {
			t.Fatalf("CreatePlan failed: %v", err)
		}
		if len(plan.Steps) != 2 {
			t.Fatalf("expected 2 steps, got %d", len(plan.Steps))
		}
		if plan.Steps[0].Type != StepGitAdd {
			t.Fatalf("expected first step to be git_add, got %s", plan.Steps[0].Type)
		}
		if plan.Steps[1].Type != StepGitCommit {
			t.Fatalf("expected second step to be git_commit, got %s", plan.Steps[1].Type)
		}
	})

	t.Run("view history plan", func(t *testing.T) {
		intent := &interpreter.UserIntent{
			Type:      interpreter.IntentViewHistory,
			UserInput: "查看历史",
			Params:    map[string]string{"limit": "10"},
		}
		plan, err := p.CreatePlan(intent)
		if err != nil {
			t.Fatalf("CreatePlan failed: %v", err)
		}
		if len(plan.Steps) != 1 {
			t.Fatalf("expected 1 step, got %d", len(plan.Steps))
		}
	})

	t.Run("init repo plan", func(t *testing.T) {
		intent := &interpreter.UserIntent{
			Type:      interpreter.IntentInitRepo,
			UserInput: "初始化仓库",
			Params:    map[string]string{},
		}
		plan, err := p.CreatePlan(intent)
		if err != nil {
			t.Fatalf("CreatePlan failed: %v", err)
		}
		if len(plan.Steps) != 1 {
			t.Fatalf("expected 1 step, got %d", len(plan.Steps))
		}
	})

	t.Run("nil intent returns error", func(t *testing.T) {
		_, err := p.CreatePlan(nil)
		if err == nil {
			t.Fatal("expected error for nil intent")
		}
	})
}

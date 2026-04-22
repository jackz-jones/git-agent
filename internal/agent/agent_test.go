package agent

import (
	"context"
	"testing"
)

func TestNewAgent(t *testing.T) {
	t.Run("creates agent with default config", func(t *testing.T) {
		a, err := New(".", nil)
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		if a == nil {
			t.Fatal("agent should not be nil")
		}
		if a.GetState() != StateIdle {
			t.Fatalf("expected state %s, got %s", StateIdle, a.GetState())
		}
	})

	t.Run("creates agent with custom config", func(t *testing.T) {
		config := &UserConfig{
			Name:  "测试用户",
			Email: "test@example.com",
			Role:  "admin",
		}
		a, err := New(".", config)
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		if a.GetUserConfig().Name != "测试用户" {
			t.Fatalf("expected name 测试用户, got %s", a.GetUserConfig().Name)
		}
	})
}

func TestAgentProcess(t *testing.T) {
	a, err := New(".", &UserConfig{
		Name:  "测试用户",
		Email: "test@example.com",
		Role:  "editor",
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	t.Run("process help intent", func(t *testing.T) {
		resp := a.Process(context.Background(), "帮助")
		if resp == nil {
			t.Fatal("response should not be nil")
		}
		if !resp.Success {
			t.Fatalf("expected success, got failure: %s", resp.Message)
		}
	})

	t.Run("process unknown intent", func(t *testing.T) {
		resp := a.Process(context.Background(), "xyzabc123")
		if resp == nil {
			t.Fatal("response should not be nil")
		}
		if resp.Success {
			t.Fatal("expected failure for unknown intent")
		}
	})

	t.Run("process status intent", func(t *testing.T) {
		resp := a.Process(context.Background(), "查看状态")
		if resp == nil {
			t.Fatal("response should not be nil")
		}
	})

	t.Run("process history intent", func(t *testing.T) {
		resp := a.Process(context.Background(), "查看历史")
		if resp == nil {
			t.Fatal("response should not be nil")
		}
	})
}

func TestAgentState(t *testing.T) {
	a, _ := New(".", nil)

	t.Run("initial state is idle", func(t *testing.T) {
		if a.GetState() != StateIdle {
			t.Fatalf("expected StateIdle, got %s", a.GetState())
		}
	})

	t.Run("state transitions", func(t *testing.T) {
		a.setState(StateThinking)
		if a.GetState() != StateThinking {
			t.Fatalf("expected StateThinking, got %s", a.GetState())
		}
		a.setState(StateIdle)
		if a.GetState() != StateIdle {
			t.Fatalf("expected StateIdle, got %s", a.GetState())
		}
	})
}

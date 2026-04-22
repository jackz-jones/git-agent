package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/tools"
)

// GitTool 将 GitAgentTool 适配为 langchaingo tools.Tool 接口
// 这样 LLM 就可以通过 function calling 调用 Git 操作
type GitTool struct {
	toolDef   GitAgentTool
	executors map[string]func(ctx context.Context, params map[string]interface{}) (string, error)
}

// NewGitTool 创建 LangChain Git 工具适配器
func NewGitTool(toolDef GitAgentTool, executor func(ctx context.Context, params map[string]interface{}) (string, error)) tools.Tool {
	t := &GitTool{
		toolDef:   toolDef,
		executors: map[string]func(ctx context.Context, params map[string]interface{}) (string, error){toolDef.Name: executor},
	}
	return t
}

func (t *GitTool) Name() string {
	return t.toolDef.Name
}

func (t *GitTool) Description() string {
	return t.toolDef.Description
}

func (t *GitTool) Call(ctx context.Context, input string) (string, error) {
	executor, ok := t.executors[t.toolDef.Name]
	if !ok {
		return "", fmt.Errorf("未找到工具执行器: %s", t.toolDef.Name)
	}

	// 解析输入参数
	params := make(map[string]interface{})
	if input != "" {
		if err := json.Unmarshal([]byte(input), &params); err != nil {
			// 如果不是 JSON，把整个输入作为 message 参数
			params["message"] = input
		}
	}

	result, err := executor(ctx, params)
	if err != nil {
		return fmt.Sprintf("工具执行失败: %v", err), nil
	}

	return result, nil
}

// GitToolRegistry Git 工具注册中心
// 管理 GitAgentTool 定义到 LangChain Tool 的映射
type GitToolRegistry struct {
	tools     map[string]tools.Tool
	executors map[string]func(ctx context.Context, params map[string]interface{}) (string, error)
}

// NewGitToolRegistry 创建 Git 工具注册中心
func NewGitToolRegistry() *GitToolRegistry {
	return &GitToolRegistry{
		tools:     make(map[string]tools.Tool),
		executors: make(map[string]func(ctx context.Context, params map[string]interface{}) (string, error)),
	}
}

// Register 注册工具执行器
func (r *GitToolRegistry) Register(name string, executor func(ctx context.Context, params map[string]interface{}) (string, error)) {
	r.executors[name] = executor
}

// BuildTools 构建所有 LangChain Tool 实例
func (r *GitToolRegistry) BuildTools() []tools.Tool {
	var result []tools.Tool
	for _, toolDef := range AllGitAgentTools {
		executor, ok := r.executors[toolDef.Name]
		if !ok {
			continue
		}
		tool := NewGitTool(toolDef, executor)
		r.tools[toolDef.Name] = tool
		result = append(result, tool)
	}
	return result
}

// BuildToolDefinitions 构建 LangChain llms.Tool 定义列表（用于 function calling）
func (r *GitToolRegistry) BuildToolDefinitions() []llms.Tool {
	var result []llms.Tool
	for _, toolDef := range AllGitAgentTools {
		if _, ok := r.executors[toolDef.Name]; !ok {
			continue
		}
		result = append(result, llms.Tool{
			Type: "function",
			Function: &llms.FunctionDefinition{
				Name:        toolDef.Name,
				Description: toolDef.Description,
				Parameters:  toolDef.Parameters,
			},
		})
	}
	return result
}

// GetTool 获取指定名称的工具
func (r *GitToolRegistry) GetTool(name string) (tools.Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider LLM 提供者接口
// 抽象不同的大模型后端（OpenAI、Azure、本地模型等）
type Provider interface {
	// Chat 发送消息并获取回复
	Chat(ctx context.Context, messages []Message) (*Response, error)
	// ChatWithTools 发送消息并支持工具调用
	ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*Response, error)
	// GetName 获取提供者名称
	GetName() string
}

// Message 聊天消息
type Message struct {
	Role       string     `json:"role"`                  // system, user, assistant, tool
	Content    string     `json:"content"`               // 消息内容
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`  // 工具调用（assistant 消息）
	ToolCallID string     `json:"tool_call_id,omitempty"` // 工具调用ID（tool 消息）
	Name       string     `json:"name,omitempty"`        // 工具名称（tool 消息）
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`     // 目前固定为 "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// Tool 工具定义
type Tool struct {
	Type     string   `json:"type"` // 目前固定为 "function"
	Function Function `json:"function"`
}

// Function 函数定义
type Function struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema
}

// Response LLM 响应
type Response struct {
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Usage      Usage      `json:"usage"`
	FinishReason string   `json:"finish_reason"`
}

// Usage token 使用量
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ==================== OpenAI 兼容 Provider ====================

// OpenAIConfig OpenAI 兼容 API 配置
type OpenAIConfig struct {
	APIKey     string        `json:"api_key"`
	BaseURL    string        `json:"base_url"`    // 默认 https://api.openai.com/v1
	Model      string        `json:"model"`        // 默认 gpt-4o
	MaxTokens  int           `json:"max_tokens"`   // 默认 4096
	Timeout    time.Duration `json:"timeout"`      // 默认 60s
}

// OpenAIProvider OpenAI 兼容的 LLM 提供者
// 支持 OpenAI、Azure OpenAI、各类国产模型的兼容接口
type OpenAIProvider struct {
	config     OpenAIConfig
	httpClient *http.Client
}

// NewOpenAIProvider 创建 OpenAI 兼容的 Provider
func NewOpenAIProvider(config OpenAIConfig) *OpenAIProvider {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")

	if config.Model == "" {
		config.Model = "gpt-4o"
	}

	if config.MaxTokens == 0 {
		config.MaxTokens = 4096
	}

	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}

	return &OpenAIProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// chatRequest OpenAI Chat API 请求体
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Tools       []chatTool    `json:"tools,omitempty"`
}

type chatMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  []chatToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

type chatToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function chatFunctionCall    `json:"function"`
}

type chatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatTool struct {
	Type     string         `json:"type"`
	Function chatFunction   `json:"function"`
}

type chatFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// chatResponse OpenAI Chat API 响应体
type chatResponse struct {
	ID      string         `json:"id"`
	Choices []chatChoice   `json:"choices"`
	Usage   chatUsage      `json:"usage"`
	Model   string         `json:"model"`
}

type chatChoice struct {
	Index        int           `json:"index"`
	Message      chatMessage   `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Chat 发送聊天请求
func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message) (*Response, error) {
	return p.doChat(ctx, messages, nil)
}

// ChatWithTools 发送带工具的聊天请求
func (p *OpenAIProvider) ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*Response, error) {
	return p.doChat(ctx, messages, tools)
}

// doChat 执行聊天请求
func (p *OpenAIProvider) doChat(ctx context.Context, messages []Message, tools []Tool) (*Response, error) {
	// 构建请求
	reqMessages := make([]chatMessage, len(messages))
	for i, m := range messages {
		reqMessages[i] = chatMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		if len(m.ToolCalls) > 0 {
			reqMessages[i].ToolCalls = make([]chatToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				reqMessages[i].ToolCalls[j] = chatToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: chatFunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
	}

	reqBody := chatRequest{
		Model:       p.config.Model,
		Messages:    reqMessages,
		MaxTokens:   p.config.MaxTokens,
		Temperature: 0.1, // Agent 场景使用低温度，减少幻觉
	}

	if len(tools) > 0 {
		reqBody.Tools = make([]chatTool, len(tools))
		for i, t := range tools {
			reqBody.Tools[i] = chatTool{
				Type: t.Type,
				Function: chatFunction{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				},
			}
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 LLM 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API 返回错误 (status=%d): %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM 未返回任何内容")
	}

	choice := chatResp.Choices[0]
	result := &Response{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Usage: Usage{
			PromptTokens:     chatResp.Usage.PromptTokens,
			CompletionTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:      chatResp.Usage.TotalTokens,
		},
	}

	// 转换工具调用
	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			result.ToolCalls[i] = ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	return result, nil
}

// GetName 获取提供者名称
func (p *OpenAIProvider) GetName() string {
	return "openai-compatible"
}

// ==================== Mock Provider（用于测试） ====================

// MockProvider 用于测试的模拟 Provider
type MockProvider struct {
	Response *Response
	Err      error
	Requests [][]Message
}

// NewMockProvider 创建模拟 Provider
func NewMockProvider() *MockProvider {
	return &MockProvider{
		Requests: make([][]Message, 0),
	}
}

// Chat 模拟聊天
func (m *MockProvider) Chat(ctx context.Context, messages []Message) (*Response, error) {
	m.Requests = append(m.Requests, messages)
	if m.Err != nil {
		return nil, m.Err
	}
	if m.Response != nil {
		return m.Response, nil
	}
	// 默认返回一个简单的回复
	return &Response{
		Content: "模拟回复",
		Usage:   Usage{TotalTokens: 100},
	}, nil
}

// ChatWithTools 模拟带工具的聊天
func (m *MockProvider) ChatWithTools(ctx context.Context, messages []Message, tools []Tool) (*Response, error) {
	return m.Chat(ctx, messages)
}

// GetName 获取提供者名称
func (m *MockProvider) GetName() string {
	return "mock"
}

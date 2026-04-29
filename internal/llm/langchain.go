package llm

import (
	"fmt"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// OpenAIConfig OpenAI 兼容 API 配置
type OpenAIConfig struct {
	APIKey    string        `json:"api_key"`
	BaseURL   string        `json:"base_url"`    // 默认 https://api.openai.com/v1
	Model     string        `json:"model"`       // 默认 gpt-4o
	MaxTokens int           `json:"max_tokens"`  // 默认 4096
	Timeout   time.Duration `json:"timeout"`     // 默认 60s
}

// Usage token 使用量
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// NewLangChainLLM 基于 LangChain Go 框架创建 LLM 实例
// 支持所有 OpenAI 兼容的 API 提供商（OpenAI、Azure、DeepSeek、通义千问、Ollama 等）
func NewLangChainLLM(config OpenAIConfig) (llms.Model, error) {
	opts := []openai.Option{
		openai.WithToken(config.APIKey),
	}

	if config.Model != "" {
		opts = append(opts, openai.WithModel(config.Model))
	}
	if config.BaseURL != "" {
		opts = append(opts, openai.WithBaseURL(config.BaseURL))
	}

	llm, err := openai.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("创建 LangChain LLM 失败: %w", err)
	}

	return llm, nil
}
package llm

import (
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

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

package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackz-jones/git-agent/internal/agent"
	"github.com/jackz-jones/git-agent/internal/llm"
	"github.com/tmc/langchaingo/llms"
)

// settingsView 是对外暴露的（已脱敏的）配置视图。
type settingsView struct {
	User struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"user"`
	LLM struct {
		Enabled    bool   `json:"enabled"`
		APIKeyMask string `json:"apiKeyMask"` // 脱敏后的展示值，如 sk-****abcd
		HasAPIKey  bool   `json:"hasApiKey"`
		BaseURL    string `json:"baseUrl"`
		Model      string `json:"model"`
		MaxTokens  int    `json:"maxTokens"`
	} `json:"llm"`
	HTTP struct {
		Username     string `json:"username"`
		HasPassword  bool   `json:"hasPassword"`
		PasswordMask string `json:"passwordMask"`
	} `json:"http"`
}

// settingsReq 是更新配置的请求体，字段为可选更新；使用指针区分 "不提供" 与 "置空"。
type settingsReq struct {
	User *struct {
		Name  *string `json:"name,omitempty"`
		Email *string `json:"email,omitempty"`
	} `json:"user,omitempty"`
	LLM *struct {
		Enabled   *bool   `json:"enabled,omitempty"`
		APIKey    *string `json:"apiKey,omitempty"`
		BaseURL   *string `json:"baseUrl,omitempty"`
		Model     *string `json:"model,omitempty"`
		MaxTokens *int    `json:"maxTokens,omitempty"`
	} `json:"llm,omitempty"`
	HTTP *struct {
		Username *string `json:"username,omitempty"`
		Password *string `json:"password,omitempty"`
	} `json:"http,omitempty"`
}

// handleSettings 对应 GET/POST /api/settings。
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if s.config == nil {
		writeError(w, http.StatusInternalServerError, "no_config", "配置存储未初始化")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, toSettingsView(s.config.Snapshot()))
	case http.MethodPost:
		var req settingsReq
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		if err := s.config.Update(func(cfg *AppConfig) {
			applySettingsReq(cfg, &req)
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
			return
		}
		// 配置保存成功后，热更新所有已打开 workspace 的 Agent
		if s.workspaces != nil {
			snap := s.config.Snapshot()
			userCfg, llmCfg := buildConfigsFromSnapshot(snap)
			s.workspaces.ReloadLLMConfig(userCfg, llmCfg)
		}
		writeJSON(w, http.StatusOK, toSettingsView(s.config.Snapshot()))
	default:
		methodNotAllowed(w)
	}
}

// applySettingsReq 将请求中明确给出的字段写回 cfg。
func applySettingsReq(cfg *AppConfig, req *settingsReq) {
	if req.User != nil {
		if req.User.Name != nil {
			cfg.User.Name = *req.User.Name
		}
		if req.User.Email != nil {
			cfg.User.Email = *req.User.Email
		}
	}
	if req.LLM != nil {
		if req.LLM.Enabled != nil {
			cfg.LLM.Enabled = *req.LLM.Enabled
		}
		if req.LLM.APIKey != nil {
			cfg.LLM.APIKey = *req.LLM.APIKey
		}
		if req.LLM.BaseURL != nil {
			cfg.LLM.BaseURL = *req.LLM.BaseURL
		}
		if req.LLM.Model != nil {
			cfg.LLM.Model = *req.LLM.Model
		}
		if req.LLM.MaxTokens != nil {
			cfg.LLM.MaxTokens = *req.LLM.MaxTokens
		}
	}
	if req.HTTP != nil {
		if req.HTTP.Username != nil {
			cfg.HTTP.Username = *req.HTTP.Username
		}
		if req.HTTP.Password != nil {
			cfg.HTTP.Password = *req.HTTP.Password
		}
	}
}

// toSettingsView 构造脱敏后的视图。
func toSettingsView(cfg AppConfig) settingsView {
	var v settingsView
	v.User.Name = cfg.User.Name
	v.User.Email = cfg.User.Email
	v.LLM.Enabled = cfg.LLM.Enabled
	v.LLM.BaseURL = cfg.LLM.BaseURL
	v.LLM.Model = cfg.LLM.Model
	v.LLM.MaxTokens = cfg.LLM.MaxTokens
	v.LLM.HasAPIKey = cfg.LLM.APIKey != ""
	v.LLM.APIKeyMask = maskSecret(cfg.LLM.APIKey)
	v.HTTP.Username = cfg.HTTP.Username
	v.HTTP.HasPassword = cfg.HTTP.Password != ""
	v.HTTP.PasswordMask = maskSecret(cfg.HTTP.Password)
	return v
}

// maskSecret 形如 sk-****abcd；总长 < 8 时全部星号。
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	if len(s) <= 8 {
		return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
	}
	prefix := s[:3]
	suffix := s[len(s)-4:]
	return prefix + strings.Repeat("*", 4) + suffix
}

// testLLMReq 用于临时测试未保存的配置；字段为空时回退到已保存配置。
type testLLMReq struct {
	APIKey  string `json:"apiKey,omitempty"`
	BaseURL string `json:"baseUrl,omitempty"`
	Model   string `json:"model,omitempty"`
}

// handleTestLLM 对应 POST /api/settings/test-llm。
// 使用请求中（或已保存）的配置创建临时 LLM，发起一次最小对话进行自检。
func (s *Server) handleTestLLM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req testLLMReq
	_ = readJSON(r, &req)

	apiKey, baseURL, model := req.APIKey, req.BaseURL, req.Model
	if s.config != nil {
		snap := s.config.Snapshot()
		if apiKey == "" {
			apiKey = snap.LLM.APIKey
		}
		if baseURL == "" {
			baseURL = snap.LLM.BaseURL
		}
		if model == "" {
			model = snap.LLM.Model
		}
	}
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, "missing_api_key", "未提供 API Key，无法测试")
		return
	}

	lc, err := llm.NewLangChainLLM(llm.OpenAIConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      false,
			"message": fmt.Sprintf("创建 LLM 失败: %v", err),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	resp, err := lc.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, "请回复一个字 OK，验证连通性"),
	}, llms.WithMaxTokens(32))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      false,
			"message": fmt.Sprintf("调用失败: %v", err),
		})
		return
	}
	sample := ""
	if resp != nil && len(resp.Choices) > 0 {
		sample = resp.Choices[0].Content
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": "LLM 连通性正常",
		"sample":  sample,
	})
}

// buildConfigsFromSnapshot 从 AppConfig 快照构建 Agent 所需的 UserConfig 和 LLMConfig。
// 用于保存设置后热更新已打开 workspace 的 Agent。
func buildConfigsFromSnapshot(cfg AppConfig) (*agent.UserConfig, *agent.LLMConfig) {
	userCfg := &agent.UserConfig{
		Name:     cfg.User.Name,
		Email:    cfg.User.Email,
		Role:     "editor",
		Language: "zh",
	}
	maxTokens := cfg.LLM.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	llmCfg := &agent.LLMConfig{
		Enabled:   cfg.LLM.Enabled && cfg.LLM.APIKey != "",
		APIKey:    cfg.LLM.APIKey,
		BaseURL:   cfg.LLM.BaseURL,
		Model:     cfg.LLM.Model,
		MaxTokens: maxTokens,
	}
	return userCfg, llmCfg
}

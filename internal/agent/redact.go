package agent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// 敏感字段名（大小写不敏感）
// 需求 13.2：emitToolCall / chatHistory 在写入前对 password/token/api_key 等字段做 *** 脱敏，
// 避免 SSE 广播与历史消息将凭据泄漏给下游订阅者或日志采集系统。
var sensitiveKeys = map[string]struct{}{
	"password":      {},
	"passwd":        {},
	"pwd":           {},
	"token":         {},
	"access_token":  {},
	"refresh_token": {},
	"api_key":       {},
	"apikey":        {},
	"secret":        {},
	"secret_key":    {},
	"authorization": {},
	"auth":          {},
	"private_key":   {},
}

const redactedMask = "***"

// isSensitiveKey 判定字段名是否为敏感字段（忽略大小写与常见分隔符）。
func isSensitiveKey(key string) bool {
	if key == "" {
		return false
	}
	normalized := strings.ToLower(key)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if _, ok := sensitiveKeys[normalized]; ok {
		return true
	}
	// 兼容包含子串的字段名，例如 "user_password"、"gitToken"
	for k := range sensitiveKeys {
		if strings.Contains(normalized, k) {
			return true
		}
	}
	return false
}

// redactJSONArgs 尝试把一个 JSON 字符串中的敏感字段值替换为 ***。
// 如果解析失败则返回原字符串（保证不会因为脱敏失败破坏调用链）。
func redactJSONArgs(args string) string {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return args
	}
	var v interface{}
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return args
	}
	redacted := redactValue(v)
	b, err := json.Marshal(redacted)
	if err != nil {
		return args
	}
	return string(b)
}

// redactValue 递归遍历任意 JSON 结构，对敏感字段做值替换。
func redactValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			if isSensitiveKey(k) {
				if s, ok := val.(string); ok && s == "" {
					out[k] = ""
				} else {
					out[k] = redactedMask
				}
				continue
			}
			out[k] = redactValue(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, item := range t {
			out[i] = redactValue(item)
		}
		return out
	default:
		return v
	}
}

// redactParams 对 map[string]interface{} 类型的调用参数做脱敏。
// 用于写入 chatHistory 前的原地拷贝，避免修改调用者传入的 map。
func redactParams(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}
	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		if isSensitiveKey(k) {
			if s, ok := v.(string); ok && s == "" {
				out[k] = ""
			} else {
				out[k] = redactedMask
			}
			continue
		}
		out[k] = redactValue(v)
	}
	return out
}

// validateRemoteHTTPSURL 校验用户/LLM 提供的 remote_url 是否可信。
// 需求 13.1：只允许 http/https 协议，且必须带有 host，避免被引导切换到
// file:///、gopher://、data:// 或空协议等非预期目标造成 SSRF/RCE 风险。
func validateRemoteHTTPSURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("remote_url 不能为空")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("remote_url 解析失败: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("remote_url 只允许 http/https 协议，收到: %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("remote_url 缺少 host: %q", raw)
	}
	return nil
}
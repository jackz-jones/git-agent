package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditLogger 向 ~/.git-agent/logs/ops.log 记录破坏性操作日志。
// 线程安全；失败时仅记录 stderr，不影响主流程。
type AuditLogger struct {
	mu   sync.Mutex
	path string
}

// NewAuditLogger 在指定目录下创建 logs/ops.log。
func NewAuditLogger(configDir string) *AuditLogger {
	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	logDir := filepath.Join(configDir, "logs")
	_ = os.MkdirAll(logDir, 0o700)
	return &AuditLogger{
		path: filepath.Join(logDir, "ops.log"),
	}
}

// auditRecord 是单行日志结构。
type auditRecord struct {
	Time        time.Time              `json:"time"`
	Operation   string                 `json:"op"`
	WorkspaceID string                 `json:"wsId,omitempty"`
	Path        string                 `json:"path,omitempty"`
	Success     bool                   `json:"success"`
	Detail      map[string]interface{} `json:"detail,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// Log 追加一条审计记录，按 JSON Lines 格式写入。
func (a *AuditLogger) Log(op string, ws *Workspace, success bool, detail map[string]interface{}, err error) {
	if a == nil {
		return
	}
	rec := auditRecord{
		Time:      time.Now(),
		Operation: op,
		Success:   success,
		Detail:    detail,
	}
	if ws != nil {
		rec.WorkspaceID = ws.ID
		rec.Path = ws.Path
	}
	if err != nil {
		rec.Error = err.Error()
	}

	line, mErr := json.Marshal(rec)
	if mErr != nil {
		fmt.Fprintf(os.Stderr, "[web] 审计日志序列化失败: %v\n", mErr)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	f, oErr := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if oErr != nil {
		fmt.Fprintf(os.Stderr, "[web] 打开审计日志失败: %v\n", oErr)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// Path 返回日志文件路径。
func (a *AuditLogger) Path() string {
	if a == nil {
		return ""
	}
	return a.path
}

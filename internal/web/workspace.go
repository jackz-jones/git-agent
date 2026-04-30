package web

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackz-jones/git-agent/internal/agent"
)

// Workspace 表示一个被打开的本地工作目录及其关联资源。
type Workspace struct {
	ID         string        // 稳定 ID：基于路径哈希，便于重复打开时复用
	Path       string        // 绝对路径
	OpenedAt   time.Time     // 首次打开时间
	LastUsedAt time.Time     // 最近访问时间
	Agent      *agent.Agent  // 关联的 Agent 实例（按需可为 nil，表示尚未初始化）
	UserConfig *agent.UserConfig
	LLMConfig  *agent.LLMConfig
}

// WorkspaceInfo 是对外（API 层）返回的精简信息，不暴露 *Agent 指针。
type WorkspaceInfo struct {
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	OpenedAt   time.Time `json:"openedAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
	// Initialized 表示对应目录是否已被 git 初始化（含 .git 目录）。
	Initialized bool `json:"initialized"`
	// LLMEnabled 表示该 workspace 的 Agent 是否启用了 LLM 模式。
	LLMEnabled bool `json:"llmEnabled"`
}

// Manager 负责 Workspace 生命周期管理：打开、查询、关闭、列表。
// 线程安全。
type Manager struct {
	mu           sync.RWMutex
	items        map[string]*Workspace // key = Workspace.ID
	defaultUser  *agent.UserConfig
	defaultLLM   *agent.LLMConfig
	maxOpenWarn  int // 打开数量超过此阈值会打印日志提醒（不强制拒绝）
	closeTimeout time.Duration

	// onOpen 在 workspace 成功创建后被调用（已存在复用时不触发）。
	// 解耦"最近打开"持久化等副作用，避免 Manager 直接依赖 RecentStore。
	onOpen func(ws *Workspace)
}

// NewManager 构造 Workspace 管理器。
//
// defaultUser / defaultLLM 是"打开新 workspace 时"使用的默认配置；
// 调用方（serve 命令）可以在每次启动时根据配置文件/环境变量组装后传入。
func NewManager(defaultUser *agent.UserConfig, defaultLLM *agent.LLMConfig) *Manager {
	return &Manager{
		items:        make(map[string]*Workspace),
		defaultUser:  defaultUser,
		defaultLLM:   defaultLLM,
		maxOpenWarn:  10,
		closeTimeout: 5 * time.Second,
	}
}

// idOf 基于绝对路径生成稳定的 Workspace ID。
// 使用 sha1 前 12 位作为短 ID，足以避免冲突且便于前端显示。
func idOf(absPath string) string {
	sum := sha1.Sum([]byte(strings.ToLower(absPath)))
	return hex.EncodeToString(sum[:])[:12]
}

// Open 打开指定目录作为 Workspace；若该路径已被打开，则复用原实例并更新 LastUsedAt。
//
// 返回：(workspace, reused, error)
// - reused == true 表示命中已有 workspace，未重新创建 Agent。
func (m *Manager) Open(path string) (*Workspace, bool, error) {
	abs, err := ValidateWorkspacePath(path)
	if err != nil {
		return nil, false, err
	}

	id := idOf(abs)

	// 快路径：已存在
	m.mu.RLock()
	if ws, ok := m.items[id]; ok {
		ws.LastUsedAt = time.Now()
		m.mu.RUnlock()
		return ws, true, nil
	}
	m.mu.RUnlock()

	// 复制默认配置，避免多 workspace 共享同一指针导致相互污染
	userCfg := cloneUserConfig(m.defaultUser)
	llmCfg := cloneLLMConfig(m.defaultLLM)

	var ag *agent.Agent
	if llmCfg != nil && llmCfg.Enabled {
		ag, err = agent.NewWithLLM(abs, userCfg, llmCfg)
	} else {
		ag, err = agent.New(abs, userCfg)
	}
	if err != nil {
		return nil, false, fmt.Errorf("创建 Agent 失败: %w", err)
	}

	// Agent 创建成功后，尝试从 .git/config 读回本地用户信息（与 CLI 行为一致）
	ag.LoadLocalUserConfig()

	ws := &Workspace{
		ID:         id,
		Path:       abs,
		OpenedAt:   time.Now(),
		LastUsedAt: time.Now(),
		Agent:      ag,
		UserConfig: userCfg,
		LLMConfig:  llmCfg,
	}

	m.mu.Lock()
	// 双检：并发下可能已被其他协程加入
	if existing, ok := m.items[id]; ok {
		m.mu.Unlock()
		ag.Close()
		existing.LastUsedAt = time.Now()
		return existing, true, nil
	}
	m.items[id] = ws
	total := len(m.items)
	hook := m.onOpen
	m.mu.Unlock()

	if hook != nil {
		hook(ws)
	}

	if total > m.maxOpenWarn {
		// 不阻止，仅提醒
		fmt.Printf("[web] 已打开 %d 个 workspace，资源占用可能较高\n", total)
	}
	return ws, false, nil
}

// SetOnOpen 注册 workspace 打开成功后的回调（不包括复用场景）。
// 典型用途：更新 RecentStore。
func (m *Manager) SetOnOpen(fn func(ws *Workspace)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onOpen = fn
}

// Get 返回指定 ID 的 workspace，未找到时返回 nil。
func (m *Manager) Get(id string) *Workspace {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ws, ok := m.items[id]; ok {
		ws.LastUsedAt = time.Now()
		return ws
	}
	return nil
}

// Close 关闭指定 workspace（释放 Agent 资源）。
func (m *Manager) Close(id string) error {
	m.mu.Lock()
	ws, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("workspace 不存在: %s", id)
	}
	delete(m.items, id)
	m.mu.Unlock()

	if ws.Agent != nil {
		ws.Agent.Close()
	}
	return nil
}

// List 返回全部已打开 workspace 的快照信息（按 LastUsedAt 倒序）。
func (m *Manager) List() []WorkspaceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]WorkspaceInfo, 0, len(m.items))
	for _, ws := range m.items {
		out = append(out, toInfo(ws))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastUsedAt.After(out[j].LastUsedAt)
	})
	return out
}

// ReloadLLMConfig 热更新所有已打开 workspace 的 Agent LLM 配置。
// 当用户在设置页保存 LLM 配置后调用，使配置变更立即生效。
func (m *Manager) ReloadLLMConfig(userCfg *agent.UserConfig, llmCfg *agent.LLMConfig) {
	m.mu.Lock()
	m.defaultUser = userCfg
	m.defaultLLM = llmCfg
	// 收集需要重建的 workspace
	toRebuild := make([]*Workspace, 0, len(m.items))
	for _, ws := range m.items {
		toRebuild = append(toRebuild, ws)
	}
	m.mu.Unlock()

	for _, ws := range toRebuild {
		newUserCfg := cloneUserConfig(userCfg)
		newLLMCfg := cloneLLMConfig(llmCfg)

		var newAgent *agent.Agent
		var err error
		if newLLMCfg != nil && newLLMCfg.Enabled {
			newAgent, err = agent.NewWithLLM(ws.Path, newUserCfg, newLLMCfg)
		} else {
			newAgent, err = agent.New(ws.Path, newUserCfg)
		}
		if err != nil {
			fmt.Printf("[web] 重建 workspace %s 的 Agent 失败: %v\n", ws.Path, err)
			continue
		}
		newAgent.LoadLocalUserConfig()

		// 替换旧 Agent
		oldAgent := ws.Agent
		ws.Agent = newAgent
		ws.UserConfig = newUserCfg
		ws.LLMConfig = newLLMCfg
		if oldAgent != nil {
			oldAgent.Close()
		}
		fmt.Printf("[web] workspace %s 的 Agent 已热更新 (LLM=%v)\n", ws.Path, newLLMCfg != nil && newLLMCfg.Enabled)
	}
}

// CloseAll 关闭全部 workspace，服务进程退出时调用。
func (m *Manager) CloseAll() {
	m.mu.Lock()
	items := m.items
	m.items = make(map[string]*Workspace)
	m.mu.Unlock()
	for _, ws := range items {
		if ws.Agent != nil {
			ws.Agent.Close()
		}
	}
}

// toInfo 将 Workspace 转为对外信息对象。
func toInfo(ws *Workspace) WorkspaceInfo {
	info := WorkspaceInfo{
		ID:         ws.ID,
		Path:       ws.Path,
		OpenedAt:   ws.OpenedAt,
		LastUsedAt: ws.LastUsedAt,
	}
	if ws.Agent != nil {
		info.LLMEnabled = ws.Agent.IsLLMEnabled()
		if gw := ws.Agent.GetGitWrapper(); gw != nil {
			info.Initialized = gw.IsInitialized()
		}
	}
	return info
}

// cloneUserConfig 浅拷贝用户配置。
func cloneUserConfig(src *agent.UserConfig) *agent.UserConfig {
	if src == nil {
		return &agent.UserConfig{Role: "editor", Language: "zh"}
	}
	c := *src
	return &c
}

// cloneLLMConfig 浅拷贝 LLM 配置。
func cloneLLMConfig(src *agent.LLMConfig) *agent.LLMConfig {
	if src == nil {
		return nil
	}
	c := *src
	return &c
}

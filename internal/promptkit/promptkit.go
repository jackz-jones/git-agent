package promptkit

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// SkillType 定义 Skill 的类型
type SkillType string

const (
	SkillTypeSkill SkillType = "skill" // 技能：具体的操作知识（如如何写 commit message）
	SkillTypeRule  SkillType = "rule"  // 规则：行为约束（如不使用 git 术语）
)

// Skill 定义一个可加载的提示词片段
type Skill struct {
	Name        string    `json:"name"`         // 唯一标识，如 "commit-message"
	Type        SkillType `json:"type"`         // skill 或 rule
	Content     string    `json:"content"`      // Markdown 内容
	Source      string    `json:"source"`       // 来源：builtin / user / project
	Intents     []string  `json:"intents"`      // 关联的意图列表，用于按需加载
	Description string    `json:"description"`  // 简短描述
	Priority    int       `json:"priority"`     // 排序优先级，数字越小越靠前
	Enabled     bool      `json:"enabled"`      // 是否启用
}

// Config 定义 Skills/Rules 的配置
type Config struct {
	// 禁用的内置 Skill/Rule 名称列表
	Disabled []string `json:"disabled"`
	// 自定义 Skill/Rule 目录路径（用户级别）
	UserDir string `json:"user_dir"`
	// 项目级配置目录路径
	ProjectDir string `json:"project_dir"`
}

// Kit 管理所有 Skills 和 Rules
type Kit struct {
	mu          sync.RWMutex
	skills      map[string]*Skill // 所有已加载的 skill（key = name）
	rules       map[string]*Skill // 所有已加载的 rule（key = name）
	intentIndex map[string][]string // intent → skill/rule names
	builtinFS   embed.FS           // 内置资源文件系统
	config      Config

	// 热加载相关
	watcher   *fsnotify.Watcher // 文件监听器
	done      chan struct{}     // 关闭信号
	reloadCh  chan struct{}     // 防抖：合并短时间内的多次事件
	onReload  func()           // 重载完成后的回调（可选，用于日志通知）
	lastReload time.Time       // 上次重载时间
}

// NewKit 创建一个新的 Kit 实例
func NewKit(builtinFS embed.FS, config Config) *Kit {
	k := &Kit{
		skills:      make(map[string]*Skill),
		rules:       make(map[string]*Skill),
		intentIndex: make(map[string][]string),
		builtinFS:   builtinFS,
		config:      config,
		done:        make(chan struct{}),
		reloadCh:    make(chan struct{}, 1), // 带缓冲，合并多次事件
	}
	return k
}

// Load 加载所有 Skills 和 Rules（内置 + 用户自定义 + 项目级）
func (k *Kit) Load() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	// 1. 加载内置 Skills
	if err := k.loadFromFS(k.builtinFS, "skills", SkillTypeSkill, "builtin"); err != nil {
		return fmt.Errorf("加载内置 Skills 失败: %w", err)
	}

	// 2. 加载内置 Rules
	if err := k.loadFromFS(k.builtinFS, "rules", SkillTypeRule, "builtin"); err != nil {
		return fmt.Errorf("加载内置 Rules 失败: %w", err)
	}

	// 3. 加载用户自定义（覆盖内置）
	if k.config.UserDir != "" {
		if err := k.loadFromDir(filepath.Join(k.config.UserDir, "skills"), SkillTypeSkill, "user"); err != nil {
			// 用户目录不存在不报错，只是跳过
			_ = err
		}
		if err := k.loadFromDir(filepath.Join(k.config.UserDir, "rules"), SkillTypeRule, "user"); err != nil {
			_ = err
		}
	}

	// 4. 加载项目级（覆盖用户级和内置）
	if k.config.ProjectDir != "" {
		if err := k.loadFromDir(filepath.Join(k.config.ProjectDir, "skills"), SkillTypeSkill, "project"); err != nil {
			_ = err
		}
		if err := k.loadFromDir(filepath.Join(k.config.ProjectDir, "rules"), SkillTypeRule, "project"); err != nil {
			_ = err
		}
	}

	// 5. 构建意图索引
	k.buildIntentIndex()

	// 6. 应用禁用列表
	k.applyDisabled()

	return nil
}

// loadFromFS 从 embed.FS 加载 Skill/Rule
func (k *Kit) loadFromFS(fsys embed.FS, subdir string, skillType SkillType, source string) error {
	root := "resources/" + subdir
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := fs.ReadFile(fsys, filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		skill := k.parseSkill(name, string(data), skillType, source)
		k.register(skill)
	}

	return nil
}

// loadFromDir 从文件系统目录加载 Skill/Rule
func (k *Kit) loadFromDir(dir string, skillType SkillType, source string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		skill := k.parseSkill(name, string(data), skillType, source)
		k.register(skill)
	}

	return nil
}

// parseSkill 解析 Markdown 文件为 Skill
// Markdown 格式约定：
//   第一行如果是 <!-- meta: ... --> 则解析为元数据
//   其余为 Content
//
// meta 格式示例：
//   <!-- meta: {"intents":["save_version","submit_change"],"priority":10,"description":"Commit message 规范"} -->
func (k *Kit) parseSkill(name, content string, skillType SkillType, source string) *Skill {
	skill := &Skill{
		Name:     name,
		Type:     skillType,
		Content:  content,
		Source:   source,
		Intents:  []string{},
		Priority: 50, // 默认优先级
		Enabled:  true,
	}

	// 解析 meta 行
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) >= 1 && strings.HasPrefix(lines[0], "<!-- meta:") {
		metaStr := strings.TrimPrefix(lines[0], "<!-- meta:")
		metaStr = strings.TrimSuffix(metaStr, "-->")
		metaStr = strings.TrimSpace(metaStr)

		// 简单解析 JSON 格式的 meta
		k.parseMeta(skill, metaStr)

		// Content 去掉 meta 行
		if len(lines) >= 2 {
			skill.Content = strings.TrimSpace(lines[1])
		} else {
			skill.Content = ""
		}
	} else {
		// 没有 meta 行，根据目录名推断意图
		skill.Intents = k.inferIntents(name, skillType)
		skill.Description = k.inferDescription(name, skillType)
	}

	return skill
}

// parseMeta 解析 meta JSON 字符串
func (k *Kit) parseMeta(skill *Skill, metaStr string) {
	// 简单的 JSON 解析（不引入额外依赖）
	// 格式: {"intents":["a","b"],"priority":10,"description":"xxx"}
	metaStr = strings.TrimSpace(metaStr)

	// 解析 intents
	if idx := strings.Index(metaStr, `"intents"`); idx >= 0 {
		if start := strings.Index(metaStr[idx:], "["); start >= 0 {
			rest := metaStr[idx+start:]
			if end := strings.Index(rest, "]"); end >= 0 {
				arrayStr := rest[1:end]
				items := strings.Split(arrayStr, ",")
				for _, item := range items {
					item = strings.TrimSpace(item)
					item = strings.Trim(item, `"`)
					if item != "" {
						skill.Intents = append(skill.Intents, item)
					}
				}
			}
		}
	}

	// 解析 priority
	if idx := strings.Index(metaStr, `"priority"`); idx >= 0 {
		rest := metaStr[idx:]
		// 查找数字
		for i := 0; i < len(rest); i++ {
			if rest[i] >= '0' && rest[i] <= '9' {
				num := 0
				for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
					num = num*10 + int(rest[i]-'0')
					i++
				}
				skill.Priority = num
				break
			}
		}
	}

	// 解析 description
	if idx := strings.Index(metaStr, `"description"`); idx >= 0 {
		rest := metaStr[idx:]
		if start := strings.Index(rest, `"`); start >= 0 {
			// 找 description 值的第二个引号
			afterKey := rest[start+1:]
			// 跳过 "description": 部分
			if colonIdx := strings.Index(afterKey, ":"); colonIdx >= 0 {
				afterColon := afterKey[colonIdx+1:]
				afterColon = strings.TrimSpace(afterColon)
				if strings.HasPrefix(afterColon, `"`) {
					afterColon = afterColon[1:]
					if endIdx := strings.Index(afterColon, `"`); endIdx >= 0 {
						skill.Description = afterColon[:endIdx]
					}
				}
			}
		}
	}
}

// inferIntents 根据名称推断关联意图
func (k *Kit) inferIntents(name string, skillType SkillType) []string {
	intentMap := map[string][]string{
		"commit-message":      {"save_version", "submit_change"},
		"batch-commit":        {"save_version", "submit_change"},
		"push-fail-guide":     {"submit_change", "push"},
		"conflict-resolution": {"detect_conflict", "approve_merge"},
		"always-execute":      {"save_version", "submit_change", "push", "restore_version", "create_branch", "switch_branch", "create_tag"},
		"no-repeat-tools":     {"save_version", "submit_change", "view_diff", "view_status", "view_history"},
		"no-git-terms":        {}, // 全局规则，不关联特定意图
		"display-format":      {"view_history", "view_status"},
		"version-restore":     {"restore_version"},
	}

	if intents, ok := intentMap[name]; ok {
		return intents
	}
	return []string{}
}

// inferDescription 根据名称推断描述
func (k *Kit) inferDescription(name string, skillType SkillType) string {
	descMap := map[string]string{
		"commit-message":      "Commit message 撰写规范",
		"batch-commit":        "分批提交操作指引",
		"push-fail-guide":     "推送失败的友好提示规则",
		"conflict-resolution": "冲突处理指引",
		"always-execute":      "直接执行操作，不要只给建议",
		"no-repeat-tools":     "避免重复调用工具",
		"no-git-terms":        "不向用户暴露 git 技术术语",
		"display-format":      "输出格式规范",
		"version-restore":     "版本恢复操作指引",
	}

	if desc, ok := descMap[name]; ok {
		return desc
	}
	return name
}

// register 注册一个 Skill/Rule（覆盖同名）
func (k *Kit) register(skill *Skill) {
	switch skill.Type {
	case SkillTypeSkill:
		k.skills[skill.Name] = skill
	case SkillTypeRule:
		k.rules[skill.Name] = skill
	}
}

// buildIntentIndex 构建意图→名称索引
func (k *Kit) buildIntentIndex() {
	k.intentIndex = make(map[string][]string)

	all := make(map[string]*Skill)
	for k, v := range k.skills {
		all[k] = v
	}
	for k, v := range k.rules {
		all[k] = v
	}

	for name, skill := range all {
		for _, intent := range skill.Intents {
			k.intentIndex[intent] = append(k.intentIndex[intent], name)
		}
	}
}

// applyDisabled 应用禁用列表
func (k *Kit) applyDisabled() {
	disabledSet := make(map[string]bool)
	for _, name := range k.config.Disabled {
		disabledSet[name] = true
	}

	for name, skill := range k.skills {
		if disabledSet[name] {
			skill.Enabled = false
		}
	}
	for name, skill := range k.rules {
		if disabledSet[name] {
			skill.Enabled = false
		}
	}
}

// GetPromptForIntents 根据意图列表获取需要注入的提示词内容
// 返回的提示词按 Priority 排序
func (k *Kit) GetPromptForIntents(intents []string) string {
	k.mu.RLock()
	defer k.mu.RUnlock()

	// 收集所有关联的 Skill/Rule 名称
	nameSet := make(map[string]bool)
	for _, intent := range intents {
		if names, ok := k.intentIndex[intent]; ok {
			for _, name := range names {
				nameSet[name] = true
			}
		}
	}

	// 也加载全局规则（没有关联意图但始终生效）
	for name, rule := range k.rules {
		if len(rule.Intents) == 0 && rule.Enabled {
			nameSet[name] = true
		}
	}
	for name, skill := range k.skills {
		if len(skill.Intents) == 0 && skill.Enabled {
			nameSet[name] = true
		}
	}

	// 收集并排序
	var items []struct {
		name     string
		priority int
		content  string
		typeName string
	}

	all := make(map[string]*Skill)
	for k, v := range k.skills {
		all[k] = v
	}
	for k, v := range k.rules {
		all[k] = v
	}

	for name := range nameSet {
		if skill, ok := all[name]; ok && skill.Enabled {
			items = append(items, struct {
				name     string
				priority int
				content  string
				typeName string
			}{name, skill.Priority, skill.Content, string(skill.Type)})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].priority < items[j].priority
	})

	// 组装提示词
	var sb strings.Builder
	for _, item := range items {
		if item.content == "" {
			continue
		}
		sectionName := "技能"
		if item.typeName == "rule" {
			sectionName = "规则"
		}
		sb.WriteString(fmt.Sprintf("## [%s] %s\n%s\n\n", sectionName, item.name, item.content))
	}

	return sb.String()
}

// GetAllPrompt 获取所有已启用的提示词（用于全局注入）
func (k *Kit) GetAllPrompt() string {
	k.mu.RLock()
	defer k.mu.RUnlock()

	var items []struct {
		name     string
		priority int
		content  string
		typeName string
	}

	for name, skill := range k.skills {
		if skill.Enabled {
			items = append(items, struct {
				name     string
				priority int
				content  string
				typeName string
			}{name, skill.Priority, skill.Content, string(skill.Type)})
		}
	}
	for name, rule := range k.rules {
		if rule.Enabled {
			items = append(items, struct {
				name     string
				priority int
				content  string
				typeName string
			}{name, rule.Priority, rule.Content, string(rule.Type)})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].priority < items[j].priority
	})

	var sb strings.Builder
	for _, item := range items {
		if item.content == "" {
			continue
		}
		sectionName := "技能"
		if item.typeName == "rule" {
			sectionName = "规则"
		}
		sb.WriteString(fmt.Sprintf("## [%s] %s\n%s\n\n", sectionName, item.name, item.content))
	}

	return sb.String()
}

// ListAll 列出所有已加载的 Skill 和 Rule
func (k *Kit) ListAll() []Skill {
	k.mu.RLock()
	defer k.mu.RUnlock()

	var result []Skill
	for _, s := range k.skills {
		result = append(result, *s)
	}
	for _, r := range k.rules {
		result = append(result, *r)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		return result[i].Priority < result[j].Priority
	})

	return result
}

// GetUserConfigDir 获取用户级别的配置目录
// 遵循 XDG 规范：~/.config/git-agent/ （macOS/Linux）
func GetUserConfigDir() string {
	// 优先使用环境变量
	if dir := os.Getenv("GIT_AGENT_CONFIG_DIR"); dir != "" {
		return dir
	}

	// XDG 规范
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "git-agent")
	}

	// 默认 ~/.config/git-agent
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "git-agent")
}

// GetProjectConfigDir 获取项目级别的配置目录
// 在仓库根目录下的 .git-agent/ 目录
func GetProjectConfigDir(repoPath string) string {
	return filepath.Join(repoPath, ".git-agent")
}

// EnsureDirs 确保配置目录存在
func EnsureDirs(configDir string) error {
	skillsDir := filepath.Join(configDir, "skills")
	rulesDir := filepath.Join(configDir, "rules")

	for _, dir := range []string{skillsDir, rulesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	return nil
}

// OnReload 设置重载完成后的回调函数
func (k *Kit) OnReload(fn func()) {
	k.onReload = fn
}

// WatchAndHotReload 启动文件监听，实现热加载
// 调用此方法后，用户级和项目级的 Skill/Rule 文件变更会自动触发重载
// 必须在 Load() 之后调用
func (k *Kit) WatchAndHotReload() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("创建文件监听器失败: %w", err)
	}
	k.watcher = watcher

	// 监听用户级和项目级目录（仅监听已存在的目录）
	dirsToWatch := k.collectExistingWatchDirs()
	watchedCount := 0
	for _, dir := range dirsToWatch {
		if err := watcher.Add(dir); err != nil {
			// 目录存在但监听失败，记录警告
			log.Printf("[promptkit] 监听目录失败 %s: %v", dir, err)
			continue
		}
		log.Printf("[promptkit] 监听目录: %s", dir)
		watchedCount++
	}

	if watchedCount > 0 {
		// 启动事件处理 goroutine
		go k.watchLoop()
	} else {
		// 没有可监听的目录，关闭 watcher 节省资源
		watcher.Close()
		k.watcher = nil
	}

	return nil
}

// collectExistingWatchDirs 收集需要监听且已存在的目录列表
func (k *Kit) collectExistingWatchDirs() []string {
	candidates := k.collectWatchDirs()
	var existing []string
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			existing = append(existing, dir)
		}
	}
	return existing
}

// collectWatchDirs 收集需要监听的目录列表
func (k *Kit) collectWatchDirs() []string {
	var dirs []string

	if k.config.UserDir != "" {
		dirs = append(dirs,
			filepath.Join(k.config.UserDir, "skills"),
			filepath.Join(k.config.UserDir, "rules"),
		)
	}

	if k.config.ProjectDir != "" {
		dirs = append(dirs,
			filepath.Join(k.config.ProjectDir, "skills"),
			filepath.Join(k.config.ProjectDir, "rules"),
		)
	}

	return dirs
}

// watchLoop 文件监听事件循环
func (k *Kit) watchLoop() {
	debounceTimer := time.NewTimer(0)
	if !debounceTimer.Stop() {
		<-debounceTimer.C
	}

	for {
		select {
		case <-k.done:
			debounceTimer.Stop()
			return

		case event, ok := <-k.watcher.Events:
			if !ok {
				return
			}
			// 只关心 .md 文件的 Write、Create、Rename、Remove 事件
			if !strings.HasSuffix(event.Name, ".md") {
				continue
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) ||
				event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
				log.Printf("[promptkit] 检测到文件变更: %s %s", event.Name, event.Op.String())
				// 防抖：重置定时器，300ms 内的多次事件合并为一次重载
				debounceTimer.Reset(300 * time.Millisecond)
			}

		case err, ok := <-k.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[promptkit] 文件监听错误: %v", err)

		case <-debounceTimer.C:
			// 防抖定时器到期，执行重载
			k.Reload()
		}
	}
}

// Reload 重新加载所有 Skills 和 Rules
// 保留内置不变，只重新加载用户级和项目级
func (k *Kit) Reload() {
	k.mu.Lock()

	// 重新加载：清空后全量加载
	k.skills = make(map[string]*Skill)
	k.rules = make(map[string]*Skill)
	k.intentIndex = make(map[string][]string)

	// 1. 内置 Skills
	if err := k.loadFromFS(k.builtinFS, "skills", SkillTypeSkill, "builtin"); err != nil {
		log.Printf("[promptkit] 重新加载内置 Skills 失败: %v", err)
	}

	// 2. 内置 Rules
	if err := k.loadFromFS(k.builtinFS, "rules", SkillTypeRule, "builtin"); err != nil {
		log.Printf("[promptkit] 重新加载内置 Rules 失败: %v", err)
	}

	// 3. 用户自定义
	if k.config.UserDir != "" {
		_ = k.loadFromDir(filepath.Join(k.config.UserDir, "skills"), SkillTypeSkill, "user")
		_ = k.loadFromDir(filepath.Join(k.config.UserDir, "rules"), SkillTypeRule, "user")
	}

	// 4. 项目级
	if k.config.ProjectDir != "" {
		_ = k.loadFromDir(filepath.Join(k.config.ProjectDir, "skills"), SkillTypeSkill, "project")
		_ = k.loadFromDir(filepath.Join(k.config.ProjectDir, "rules"), SkillTypeRule, "project")
	}

	// 5. 重建索引
	k.buildIntentIndex()

	// 6. 重新应用禁用列表
	k.applyDisabled()

	k.lastReload = time.Now()

	k.mu.Unlock()

	// 7. 动态补充监听新出现的目录（在锁外执行，避免死锁）
	k.addNewWatchDirs()

	log.Printf("[promptkit] Skills/Rules 热重载完成 (skills=%d, rules=%d)",
		len(k.skills), len(k.rules))

	// 触发回调
	if k.onReload != nil {
		k.onReload()
	}
}

// addNewWatchDirs 检查并补充监听新出现的目录
// 在 Reload 中调用，处理用户在运行期间创建了自定义目录的场景
func (k *Kit) addNewWatchDirs() {
	dirs := k.collectExistingWatchDirs()
	if len(dirs) == 0 {
		return
	}

	if k.watcher == nil {
		// watcher 未启动（启动时没有可监听目录），尝试初始化并启动监听
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return
		}
		for _, dir := range dirs {
			if err := watcher.Add(dir); err != nil {
				continue
			}
			log.Printf("[promptkit] 新增监听目录: %s", dir)
		}
		k.watcher = watcher
		go k.watchLoop()
		return
	}

	// watcher 已存在，检查是否有新目录需要补充监听
	for _, dir := range dirs {
		if err := k.watcher.Add(dir); err != nil {
			continue
		}
		log.Printf("[promptkit] 新增监听目录: %s", dir)
	}
}

// Close 关闭文件监听器，释放资源
func (k *Kit) Close() {
	if k.watcher != nil {
		close(k.done)
		k.watcher.Close()
		log.Printf("[promptkit] 文件监听已关闭")
	}
}

// LastReloadTime 返回上次重载时间
func (k *Kit) LastReloadTime() time.Time {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.lastReload
}

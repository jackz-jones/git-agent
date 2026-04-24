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

// dirSubdirToType 子目录名到 SkillType 的映射，用于从目录结构自动推断类型
var dirSubdirToType = map[string]SkillType{
	"skills": SkillTypeSkill,
	"rules":  SkillTypeRule,
}

// Skill 定义一个可加载的提示词片段
type Skill struct {
	Name        string    `json:"name"`         // 唯一标识，如 "commit-message"
	Type        SkillType `json:"type"`         // skill 或 rule，由所在目录自动推断
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
	items       map[string]*Skill   // 所有已加载的 Skill/Rule（key = name，同名覆盖）
	intentIndex map[string][]string // intent → skill/rule names
	builtinFS   embed.FS            // 内置资源文件系统
	config      Config

	// 热加载相关
	watcher    *fsnotify.Watcher // 文件监听器
	done       chan struct{}     // 关闭信号
	reloadCh   chan struct{}     // 防抖：合并短时间内的多次事件
	onReload   func()           // 重载完成后的回调（可选，用于日志通知）
	lastReload time.Time        // 上次重载时间
}

// NewKit 创建一个新的 Kit 实例
func NewKit(builtinFS embed.FS, config Config) *Kit {
	k := &Kit{
		items:       make(map[string]*Skill),
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

	// 1. 加载内置资源（从 embed.FS 的 resources/ 目录）
	if err := k.loadBuiltin(); err != nil {
		return err
	}

	// 2. 加载用户自定义（覆盖内置）
	if k.config.UserDir != "" {
		k.loadUserCustom()
	}

	// 3. 加载项目级（覆盖用户级和内置）
	if k.config.ProjectDir != "" {
		k.loadProjectCustom()
	}

	// 4. 构建意图索引
	k.buildIntentIndex()

	// 5. 应用禁用列表
	k.applyDisabled()

	return nil
}

// loadBuiltin 从内置 embed.FS 加载，自动按子目录名推断类型
func (k *Kit) loadBuiltin() error {
	// 遍历 resources/ 下的子目录（skills/、rules/）
	entries, err := fs.ReadDir(k.builtinFS, "resources")
	if err != nil {
		return fmt.Errorf("读取内置资源目录失败: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subdir := entry.Name()
		skillType, ok := dirSubdirToType[subdir]
		if !ok {
			continue // 忽略未知子目录
		}

		if err := k.loadFromFS(k.builtinFS, subdir, skillType, "builtin"); err != nil {
			return fmt.Errorf("加载内置 %s 失败: %w", subdir, err)
		}
	}
	return nil
}

// loadUserCustom 加载用户级自定义 Skill/Rule
func (k *Kit) loadUserCustom() {
	for subdir, skillType := range dirSubdirToType {
		dir := filepath.Join(k.config.UserDir, subdir)
		_ = k.loadFromDir(dir, skillType, "user")
	}
}

// loadProjectCustom 加载项目级自定义 Skill/Rule
func (k *Kit) loadProjectCustom() {
	for subdir, skillType := range dirSubdirToType {
		dir := filepath.Join(k.config.ProjectDir, subdir)
		_ = k.loadFromDir(dir, skillType, "project")
	}
}

// loadFromFS 从 embed.FS 加载 Skill/Rule
// subdir 为子目录名（如 "skills"、"rules"），skillType 从 dirSubdirToType 映射获取
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
// skillType 从 dirSubdirToType 映射获取，由调用方传入
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
//
// 没有 meta 行时：intents 为空（全局生效），description 已默认为 name
func (k *Kit) parseSkill(name, content string, skillType SkillType, source string) *Skill {
	skill := &Skill{
		Name:        name,
		Type:        skillType,
		Content:     content,
		Source:      source,
		Intents:     []string{},
		Priority:    50, // 默认优先级
		Enabled:     true,
		Description: name, // 默认描述使用文件名
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
	}
	// 没有 meta 行时：intents 保持为空（全局生效），description 已默认为 name

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

// register 注册一个 Skill/Rule（同名覆盖）
func (k *Kit) register(skill *Skill) {
	k.items[skill.Name] = skill
}

// buildIntentIndex 构建意图→名称索引
func (k *Kit) buildIntentIndex() {
	k.intentIndex = make(map[string][]string)

	for name, skill := range k.items {
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

	for name, skill := range k.items {
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
	for name, item := range k.items {
		if len(item.Intents) == 0 && item.Enabled {
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

	for name := range nameSet {
		if skill, ok := k.items[name]; ok && skill.Enabled {
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

	for name, skill := range k.items {
		if skill.Enabled {
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
	for _, s := range k.items {
		result = append(result, *s)
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
func (k *Kit) Reload() {
	k.mu.Lock()

	// 重新加载：清空后全量加载
	k.items = make(map[string]*Skill)
	k.intentIndex = make(map[string][]string)

	// 1. 内置资源
	if err := k.loadBuiltin(); err != nil {
		log.Printf("[promptkit] 重新加载内置资源失败: %v", err)
	}

	// 2. 用户自定义
	if k.config.UserDir != "" {
		k.loadUserCustom()
	}

	// 3. 项目级
	if k.config.ProjectDir != "" {
		k.loadProjectCustom()
	}

	// 4. 重建索引
	k.buildIntentIndex()

	// 5. 重新应用禁用列表
	k.applyDisabled()

	k.lastReload = time.Now()

	k.mu.Unlock()

	// 6. 动态补充监听新出现的目录（在锁外执行，避免死锁）
	k.addNewWatchDirs()

	// 统计各类型数量
	skillCount, ruleCount := 0, 0
	for _, item := range k.items {
		switch item.Type {
		case SkillTypeSkill:
			skillCount++
		case SkillTypeRule:
			ruleCount++
		}
	}
	log.Printf("[promptkit] Skills/Rules 热重载完成 (skills=%d, rules=%d)", skillCount, ruleCount)

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
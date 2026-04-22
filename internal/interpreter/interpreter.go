package interpreter

import (
	"fmt"
	"strings"
)

// IntentType 表示意图类型的枚举
type IntentType string

const (
	IntentSaveVersion    IntentType = "save_version"    // 保存版本
	IntentViewHistory    IntentType = "view_history"    // 查看历史
	IntentRestoreVersion IntentType = "restore_version" // 恢复版本
	IntentViewDiff       IntentType = "view_diff"       // 查看差异
	IntentSubmitChange   IntentType = "submit_change"   // 提交更改
	IntentViewTeamChange IntentType = "view_team_change" // 查看团队更改
	IntentApproveMerge   IntentType = "approve_merge"   // 批准合并
	IntentInitRepo       IntentType = "init_repo"       // 初始化仓库
	IntentViewStatus     IntentType = "view_status"     // 查看状态
	IntentCreateBranch   IntentType = "create_branch"   // 创建分支
	IntentSwitchBranch   IntentType = "switch_branch"   // 切换分支
	IntentListBranches   IntentType = "list_branches"   // 列出分支
	IntentCreateTag      IntentType = "create_tag"      // 创建标签
	IntentPush           IntentType = "push"            // 推送
	IntentPull           IntentType = "pull"            // 拉取
	IntentDetectConflict IntentType = "detect_conflict" // 检测冲突
	IntentHelp           IntentType = "help"            // 帮助
	IntentUnknown        IntentType = "unknown"         // 未知意图
)

// UserIntent 表示解析后的用户意图
type UserIntent struct {
	Type        IntentType         `json:"type"`
	Target      string             `json:"target"`       // 操作目标（文件名、分支名等）
	UserInput   string             `json:"description"`  // 用户原始描述
	Params      map[string]string  `json:"params"`       // 附加参数
	Confidence  float64            `json:"confidence"`   // 解析置信度 0-1
}

// Description 返回意图的友好描述
func (i *UserIntent) Description() string {
	return i.UserInput
}

// Interpreter 用户意图解析引擎
// 将自然语言输入解析为结构化的操作意图
type Interpreter struct {
	language string
	patterns []intentPattern
}

// intentPattern 意图匹配模式
type intentPattern struct {
	intentType IntentType
	keywords   []string   // 匹配关键词
	extractor  func(input string, matches []string) *UserIntent // 参数提取器
}

// New 创建新的意图解析器
func New(language string) *Interpreter {
	interp := &Interpreter{
		language: language,
	}
	interp.initPatterns()
	return interp
}

// initPatterns 初始化意图匹配模式
func (i *Interpreter) initPatterns() {
	i.patterns = []intentPattern{
		// 保存版本
		{
			intentType: IntentSaveVersion,
			keywords:   []string{"保存", "提交", "存一下", "保存修改", "保存版本", "commit", "save", "暂存"},
			extractor:  extractSaveVersion,
		},
		// 查看历史
		{
			intentType: IntentViewHistory,
			keywords:   []string{"历史", "记录", "提交记录", "版本历史", "修改记录", "log", "history", "谁改的"},
			extractor:  extractViewHistory,
		},
		// 恢复版本
		{
			intentType: IntentRestoreVersion,
			keywords:   []string{"恢复", "回滚", "还原", "回退", "撤销", "restore", "revert", "rollback", "回到"},
			extractor:  extractRestoreVersion,
		},
		// 查看差异
		{
			intentType: IntentViewDiff,
			keywords:   []string{"差异", "区别", "变更", "不同", "改了什么", "diff", "changes", "对比"},
			extractor:  extractViewDiff,
		},
		// 提交更改给团队
		{
			intentType: IntentSubmitChange,
			keywords:   []string{"提交给团队", "申请合并", "提交审核", "发起合并", "submit", "pr", "pull request"},
			extractor:  extractSubmitChange,
		},
		// 查看团队更改
		{
			intentType: IntentViewTeamChange,
			keywords:   []string{"看看", "查看他人", "团队修改", "别人改了", "同事", "小李", "老王", "小张", "team", "review"},
			extractor:  extractViewTeamChange,
		},
		// 批准合并
		{
			intentType: IntentApproveMerge,
			keywords:   []string{"批准", "同意合并", "通过", "approve", "merge", "合并", "接受"},
			extractor:  extractApproveMerge,
		},
		// 初始化仓库
		{
			intentType: IntentInitRepo,
			keywords:   []string{"初始化", "创建仓库", "新建仓库", "init", "新建项目"},
			extractor:  extractInitRepo,
		},
		// 查看状态
		{
			intentType: IntentViewStatus,
			keywords:   []string{"状态", "当前状态", "什么情况", "status", "有没有改", "哪些文件改了"},
			extractor:  extractViewStatus,
		},
		// 创建分支
		{
			intentType: IntentCreateBranch,
			keywords:   []string{"创建分支", "新建分支", "开个分支", "create branch", "工作副本"},
			extractor:  extractCreateBranch,
		},
		// 切换分支
		{
			intentType: IntentSwitchBranch,
			keywords:   []string{"切换分支", "换到", "转到", "checkout", "switch"},
			extractor:  extractSwitchBranch,
		},
		// 列出分支
		{
			intentType: IntentListBranches,
			keywords:   []string{"有哪些分支", "分支列表", "branches", "列出分支"},
			extractor:  extractListBranches,
		},
		// 创建标签
		{
			intentType: IntentCreateTag,
			keywords:   []string{"标签", "打标签", "标记版本", "tag", "milestone"},
			extractor:  extractCreateTag,
		},
		// 推送
		{
			intentType: IntentPush,
			keywords:   []string{"推送", "上传", "同步到远程", "push", "发送"},
			extractor:  extractPush,
		},
		// 拉取
		{
			intentType: IntentPull,
			keywords:   []string{"拉取", "更新", "同步", "pull", "下载最新", "获取最新"},
			extractor:  extractPull,
		},
		// 检测冲突
		{
			intentType: IntentDetectConflict,
			keywords:   []string{"冲突", "有没有冲突", "检测冲突", "conflict"},
			extractor:  extractDetectConflict,
		},
		// 帮助
		{
			intentType: IntentHelp,
			keywords:   []string{"帮助", "help", "怎么用", "能做什么", "使用说明"},
			extractor:  extractHelp,
		},
	}
}

// Parse 解析用户输入为结构化意图
// 这是意图解析引擎的核心方法，使用多策略匹配
func (i *Interpreter) Parse(input string) (*UserIntent, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("输入不能为空")
	}

	lowerInput := strings.ToLower(input)

	// 多策略匹配：遍历所有模式，找最佳匹配
	var bestMatch *UserIntent
	bestScore := 0.0

	for _, pattern := range i.patterns {
		score := i.matchScore(lowerInput, pattern.keywords)
		if score > bestScore {
			bestScore = score
			intent := pattern.extractor(input, pattern.keywords)
			intent.Type = pattern.intentType
			intent.Confidence = score
			bestMatch = intent
		}
	}

	if bestMatch == nil || bestScore < 0.2 {
		return &UserIntent{
			Type:        IntentUnknown,
			Target:      "",
			UserInput: input,
			Params:      make(map[string]string),
			Confidence:  0,
		}, fmt.Errorf("无法识别操作意图：%s", input)
	}

	return bestMatch, nil
}

// matchScore 计算输入与关键词模式的匹配分数
func (i *Interpreter) matchScore(input string, keywords []string) float64 {
	score := 0.0
	totalKeywords := float64(len(keywords))

	for _, kw := range keywords {
		if strings.Contains(input, strings.ToLower(kw)) {
			// 精确匹配得分更高
			score += 1.0 / totalKeywords
		}
	}

	// 对长关键词给予额外权重
	for _, kw := range keywords {
		if len(kw) > 2 && strings.Contains(input, strings.ToLower(kw)) {
			score += 0.1
		}
	}

	return score
}

// TranslateResult 将执行结果翻译为用户友好的消息
func (i *Interpreter) TranslateResult(intent *UserIntent, result interface{}) string {
	switch intent.Type {
	case IntentSaveVersion:
		return i.translateSaveResult(result)
	case IntentViewHistory:
		return i.translateHistoryResult(result)
	case IntentRestoreVersion:
		return fmt.Sprintf("✅ 已恢复到指定版本")
	case IntentViewDiff:
		return i.translateDiffResult(result)
	case IntentViewStatus:
		return i.translateStatusResult(result)
	case IntentInitRepo:
		return "✅ 文档仓库已创建，您可以开始添加文件了"
	case IntentCreateBranch:
		return fmt.Sprintf("✅ 已创建工作副本「%s」", intent.Target)
	case IntentSwitchBranch:
		return fmt.Sprintf("✅ 已切换到工作副本「%s」", intent.Target)
	case IntentListBranches:
		return i.translateBranchListResult(result)
	case IntentPush:
		return "✅ 已同步到远程仓库"
	case IntentPull:
		return "✅ 已获取最新内容"
	case IntentCreateTag:
		return fmt.Sprintf("✅ 已标记版本「%s」", intent.Target)
	case IntentDetectConflict:
		return i.translateConflictResult(result)
	case IntentHelp:
		return i.translateHelp()
	default:
		return "操作完成"
	}
}

// SuggestNext 根据当前意图推荐后续操作
func (i *Interpreter) SuggestNext(intent *UserIntent) []string {
	switch intent.Type {
	case IntentSaveVersion:
		return []string{"推送到远程仓库", "查看修改历史"}
	case IntentViewHistory:
		return []string{"恢复某个版本", "查看具体差异"}
	case IntentViewStatus:
		return []string{"保存当前修改", "查看修改详情"}
	case IntentInitRepo:
		return []string{"添加文件到仓库", "保存初始版本"}
	case IntentDetectConflict:
		return []string{"查看冲突详情", "选择解决方式"}
	default:
		return []string{"查看当前状态", "保存修改"}
	}
}

// ==================== 参数提取器 ====================

func extractSaveVersion(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	// 尝试提取描述信息
	desc := input
	for _, kw := range matches {
		desc = strings.ReplaceAll(desc, kw, "")
	}
	desc = strings.TrimSpace(desc)
	if desc != "" {
		intent.Params["message"] = desc
	} else {
		intent.Params["message"] = "保存修改"
	}

	// 尝试提取文件名
	parts := strings.Fields(input)
	for _, p := range parts {
		if strings.Contains(p, ".") && !strings.Contains(p, "保存") && !strings.Contains(p, "提交") {
			intent.Params["file"] = p
			intent.Target = p
			break
		}
	}

	return intent
}

func extractViewHistory(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	// 提取数量
	intent.Params["limit"] = "10"

	// 提取作者
	lower := strings.ToLower(input)
	authors := []string{"小李", "老王", "小张", "小陈", "小赵"}
	for _, a := range authors {
		if strings.Contains(lower, a) {
			intent.Params["author"] = a
			intent.Target = a
			break
		}
	}

	// 提取文件名
	parts := strings.Fields(input)
	for _, p := range parts {
		if strings.Contains(p, ".") {
			intent.Params["file"] = p
			intent.Target = p
			break
		}
	}

	return intent
}

func extractRestoreVersion(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	parts := strings.Fields(input)
	for _, p := range parts {
		if len(p) >= 7 && isHexString(p) {
			intent.Params["version_id"] = p
			intent.Target = p
			break
		}
	}

	// 提取文件名
	for _, p := range parts {
		if strings.Contains(p, ".") {
			intent.Params["file"] = p
			intent.Target = p
			break
		}
	}

	return intent
}

func extractViewDiff(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	parts := strings.Fields(input)
	for _, p := range parts {
		if strings.Contains(p, ".") {
			intent.Params["file"] = p
			intent.Target = p
			break
		}
	}

	return intent
}

func extractSubmitChange(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	desc := input
	for _, kw := range matches {
		desc = strings.ReplaceAll(desc, kw, "")
	}
	desc = strings.TrimSpace(desc)
	if desc != "" {
		intent.Params["message"] = desc
	} else {
		intent.Params["message"] = "提交团队审核"
	}

	return intent
}

func extractViewTeamChange(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	// 提取作者
	lower := strings.ToLower(input)
	authors := []string{"小李", "老王", "小张", "小陈", "小赵"}
	for _, a := range authors {
		if strings.Contains(lower, a) {
			intent.Params["author"] = a
			intent.Target = a
			break
		}
	}

	intent.Params["limit"] = "10"
	return intent
}

func extractApproveMerge(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	parts := strings.Fields(input)
	for _, p := range parts {
		if p != "批准" && p != "同意" && p != "合并" && p != "approve" && p != "merge" {
			intent.Params["branch"] = p
			intent.Target = p
			break
		}
	}

	return intent
}

func extractInitRepo(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	parts := strings.Fields(input)
	for _, p := range parts {
		if p != "初始化" && p != "创建" && p != "仓库" && p != "init" && p != "新建" && p != "项目" {
			intent.Params["name"] = p
			intent.Target = p
			break
		}
	}

	return intent
}

func extractViewStatus(input string, matches []string) *UserIntent {
	return &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}
}

func extractCreateBranch(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	parts := strings.Fields(input)
	for _, p := range parts {
		if p != "创建" && p != "新建" && p != "分支" && p != "工作" && p != "副本" && p != "create" && p != "branch" && p != "开个" {
			intent.Params["name"] = p
			intent.Target = p
			break
		}
	}

	return intent
}

func extractSwitchBranch(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	parts := strings.Fields(input)
	for _, p := range parts {
		if p != "切换" && p != "换到" && p != "转到" && p != "checkout" && p != "switch" && p != "分支" {
			intent.Params["name"] = p
			intent.Target = p
			break
		}
	}

	return intent
}

func extractListBranches(input string, matches []string) *UserIntent {
	return &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}
}

func extractCreateTag(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	parts := strings.Fields(input)
	for _, p := range parts {
		if p != "标签" && p != "标记" && p != "版本" && p != "tag" && p != "打" && p != "milestone" {
			intent.Params["name"] = p
			intent.Target = p
			break
		}
	}

	return intent
}

func extractPush(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	intent.Params["remote"] = "origin"

	parts := strings.Fields(input)
	for _, p := range parts {
		if p != "推送" && p != "上传" && p != "同步" && p != "push" && p != "发送" && p != "到" && p != "远程" {
			intent.Params["remote"] = p
			intent.Target = p
			break
		}
	}

	return intent
}

func extractPull(input string, matches []string) *UserIntent {
	intent := &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}

	intent.Params["remote"] = "origin"

	parts := strings.Fields(input)
	for _, p := range parts {
		if p != "拉取" && p != "更新" && p != "同步" && p != "pull" && p != "下载" && p != "获取" && p != "最新" {
			intent.Params["remote"] = p
			intent.Target = p
			break
		}
	}

	return intent
}

func extractDetectConflict(input string, matches []string) *UserIntent {
	return &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}
}

func extractHelp(input string, matches []string) *UserIntent {
	return &UserIntent{
		Target:      "",
		UserInput: input,
		Params:      make(map[string]string),
	}
}

// ==================== 结果翻译器 ====================

func (i *Interpreter) translateSaveResult(result interface{}) string {
	if m, ok := result.(map[string]string); ok {
		if hash, exists := m["hash"]; exists {
			return fmt.Sprintf("✅ 已保存为新版本 #%s", hash)
		}
	}
	return "✅ 已保存新版本"
}

func (i *Interpreter) translateHistoryResult(result interface{}) string {
	versions, ok := result.([]interface{})
	if !ok {
		return "暂无历史记录"
	}
	if len(versions) == 0 {
		return "暂无历史记录"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 共找到 %d 条历史记录：\n", len(versions)))
	for idx, v := range versions {
		if vi, ok := v.(interface{ GetShortHash() string }); ok {
			sb.WriteString(fmt.Sprintf("  %d. 版本#%s\n", idx+1, vi.GetShortHash()))
		}
	}
	return sb.String()
}

func (i *Interpreter) translateDiffResult(result interface{}) string {
	if s, ok := result.(string); ok {
		return fmt.Sprintf("📋 修改详情：\n%s", s)
	}
	return "没有发现修改"
}

func (i *Interpreter) translateStatusResult(result interface{}) string {
	if si, ok := result.(interface {
		GetIsClean() bool
		GetStagedCount() int
		GetUnstagedCount() int
		GetUntrackedCount() int
	}); ok {
		if si.GetIsClean() {
			return "✅ 当前没有未保存的修改"
		}
		return fmt.Sprintf("📋 当前状态：\n  已暂存修改：%d 个文件\n  未暂存修改：%d 个文件\n  新文件：%d 个",
			si.GetStagedCount(), si.GetUnstagedCount(), si.GetUntrackedCount())
	}
	return "已获取当前状态"
}

func (i *Interpreter) translateBranchListResult(result interface{}) string {
	return fmt.Sprintf("📋 分支列表：%v", result)
}

func (i *Interpreter) translateConflictResult(result interface{}) string {
	return fmt.Sprintf("📋 冲突检测结果：%v", result)
}

func (i *Interpreter) translateHelp() string {
	return `🤖 Git Agent - 您的文件版本管理助手

我可以帮您完成以下操作：

📂 版本管理：
  • "保存修改" - 保存当前文件为新版本
  • "查看历史" - 查看修改记录
  • "恢复版本" - 回到之前的版本
  • "查看差异" - 对比修改内容

👥 团队协作：
  • "提交给团队" - 提交修改供团队审核
  • "看看小李改了什么" - 查看他人修改
  • "合并老王的方案" - 合并他人的修改

🔧 仓库管理：
  • "初始化仓库" - 创建新文档仓库
  • "查看状态" - 查看当前修改状态
  • "推送" / "拉取" - 同步远程仓库

只需用自然语言告诉我您想做什么！`
}

// 辅助函数
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

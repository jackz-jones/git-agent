package planner

import (
	"fmt"

	"github.com/jackz-jones/git-agent/internal/interpreter"
)

// StepType 步骤类型
type StepType string

const (
	StepGitInit         StepType = "git_init"
	StepGitAdd          StepType = "git_add"
	StepGitCommit       StepType = "git_commit"
	StepGitPush         StepType = "git_push"
	StepGitPull         StepType = "git_pull"
	StepGitLog          StepType = "git_log"
	StepGitDiff         StepType = "git_diff"
	StepGitStatus       StepType = "git_status"
	StepGitBranch       StepType = "git_branch"
	StepGitMerge        StepType = "git_merge"
	StepGitTag          StepType = "git_tag"
	StepGitRestore      StepType = "git_restore"
	StepConflictDetect  StepType = "conflict_detect"
	StepConflictResolve StepType = "conflict_resolve"
	StepRepoCreate      StepType = "repo_create"
	StepRepoClone       StepType = "repo_clone"
	StepUpdateUserInfo  StepType = "update_user_info"
)

// Step 表示执行计划中的一个步骤
type Step struct {
	Type     StepType          `json:"type"`
	Params   map[string]string `json:"params"`
	Required bool              `json:"required"`    // 是否为必要步骤（失败时终止计划）
	Desc     string            `json:"description"` // 步骤描述
}

// Plan 表示一个执行计划
type Plan struct {
	Intent     *interpreter.UserIntent `json:"intent"`
	Steps      []*Step                 `json:"steps"`
	TotalSteps int                     `json:"total_steps"`
}

// ExecutionResult 执行结果
type ExecutionResult struct {
	CompletedSteps []StepResult `json:"completed_steps"`
	FailedSteps    []StepResult `json:"failed_steps"`
}

// StepResult 单步执行结果
type StepResult struct {
	Index  int         `json:"index"`
	Step   *Step       `json:"step"`
	Result interface{} `json:"result"`
	Err    error       `json:"error,omitempty"`
}

// NewExecutionResult 创建执行结果
func NewExecutionResult() *ExecutionResult {
	return &ExecutionResult{
		CompletedSteps: make([]StepResult, 0),
		FailedSteps:    make([]StepResult, 0),
	}
}

// AddCompletedStep 添加已完成的步骤
func (r *ExecutionResult) AddCompletedStep(index int, step *Step, result interface{}) {
	r.CompletedSteps = append(r.CompletedSteps, StepResult{
		Index:  index,
		Step:   step,
		Result: result,
	})
}

// AddFailedStep 添加失败的步骤
func (r *ExecutionResult) AddFailedStep(index int, step *Step, err error) {
	r.FailedSteps = append(r.FailedSteps, StepResult{
		Index: index,
		Step:  step,
		Err:   err,
	})
}

// IsSuccess 判断整体执行是否成功
func (r *ExecutionResult) IsSuccess() bool {
	return len(r.FailedSteps) == 0
}

// Planner 执行规划器
// 将用户意图转化为具体的多步骤执行计划
type Planner struct{}

// New 创建新的规划器
func New() *Planner {
	return &Planner{}
}

// CreatePlan 根据用户意图创建执行计划
func (p *Planner) CreatePlan(intent *interpreter.UserIntent) (*Plan, error) {
	if intent == nil {
		return nil, fmt.Errorf("意图不能为空")
	}

	plan := &Plan{
		Intent: intent,
		Steps:  make([]*Step, 0),
	}

	switch intent.Type {
	case interpreter.IntentSaveVersion:
		plan.Steps = p.planSaveVersion(intent)
	case interpreter.IntentViewHistory:
		plan.Steps = p.planViewHistory(intent)
	case interpreter.IntentRestoreVersion:
		plan.Steps = p.planRestoreVersion(intent)
	case interpreter.IntentViewDiff:
		plan.Steps = p.planViewDiff(intent)
	case interpreter.IntentSubmitChange:
		plan.Steps = p.planSubmitChange(intent)
	case interpreter.IntentViewTeamChange:
		plan.Steps = p.planViewTeamChange(intent)
	case interpreter.IntentApproveMerge:
		plan.Steps = p.planApproveMerge(intent)
	case interpreter.IntentInitRepo:
		plan.Steps = p.planInitRepo(intent)
	case interpreter.IntentViewStatus:
		plan.Steps = p.planViewStatus(intent)
	case interpreter.IntentCreateBranch:
		plan.Steps = p.planCreateBranch(intent)
	case interpreter.IntentSwitchBranch:
		plan.Steps = p.planSwitchBranch(intent)
	case interpreter.IntentListBranches:
		plan.Steps = p.planListBranches(intent)
	case interpreter.IntentCreateTag:
		plan.Steps = p.planCreateTag(intent)
	case interpreter.IntentPush:
		plan.Steps = p.planPush(intent)
	case interpreter.IntentPull:
		plan.Steps = p.planPull(intent)
	case interpreter.IntentDetectConflict:
		plan.Steps = p.planDetectConflict(intent)
	case interpreter.IntentUpdateUserInfo:
		plan.Steps = p.planUpdateUserInfo(intent)
	case interpreter.IntentHelp:
		// 帮助不需要执行步骤
	default:
		return nil, fmt.Errorf("无法为意图类型 %s 创建执行计划", intent.Type)
	}

	plan.TotalSteps = len(plan.Steps)
	return plan, nil
}

// ==================== 各意图的执行计划 ====================

// 保存版本：add → commit
func (p *Planner) planSaveVersion(intent *interpreter.UserIntent) []*Step {
	message := intent.Params["message"]
	if message == "" {
		message = "chore: save changes"
	}

	file := intent.Params["file"]
	addParams := make(map[string]string)
	if file != "" {
		addParams["files"] = file
	}

	return []*Step{
		{
			Type:     StepGitAdd,
			Params:   addParams,
			Required: true,
			Desc:     "添加文件到暂存区",
		},
		{
			Type: StepGitCommit,
			Params: map[string]string{
				"message": message,
			},
			Required: true,
			Desc:     fmt.Sprintf("保存版本「%s」", message),
		},
	}
}

// 查看历史：log
func (p *Planner) planViewHistory(intent *interpreter.UserIntent) []*Step {
	limit := intent.Params["limit"]
	if limit == "" {
		limit = "10"
	}

	params := map[string]string{
		"limit": limit,
	}
	if author := intent.Params["author"]; author != "" {
		params["author"] = author
	}
	if file := intent.Params["file"]; file != "" {
		params["file"] = file
	}

	return []*Step{
		{
			Type:     StepGitLog,
			Params:   params,
			Required: true,
			Desc:     "获取版本历史记录",
		},
	}
}

// 恢复版本：restore
func (p *Planner) planRestoreVersion(intent *interpreter.UserIntent) []*Step {
	params := make(map[string]string)
	if versionID := intent.Params["version_id"]; versionID != "" {
		params["version_id"] = versionID
	}
	if file := intent.Params["file"]; file != "" {
		params["file"] = file
	}

	return []*Step{
		{
			Type:     StepGitRestore,
			Params:   params,
			Required: true,
			Desc:     "恢复到指定版本",
		},
	}
}

// 查看差异：diff
func (p *Planner) planViewDiff(intent *interpreter.UserIntent) []*Step {
	params := make(map[string]string)
	if file := intent.Params["file"]; file != "" {
		params["file"] = file
	}
	if commitHash := intent.Params["commit_hash"]; commitHash != "" {
		params["commit_hash"] = commitHash
	}

	desc := "查看文件修改差异"
	if commitHash := intent.Params["commit_hash"]; commitHash != "" {
		desc = fmt.Sprintf("查看提交 %s 的修改内容", commitHash)
	}

	return []*Step{
		{
			Type:     StepGitDiff,
			Params:   params,
			Required: true,
			Desc:     desc,
		},
	}
}

// 提交更改给团队：add → commit → push
func (p *Planner) planSubmitChange(intent *interpreter.UserIntent) []*Step {
	message := intent.Params["message"]
	if message == "" {
		message = "提交团队审核"
	}

	remote := "origin"

	return []*Step{
		{
			Type:     StepGitAdd,
			Params:   map[string]string{}, // 添加所有
			Required: true,
			Desc:     "添加所有修改到暂存区",
		},
		{
			Type: StepGitCommit,
			Params: map[string]string{
				"message": message,
			},
			Required: true,
			Desc:     fmt.Sprintf("提交修改「%s」", message),
		},
		{
			Type: StepGitPush,
			Params: map[string]string{
				"remote": remote,
			},
			Required: true,
			Desc:     "推送到远程仓库",
		},
	}
}

// 查看团队更改：pull → log
func (p *Planner) planViewTeamChange(intent *interpreter.UserIntent) []*Step {
	params := map[string]string{
		"limit": "10",
	}
	if author := intent.Params["author"]; author != "" {
		params["author"] = author
	}

	return []*Step{
		{
			Type: StepGitPull,
			Params: map[string]string{
				"remote": "origin",
			},
			Required: false, // 拉取失败不影响查看本地历史
			Desc:     "获取最新团队更改",
		},
		{
			Type:     StepGitLog,
			Params:   params,
			Required: true,
			Desc:     "查看团队修改历史",
		},
	}
}

// 批准合并：merge
func (p *Planner) planApproveMerge(intent *interpreter.UserIntent) []*Step {
	branch := intent.Params["branch"]

	return []*Step{
		{
			Type: StepGitMerge,
			Params: map[string]string{
				"branch": branch,
			},
			Required: true,
			Desc:     fmt.Sprintf("合并「%s」的修改", branch),
		},
	}
}

// 初始化仓库：init
func (p *Planner) planInitRepo(intent *interpreter.UserIntent) []*Step {
	return []*Step{
		{
			Type:     StepGitInit,
			Params:   make(map[string]string),
			Required: true,
			Desc:     "初始化文档仓库",
		},
	}
}

// 查看状态：status
func (p *Planner) planViewStatus(intent *interpreter.UserIntent) []*Step {
	return []*Step{
		{
			Type:     StepGitStatus,
			Params:   make(map[string]string),
			Required: true,
			Desc:     "查看当前仓库状态",
		},
	}
}

// 创建分支
func (p *Planner) planCreateBranch(intent *interpreter.UserIntent) []*Step {
	name := intent.Params["name"]

	return []*Step{
		{
			Type: StepGitBranch,
			Params: map[string]string{
				"action": "create",
				"name":   name,
			},
			Required: true,
			Desc:     fmt.Sprintf("创建工作副本「%s」", name),
		},
	}
}

// 切换分支
func (p *Planner) planSwitchBranch(intent *interpreter.UserIntent) []*Step {
	name := intent.Params["name"]

	return []*Step{
		{
			Type: StepGitBranch,
			Params: map[string]string{
				"action": "switch",
				"name":   name,
			},
			Required: true,
			Desc:     fmt.Sprintf("切换到工作副本「%s」", name),
		},
	}
}

// 列出分支
func (p *Planner) planListBranches(intent *interpreter.UserIntent) []*Step {
	return []*Step{
		{
			Type: StepGitBranch,
			Params: map[string]string{
				"action": "list",
			},
			Required: true,
			Desc:     "列出所有工作副本",
		},
	}
}

// 创建标签
func (p *Planner) planCreateTag(intent *interpreter.UserIntent) []*Step {
	name := intent.Params["name"]

	return []*Step{
		{
			Type: StepGitTag,
			Params: map[string]string{
				"name": name,
			},
			Required: true,
			Desc:     fmt.Sprintf("标记版本「%s」", name),
		},
	}
}

// 推送
func (p *Planner) planPush(intent *interpreter.UserIntent) []*Step {
	remote := intent.Params["remote"]
	if remote == "" {
		remote = "origin"
	}

	return []*Step{
		{
			Type:     StepGitStatus,
			Params:   map[string]string{},
			Required: false,
			Desc:     "检查当前仓库状态和同步情况",
		},
		{
			Type: StepGitPush,
			Params: map[string]string{
				"remote": remote,
			},
			Required: true,
			Desc:     "推送到远程仓库",
		},
	}
}

// 拉取
func (p *Planner) planPull(intent *interpreter.UserIntent) []*Step {
	remote := intent.Params["remote"]
	if remote == "" {
		remote = "origin"
	}

	return []*Step{
		{
			Type: StepGitPull,
			Params: map[string]string{
				"remote": remote,
			},
			Required: true,
			Desc:     "从远程仓库拉取最新内容",
		},
	}
}

// 检测冲突
func (p *Planner) planDetectConflict(intent *interpreter.UserIntent) []*Step {
	return []*Step{
		{
			Type:     StepConflictDetect,
			Params:   make(map[string]string),
			Required: true,
			Desc:     "检测文件冲突",
		},
	}
}

// 更新用户信息
func (p *Planner) planUpdateUserInfo(intent *interpreter.UserIntent) []*Step {
	params := make(map[string]string)
	if name := intent.Params["name"]; name != "" {
		params["name"] = name
	}
	if email := intent.Params["email"]; email != "" {
		params["email"] = email
	}

	return []*Step{
		{
			Type:     StepUpdateUserInfo,
			Params:   params,
			Required: true,
			Desc:     "更新用户信息",
		},
	}
}

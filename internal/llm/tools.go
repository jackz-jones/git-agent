package llm

// GitAgentTool Git Agent 工具定义
// 用于 LLM Function Calling，同时适配 LangChain Go 框架
type GitAgentTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema
}

// AllGitAgentTools 所有 Git Agent 可用的工具定义
var AllGitAgentTools = []GitAgentTool{
	{
		Name:        "save_version",
		Description: "保存当前文件修改为新版本。用户完成编辑后使用此功能保存。**重要：调用此工具前，必须先调用 view_diff 查看实际修改内容，基于 diff 结果撰写具体的 commit message，不可凭空猜测。**",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "Commit message in English. MUST be based on the actual diff content you just viewed — summarize the specific changes you saw, not vague descriptions. Use conventional commit style, e.g.: 'feat: add commit_hash param to view_diff and update status display' (NOT just 'docs: update files')",
				},
				"files": map[string]any{
					"type":        "string",
					"description": "要保存的文件路径，多个用逗号分隔。留空表示保存所有修改",
				},
			},
			"required": []string{"message"},
		},
	},
	{
		Name:        "view_history",
		Description: "查看文件或仓库的修改历史记录。当用户问「最新提交」「最近一次提交」「最后一次提交」时，设置 limit=1 来获取。当用户想看更多历史时，可增大 limit。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "显示的记录条数，默认10。用户问「最新提交」时设为1",
				},
				"author": map[string]any{
					"type":        "string",
					"description": "按作者筛选，输入作者名",
				},
				"file": map[string]any{
					"type":        "string",
					"description": "按文件筛选，输入文件路径",
				},
			},
		},
	},
	{
		Name:        "restore_version",
		Description: "恢复到之前的某个版本。注意：此操作不可逆，请先确认。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"version_id": map[string]any{
					"type":        "string",
					"description": "要恢复到的版本ID（commit hash）",
				},
				"file": map[string]any{
					"type":        "string",
					"description": "只恢复指定文件。留空表示恢复整个仓库",
				},
			},
		},
	},
	{
		Name:        "view_diff",
		Description: "查看文件的修改内容。如果指定 commit_hash，则查看该提交的修改点（相对于其父提交）；否则对比当前工作区和已保存版本的差异。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file": map[string]any{
					"type":        "string",
					"description": "要查看的文件路径。留空表示查看所有修改",
				},
				"commit_hash": map[string]any{
					"type":        "string",
					"description": "提交的 hash 值。指定后会查看该提交相对于父提交的修改内容，例如用户想查看某个提交的具体修改点",
				},
			},
		},
	},
	{
		Name:        "view_status",
		Description: "查看当前仓库的修改状态。返回内容包括：已暂存/未暂存的文件变更、最新一次提交信息（latest_commit）、以及本地与远程仓库的同步情况（ahead_behind，包含 ahead 领先数和 behind 落数）。当用户问「最新提交」「最近一次提交」时，请优先使用 view_history 工具而非此工具。",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		Name:        "submit_change",
		Description: "将修改提交给团队审核。会先保存版本，再推送到远程仓库。**重要：调用此工具前，必须先调用 view_diff 查看实际修改内容，基于 diff 结果撰写具体的 commit message，不可凭空猜测。**",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "Commit message in English. MUST be based on the actual diff content you just viewed — summarize the specific changes you saw, not vague descriptions. Use conventional commit style, e.g.: 'feat: add HTTPS auth support for push operations' (NOT just 'chore: submit changes')",
				},
			},
			"required": []string{"message"},
		},
	},
	{
		Name:        "view_team_change",
		Description: "查看团队成员的修改记录。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"author": map[string]any{
					"type":        "string",
					"description": "查看某位同事的修改，输入作者名",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "显示的记录条数，默认10",
				},
			},
		},
	},
	{
		Name:        "merge_branch",
		Description: "合并他人的修改到当前工作副本。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "要合并的工作副本名称",
				},
			},
			"required": []string{"branch"},
		},
	},
	{
		Name:        "init_repo",
		Description: "初始化一个新的文档仓库。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "仓库名称",
				},
			},
		},
	},
	{
		Name:        "create_branch",
		Description: "创建一个新的工作副本（分支），用于独立开展工作。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "工作副本名称",
				},
			},
			"required": []string{"name"},
		},
	},
	{
		Name:        "switch_branch",
		Description: "切换到另一个工作副本。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "要切换到的工作副本名称",
				},
			},
			"required": []string{"name"},
		},
	},
	{
		Name:        "list_branches",
		Description: "列出所有工作副本。",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		Name:        "create_tag",
		Description: "给当前版本打标签，方便以后查找。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "标签名称，例如：v1.0、终稿",
				},
			},
			"required": []string{"name"},
		},
	},
	{
		Name:        "push_to_remote",
		Description: "将本地修改推送到远程仓库，同步给团队。认证方式取决于远程仓库协议：HTTPS 仓库使用用户名+访问令牌认证，SSH 仓库自动使用本地 SSH 密钥。如果 SSH 认证失败，可以通过 remote_url 参数将远程地址切换为 HTTPS 格式再使用令牌认证。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"remote": map[string]any{
					"type":        "string",
					"description": "远程仓库名称，默认origin",
				},
				"username": map[string]any{
					"type":        "string",
					"description": "HTTPS 认证用户名（仅在 HTTPS 远程仓库认证失败后用户提供，如 GitHub 用户名）",
				},
				"password": map[string]any{
					"type":        "string",
					"description": "HTTPS 认证密码或访问令牌（仅在 HTTPS 远程仓库认证失败后用户提供，如 GitHub Personal Access Token）",
				},
				"remote_url": map[string]any{
					"type":        "string",
					"description": "新的远程仓库 HTTPS 地址（当 SSH 认证失败时，用户可提供 HTTPS 地址切换协议，如 https://github.com/用户名/仓库名.git）",
				},
			},
		},
	},
	{
		Name:        "pull_from_remote",
		Description: "从远程仓库拉取最新内容，获取团队的最新修改。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"remote": map[string]any{
					"type":        "string",
					"description": "远程仓库名称，默认origin",
				},
			},
		},
	},
	{
		Name:        "detect_conflict",
		Description: "检测仓库中是否存在文件冲突。",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		Name:        "resolve_conflict",
		Description: "解决文件冲突。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "冲突文件路径",
				},
				"strategy": map[string]any{
					"type":        "string",
					"description": "解决策略：ours（保留我的）、theirs（采用对方的）、merge（自动合并）",
				},
			},
			"required": []string{"file_path", "strategy"},
		},
	},
	{
		Name:        "update_user_info",
		Description: "设置或更新用户的姓名和邮箱，用于标识提交者身份。当用户告诉您他们的名字或邮箱时使用此工具。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "用户的姓名，将显示在提交记录中",
				},
				"email": map[string]any{
					"type":        "string",
					"description": "用户的邮箱地址，将显示在提交记录中",
				},
			},
			"required": []string{"name", "email"},
		},
	},
}

// GetGitAgentTools 返回 Agent 可用的工具列表（兼容旧接口）
// 已废弃：请使用 AllGitAgentTools + GitToolRegistry.BuildToolDefinitions()
func GetGitAgentTools() []Tool {
	result := make([]Tool, len(AllGitAgentTools))
	for i, t := range AllGitAgentTools {
		result[i] = Tool{
			Type: "function",
			Function: Function{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}
	return result
}

// 与后端 Go 结构体对应的类型定义。
// 字段命名与 JSON tag 保持一致（驼峰 / 下划线视具体 handler 而定）。

export interface WorkspaceInfo {
  id: string
  path: string
  openedAt: string
  lastUsedAt: string
  initialized: boolean
  llmEnabled: boolean
}

export interface RecentEntry {
  path: string
  openedAt: string
}

export interface VersionInfo {
  hash: string
  short_hash: string
  author: string
  email: string
  date: string
  message: string
}

export interface FileChange {
  path: string
  status: string
  insertions: number
  deletions: number
}

export interface AheadBehind {
  ahead: number
  behind: number
  remote: string
  branch: string
}

export interface StatusInfo {
  staged: FileChange[]
  unstaged: FileChange[]
  untracked: string[]
  is_clean: boolean
  latest_commit?: VersionInfo
  ahead_behind?: AheadBehind
}

export interface WorkspaceStatus {
  initialized: boolean
  status?: StatusInfo
}

export interface TreeNode {
  name: string
  path: string
  isDir: boolean
  size?: number
  hasMore?: boolean
}

export interface TreeResult {
  path: string
  nodes: TreeNode[]
  hasMore: boolean
}

export interface FileContent {
  path: string
  size: number
  binary: boolean
  content: string
  truncate: boolean
}

export interface BranchInfo {
  name: string
  is_current: boolean
  last_commit?: VersionInfo
}

// Agent SSE 事件；与 Go 端 AgentEvent 对齐
export type AgentEventKind =
  | 'thinking'
  | 'tool_call'
  | 'tool_result'
  | 'state_changed'
  | 'final'
  | 'workspace_updated'
  | 'error'

export interface AgentEvent {
  kind: AgentEventKind
  timestamp: string
  message?: string
  toolName?: string
  args?: string
  result?: string
  state?: string
  response?: AgentResponse
  extra?: Record<string, unknown>
}

export interface AgentResponse {
  success: boolean
  message: string
  details?: string
  state: string
  used_llm: boolean
  token_usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number }
  suggestions?: string[]
  timestamp: string
}

export interface BrowseDirEntry {
  name: string
  path: string
}

export interface BrowseResult {
  current: string
  parent: string
  dirs: BrowseDirEntry[]
}

export interface SettingsView {
  user: { name: string; email: string }
  llm: {
    enabled: boolean
    apiKeyMask: string
    hasApiKey: boolean
    baseUrl: string
    model: string
    maxTokens: number
  }
  http: { username: string; hasPassword: boolean; passwordMask: string }
}

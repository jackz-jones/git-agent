import type {
  AgentEvent,
  BranchInfo,
  BrowseResult,
  FileContent,
  RecentEntry,
  SettingsView,
  StatusInfo,
  TreeResult,
  VersionInfo,
  WorkspaceInfo,
  WorkspaceStatus,
} from './types'

// 统一错误外层：{ "error": { "code": "...", "message": "..." } }
export interface ApiErrorPayload {
  error?: { code: string; message: string }
}

export class ApiError extends Error {
  code: string
  status: number
  payload: unknown

  constructor(message: string, code: string, status: number, payload?: unknown) {
    super(message)
    this.code = code
    this.status = status
    this.payload = payload
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...((init.headers as Record<string, string>) || {}),
  }
  if (init.body && !(init.body instanceof FormData)) {
    headers['Content-Type'] = 'application/json'
  }
  const resp = await fetch(path, { ...init, headers })
  const text = await resp.text()
  let data: unknown = undefined
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      // 非 JSON，忽略
    }
  }
  if (!resp.ok) {
    const err = (data as ApiErrorPayload | undefined)?.error
    throw new ApiError(err?.message || resp.statusText, err?.code || 'http_error', resp.status, data)
  }
  return data as T
}

// --- Workspaces ---
export const listWorkspaces = () => request<WorkspaceInfo[]>('/api/workspaces')

export const openWorkspace = (path: string) =>
  request<{ workspace: WorkspaceInfo; reused: boolean }>('/api/workspaces', {
    method: 'POST',
    body: JSON.stringify({ path }),
  })

export const closeWorkspace = (id: string) =>
  request<{ ok: boolean }>(`/api/workspaces/${id}`, { method: 'DELETE' })

export const initWorkspace = (id: string) =>
  request<{ ok: boolean; gitignoreAdded?: boolean; needUserConfig?: boolean }>(
    `/api/workspaces/${id}/init`,
    { method: 'POST' },
  )

export const getWorkspace = (id: string) => request<WorkspaceInfo>(`/api/workspaces/${id}`)

// --- Status / Tree / File ---
export const getStatus = (id: string) => request<WorkspaceStatus>(`/api/workspaces/${id}/status`)

export const getTree = (id: string, path = '') =>
  request<TreeResult>(`/api/workspaces/${id}/tree?path=${encodeURIComponent(path)}`)

export const getFile = (id: string, path: string) =>
  request<FileContent>(`/api/workspaces/${id}/file?path=${encodeURIComponent(path)}`)

// --- Git ops ---
export const getLog = (id: string, opts: { limit?: number; author?: string; path?: string } = {}) => {
  const qs = new URLSearchParams()
  if (opts.limit) qs.set('limit', String(opts.limit))
  if (opts.author) qs.set('author', opts.author)
  if (opts.path) qs.set('path', opts.path)
  return request<{ versions: VersionInfo[] }>(`/api/workspaces/${id}/log?${qs.toString()}`)
}

export const getDiff = (id: string, opts: { path?: string; commit?: string } = {}) => {
  const qs = new URLSearchParams()
  if (opts.path) qs.set('path', opts.path)
  if (opts.commit) qs.set('commit', opts.commit)
  return request<{ diff: string }>(`/api/workspaces/${id}/diff?${qs.toString()}`)
}

export const commitChanges = (id: string, message: string, files?: string[]) =>
  request<{ hash: string; shortHash: string }>(`/api/workspaces/${id}/commit`, {
    method: 'POST',
    body: JSON.stringify({ message, files }),
  })

export const restoreVersion = (id: string, version: string, opts: { path?: string; force?: boolean } = {}) =>
  request<{ ok: boolean; requiresConfirm?: boolean }>(`/api/workspaces/${id}/restore`, {
    method: 'POST',
    body: JSON.stringify({ version, ...opts }),
  })

export const push = (
  id: string,
  opts: { remote?: string; username?: string; password?: string } = {},
) =>
  request<{ ok: boolean; needSync?: boolean }>(`/api/workspaces/${id}/push`, {
    method: 'POST',
    body: JSON.stringify(opts),
  })

// --- Branches ---
export const listBranches = (id: string) =>
  request<{ branches: BranchInfo[] }>(`/api/workspaces/${id}/branches`)

export const createBranch = (id: string, name: string) =>
  request<{ ok: boolean }>(`/api/workspaces/${id}/branches`, {
    method: 'POST',
    body: JSON.stringify({ name }),
  })

export const switchBranch = (id: string, name: string, force = false) =>
  request<{ ok: boolean; requiresConfirm?: boolean }>(
    `/api/workspaces/${id}/branches/switch`,
    {
      method: 'POST',
      body: JSON.stringify({ name, force }),
    },
  )

// --- Browse (目录选择器) ---
export const browseDirs = (path = '') =>
  request<BrowseResult>(`/api/browse?path=${encodeURIComponent(path)}`)

// --- Recent ---
export const listRecent = () => request<RecentEntry[]>('/api/recent')

// --- Settings ---
export const getSettings = () => request<SettingsView>('/api/settings')
export const updateSettings = (patch: unknown) =>
  request<SettingsView>('/api/settings', {
    method: 'POST',
    body: JSON.stringify(patch),
  })
export const testLLM = (patch: { apiKey?: string; baseUrl?: string; model?: string } = {}) =>
  request<{ ok: boolean; message: string; sample?: string }>('/api/settings/test-llm', {
    method: 'POST',
    body: JSON.stringify(patch),
  })

// --- Agent SSE ---
// 由于 EventSource 不支持 POST body，这里使用 fetch + ReadableStream 解析 SSE。
// onEvent 会在每次收到事件（thinking/tool_call/...）时被调用。
export async function streamAgent(
  id: string,
  input: string,
  onEvent: (ev: AgentEvent) => void,
  opts: { mode?: string; signal?: AbortSignal } = {},
): Promise<void> {
  const resp = await fetch(`/api/workspaces/${id}/agent`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
    },
    body: JSON.stringify({ input, mode: opts.mode || 'auto' }),
    signal: opts.signal,
  })
  if (!resp.ok || !resp.body) {
    const text = await resp.text().catch(() => '')
    throw new ApiError(text || resp.statusText, 'stream_failed', resp.status, text)
  }
  const reader = resp.body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buf = ''
  // eslint-disable-next-line no-constant-condition
  while (true) {
    const { value, done } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    // SSE 以 "\n\n" 分隔消息
    let idx: number
    while ((idx = buf.indexOf('\n\n')) !== -1) {
      const raw = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
      const dataLine = raw
        .split('\n')
        .find((l) => l.startsWith('data:'))
      if (!dataLine) continue
      const json = dataLine.slice('data:'.length).trim()
      try {
        const ev = JSON.parse(json) as AgentEvent
        onEvent(ev)
      } catch {
        // 忽略无法解析的数据块
      }
    }
  }
}

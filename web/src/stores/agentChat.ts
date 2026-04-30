import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'

import * as api from '@/api'
import type { AgentEvent } from '@/api/types'

// 前端聚合的"对话消息"结构。
export interface ChatMessage {
  id: string
  role: 'user' | 'agent' | 'system'
  content: string
  // Agent 回复可能附带的工具调用过程
  steps?: AgentStep[]
  success?: boolean
  suggestions?: string[]
  tokenUsage?: { prompt: number; completion: number; total: number }
  timestamp: string
}

export interface AgentStep {
  kind: 'thinking' | 'tool_call' | 'tool_result' | 'state_changed' | 'error'
  message?: string
  toolName?: string
  args?: string
  result?: string
  state?: string
  timestamp: string
}

/**
 * agentChat store
 * - 按 workspaceId 维护独立对话历史
 * - 提供 send / clear / 实时事件累积
 */
export const useAgentChatStore = defineStore('agentChat', () => {
  // key = workspaceId，value = 消息数组
  const byWorkspace = reactive<Record<string, ChatMessage[]>>({})
  const pending = ref<Record<string, boolean>>({})

  function list(wsId: string): ChatMessage[] {
    if (!byWorkspace[wsId]) byWorkspace[wsId] = []
    return byWorkspace[wsId]
  }

  function clear(wsId: string) {
    byWorkspace[wsId] = []
  }

  function nextId(): string {
    return Math.random().toString(36).slice(2, 10)
  }

  /**
   * 发送一条用户消息并通过 SSE 接收 Agent 回复。
   * onWorkspaceUpdated：当收到 workspace_updated 事件时触发（通常用于刷新文件树/状态）。
   */
  async function send(
    wsId: string,
    input: string,
    onWorkspaceUpdated?: () => void,
  ): Promise<void> {
    if (pending.value[wsId]) return

    pending.value[wsId] = true
    const history = list(wsId)
    history.push({
      id: nextId(),
      role: 'user',
      content: input,
      timestamp: new Date().toISOString(),
    })

    // 预插一条空的 agent 回复占位，收到事件逐步填充
    const agentMsg: ChatMessage = {
      id: nextId(),
      role: 'agent',
      content: '',
      steps: [],
      timestamp: new Date().toISOString(),
    }
    history.push(agentMsg)

    try {
      await api.streamAgent(wsId, input, (ev: AgentEvent) => {
        switch (ev.kind) {
          case 'thinking':
          case 'tool_call':
          case 'tool_result':
          case 'state_changed':
          case 'error':
            agentMsg.steps!.push({
              kind: ev.kind,
              message: ev.message,
              toolName: ev.toolName,
              args: ev.args,
              result: ev.result,
              state: ev.state,
              timestamp: ev.timestamp,
            })
            break
          case 'final':
            if (ev.response) {
              agentMsg.content = ev.response.message
              agentMsg.success = ev.response.success
              agentMsg.suggestions = ev.response.suggestions
              if (ev.response.token_usage) {
                agentMsg.tokenUsage = {
                  prompt: ev.response.token_usage.prompt_tokens,
                  completion: ev.response.token_usage.completion_tokens,
                  total: ev.response.token_usage.total_tokens,
                }
              }
            }
            break
          case 'workspace_updated':
            if (onWorkspaceUpdated) onWorkspaceUpdated()
            break
        }
      })
    } catch (err) {
      agentMsg.content = `对话失败：${(err as Error).message}`
      agentMsg.success = false
    } finally {
      pending.value[wsId] = false
    }
  }

  return { byWorkspace, pending, list, clear, send }
})

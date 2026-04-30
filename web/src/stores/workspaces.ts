import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import * as api from '@/api'
import type { WorkspaceInfo } from '@/api/types'

/**
 * workspaces store
 * - 维护已打开 workspace 列表
 * - 维护"当前选中 workspace"
 * - 提供打开 / 关闭 / 刷新 方法
 */
export const useWorkspacesStore = defineStore('workspaces', () => {
  const items = ref<WorkspaceInfo[]>([])
  const currentId = ref<string>('')

  const current = computed(() => items.value.find((w) => w.id === currentId.value) || null)

  async function refresh() {
    items.value = await api.listWorkspaces()
  }

  async function open(path: string): Promise<{ ws: WorkspaceInfo; reused: boolean }> {
    const { workspace, reused } = await api.openWorkspace(path)
    const idx = items.value.findIndex((w) => w.id === workspace.id)
    if (idx >= 0) items.value[idx] = workspace
    else items.value.push(workspace)
    currentId.value = workspace.id
    return { ws: workspace, reused }
  }

  async function close(id: string) {
    await api.closeWorkspace(id)
    items.value = items.value.filter((w) => w.id !== id)
    if (currentId.value === id) {
      currentId.value = items.value[0]?.id || ''
    }
  }

  function setCurrent(id: string) {
    currentId.value = id
  }

  async function initRepo(id: string) {
    const res = await api.initWorkspace(id)
    // 重新拉取一次该 workspace 的最新信息
    const updated = await api.getWorkspace(id)
    const idx = items.value.findIndex((w) => w.id === id)
    if (idx >= 0) items.value[idx] = updated
    return res
  }

  return { items, currentId, current, refresh, open, close, setCurrent, initRepo }
})

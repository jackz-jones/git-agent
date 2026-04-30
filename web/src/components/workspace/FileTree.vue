<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'

import * as api from '@/api'
import type { TreeNode } from '@/api/types'
import TreeNodeItem from './TreeNodeItem.vue'

const props = defineProps<{ wsId: string }>()

const emit = defineEmits<{ (e: 'open-file', path: string): void }>()

const children = ref<Record<string, TreeNode[]>>({})
const expanded = ref<Record<string, boolean>>({})
const loading = ref<Record<string, boolean>>({})

function onWorkspaceUpdated(ev: Event) {
  const detail = (ev as CustomEvent<{ wsId: string }>).detail
  if (!detail || detail.wsId === props.wsId) loadRoot()
}

onMounted(() => {
  loadRoot()
  window.addEventListener('workspace-updated', onWorkspaceUpdated)
})
onUnmounted(() => {
  window.removeEventListener('workspace-updated', onWorkspaceUpdated)
})
watch(() => props.wsId, loadRoot)

async function loadRoot() {
  children.value = {}
  expanded.value = { '': true }
  await loadDir('')
}

async function loadDir(path: string) {
  if (loading.value[path]) return
  loading.value[path] = true
  try {
    const res = await api.getTree(props.wsId, path)
    children.value[path] = res.nodes
  } catch (err) {
    ElMessage.error(`读取目录失败：${(err as Error).message}`)
  } finally {
    loading.value[path] = false
  }
}

async function toggle(node: TreeNode) {
  if (!node.isDir) {
    emit('open-file', node.path)
    return
  }
  expanded.value[node.path] = !expanded.value[node.path]
  if (expanded.value[node.path] && !children.value[node.path]) {
    await loadDir(node.path)
  }
}

defineExpose({ refresh: loadRoot })
</script>

<template>
  <div class="file-tree">
    <div class="tree-header">
      <span class="header-label">📁 文件浏览</span>
      <button class="refresh-btn" @click="loadRoot" title="刷新">↻</button>
    </div>

    <ul class="tree">
      <TreeNodeItem
        v-for="node in children[''] || []"
        :key="node.path"
        :node="node"
        :children-map="children"
        :expanded="expanded"
        @toggle="toggle"
      />
    </ul>
  </div>
</template>

<style scoped>
.file-tree {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--bg-sidebar);
}

.tree-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.header-label {
  font-weight: 600;
  font-size: 12px;
  color: #a6adc8;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.refresh-btn {
  border: none;
  background: rgba(255, 255, 255, 0.06);
  color: #a6adc8;
  width: 24px;
  height: 24px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s;
}

.refresh-btn:hover {
  background: rgba(255, 255, 255, 0.12);
  color: #cdd6f4;
}

.tree {
  list-style: none;
  margin: 0;
  padding: 6px 4px;
  overflow: auto;
  flex: 1;
  font-size: 13px;
}
</style>
<script setup lang="ts">
import { computed, type PropType } from 'vue'
import type { TreeNode } from '@/api/types'
import TreeNodeItem from './TreeNodeItem.vue'

const props = defineProps({
  node: { type: Object as PropType<TreeNode>, required: true },
  childrenMap: { type: Object as PropType<Record<string, TreeNode[]>>, required: true },
  expanded: { type: Object as PropType<Record<string, boolean>>, required: true },
})
const emit = defineEmits<{ (e: 'toggle', node: TreeNode): void }>()

// 根据文件扩展名返回不同颜色的图标
const fileIcon = computed(() => {
  if (props.node.isDir) {
    return props.expanded[props.node.path] ? '📂' : '📁'
  }
  const ext = props.node.name.split('.').pop()?.toLowerCase() || ''
  const iconMap: Record<string, string> = {
    go: '🔵', ts: '🟦', js: '🟨', vue: '🟩',
    py: '🐍', rs: '🦀', java: '☕', md: '📝',
    json: '📋', yaml: '⚙️', yml: '⚙️', toml: '⚙️',
    css: '🎨', html: '🌐', sh: '💻', mod: '📦',
    sum: '🔒', txt: '📄', gitignore: '🙈',
  }
  return iconMap[ext] || '📄'
})

function onRowClick(node: TreeNode) {
  emit('toggle', node)
}
function onChildToggle(n: TreeNode) {
  emit('toggle', n)
}
</script>

<template>
  <li class="tree-item">
    <div class="tree-row" @click="onRowClick(node)">
      <span v-if="node.isDir" class="tree-arrow">{{ expanded[node.path] ? '▾' : '▸' }}</span>
      <span v-else class="tree-arrow-placeholder" />
      <span class="tree-icon">{{ fileIcon }}</span>
      <span class="tree-name" :class="{ 'is-dir': node.isDir }">{{ node.name }}</span>
    </div>
    <ul v-if="node.isDir && expanded[node.path]" class="tree-sub">
      <TreeNodeItem
        v-for="c in childrenMap[node.path] || []"
        :key="c.path"
        :node="c"
        :children-map="childrenMap"
        :expanded="expanded"
        @toggle="onChildToggle"
      />
    </ul>
  </li>
</template>

<style scoped>
.tree-item { margin: 0; }

.tree-row {
  display: flex;
  gap: 4px;
  padding: 3px 8px;
  cursor: pointer;
  border-radius: 4px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  align-items: center;
  transition: background 0.12s;
}

.tree-row:hover {
  background: rgba(255, 255, 255, 0.06);
}

.tree-arrow {
  width: 14px;
  font-size: 10px;
  color: #6c7086;
  flex-shrink: 0;
  text-align: center;
}

.tree-arrow-placeholder {
  width: 14px;
  flex-shrink: 0;
}

.tree-icon {
  flex-shrink: 0;
  font-size: 13px;
}

.tree-name {
  overflow: hidden;
  text-overflow: ellipsis;
  color: #bac2de;
  font-size: 13px;
}

.tree-name.is-dir {
  color: #cdd6f4;
  font-weight: 500;
}

.tree-sub {
  list-style: none;
  margin: 0 0 0 12px;
  padding: 0;
}
</style>

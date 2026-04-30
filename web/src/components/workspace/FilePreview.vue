<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'

import * as api from '@/api'
import type { FileContent } from '@/api/types'

const props = defineProps<{ wsId: string; filePath: string }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const content = ref<FileContent | null>(null)
const loading = ref(false)

// 根据文件扩展名返回语言标签
function langLabel(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() || ''
  const map: Record<string, string> = {
    go: 'Go', ts: 'TypeScript', js: 'JavaScript', vue: 'Vue',
    py: 'Python', rs: 'Rust', java: 'Java', md: 'Markdown',
    json: 'JSON', yaml: 'YAML', yml: 'YAML', toml: 'TOML',
    css: 'CSS', html: 'HTML', sh: 'Shell', mod: 'Go Module',
    sum: 'Go Sum', txt: 'Text',
  }
  return map[ext] || ext.toUpperCase() || 'FILE'
}

async function loadFile() {
  if (!props.wsId || !props.filePath) return
  loading.value = true
  try {
    content.value = await api.getFile(props.wsId, props.filePath)
  } catch (err) {
    ElMessage.error(`读取文件失败：${(err as Error).message}`)
    content.value = null
  } finally {
    loading.value = false
  }
}

watch(() => props.filePath, loadFile, { immediate: true })
</script>

<template>
  <div class="file-preview" v-loading="loading" element-loading-background="rgba(30,30,46,0.8)">
    <!-- 头部 -->
    <div class="preview-header">
      <div class="header-left">
        <span class="file-icon">📄</span>
        <span class="file-name">{{ filePath.split('/').pop() }}</span>
        <span class="file-path">{{ filePath }}</span>
      </div>
      <div class="header-right">
        <span class="lang-badge">{{ langLabel(filePath) }}</span>
        <button class="close-btn" @click="emit('close')" title="关闭预览">✕</button>
      </div>
    </div>

    <!-- 内容区 -->
    <div v-if="content" class="preview-content">
      <template v-if="!content.binary">
        <div class="code-container">
          <div class="line-numbers">
            <span v-for="n in (content.content?.split('\n').length || 0)" :key="n">{{ n }}</span>
          </div>
          <pre class="code-body">{{ content.content }}</pre>
        </div>
      </template>
      <div v-else class="preview-placeholder">
        <span class="placeholder-icon">🚫</span>
        <span>二进制文件，不支持预览</span>
      </div>
      <div v-if="content.truncate" class="truncate-tip">⚠ 内容过长已截断显示</div>
    </div>
    <div v-else-if="!loading" class="preview-placeholder">
      <span class="placeholder-icon">📭</span>
      <span>无法加载文件内容</span>
    </div>
  </div>
</template>

<style scoped>
.file-preview {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: #1e1e2e;
  color: #cdd6f4;
}

.preview-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 16px;
  background: #181825;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  overflow: hidden;
  flex: 1;
}

.file-icon {
  font-size: 16px;
  flex-shrink: 0;
}

.file-name {
  font-weight: 600;
  font-size: 13px;
  color: #cdd6f4;
  white-space: nowrap;
}

.file-path {
  font-size: 11px;
  color: #6c7086;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.lang-badge {
  font-size: 11px;
  padding: 2px 8px;
  background: rgba(137, 180, 250, 0.12);
  color: #89b4fa;
  border-radius: 10px;
  font-weight: 500;
}

.close-btn {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: rgba(255, 255, 255, 0.06);
  color: #a6adc8;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  transition: all 0.15s;
}

.close-btn:hover {
  background: rgba(243, 139, 168, 0.2);
  color: #f38ba8;
}

.preview-content {
  flex: 1;
  overflow: auto;
}

.code-container {
  display: flex;
  min-height: 100%;
}

.line-numbers {
  display: flex;
  flex-direction: column;
  padding: 12px 0;
  min-width: 48px;
  text-align: right;
  padding-right: 12px;
  border-right: 1px solid rgba(255, 255, 255, 0.04);
  background: rgba(0, 0, 0, 0.15);
  user-select: none;
  position: sticky;
  left: 0;
}

.line-numbers span {
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.7;
  color: #45475a;
  padding: 0 8px;
}

.code-body {
  margin: 0;
  padding: 12px 16px;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.7;
  white-space: pre;
  color: #cdd6f4;
  flex: 1;
  overflow-x: auto;
}

.preview-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 12px;
  color: #6c7086;
  font-size: 14px;
}

.placeholder-icon {
  font-size: 32px;
}

.truncate-tip {
  padding: 8px 16px;
  background: rgba(249, 226, 175, 0.08);
  color: #f9e2af;
  font-size: 12px;
  border-top: 1px solid rgba(249, 226, 175, 0.15);
}
</style>

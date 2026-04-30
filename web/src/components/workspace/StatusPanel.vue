<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { WarnTriangleFilled } from '@element-plus/icons-vue'

import * as api from '@/api'
import type { FileChange, WorkspaceStatus } from '@/api/types'
import { useWorkspacesStore } from '@/stores/workspaces'

const props = defineProps<{ wsId: string }>()

const workspaces = useWorkspacesStore()
const status = ref<WorkspaceStatus | null>(null)
const loading = ref(false)
const initializing = ref(false)
const selected = ref<string[]>([]) // 待提交文件；空数组 = 全部
const diffFor = ref('') // 当前查看 diff 的文件
const diffText = ref('')

function onWorkspaceUpdated(ev: Event) {
  const detail = (ev as CustomEvent<{ wsId: string }>).detail
  if (!detail || detail.wsId === props.wsId) refresh()
}

onMounted(() => {
  refresh()
  window.addEventListener('workspace-updated', onWorkspaceUpdated)
})
onUnmounted(() => {
  window.removeEventListener('workspace-updated', onWorkspaceUpdated)
})
watch(() => props.wsId, refresh)

async function refresh() {
  if (!props.wsId) return // wsId 为空时跳过请求
  loading.value = true
  try {
    status.value = await api.getStatus(props.wsId)
  } catch (err) {
    ElMessage.error(`读取状态失败：${(err as Error).message}`)
  } finally {
    loading.value = false
  }
}

async function doInit() {
  initializing.value = true
  try {
    const res = await workspaces.initRepo(props.wsId)
    if (res.needUserConfig) {
      ElMessage.info('请先在设置页面填写姓名和邮箱，后续才能保存版本')
    }
    ElMessage.success('仓库初始化成功')
    await refresh()
  } catch (err) {
    ElMessage.error(`初始化失败：${(err as Error).message}`)
  } finally {
    initializing.value = false
  }
}

async function viewDiff(path: string) {
  diffFor.value = path
  try {
    const r = await api.getDiff(props.wsId, { path })
    diffText.value = r.diff || '（无差异）'
  } catch (err) {
    ElMessage.error(`读取差异失败：${(err as Error).message}`)
  }
}

async function commit() {
  if (!status.value?.status) return
  const allFiles: string[] = []
  ;(status.value.status.staged || []).forEach((f) => allFiles.push(f.path))
  ;(status.value.status.unstaged || []).forEach((f) => allFiles.push(f.path))
  ;(status.value.status.untracked || []).forEach((p) => allFiles.push(p))
  if (allFiles.length === 0) {
    ElMessage.info('没有可提交的变更')
    return
  }

  try {
    const { value } = await ElMessageBox.prompt('请输入提交描述', '保存版本', {
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputPattern: /.+/,
      inputErrorMessage: '描述不能为空',
    })
    const files = selected.value.length > 0 ? selected.value : undefined
    const res = await api.commitChanges(props.wsId, value, files)
    ElMessage.success(`已保存为版本 ${res.shortHash}`)
    selected.value = []
    diffFor.value = ''
    diffText.value = ''
    await refresh()
  } catch {
    // 用户取消
  }
}

function statusTag(status: string): string {
  switch (status) {
    case 'added': return 'A'
    case 'modified': return 'M'
    case 'deleted': return 'D'
    case 'renamed': return 'R'
    default: return '?'
  }
}

function tagType(s: string): 'success' | 'warning' | 'danger' | 'info' {
  if (s === 'added') return 'success'
  if (s === 'modified') return 'warning'
  if (s === 'deleted') return 'danger'
  return 'info'
}

function isSelected(path: string): boolean {
  return selected.value.includes(path)
}

function toggleFile(path: string) {
  const idx = selected.value.indexOf(path)
  if (idx >= 0) selected.value.splice(idx, 1)
  else selected.value.push(path)
}

function allChanges(): Array<{ path: string; status: string }> {
  const st = status.value?.status
  if (!st) return []
  const result: Array<{ path: string; status: string }> = []
  ;(st.staged || []).forEach((f: FileChange) => result.push({ path: f.path, status: f.status }))
  ;(st.unstaged || []).forEach((f: FileChange) => result.push({ path: f.path, status: f.status }))
  ;(st.untracked || []).forEach((p) => result.push({ path: p, status: 'untracked' }))
  return result
}
</script>

<template>
  <div v-loading="loading" class="status-panel">
    <!-- 加载中：status 尚未获取到 -->
    <template v-if="loading && !status">
      <!-- 由 v-loading 指令展示加载动画，无需额外内容 -->
    </template>

    <!-- 未初始化：明确从后端获取到 initialized=false -->
    <template v-else-if="status && !status.initialized">
      <div class="not-initialized">
        <el-icon class="warn-icon" :size="48"><WarnTriangleFilled /></el-icon>
        <h3 class="warn-title">当前目录尚未初始化为 Git 版本库</h3>
        <p class="warn-desc">该目录还没有进行版本管理，初始化后即可开始跟踪文件变更。</p>
        <el-button type="primary" size="large" :loading="initializing" @click="doInit">
          立即初始化
        </el-button>
        <p class="warn-hint">或通过底部对话面板让 Agent 帮你完成</p>
      </div>
    </template>

    <template v-else-if="status?.status">
      <div class="top-line">
        <div class="summary">
          <el-tag v-if="status.status.is_clean" type="success">工作区干净</el-tag>
          <el-tag v-else type="warning">有未保存的修改</el-tag>
          <span v-if="status.status.ahead_behind" class="muted">
            分支：{{ status.status.ahead_behind.branch }}
            <template v-if="status.status.ahead_behind.ahead > 0">
              · 领先 {{ status.status.ahead_behind.ahead }}
            </template>
            <template v-if="status.status.ahead_behind.behind > 0">
              · 落后 {{ status.status.ahead_behind.behind }}
            </template>
          </span>
        </div>
        <div class="actions">
          <el-button size="small" @click="refresh">刷新</el-button>
          <el-button size="small" type="primary" :disabled="status.status.is_clean" @click="commit">
            保存版本
          </el-button>
        </div>
      </div>

      <el-table v-if="!status.status.is_clean" :data="allChanges()" stripe size="small">
        <el-table-column label="" width="40">
          <template #default="{ row }">
            <el-checkbox :model-value="isSelected(row.path)" @change="toggleFile(row.path)" />
          </template>
        </el-table-column>
        <el-table-column label="状态" width="70">
          <template #default="{ row }">
            <el-tag size="small" :type="tagType(row.status)">{{ statusTag(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="文件路径" />
        <el-table-column label="操作" width="110" align="right">
          <template #default="{ row }">
            <el-button link size="small" @click="viewDiff(row.path)">查看差异</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="diffFor" class="diff-box">
        <div class="diff-title">差异：{{ diffFor }}</div>
        <pre class="diff-body">{{ diffText }}</pre>
      </div>
    </template>
  </div>
</template>

<style scoped>
.status-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: 200px;
}

.not-initialized {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  text-align: center;
  background: linear-gradient(135deg, #fff9e6 0%, #fff3cd 100%);
  border-radius: var(--radius-lg);
  border: 1px dashed var(--warning);
}

.warn-icon { color: var(--warning); margin-bottom: 16px; }
.warn-title { margin: 0 0 8px; font-size: 18px; color: var(--text-primary); }
.warn-desc { margin: 0 0 24px; color: var(--text-secondary); font-size: 14px; }
.warn-hint { margin-top: 12px; color: var(--text-muted); font-size: 12px; }

.top-line {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  background: var(--bg-sunken);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-light);
}

.summary { display: flex; gap: 10px; align-items: center; }
.muted { color: var(--text-muted); font-size: 12px; }
.actions { display: flex; gap: 8px; }

.diff-box {
  background: #1e1e2e;
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: var(--radius-md);
  padding: 12px 16px;
  max-height: 320px;
  overflow: auto;
}

.diff-title {
  font-weight: 600;
  font-size: 13px;
  margin-bottom: 8px;
  color: #cdd6f4;
}

.diff-body {
  margin: 0;
  font-family: var(--font-mono);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  color: #cdd6f4;
  line-height: 1.6;
}
</style>

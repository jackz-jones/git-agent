<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

import * as api from '@/api'
import type { VersionInfo } from '@/api/types'

const props = defineProps<{ wsId: string }>()

const versions = ref<VersionInfo[]>([])
const author = ref('')
const limit = ref(50)
const loading = ref(false)

const selected = ref<VersionInfo | null>(null)
const diffText = ref('')
const diffLoading = ref(false)

onMounted(refresh)
watch(() => props.wsId, refresh)

async function refresh() {
  loading.value = true
  try {
    const res = await api.getLog(props.wsId, {
      limit: limit.value,
      author: author.value || undefined,
    })
    versions.value = res.versions || []
  } catch (err) {
    ElMessage.error(`读取历史失败：${(err as Error).message}`)
  } finally {
    loading.value = false
  }
}

async function inspect(v: VersionInfo) {
  selected.value = v
  diffLoading.value = true
  try {
    const r = await api.getDiff(props.wsId, { commit: v.hash })
    diffText.value = r.diff || '（无差异）'
  } catch (err) {
    diffText.value = `读取差异失败：${(err as Error).message}`
  } finally {
    diffLoading.value = false
  }
}

async function restore(v: VersionInfo) {
  try {
    await ElMessageBox.confirm(
      `确认回滚到版本 ${v.short_hash}（${v.message.slice(0, 40)}）？此操作会覆盖当前工作区。`,
      '回滚确认',
      { type: 'warning', confirmButtonText: '回滚', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    const res = await api.restoreVersion(props.wsId, v.hash)
    if (res.requiresConfirm) {
      try {
        await ElMessageBox.confirm(
          '当前存在未提交修改，回滚将丢失这些修改，是否继续？',
          '二次确认',
          { type: 'warning', confirmButtonText: '强制回滚', cancelButtonText: '取消' },
        )
        await api.restoreVersion(props.wsId, v.hash, { force: true })
      } catch {
        return
      }
    }
    ElMessage.success('已回滚')
  } catch (err) {
    ElMessage.error(`回滚失败：${(err as Error).message}`)
  }
}
</script>

<template>
  <div v-loading="loading" class="history-panel">
    <div class="filter">
      <el-input v-model="author" placeholder="按作者筛选" style="width: 180px" clearable />
      <el-input-number v-model="limit" :min="10" :max="500" :step="10" />
      <el-button size="small" @click="refresh">刷新</el-button>
    </div>

    <el-table :data="versions" stripe size="small">
      <el-table-column prop="short_hash" label="版本" width="110" />
      <el-table-column prop="author" label="作者" width="160" />
      <el-table-column label="时间" width="170">
        <template #default="{ row }">
          {{ new Date(row.date).toLocaleString() }}
        </template>
      </el-table-column>
      <el-table-column prop="message" label="描述" show-overflow-tooltip />
      <el-table-column label="操作" width="180" align="right">
        <template #default="{ row }">
          <el-button link size="small" @click="inspect(row)">查看</el-button>
          <el-button link size="small" type="warning" @click="restore(row)">回滚到此</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div v-if="selected" class="diff-box" v-loading="diffLoading">
      <div class="diff-title">
        {{ selected.short_hash }} · {{ selected.author }} · {{ new Date(selected.date).toLocaleString() }}
      </div>
      <pre class="diff-body">{{ diffText }}</pre>
    </div>
  </div>
</template>

<style scoped>
.history-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  height: 100%;
}

.filter {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 12px 16px;
  background: var(--bg-sunken);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-light);
}

.diff-box {
  background: #1e1e2e;
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: var(--radius-md);
  padding: 12px 16px;
  max-height: 360px;
  overflow: auto;
}

.diff-title {
  font-weight: 600;
  margin-bottom: 8px;
  font-size: 13px;
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

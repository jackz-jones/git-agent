<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

import * as api from '@/api'
import type { BranchInfo } from '@/api/types'

const props = defineProps<{ wsId: string }>()

const branches = ref<BranchInfo[]>([])
const loading = ref(false)
const pushing = ref(false)

onMounted(refresh)
watch(() => props.wsId, refresh)

async function refresh() {
  loading.value = true
  try {
    const r = await api.listBranches(props.wsId)
    branches.value = r.branches || []
  } catch (err) {
    ElMessage.error(`读取分支失败：${(err as Error).message}`)
  } finally {
    loading.value = false
  }
}

async function switchTo(name: string, force = false) {
  try {
    const res = await api.switchBranch(props.wsId, name, force)
    if (res.requiresConfirm) {
      try {
        await ElMessageBox.confirm(
          '当前存在未提交修改，切换分支可能丢失这些修改，是否继续？',
          '二次确认',
          { type: 'warning', confirmButtonText: '强制切换', cancelButtonText: '取消' },
        )
        await switchTo(name, true)
      } catch {
        return
      }
      return
    }
    ElMessage.success(`已切换到 ${name}`)
    await refresh()
  } catch (err) {
    ElMessage.error(`切换失败：${(err as Error).message}`)
  }
}

async function create() {
  try {
    const { value } = await ElMessageBox.prompt('请输入新分支名', '新建分支', {
      confirmButtonText: '创建',
      cancelButtonText: '取消',
      inputPattern: /^[\w./-]+$/,
      inputErrorMessage: '只允许字母数字以及 . / _ -',
    })
    await api.createBranch(props.wsId, value)
    ElMessage.success(`已创建分支 ${value}`)
    await refresh()
  } catch {
    // 取消
  }
}

async function push() {
  pushing.value = true
  try {
    const res = await api.push(props.wsId, { remote: 'origin' })
    if (res.ok) {
      ElMessage.success('已推送到远程')
    } else if (res.needSync) {
      ElMessage.warning('推送失败：需要先同步远程更新')
    }
  } catch (err) {
    const msg = (err as Error).message
    const needSync = /reject|fetch first|non[- ]fast/i.test(msg)
    if (needSync) {
      ElMessage.warning(`推送失败（${msg}）。请在"同步远程"后重试。`)
    } else {
      ElMessage.error(`推送失败：${msg}`)
    }
  } finally {
    pushing.value = false
  }
}
</script>

<template>
  <div v-loading="loading" class="branches-panel">
    <div class="toolbar">
      <el-button size="small" @click="refresh">刷新</el-button>
      <el-button size="small" type="primary" @click="create">新建分支</el-button>
      <el-button size="small" :loading="pushing" @click="push">推送到远程</el-button>
    </div>

    <el-table :data="branches" stripe size="small">
      <el-table-column label="" width="60">
        <template #default="{ row }">
          <el-tag v-if="row.is_current" size="small" type="success">当前</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="name" label="分支名" />
      <el-table-column label="最新提交" show-overflow-tooltip>
        <template #default="{ row }">
          <span v-if="row.last_commit">
            {{ row.last_commit.short_hash }} · {{ row.last_commit.message }}
          </span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="120" align="right">
        <template #default="{ row }">
          <el-button
            link
            size="small"
            :disabled="row.is_current"
            @click="switchTo(row.name)"
          >切换</el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<style scoped>
.branches-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.toolbar {
  display: flex;
  gap: 8px;
  padding: 12px 16px;
  background: var(--bg-sunken);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-light);
}
</style>

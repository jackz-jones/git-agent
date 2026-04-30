<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { FolderOpened } from '@element-plus/icons-vue'

import * as api from '@/api'
import type { RecentEntry } from '@/api/types'
import type { BrowseDirEntry } from '@/api/types'
import { useWorkspacesStore } from '@/stores/workspaces'

const router = useRouter()
const workspaces = useWorkspacesStore()

const pathInput = ref('')
const recent = ref<RecentEntry[]>([])
const loading = ref(false)

// 目录浏览对话框状态
const browseVisible = ref(false)
const browseCurrent = ref('')
const browseParent = ref('')
const browseDirList = ref<BrowseDirEntry[]>([])
const browseLoading = ref(false)

onMounted(async () => {
  await refreshRecent()
  await workspaces.refresh().catch(() => {})
})

async function refreshRecent() {
  recent.value = await api.listRecent().catch(() => [])
}

async function openPath(path: string) {
  if (!path.trim()) {
    ElMessage.warning('请输入有效的本地目录路径')
    return
  }
  loading.value = true
  try {
    const { ws } = await workspaces.open(path.trim())

    if (!ws.initialized) {
      await handleInit(ws.id, ws.path)
    } else {
      router.push({ name: 'workspace', params: { id: ws.id } })
    }
    await refreshRecent()
  } catch (err) {
    ElMessage.error(`打开失败：${(err as Error).message}`)
  } finally {
    loading.value = false
  }
}

async function handleInit(id: string, path: string) {
  try {
    await ElMessageBox.confirm(
      `目录 ${path} 还不是版本库，是否立即初始化？`,
      '初始化仓库',
      {
        confirmButtonText: '初始化',
        cancelButtonText: '取消',
        type: 'info',
      },
    )
  } catch {
    await workspaces.close(id)
    return
  }

  try {
    const res = await workspaces.initRepo(id)
    if (res.needUserConfig) {
      ElMessage.info('请先在设置页面填写姓名和邮箱，后续才能保存版本')
    }
    router.push({ name: 'workspace', params: { id } })
  } catch (err) {
    ElMessage.error(`初始化失败：${(err as Error).message}`)
    await workspaces.close(id).catch(() => {})
  }
}

// ---- 目录浏览对话框 ----
async function openBrowseDialog() {
  browseVisible.value = true
  // 默认从用户主目录开始浏览，如果输入框已有路径则从该路径开始
  const startPath = pathInput.value.trim() || ''
  await loadBrowseDirs(startPath)
}

async function loadBrowseDirs(path: string) {
  browseLoading.value = true
  try {
    const res = await api.browseDirs(path)
    browseCurrent.value = res.current
    browseParent.value = res.parent
    browseDirList.value = res.dirs
  } catch (err) {
    ElMessage.error(`浏览目录失败：${(err as Error).message}`)
  } finally {
    browseLoading.value = false
  }
}

async function navigateTo(dir: BrowseDirEntry) {
  await loadBrowseDirs(dir.path)
}

async function navigateUp() {
  if (browseParent.value) {
    await loadBrowseDirs(browseParent.value)
  }
}

function selectCurrentDir() {
  if (!browseCurrent.value) {
    ElMessage.warning('请先进入一个目录')
    return
  }
  pathInput.value = browseCurrent.value
  browseVisible.value = false
}
</script>

<template>
  <div class="home">
    <header class="header">
      <div class="title">Git Agent</div>
      <el-button link style="color: var(--text-muted)" @click="router.push({ name: 'settings' })">⚙ 设置</el-button>
    </header>

    <main class="main">
      <section class="card">
        <h2>打开工作目录</h2>
        <p class="muted">点击"浏览"从文件系统中选择目录，或直接输入绝对路径</p>
        <div class="row">
          <el-input
            v-model="pathInput"
            placeholder="/Users/you/projects/my-doc"
            clearable
            @keyup.enter="openPath(pathInput)"
          />
          <el-button :icon="FolderOpened" @click="openBrowseDialog">
            浏览
          </el-button>
          <el-button type="primary" :loading="loading" @click="openPath(pathInput)">
            打开
          </el-button>
        </div>
      </section>

      <section class="card">
        <h2>最近打开</h2>
        <el-empty v-if="recent.length === 0" description="暂无历史记录" :image-size="80" />
        <el-table v-else :data="recent" stripe>
          <el-table-column prop="path" label="路径" />
          <el-table-column prop="openedAt" label="上次打开" width="220">
            <template #default="{ row }">
              <span>{{ new Date(row.openedAt).toLocaleString() }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="100" align="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="openPath(row.path)">打开</el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>

      <section v-if="workspaces.items.length" class="card">
        <h2>已打开的工作区</h2>
        <el-table :data="workspaces.items" stripe>
          <el-table-column prop="path" label="路径" />
          <el-table-column label="状态" width="120">
            <template #default="{ row }">
              <el-tag v-if="row.initialized" type="success" size="small">已初始化</el-tag>
              <el-tag v-else type="warning" size="small">未初始化</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="180" align="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="router.push({ name: 'workspace', params: { id: row.id } })">
                进入
              </el-button>
              <el-button link type="danger" @click="workspaces.close(row.id)">关闭</el-button>
            </template>
          </el-table-column>
        </el-table>
      </section>
    </main>

    <!-- 目录浏览对话框 -->
    <el-dialog
      v-model="browseVisible"
      title="选择目录"
      width="600px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <div class="browse-dialog">
        <!-- 当前路径 & 返回上级 -->
        <div class="browse-toolbar">
          <el-button
            :disabled="!browseParent"
            size="small"
            @click="navigateUp"
          >
            ⬆ 上级目录
          </el-button>
          <span class="browse-current-path">{{ browseCurrent || '/' }}</span>
        </div>

        <!-- 目录列表 -->
        <div v-loading="browseLoading" class="browse-list">
          <div
            v-for="dir in browseDirList"
            :key="dir.path"
            class="browse-item"
            @dblclick="navigateTo(dir)"
            @click="navigateTo(dir)"
          >
            <span class="browse-icon">📁</span>
            <span class="browse-name">{{ dir.name }}</span>
          </div>
          <el-empty
            v-if="!browseLoading && browseDirList.length === 0"
            description="该目录下没有子目录"
            :image-size="60"
          />
        </div>
      </div>

      <template #footer>
        <el-button @click="browseVisible = false">取消</el-button>
        <el-button type="primary" :disabled="!browseCurrent" @click="selectCurrentDir">
          选择当前目录
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.home {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-base);
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 24px;
  background: var(--bg-topbar);
  min-height: 48px;
}

.title {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-inverse);
  display: flex;
  align-items: center;
  gap: 8px;
}

.title::before {
  content: '⚡';
  font-size: 20px;
}

.main {
  flex: 1;
  overflow: auto;
  padding: 32px 24px;
  max-width: 960px;
  margin: 0 auto;
  width: 100%;
  box-sizing: border-box;
}

.card {
  background: var(--bg-elevated);
  padding: 24px 28px;
  margin-bottom: 24px;
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-md);
  border: 1px solid var(--border-light);
}

.card h2 {
  margin: 0 0 8px;
  font-size: 16px;
  color: var(--text-primary);
}

.muted {
  color: var(--text-muted);
  margin: 0 0 16px;
  font-size: 13px;
}

.row {
  display: flex;
  gap: 8px;
  align-items: center;
}

.browse-dialog {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.browse-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--border);
}

.browse-current-path {
  font-size: 13px;
  color: var(--text-secondary);
  word-break: break-all;
  flex: 1;
  font-family: var(--font-mono);
}

.browse-list {
  max-height: 400px;
  min-height: 200px;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 4px;
  background: var(--bg-sunken);
}

.browse-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 14px;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: all 0.15s;
}

.browse-item:hover {
  background: var(--primary-bg);
}

.browse-icon {
  font-size: 16px;
  flex-shrink: 0;
}

.browse-name {
  font-size: 14px;
  word-break: break-all;
  color: var(--text-primary);
}
</style>

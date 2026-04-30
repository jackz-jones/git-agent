<script setup lang="ts">
// Workspace 视图骨架：顶部切换栏 + 左侧文件树 + 右侧功能 Tab + 底部对话面板。
// 文件预览使用右侧 Drawer 抽屉展示，不占用功能面板空间。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Setting, FolderAdd } from '@element-plus/icons-vue'

import { useWorkspacesStore } from '@/stores/workspaces'
import StatusPanel from '@/components/workspace/StatusPanel.vue'
import HistoryPanel from '@/components/workspace/HistoryPanel.vue'
import BranchesPanel from '@/components/workspace/BranchesPanel.vue'
import FileTree from '@/components/workspace/FileTree.vue'
import FilePreview from '@/components/workspace/FilePreview.vue'
import ChatPanel from '@/components/workspace/ChatPanel.vue'

const route = useRoute()
const router = useRouter()
const workspaces = useWorkspacesStore()

const activeTab = ref<'status' | 'history' | 'branches'>('status')
const wsId = computed(() => String(route.params.id || ''))
const ready = ref(false)
const previewFile = ref('')

function onOpenFile(path: string) {
  previewFile.value = path
}

function closePreview() {
  previewFile.value = ''
}

async function ensureLoaded() {
  ready.value = false
  if (!workspaces.items.length) {
    await workspaces.refresh().catch(() => {})
  }
  if (wsId.value && !workspaces.items.find((w) => w.id === wsId.value)) {
    ElMessage.warning('工作区不存在或已关闭')
    router.push({ name: 'home' })
    return
  }
  workspaces.setCurrent(wsId.value)
  ready.value = true
}

onMounted(ensureLoaded)
watch(() => route.params.id, ensureLoaded)

async function closeWs(id: string) {
  await workspaces.close(id)
  if (workspaces.items.length === 0) {
    router.push({ name: 'home' })
  } else {
    router.push({ name: 'workspace', params: { id: workspaces.items[0].id } })
  }
}
</script>

<template>
  <div class="workspace-page">
    <!-- 顶部 Workspace 切换栏 -->
    <header class="top-bar">
      <div class="tabs">
        <div
          v-for="w in workspaces.items"
          :key="w.id"
          class="tab"
          :class="{ active: w.id === wsId }"
          @click="router.push({ name: 'workspace', params: { id: w.id } })"
        >
          <span class="tab-dot" />
          <span class="path">{{ w.path.split('/').pop() || w.path }}</span>
          <span class="close" @click.stop="closeWs(w.id)">×</span>
        </div>
        <el-button class="add-btn" link @click="router.push({ name: 'home' })">
          <el-icon><FolderAdd /></el-icon>
          <span>打开目录</span>
        </el-button>
      </div>
      <el-button class="settings-btn" link @click="router.push({ name: 'settings' })">
        <el-icon><Setting /></el-icon>
      </el-button>
    </header>

    <!-- 主体：左文件树 / 右功能面板 -->
    <template v-if="ready && wsId">
      <div class="body">
        <aside class="sidebar">
          <FileTree :ws-id="wsId" @open-file="onOpenFile" />
        </aside>
        <main class="main">
          <div class="tab-nav">
            <div
              class="tab-nav-item"
              :class="{ active: activeTab === 'status' }"
              @click="activeTab = 'status'"
            >
              <span class="tab-icon">◉</span> 状态
            </div>
            <div
              class="tab-nav-item"
              :class="{ active: activeTab === 'history' }"
              @click="activeTab = 'history'"
            >
              <span class="tab-icon">⏱</span> 历史记录
            </div>
            <div
              class="tab-nav-item"
              :class="{ active: activeTab === 'branches' }"
              @click="activeTab = 'branches'"
            >
              <span class="tab-icon">⑂</span> 分支
            </div>
          </div>
          <div class="tab-body">
            <StatusPanel v-show="activeTab === 'status'" :ws-id="wsId" />
            <HistoryPanel v-show="activeTab === 'history'" :ws-id="wsId" />
            <BranchesPanel v-show="activeTab === 'branches'" :ws-id="wsId" />
          </div>
        </main>
      </div>

      <!-- 文件预览 Drawer 抽屉 -->
      <Transition name="drawer">
        <div v-if="previewFile" class="preview-drawer">
          <FilePreview
            :ws-id="wsId"
            :file-path="previewFile"
            @close="closePreview"
          />
        </div>
      </Transition>

      <!-- Agent 对话面板（右侧抽屉） -->
      <ChatPanel :ws-id="wsId" />
    </template>

    <!-- 加载中占位 -->
    <div v-else class="loading-placeholder" v-loading="true" />
  </div>
</template>

<style scoped>
.workspace-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-base);
  position: relative;
  overflow: hidden;
}

/* ===== 顶部栏 ===== */
.top-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 12px;
  background: var(--bg-topbar);
  min-height: 42px;
  box-shadow: var(--shadow-sm);
  z-index: 10;
}

.tabs {
  display: flex;
  align-items: center;
  gap: 2px;
  overflow-x: auto;
}

.tab {
  display: flex;
  align-items: center;
  padding: 6px 14px;
  background: rgba(255, 255, 255, 0.06);
  border-radius: var(--radius-sm) var(--radius-sm) 0 0;
  cursor: pointer;
  font-size: 13px;
  gap: 8px;
  color: var(--text-muted);
  transition: all 0.2s;
  position: relative;
}

.tab:hover {
  background: rgba(255, 255, 255, 0.1);
  color: var(--text-inverse);
}

.tab.active {
  background: var(--bg-elevated);
  color: var(--text-primary);
}

.tab-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--success);
  flex-shrink: 0;
}

.tab .close {
  font-size: 16px;
  line-height: 1;
  opacity: 0;
  transition: opacity 0.15s;
  cursor: pointer;
  padding: 0 2px;
  border-radius: 2px;
}

.tab:hover .close { opacity: 0.7; }
.tab .close:hover { opacity: 1; background: rgba(0,0,0,0.1); }

.add-btn {
  color: var(--text-muted) !important;
  font-size: 12px;
  margin-left: 8px;
}
.add-btn:hover { color: var(--text-inverse) !important; }

.settings-btn {
  color: var(--text-muted) !important;
  font-size: 18px;
}
.settings-btn:hover { color: var(--text-inverse) !important; }

/* ===== 主体 ===== */
.body {
  flex: 1;
  display: flex;
  overflow: hidden;
}

/* ===== 侧边栏 ===== */
.sidebar {
  width: 260px;
  background: var(--bg-sidebar);
  overflow: auto;
  border-right: 1px solid rgba(255, 255, 255, 0.06);
}

/* ===== 右侧主面板 ===== */
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--bg-elevated);
}

.tab-nav {
  display: flex;
  gap: 0;
  padding: 0 16px;
  background: var(--bg-sunken);
  border-bottom: 1px solid var(--border);
}

.tab-nav-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 16px;
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all 0.2s;
}

.tab-nav-item:hover {
  color: var(--primary);
  background: rgba(91, 106, 191, 0.04);
}

.tab-nav-item.active {
  color: var(--primary);
  border-bottom-color: var(--primary);
  background: var(--bg-elevated);
}

.tab-icon {
  font-size: 14px;
}

.tab-body {
  flex: 1;
  overflow: auto;
  padding: 20px;
}

/* ===== 文件预览 Drawer ===== */
.preview-drawer {
  position: absolute;
  top: 42px; /* 顶部栏高度 */
  right: 0;
  bottom: 0;
  width: 55%;
  max-width: 800px;
  min-width: 400px;
  background: #1e1e2e;
  box-shadow: -4px 0 24px rgba(0, 0, 0, 0.2);
  z-index: 100;
  border-left: 1px solid rgba(255, 255, 255, 0.08);
}

/* Drawer 动画 */
.drawer-enter-active,
.drawer-leave-active {
  transition: transform 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}
.drawer-enter-from,
.drawer-leave-to {
  transform: translateX(100%);
}

/* ===== 加载占位 ===== */
.loading-placeholder {
  flex: 1;
  min-height: 300px;
}
</style>

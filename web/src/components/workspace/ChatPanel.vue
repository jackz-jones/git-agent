<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { ChatDotRound, Close, Delete, ArrowRight, ArrowDown } from '@element-plus/icons-vue'

import { useAgentChatStore } from '@/stores/agentChat'

const props = defineProps<{ wsId: string }>()

const chat = useAgentChatStore()
const input = ref('')
const expanded = ref(false)
const scrollRef = ref<HTMLDivElement | null>(null)
const panelWidth = ref(420)
const isResizing = ref(false)
// 记录每条消息的思考步骤是否展开
const expandedSteps = ref<Record<string, boolean>>({})

const messages = computed(() => chat.list(props.wsId))
const pending = computed(() => !!chat.pending[props.wsId])

function toggleSteps(msgId: string) {
  expandedSteps.value[msgId] = !expandedSteps.value[msgId]
}

function isStepsExpanded(msgId: string): boolean {
  return !!expandedSteps.value[msgId]
}

async function send() {
  const text = input.value.trim()
  if (!text) return
  input.value = ''
  try {
    await chat.send(props.wsId, text, () => {
      window.dispatchEvent(new CustomEvent('workspace-updated', { detail: { wsId: props.wsId } }))
    })
  } catch (err) {
    ElMessage.error(`对话失败：${(err as Error).message}`)
  } finally {
    await nextTick()
    scrollRef.value?.scrollTo({ top: scrollRef.value.scrollHeight })
  }
}

function clearChat() {
  chat.clear(props.wsId)
  expandedSteps.value = {}
}

function togglePanel() {
  expanded.value = !expanded.value
}

watch(
  () => messages.value.length,
  async () => {
    await nextTick()
    scrollRef.value?.scrollTo({ top: scrollRef.value.scrollHeight })
  },
)

// 拖拽调整宽度
function startResize(e: MouseEvent) {
  isResizing.value = true
  const startX = e.clientX
  const startWidth = panelWidth.value

  const onMouseMove = (ev: MouseEvent) => {
    const delta = startX - ev.clientX
    panelWidth.value = Math.max(320, Math.min(800, startWidth + delta))
  }
  const onMouseUp = () => {
    isResizing.value = false
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
  }
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

function stepLabel(kind: string): string {
  switch (kind) {
    case 'thinking': return '💭 思考'
    case 'tool_call': return '🔧 调用工具'
    case 'tool_result': return '📋 工具结果'
    case 'state_changed': return '🔄 状态变更'
    case 'error': return '❌ 错误'
    default: return kind
  }
}

function stepIcon(kind: string): string {
  switch (kind) {
    case 'thinking': return '💭'
    case 'tool_call': return '🔧'
    case 'tool_result': return '📋'
    case 'state_changed': return '🔄'
    case 'error': return '❌'
    default: return '📌'
  }
}
</script>

<template>
  <!-- 悬浮触发按钮 -->
  <Transition name="fab">
    <button
      v-if="!expanded"
      class="chat-fab"
      @click="togglePanel"
      title="与 Agent 对话"
    >
      <el-icon :size="22"><ChatDotRound /></el-icon>
      <span v-if="messages.length" class="fab-badge">{{ messages.length }}</span>
    </button>
  </Transition>

  <!-- 右侧对话面板 -->
  <Transition name="slide-right">
    <section
      v-if="expanded"
      class="chat-drawer"
      :style="{ width: panelWidth + 'px' }"
    >
      <!-- 拖拽调整宽度的手柄 -->
      <div class="resize-handle" @mousedown="startResize" />

      <!-- 头部 -->
      <header class="chat-header">
        <div class="header-left">
          <el-icon :size="18"><ChatDotRound /></el-icon>
          <span class="header-title">Agent 助手</span>
        </div>
        <div class="header-actions">
          <button class="header-btn" @click="clearChat" title="清空对话">
            <el-icon :size="16"><Delete /></el-icon>
          </button>
          <button class="header-btn" @click="togglePanel" title="关闭">
            <el-icon :size="16"><Close /></el-icon>
          </button>
        </div>
      </header>

      <!-- 消息区域 -->
      <div ref="scrollRef" class="chat-messages">
        <div v-if="messages.length === 0" class="empty-state">
          <div class="empty-icon">🤖</div>
          <div class="empty-title">你好，我是 Git Agent</div>
          <div class="empty-desc">试试对我说：</div>
          <div class="quick-actions">
            <button class="quick-btn" @click="input = '查看当前状态'; send()">📊 查看当前状态</button>
            <button class="quick-btn" @click="input = '保存修改'; send()">💾 保存修改</button>
            <button class="quick-btn" @click="input = '查看最近的改动'; send()">🔍 查看最近改动</button>
            <button class="quick-btn" @click="input = '新建一个 feat 分支'; send()">🌿 新建分支</button>
          </div>
        </div>

        <div v-for="m in messages" :key="m.id" class="msg" :class="m.role">
          <!-- 用户消息 -->
          <div v-if="m.role === 'user'" class="user-msg">
            <div class="user-bubble">{{ m.content }}</div>
          </div>

          <!-- Agent 消息（跳过空占位消息，避免与 loading 指示器重复显示头像） -->
          <div v-else-if="m.content || (m.steps && m.steps.length) || m.tokenUsage" class="agent-msg">
            <div class="agent-avatar">🤖</div>
            <div class="agent-body">
              <!-- 思考过程（可折叠） -->
              <div v-if="m.steps && m.steps.length" class="thinking-section">
                <button class="thinking-toggle" @click="toggleSteps(m.id)">
                  <el-icon :size="12">
                    <ArrowRight v-if="!isStepsExpanded(m.id)" />
                    <ArrowDown v-else />
                  </el-icon>
                  <span class="thinking-label">推理过程</span>
                  <span class="thinking-count">{{ m.steps.length }} 步</span>
                </button>

                <Transition name="collapse">
                  <div v-if="isStepsExpanded(m.id)" class="steps-detail">
                    <div v-for="(s, i) in m.steps" :key="i" class="step-item">
                      <span class="step-icon">{{ stepIcon(s.kind) }}</span>
                      <div class="step-content">
                        <div class="step-header">
                          <span class="step-kind" :class="s.kind">{{ stepLabel(s.kind) }}</span>
                          <span v-if="s.toolName" class="step-tool">{{ s.toolName }}</span>
                        </div>
                        <div v-if="s.message" class="step-text">{{ s.message }}</div>
                        <pre v-if="s.args" class="step-code">{{ s.args }}</pre>
                        <pre v-if="s.result" class="step-code result">{{ s.result }}</pre>
                      </div>
                    </div>
                  </div>
                </Transition>
              </div>

              <!-- 最终回复内容 -->
              <div v-if="m.content" class="agent-content" :class="{ success: m.success, error: m.success === false }">
                {{ m.content }}
              </div>

              <!-- 建议操作 -->
              <div v-if="m.suggestions && m.suggestions.length" class="suggestions">
                <button
                  v-for="s in m.suggestions"
                  :key="s"
                  class="suggestion-btn"
                  @click="input = s; send()"
                >
                  {{ s }}
                </button>
              </div>

              <!-- Token 用量 -->
              <div v-if="m.tokenUsage" class="token-info">
                🔢 tokens：输入 {{ m.tokenUsage.prompt }} · 输出 {{ m.tokenUsage.completion }} · 合计 {{ m.tokenUsage.total }}
              </div>
            </div>
          </div>
        </div>

        <!-- 加载指示器 -->
        <div v-if="pending" class="loading-indicator">
          <div class="agent-avatar">🤖</div>
          <div class="typing-dots">
            <span></span><span></span><span></span>
          </div>
        </div>
      </div>

      <!-- 输入区域 -->
      <div class="chat-input">
        <div class="input-wrapper">
          <el-input
            v-model="input"
            type="textarea"
            :autosize="{ minRows: 1, maxRows: 4 }"
            placeholder="输入指令，如：保存修改、查看历史..."
            :disabled="pending"
            @keydown.enter.exact.prevent="send"
            resize="none"
          />
          <button
            class="send-btn"
            :class="{ active: input.trim() }"
            :disabled="pending || !input.trim()"
            @click="send"
          >
            <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
              <path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/>
            </svg>
          </button>
        </div>
      </div>
    </section>
  </Transition>
</template>

<style scoped>
/* ===== 悬浮按钮 ===== */
.chat-fab {
  position: fixed;
  bottom: 24px;
  right: 24px;
  width: 52px;
  height: 52px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--primary) 0%, var(--primary-dark) 100%);
  color: white;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 16px rgba(91, 106, 191, 0.4);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  z-index: 1000;
}

.chat-fab:hover {
  transform: scale(1.08);
  box-shadow: 0 6px 24px rgba(91, 106, 191, 0.5);
}

.fab-badge {
  position: absolute;
  top: -4px;
  right: -4px;
  min-width: 20px;
  height: 20px;
  border-radius: 10px;
  background: var(--danger);
  color: white;
  font-size: 11px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 5px;
}

/* ===== 右侧抽屉面板 ===== */
.chat-drawer {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  display: flex;
  flex-direction: column;
  background: #fafbfc;
  box-shadow: -4px 0 24px rgba(0, 0, 0, 0.12);
  z-index: 1000;
  border-left: 1px solid var(--border);
}

/* 拖拽手柄 */
.resize-handle {
  position: absolute;
  left: -3px;
  top: 0;
  bottom: 0;
  width: 6px;
  cursor: col-resize;
  z-index: 10;
  transition: background 0.2s;
}

.resize-handle:hover,
.resize-handle:active {
  background: var(--primary);
  opacity: 0.3;
}

/* ===== 头部 ===== */
.chat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: linear-gradient(135deg, var(--primary) 0%, var(--primary-dark) 100%);
  color: white;
  flex-shrink: 0;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-title {
  font-weight: 600;
  font-size: 15px;
}

.header-actions {
  display: flex;
  gap: 4px;
}

.header-btn {
  width: 30px;
  height: 30px;
  border-radius: 6px;
  border: none;
  background: rgba(255, 255, 255, 0.15);
  color: rgba(255, 255, 255, 0.9);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.header-btn:hover {
  background: rgba(255, 255, 255, 0.25);
  color: white;
}

/* ===== 消息区域 ===== */
.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 空状态 */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  padding: 32px 16px;
  text-align: center;
}

.empty-icon {
  font-size: 48px;
  margin-bottom: 12px;
}

.empty-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 4px;
}

.empty-desc {
  font-size: 13px;
  color: var(--text-muted);
  margin-bottom: 16px;
}

.quick-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  max-width: 260px;
}

.quick-btn {
  padding: 10px 16px;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: white;
  color: var(--text-primary);
  font-size: 13px;
  cursor: pointer;
  text-align: left;
  transition: all 0.2s;
}

.quick-btn:hover {
  border-color: var(--primary);
  background: var(--primary-bg);
  color: var(--primary);
}

/* 用户消息 */
.user-msg {
  display: flex;
  justify-content: flex-end;
}

.user-bubble {
  max-width: 85%;
  padding: 10px 14px;
  background: linear-gradient(135deg, var(--primary) 0%, var(--primary-dark) 100%);
  color: white;
  border-radius: 16px 16px 4px 16px;
  font-size: 13px;
  line-height: 1.6;
  word-break: break-word;
}

/* Agent 消息 */
.agent-msg {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.agent-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: var(--primary-bg);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  flex-shrink: 0;
}

.agent-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/* 思考过程折叠区 */
.thinking-section {
  border-radius: var(--radius-md);
  overflow: hidden;
}

.thinking-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: #f5f6f8;
  color: var(--text-secondary);
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
  width: auto;
}

.thinking-toggle:hover {
  background: #eef0f4;
  border-color: var(--primary);
  color: var(--primary);
}

.thinking-label {
  font-weight: 500;
}

.thinking-count {
  color: var(--text-muted);
  font-size: 11px;
}

.steps-detail {
  margin-top: 8px;
  padding: 10px;
  background: #f8f9fb;
  border: 1px solid var(--border-light);
  border-radius: var(--radius-md);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.step-item {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}

.step-icon {
  font-size: 14px;
  flex-shrink: 0;
  margin-top: 1px;
}

.step-content {
  flex: 1;
  min-width: 0;
}

.step-header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 2px;
}

.step-kind {
  font-size: 11px;
  font-weight: 600;
  color: var(--text-secondary);
}

.step-kind.error {
  color: var(--danger);
}

.step-tool {
  font-size: 11px;
  color: var(--primary);
  font-weight: 500;
  background: var(--primary-bg);
  padding: 1px 6px;
  border-radius: 3px;
}

.step-text {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.5;
}

.step-code {
  margin: 4px 0 0;
  padding: 6px 8px;
  background: #1e1e2e;
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 120px;
  overflow: auto;
  color: #cdd6f4;
  border: 1px solid rgba(255, 255, 255, 0.06);
}

.step-code.result {
  background: #1a2332;
  border-color: rgba(82, 196, 26, 0.15);
}

/* Agent 回复内容 */
.agent-content {
  padding: 10px 14px;
  background: white;
  border: 1px solid var(--border-light);
  border-radius: 4px 16px 16px 16px;
  font-size: 13px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  box-shadow: var(--shadow-sm);
}

.agent-content.success {
  border-left: 3px solid var(--success);
}

.agent-content.error {
  border-left: 3px solid var(--danger);
  background: #fff5f5;
}

/* 建议按钮 */
.suggestions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.suggestion-btn {
  padding: 5px 12px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: white;
  color: var(--primary);
  font-size: 12px;
  cursor: pointer;
  transition: all 0.2s;
}

.suggestion-btn:hover {
  background: var(--primary-bg);
  border-color: var(--primary);
}

/* Token 信息 */
.token-info {
  font-size: 11px;
  color: var(--text-muted);
  padding: 4px 0;
}

/* 加载指示器 */
.loading-indicator {
  display: flex;
  gap: 10px;
  align-items: center;
}

.typing-dots {
  display: flex;
  gap: 4px;
  padding: 12px 16px;
  background: white;
  border: 1px solid var(--border-light);
  border-radius: 4px 16px 16px 16px;
  box-shadow: var(--shadow-sm);
}

.typing-dots span {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--primary);
  opacity: 0.4;
  animation: typing 1.4s infinite;
}

.typing-dots span:nth-child(2) { animation-delay: 0.2s; }
.typing-dots span:nth-child(3) { animation-delay: 0.4s; }

@keyframes typing {
  0%, 60%, 100% { opacity: 0.4; transform: translateY(0); }
  30% { opacity: 1; transform: translateY(-4px); }
}

/* ===== 输入区域 ===== */
.chat-input {
  padding: 12px 16px;
  background: white;
  border-top: 1px solid var(--border);
  flex-shrink: 0;
}

.input-wrapper {
  display: flex;
  align-items: flex-end;
  gap: 8px;
  background: #f5f6f8;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 6px 8px 6px 12px;
  transition: border-color 0.2s;
}

.input-wrapper:focus-within {
  border-color: var(--primary);
  box-shadow: 0 0 0 2px rgba(91, 106, 191, 0.1);
}

.input-wrapper :deep(.el-textarea__inner) {
  background: transparent !important;
  border: none !important;
  box-shadow: none !important;
  padding: 4px 0;
  font-size: 13px;
  line-height: 1.5;
  resize: none;
}

.send-btn {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: none;
  background: #d1d5db;
  color: white;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  transition: all 0.2s;
}

.send-btn.active {
  background: var(--primary);
}

.send-btn:hover:not(:disabled) {
  transform: scale(1.05);
}

.send-btn:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

/* ===== 动画 ===== */
.slide-right-enter-active,
.slide-right-leave-active {
  transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.slide-right-enter-from,
.slide-right-leave-to {
  transform: translateX(100%);
}

.fab-enter-active,
.fab-leave-active {
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.fab-enter-from,
.fab-leave-to {
  opacity: 0;
  transform: scale(0.5);
}

.collapse-enter-active,
.collapse-leave-active {
  transition: all 0.25s ease;
  overflow: hidden;
}

.collapse-enter-from,
.collapse-leave-to {
  opacity: 0;
  max-height: 0;
}

.collapse-enter-to,
.collapse-leave-from {
  opacity: 1;
  max-height: 2000px;
}
</style>

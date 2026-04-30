<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'

import * as api from '@/api'
import type { SettingsView } from '@/api/types'

const router = useRouter()
const loading = ref(false)
const remote = ref<SettingsView | null>(null)

// 本地编辑态：按钮保存时把有变更的字段提交给后端
const form = reactive({
  userName: '',
  userEmail: '',
  llmEnabled: false,
  llmApiKey: '',
  llmBaseUrl: '',
  llmModel: '',
  llmMaxTokens: 4096,
  httpUsername: '',
  httpPassword: '',
})

onMounted(async () => {
  await load()
})

async function load() {
  loading.value = true
  try {
    remote.value = await api.getSettings()
    form.userName = remote.value.user.name
    form.userEmail = remote.value.user.email
    form.llmEnabled = remote.value.llm.enabled
    form.llmApiKey = '' // 密钥不回显，留空表示"保持不变"
    form.llmBaseUrl = remote.value.llm.baseUrl
    form.llmModel = remote.value.llm.model
    form.llmMaxTokens = remote.value.llm.maxTokens || 4096
    form.httpUsername = remote.value.http.username
    form.httpPassword = ''
  } finally {
    loading.value = false
  }
}

async function save() {
  const patch: Record<string, unknown> = {
    user: { name: form.userName, email: form.userEmail },
    llm: {
      enabled: form.llmEnabled,
      baseUrl: form.llmBaseUrl,
      model: form.llmModel,
      maxTokens: form.llmMaxTokens,
    },
    http: {
      username: form.httpUsername,
    },
  }
  // 只有填了新值才写入，避免覆盖成空串
  if (form.llmApiKey) (patch.llm as Record<string, unknown>).apiKey = form.llmApiKey
  if (form.httpPassword) (patch.http as Record<string, unknown>).password = form.httpPassword

  loading.value = true
  try {
    remote.value = await api.updateSettings(patch)
    ElMessage.success('已保存')
    form.llmApiKey = ''
    form.httpPassword = ''
  } catch (err) {
    ElMessage.error(`保存失败：${(err as Error).message}`)
  } finally {
    loading.value = false
  }
}

async function testLLM() {
  loading.value = true
  try {
    const res = await api.testLLM({
      apiKey: form.llmApiKey || undefined,
      baseUrl: form.llmBaseUrl || undefined,
      model: form.llmModel || undefined,
    })
    if (res.ok) {
      ElMessage.success(`${res.message}${res.sample ? `：${res.sample}` : ''}`)
    } else {
      ElMessage.error(res.message)
    }
  } catch (err) {
    ElMessage.error(`测试失败：${(err as Error).message}`)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="settings-page">
    <header class="header">
      <el-button link @click="router.back()">← 返回</el-button>
      <div class="title">设置</div>
      <div />
    </header>

    <main class="main" v-loading="loading">
      <section class="card">
        <h3>用户信息</h3>
        <p class="muted">用于保存版本时的作者署名</p>
        <el-form label-width="90px">
          <el-form-item label="姓名"><el-input v-model="form.userName" /></el-form-item>
          <el-form-item label="邮箱"><el-input v-model="form.userEmail" /></el-form-item>
        </el-form>
      </section>

      <section class="card">
        <h3>LLM 配置</h3>
        <p class="muted">启用后，Agent 可使用自然语言对话</p>
        <el-form label-width="90px">
          <el-form-item label="启用 LLM">
            <el-switch v-model="form.llmEnabled" />
          </el-form-item>
          <el-form-item label="API Key">
            <el-input v-model="form.llmApiKey" type="password" show-password
                      :placeholder="remote?.llm.hasApiKey ? remote.llm.apiKeyMask + '（留空保持不变）' : '未设置'" />
          </el-form-item>
          <el-form-item label="Base URL">
            <el-input v-model="form.llmBaseUrl" placeholder="https://api.openai.com/v1" />
          </el-form-item>
          <el-form-item label="模型">
            <el-input v-model="form.llmModel" placeholder="gpt-4o" />
          </el-form-item>
          <el-form-item label="MaxTokens">
            <el-input-number v-model="form.llmMaxTokens" :min="256" :max="32768" :step="256" />
          </el-form-item>
          <el-form-item>
            <el-button @click="testLLM">测试连接</el-button>
          </el-form-item>
        </el-form>
      </section>

      <section class="card">
        <h3>远程仓库 HTTP 认证</h3>
        <p class="muted">用于 https:// 协议的远程推送</p>
        <el-form label-width="90px">
          <el-form-item label="用户名"><el-input v-model="form.httpUsername" /></el-form-item>
          <el-form-item label="密码/Token">
            <el-input v-model="form.httpPassword" type="password" show-password
                      :placeholder="remote?.http.hasPassword ? remote.http.passwordMask + '（留空保持不变）' : '未设置'" />
          </el-form-item>
        </el-form>
      </section>

      <div class="actions">
        <el-button type="primary" :loading="loading" @click="save">保存</el-button>
      </div>
    </main>
  </div>
</template>

<style scoped>
.settings-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
}
.header {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  padding: 12px 24px;
  background: #fff;
  border-bottom: 1px solid #ebeef5;
}
.title {
  font-size: 16px;
  font-weight: 600;
}
.main {
  flex: 1;
  overflow: auto;
  padding: 24px;
  max-width: 720px;
  margin: 0 auto;
  width: 100%;
  box-sizing: border-box;
}
.card {
  background: #fff;
  padding: 20px 24px;
  margin-bottom: 16px;
  border-radius: 6px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
}
.card h3 { margin: 0 0 4px; font-size: 15px; }
.muted { color: #909399; font-size: 12px; margin: 0 0 12px; }
.actions { text-align: right; }
</style>

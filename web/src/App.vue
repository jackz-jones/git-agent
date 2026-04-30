<script setup lang="ts">
import { RouterView } from 'vue-router'
</script>

<template>
  <div class="app-root">
    <RouterView />
  </div>
</template>

<style>
:root {
  /* 主色调 - 蓝紫渐变 */
  --primary: #5b6abf;
  --primary-light: #7c8ae8;
  --primary-dark: #3d4a9e;
  --primary-bg: #eef0ff;

  /* 功能色 */
  --success: #52c41a;
  --warning: #faad14;
  --danger: #f5222d;
  --info: #909399;

  /* 背景层次 */
  --bg-base: #f0f2f5;
  --bg-elevated: #ffffff;
  --bg-sunken: #f7f8fa;
  --bg-sidebar: #1e2233;
  --bg-topbar: #252a3a;

  /* 文字 */
  --text-primary: #1f2937;
  --text-secondary: #6b7280;
  --text-muted: #9ca3af;
  --text-inverse: #f9fafb;

  /* 边框 */
  --border: #e5e7eb;
  --border-light: #f0f0f0;

  /* 阴影 */
  --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.05);
  --shadow-md: 0 4px 12px rgba(0, 0, 0, 0.08);
  --shadow-lg: 0 8px 24px rgba(0, 0, 0, 0.12);

  /* 圆角 */
  --radius-sm: 4px;
  --radius-md: 8px;
  --radius-lg: 12px;

  /* 字体 */
  --font-mono: ui-monospace, 'SF Mono', Menlo, Consolas, 'Liberation Mono', monospace;
}

html,
body,
#app,
.app-root {
  height: 100%;
  margin: 0;
  padding: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  color: var(--text-primary);
  background: var(--bg-base);
}

/* 全局滚动条美化 */
::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: #c1c1c1;
  border-radius: 3px;
}
::-webkit-scrollbar-thumb:hover {
  background: #a8a8a8;
}
</style>

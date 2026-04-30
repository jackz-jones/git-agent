import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      // 开发模式下把 /api 代理到后端 Go 服务
      '/api': {
        target: 'http://127.0.0.1:8088',
        changeOrigin: true,
        // WebSocket 代理（Agent 对话等场景）
        ws: true,
      },
    },
  },
  build: {
    // 生产构建直接输出到 Go embed 目录
    outDir: path.resolve(__dirname, '../internal/web/dist'),
    emptyOutDir: true,
    sourcemap: false,
  },
})

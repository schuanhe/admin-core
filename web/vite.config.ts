import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath } from 'node:url'

export default defineConfig({
  plugins: [vue()],
  server: {
    host: '127.0.0.1', port: 5173, strictPort: true,
    // 开发入口只用于本机同源调试，不给其他站点开放CORS或项目根目录。
    cors: false,
    fs: { strict: true, allow: [fileURLToPath(new URL('.', import.meta.url))] },
    proxy: { '/api': 'http://127.0.0.1:8080' },
  },
  build: { sourcemap: false },
  test: { environment: 'jsdom', clearMocks: true },
})

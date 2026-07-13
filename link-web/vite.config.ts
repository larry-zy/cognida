import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  server: {
    port: 5173,
    // 独占端口：若 5173 被其它 dev server（如别的项目）占用则报错退出，
    // 避免 IPv4/IPv6 双服务共存导致的“页面覆盖 / 接口 502”疑难问题。
    strictPort: true,
    proxy: {
      '/api': {
        // 固定走 IPv4，避免 localhost 解析到 [::1] 而 Go 仅在 127.0.0.1:8080 可达时代理 502。
        target: 'http://127.0.0.1:8080',
        changeOrigin: true
      }
    }
  }
})

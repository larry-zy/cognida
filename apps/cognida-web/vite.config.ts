import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    // Element Plus 按需引入（〔FE-8〕）：模板里的 <el-*>（当前仅 <el-icon>）与其配套 API
    // 按需解析并 tree-shake，替代 main.ts 里的全量 app.use(ElementPlus) 及全量图标注册。
    // importStyle: false —— 样式仍走 main.ts 的全量 element-plus/dist/index.css；因 ElMessage/
    // ElMessageBox 在 utils/element.ts 为「显式导入」，不会触发按需样式注入，保留全量样式可
    // 确保其样式不丢失（保守取舍：JS 侧按需 tree-shake，CSS 侧保持全量、零样式回归）。
    AutoImport({
      resolvers: [ElementPlusResolver({ importStyle: false })],
      dts: 'src/types/auto-imports.d.ts',
    }),
    Components({
      resolvers: [ElementPlusResolver({ importStyle: false })],
      dts: 'src/types/components.d.ts',
    }),
  ],
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

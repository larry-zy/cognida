import { createApp } from 'vue'
import { createPinia } from 'pinia'
// 保留全量 Element Plus 样式：JS 侧组件已改按需引入（见 vite.config.ts〔FE-8〕），但
// ElMessage/ElMessageBox 为显式导入、不触发按需样式注入，保留全量样式以零风险覆盖其样式。
import 'element-plus/dist/index.css'

import App from './App.vue'
import router from './router'
import { i18n } from './i18n'
import { useSettingsStore } from './stores/settings'

// 引入全局样式
import './components/styles/global.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(i18n)

// 应用用户偏好（主题 / 语言 / 字体）——须在 pinia 与 i18n 安装之后执行
useSettingsStore().init()
// 不再 app.use(ElementPlus) 全量注册，也不再全量注册图标：
// - 模板 <el-icon> 由 unplugin-vue-components + ElementPlusResolver 按需解析；
// - 各视图用到的图标已在各自文件显式 import（如 import { Plus } from '@element-plus/icons-vue'），
//   原先的全量图标注册循环是冗余死码；
// - 语言：唯一的命令式组件 ElMessage/ElMessageBox 已在 utils/element.ts 显式设中文按钮文案，
//   无 locale 依赖，故无需全局 locale 或 el-config-provider。

app.mount('#app')

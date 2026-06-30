// ==================== 基础组件 ====================
import BaseButton from './BaseButton.vue'
import BaseCard from './BaseCard.vue'
import BaseInput from './BaseInput.vue'
import BaseModal from './BaseModal.vue'
import BaseTag from './BaseTag.vue'

// ==================== 数据组件 ====================
import BaseTable from './BaseTable.vue'
import BaseLoader from './BaseLoader.vue'
import EmptyState from './EmptyState.vue'

// ==================== 布局组件 ====================
import AppBackground from './AppBackground.vue'

// ==================== 样式 ====================
import GlobalStyles from './styles/global.css'

// ==================== 类型 ====================
export type * from './types'

// ==================== 导出组件 ====================
export { BaseButton, BaseCard, BaseInput, BaseModal, BaseTag }
export { BaseTable, BaseLoader, EmptyState }
export { AppBackground }
export { GlobalStyles }

// ==================== 便捷插件 ====================
import type { App } from 'vue'

// 自动注册所有组件
export function installComponents(app: App) {
  // 基础组件
  app.component('BaseButton', BaseButton)
  app.component('BaseCard', BaseCard)
  app.component('BaseInput', BaseInput)
  app.component('BaseModal', BaseModal)
  app.component('BaseTag', BaseTag)

  // 数据组件
  app.component('BaseTable', BaseTable)
  app.component('BaseLoader', BaseLoader)
  app.component('EmptyState', EmptyState)

  // 布局组件
  app.component('AppBackground', AppBackground)
}

// 默认导出用于 app.use()
export default {
  install: installComponents
}

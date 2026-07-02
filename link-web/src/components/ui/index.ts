// ==================== 自研 UI 组件库统一出口 ====================
// 所有页面统一从 '@/components/ui' 引入，避免直接使用 Element Plus el-*。
// 用法：import { UiButton, UiForm, UiFormItem } from '@/components/ui'

// ---------- 基础 / 交互 ----------
export { default as UiButton } from './UiButton.vue'
export { default as UiInput } from './UiInput.vue'
export { default as UiTextarea } from './UiTextarea.vue'
export { default as UiInputNumber } from './UiInputNumber.vue'
export { default as UiSelect } from './UiSelect.vue'
export { default as UiCheckbox } from './UiCheckbox.vue'
export { default as UiRadio } from './UiRadio.vue'
export { default as UiRadioGroup } from './UiRadioGroup.vue'
export { default as UiSwitch } from './UiSwitch.vue'
export { default as UiSlider } from './UiSlider.vue'

// ---------- 表单 ----------
export { default as UiForm } from './UiForm.vue'
export { default as UiFormItem } from './UiFormItem.vue'
export type { FormRule, FormItemContext, FormContext } from './form-context'
export { formContextKey } from './form-context'

// ---------- 数据展示 ----------
export { default as UiTable } from './UiTable.vue'
export { default as UiTag } from './UiTag.vue'
export { default as UiDescriptions } from './UiDescriptions.vue'
export { default as UiDescriptionsItem } from './UiDescriptionsItem.vue'
export { default as UiStatistic } from './UiStatistic.vue'
export { default as UiProgress } from './UiProgress.vue'
export { default as UiText } from './UiText.vue'
export { default as UiPagination } from './UiPagination.vue'
export { default as UiEmpty } from './UiEmpty.vue'
export { default as UiSkeleton } from './UiSkeleton.vue'

// ---------- 反馈 / 浮层 ----------
export { default as UiModal } from './UiModal.vue'
export { default as UiDrawer } from './UiDrawer.vue'
export { default as UiConfirm } from './UiConfirm.vue'
export { default as UiAlert } from './UiAlert.vue'
export { default as UiToast } from './UiToast.vue'
export { default as UiTooltip } from './UiTooltip.vue'
export { default as UiLoader } from './UiLoader.vue'

// ---------- 导航 ----------
export { default as UiTabs } from './UiTabs.vue'
export { default as UiBreadcrumb } from './UiBreadcrumb.vue'

// ---------- 卡片 ----------
export { default as UiCard } from './UiCard.vue'

// ---------- 布局（components/layout/） ----------
export { default as UiContainer } from '../layout/UiContainer.vue'
export { default as UiGrid } from '../layout/UiGrid.vue'
export { default as UiDivider } from '../layout/UiDivider.vue'
export { default as UiPageHeader } from '../layout/UiPageHeader.vue'
export { default as UiPageSection } from '../layout/UiPageSection.vue'

// ---------- 图标 ----------
export { default as UiIcon } from '../icons/UiIcon.vue'

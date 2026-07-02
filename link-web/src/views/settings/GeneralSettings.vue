<template>
  <div class="general-settings">
    <UiForm :model="form" label-position="left" label-width="120px" style="max-width: 600px">
      <UiFormItem label="主题">
        <UiRadioGroup
          v-model="form.theme"
          :options="[
            { label: '浅色', value: 'light' },
            { label: '深色', value: 'dark' },
            { label: '跟随系统', value: 'auto' }
          ]"
        />
      </UiFormItem>

      <UiFormItem label="语言">
        <UiSelect
          v-model="form.language"
          :options="[
            { label: '简体中文', value: 'zh-CN' },
            { label: 'English', value: 'en-US' }
          ]"
        />
      </UiFormItem>

      <UiFormItem label="字体大小">
        <UiSlider v-model="form.fontSize" :min="12" :max="20" :marks="[{ value: 14, label: '默认' }]" />
      </UiFormItem>

      <UiFormItem label="显示思考过程">
        <UiSwitch v-model="form.showThinking" />
      </UiFormItem>

      <UiFormItem label="自动保存">
        <UiSwitch v-model="form.autoSave" />
      </UiFormItem>

      <UiFormItem>
        <UiButton variant="primary" @click="handleSave">保存设置</UiButton>
        <UiButton variant="secondary" @click="handleReset">重置</UiButton>
      </UiFormItem>
    </UiForm>
  </div>
</template>

<script setup lang="ts">
import { reactive } from 'vue'
import { UiForm, UiFormItem, UiRadioGroup, UiSelect, UiSlider, UiSwitch, UiButton } from '@/components/ui'
import toast from '@/utils/toast'
import { useSettingsStore } from '@/stores/settings'

const settingsStore = useSettingsStore()

const form = reactive({
  theme: settingsStore.settings.theme,
  language: settingsStore.settings.language,
  fontSize: settingsStore.settings.fontSize,
  showThinking: settingsStore.settings.showThinking,
  autoSave: settingsStore.settings.autoSave
})

function handleSave() {
  settingsStore.updateSettings(form)
  toast.success('保存成功')
}

function handleReset() {
  settingsStore.resetSettings()
  toast.success('已重置')
}
</script>

<style scoped>
.general-settings {
  padding: 16px 0;
}
</style>

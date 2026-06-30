<template>
  <div class="general-settings">
    <el-form :model="form" label-width="120px" style="max-width: 600px">
      <el-form-item label="主题">
        <el-radio-group v-model="form.theme">
          <el-radio label="light">浅色</el-radio>
          <el-radio label="dark">深色</el-radio>
          <el-radio label="auto">跟随系统</el-radio>
        </el-radio-group>
      </el-form-item>

      <el-form-item label="语言">
        <el-select v-model="form.language">
          <el-option label="简体中文" value="zh-CN" />
          <el-option label="English" value="en-US" />
        </el-select>
      </el-form-item>

      <el-form-item label="字体大小">
        <el-slider v-model="form.fontSize" :min="12" :max="20" show-stops :marks="{ 14: '默认' }" />
      </el-form-item>

      <el-form-item label="显示思考过程">
        <el-switch v-model="form.showThinking" />
      </el-form-item>

      <el-form-item label="自动保存">
        <el-switch v-model="form.autoSave" />
      </el-form-item>

      <el-form-item>
        <el-button type="primary" @click="handleSave">保存设置</el-button>
        <el-button @click="handleReset">重置</el-button>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup lang="ts">
import { reactive } from 'vue'
import { ElMessage } from '@/utils/element'
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
  ElMessage.success('保存成功')
}

function handleReset() {
  settingsStore.resetSettings()
  ElMessage.success('已重置')
}
</script>

<style scoped>
.general-settings {
  padding: 16px 0;
}
</style>

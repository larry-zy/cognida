<template>
  <div class="settings-view">
    <main class="settings-main">
      <header class="settings-header">
        <h1 class="settings-title">通用设置</h1>
        <p class="settings-description">配置界面和编辑器偏好</p>
      </header>

      <section class="settings-section">
        <div class="settings-card">
          <div class="settings-card-header">
            <h3>外观</h3>
            <p>自定义界面的显示方式</p>
          </div>
          <div class="settings-card-body">
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">主题</label>
                <p class="setting-description">选择您偏好的界面主题</p>
              </div>
              <div class="setting-control">
                <UiRadioGroup v-model="settings.theme" :options="themeOptions" />
              </div>
            </div>
            <UiDivider />
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">语言</label>
                <p class="setting-description">选择界面显示语言</p>
              </div>
              <div class="setting-control">
                <UiSelect v-model="settings.language" style="width: 200px">
                  <option value="zh-CN">简体中文</option>
                  <option value="en-US">English</option>
                </UiSelect>
              </div>
            </div>
            <UiDivider />
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">字体大小</label>
                <p class="setting-description">调整界面文字大小</p>
              </div>
              <div class="setting-control">
                <UiSlider v-model="settings.fontSize" :min="12" :max="20" :marks="[{ value: 16, label: '默认' }]" />
              </div>
            </div>
          </div>
        </div>

        <div class="settings-card">
          <div class="settings-card-header">
            <h3>编辑器</h3>
            <p>配置编辑器相关选项</p>
          </div>
          <div class="settings-card-body">
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">显示思考过程</label>
                <p class="setting-description">在对话中显示 Agent 的思考过程</p>
              </div>
              <div class="setting-control">
                <UiSwitch v-model="settings.showThinking" />
              </div>
            </div>
            <UiDivider />
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">自动保存</label>
                <p class="setting-description">编辑内容时自动保存</p>
              </div>
              <div class="setting-control">
                <UiSwitch v-model="settings.autoSave" />
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 固定底部保存按钮 -->
      <footer v-if="hasChanges" class="settings-footer">
        <div class="settings-footer-content">
          <span class="settings-notice">您有未保存的更改</span>
          <div class="settings-footer-actions">
            <UiButton variant="ghost" @click="resetChanges">放弃更改</UiButton>
            <UiButton variant="primary" @click="saveSettings">保存更改</UiButton>
          </div>
        </div>
      </footer>
    </main>
  </div>
</template>

<script setup lang="ts">
import { reactive, computed, ref } from 'vue'
import { ElMessage } from '@/utils/element'
import { useSettingsStore } from '@/stores/settings'
import UiRadioGroup from '@/components/ui/UiRadioGroup.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiSlider from '@/components/ui/UiSlider.vue'
import UiSwitch from '@/components/ui/UiSwitch.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiDivider from '@/components/layout/UiDivider.vue'

const settingsStore = useSettingsStore()

// 设置数据（从 store 快照初始化，保存时统一交由 store 持久化并应用）
const settings = reactive({
  theme: settingsStore.settings.theme,
  language: settingsStore.settings.language,
  fontSize: settingsStore.settings.fontSize,
  showThinking: settingsStore.settings.showThinking,
  autoSave: settingsStore.settings.autoSave
})

const themeOptions = [
  { label: '浅色', value: 'light' },
  { label: '深色', value: 'dark' },
  { label: '跟随系统', value: 'auto' }
]

// 已保存状态快照，用于检测是否存在未保存更改
function snapshot() {
  return JSON.stringify(settings)
}
const savedSnapshot = ref(snapshot())
const hasChanges = computed(() => snapshot() !== savedSnapshot.value)

function saveSettings() {
  settingsStore.updateSettings({ ...settings })
  savedSnapshot.value = snapshot()
  ElMessage.success('设置已保存')
}

function resetChanges() {
  Object.assign(settings, JSON.parse(savedSnapshot.value))
}
</script>

<style scoped>
.settings-view {
  height: calc(100vh - 64px);
  background: var(--bg-primary);
}

.settings-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  position: relative;
}

.settings-header {
  padding: var(--space-8) var(--space-8) var(--space-4);
}

.settings-title {
  font: var(--font-display);
  font-size: var(--text-2xl);
  font-weight: 700;
  color: var(--text-primary);
  margin-bottom: var(--space-2);
}

.settings-description {
  font-size: var(--text-base);
  color: var(--text-secondary);
}

/* 设置区块 */
.settings-section {
  padding: var(--space-4) var(--space-8) var(--space-8);
  display: flex;
  flex-direction: column;
  gap: var(--space-6);
}

.settings-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

.settings-card-header {
  padding: var(--space-5) var(--space-6);
  border-bottom: 1px solid var(--border-subtle);
}

.settings-card-header h3 {
  font: var(--font-display);
  font-size: var(--text-lg);
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 var(--space-1) 0;
}

.settings-card-header p {
  font-size: var(--text-sm);
  color: var(--text-secondary);
  margin: 0;
}

.settings-card-body {
  padding: var(--space-5) var(--space-6);
}

.setting-item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-6);
  padding: var(--space-3) 0;
}

.setting-info {
  flex: 1;
}

.setting-label {
  display: block;
  font-size: var(--text-sm);
  font-weight: 500;
  color: var(--text-primary);
  margin-bottom: var(--space-1);
}

.setting-description {
  font-size: var(--text-sm);
  color: var(--text-muted);
  margin: 0;
}

.setting-control {
  flex-shrink: 0;
}

/* 底部保存按钮 */
.settings-footer {
  position: sticky;
  bottom: 0;
  background: var(--bg-secondary);
  border-top: 1px solid var(--border-subtle);
  padding: var(--space-4) var(--space-8);
  z-index: 10;
}

.settings-footer-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.settings-notice {
  font-size: var(--text-sm);
  color: var(--text-secondary);
}

.settings-footer-actions {
  display: flex;
  gap: var(--space-3);
}

/* 响应式 */
@media (max-width: 767px) {
  .setting-item {
    flex-direction: column;
    gap: var(--space-3);
  }

  .setting-control {
    width: 100%;
  }

  .settings-footer-content {
    flex-direction: column;
    gap: var(--space-3);
    align-items: stretch;
  }

  .settings-footer-actions {
    justify-content: stretch;
  }

  .settings-footer-actions button {
    flex: 1;
  }
}
</style>

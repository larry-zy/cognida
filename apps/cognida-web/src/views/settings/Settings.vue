<template>
  <div class="settings-view">
    <!-- 设置内容区 -->
    <main class="settings-main">
      <header class="settings-header">
        <h1 class="settings-title">{{ currentNav?.label }}</h1>
        <p class="settings-description">{{ currentNav?.description }}</p>
      </header>

      <!-- 通用设置 -->
      <section v-if="activeSection === 'general'" class="settings-section">
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
                <UiSlider v-model="settings.fontSize" :min="12" :max="20" :marks="[{ value: 14, label: '默认' }]" />
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

      <!-- 模型设置 -->
      <section v-if="activeSection === 'model'" class="settings-section">
        <div class="settings-card">
          <div class="settings-card-header">
            <h3>默认模型</h3>
            <p>配置对话和 Agent 使用的默认模型</p>
          </div>
          <div class="settings-card-body">
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">对话模型</label>
                <p class="setting-description">用于普通对话的模型</p>
              </div>
              <div class="setting-control">
                <UiSelect v-model="modelSettings.chatModel" placeholder="选择模型" style="width: 240px">
                  <option value="gpt-4">GPT-4</option>
                  <option value="gpt-3.5-turbo">GPT-3.5 Turbo</option>
                  <option value="claude-3-opus">Claude 3 Opus</option>
                  <option value="claude-3-sonnet">Claude 3 Sonnet</option>
                </UiSelect>
              </div>
            </div>
            <UiDivider />
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">Agent 模型</label>
                <p class="setting-description">用于 Agent 任务的模型</p>
              </div>
              <div class="setting-control">
                <UiSelect v-model="modelSettings.agentModel" placeholder="选择模型" style="width: 240px">
                  <option value="gpt-4">GPT-4</option>
                  <option value="claude-3-opus">Claude 3 Opus</option>
                  <option value="claude-3-sonnet">Claude 3 Sonnet</option>
                </UiSelect>
              </div>
            </div>
            <UiDivider />
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">温度</label>
                <p class="setting-description">控制输出的随机性</p>
              </div>
              <div class="setting-control">
                <div class="slider-with-value">
                  <UiSlider v-model="modelSettings.temperature" :min="0" :max="2" :step="0.1" />
                  <span class="slider-value">{{ modelSettings.temperature }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="settings-card">
          <div class="settings-card-header">
            <h3>API 配置</h3>
            <p>配置 API 端点和密钥</p>
          </div>
          <div class="settings-card-body">
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">API 端点</label>
                <p class="setting-description">自定义 API 服务地址</p>
              </div>
              <div class="setting-control">
                <UiInput v-model="apiSettings.endpoint" placeholder="https://api.example.com" style="width: 320px" />
              </div>
            </div>
            <UiDivider />
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">API 密钥</label>
                <p class="setting-description">用于身份验证的密钥</p>
              </div>
              <div class="setting-control">
                <UiInput v-model="apiSettings.apiKey" type="password" placeholder="sk-..." style="width: 320px" />
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- Agent 设置 -->
      <section v-if="activeSection === 'agent'" class="settings-section">
        <div class="settings-card">
          <div class="settings-card-header">
            <h3>Agent 配置</h3>
            <p>配置 Agent 行为参数</p>
          </div>
          <div class="settings-card-body">
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">最大迭代次数</label>
                <p class="setting-description">Agent 执行的最大步骤数</p>
              </div>
              <div class="setting-control">
                <div class="slider-with-value">
                  <UiSlider v-model="agentSettings.maxIterations" :min="5" :max="50" :step="5" />
                  <span class="slider-value">{{ agentSettings.maxIterations }}</span>
                </div>
              </div>
            </div>
            <UiDivider />
            <div class="setting-item">
              <div class="setting-info">
                <label class="setting-label">超时时间（秒）</label>
                <p class="setting-description">单次执行的超时时间</p>
              </div>
              <div class="setting-control">
                <div class="slider-with-value">
                  <UiSlider v-model="agentSettings.timeout" :min="30" :max="300" :step="30" />
                  <span class="slider-value">{{ agentSettings.timeout }}s</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 其他设置的占位 -->
      <section v-if="['mcp', 'webSearch', 'apiInfo', 'systemInfo', 'tenantInfo'].includes(activeSection)" class="settings-section">
        <UiEmpty
          :title="`${currentNav?.label}功能开发中`"
          description="敬请期待"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" slot="icon">
            <circle cx="12" cy="12" r="10"/>
            <path d="M12 16v-4M12 8h.01"/>
          </svg>
        </UiEmpty>
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
import { ref, reactive, computed, h } from 'vue'
import { ElMessage } from '@/utils/element'
import { useSettingsStore } from '@/stores/settings'
import { storage } from '@/utils/security'
import UiRadioGroup from '@/components/ui/UiRadioGroup.vue'
import UiSelect from '@/components/ui/UiSelect.vue'
import UiSlider from '@/components/ui/UiSlider.vue'
import UiSwitch from '@/components/ui/UiSwitch.vue'
import UiInput from '@/components/ui/UiInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import UiDivider from '@/components/layout/UiDivider.vue'
import UiEmpty from '@/components/ui/UiEmpty.vue'

const settingsStore = useSettingsStore()

// 图标组件
const GeneralIcon = () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': 2 }, [
  h('circle', { cx: '12', cy: '12', r: '3' }),
  h('path', { d: 'M12 1v6m0 6v6m9-9l-4.5 4.5M15 15l-4.5 4.5M9 9l4.5-4.5M15 9l-4.5 4.5' })
])

const ModelIcon = () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': 2 }, [
  h('path', { d: 'M12 2a2 2 0 0 1 2 2c0 .74-.4 1.39-1 1.73V7h1a1 1 0 0 1 1 1v3a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1V5.73A2 2 0 0 1 12 2Z' })
])

const AgentIcon = () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': 2 }, [
  h('circle', { cx: '12', cy: '12', r: '10' }),
  h('path', { d: 'M12 16v-4M12 8h.01' })
])

const McpIcon = () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': 2 }, [
  h('path', { d: 'M13 2L3 14h9l-1 8 10-12h-9l1-8z' })
])

const SearchIcon = () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': 2 }, [
  h('circle', { cx: '11', cy: '11', r: '8' }),
  h('path', { d: 'm21 21-4.35-4.35' })
])

const ApiIcon = () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': 2 }, [
  h('rect', { x: '2', y: '2', width: '20', height: '20', rx: '2', ry: '2' }),
  h('line', { x1: '12', y1: '6', x2: '12', y2: '18' })
])

const InfoIcon = () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': 2 }, [
  h('circle', { cx: '12', cy: '12', r: '10' }),
  h('line', { x1: '12', y1: '16', x2: '12', y2: '12' }),
  h('line', { x1: '12', y1: '8', x2: '12.01', y2: '8' })
])

const TenantIcon = () => h('svg', { viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', 'stroke-width': 2 }, [
  h('path', { d: 'M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2' }),
  h('circle', { cx: '9', cy: '7', r: '4' }),
  h('path', { d: 'M23 21v-2a4 4 0 0 0-3-3.87' }),
  h('path', { d: 'M16 3.13a4 4 0 0 1 0 7.75' })
])

const navItems = [
  { key: 'general', label: '通用设置', description: '配置界面和编辑器偏好', icon: GeneralIcon },
  { key: 'model', label: '模型配置', description: '配置 AI 模型和 API', icon: ModelIcon },
  { key: 'agent', label: 'Agent 设置', description: '配置 Agent 行为参数', icon: AgentIcon },
  { key: 'mcp', label: 'MCP 服务', description: '配置 MCP 服务器', icon: McpIcon },
  { key: 'webSearch', label: '网络搜索', description: '配置搜索引擎', icon: SearchIcon },
  { key: 'apiInfo', label: 'API 信息', description: '查看 API 密钥和用量', icon: ApiIcon },
  { key: 'systemInfo', label: '系统信息', description: '查看系统状态和版本', icon: InfoIcon },
  { key: 'tenantInfo', label: '租户信息', description: '管理租户设置', icon: TenantIcon }
]

const activeSection = ref('general')

// 本地存储 key（模型/API/Agent 设置不在 settingsStore 中，需单独持久化）
const MODEL_SETTINGS_KEY = 'model_settings'
const API_SETTINGS_KEY = 'api_settings'
const AGENT_SETTINGS_KEY = 'agent_settings'

const modelDefaults = { chatModel: 'gpt-4', agentModel: 'gpt-4', temperature: 0.7 }
const apiDefaults = { endpoint: '', apiKey: '' }
const agentDefaults = { maxIterations: 20, timeout: 120 }

// 设置数据
const settings = reactive({
  theme: settingsStore.settings.theme,
  language: settingsStore.settings.language,
  fontSize: settingsStore.settings.fontSize,
  showThinking: settingsStore.settings.showThinking,
  autoSave: settingsStore.settings.autoSave
})

const modelSettings = reactive({
  ...modelDefaults,
  ...(storage.get<typeof modelDefaults>(MODEL_SETTINGS_KEY) ?? {})
})

const apiSettings = reactive({
  ...apiDefaults,
  ...(storage.get<typeof apiDefaults>(API_SETTINGS_KEY) ?? {})
})

const agentSettings = reactive({
  ...agentDefaults,
  ...(storage.get<typeof agentDefaults>(AGENT_SETTINGS_KEY) ?? {})
})

// 已保存状态快照，用于检测是否存在未保存更改
function snapshot() {
  return JSON.stringify({ settings, modelSettings, apiSettings, agentSettings })
}
const savedSnapshot = ref(snapshot())

// hasChanges 通过对比当前状态与最近一次保存的快照计算得出，
// 任意设置项变更都会自动反映到底部保存栏。
const hasChanges = computed(() => snapshot() !== savedSnapshot.value)

const themeOptions = [
  { label: '浅色', value: 'light' },
  { label: '深色', value: 'dark' },
  { label: '跟随系统', value: 'auto' }
]

const currentNav = computed(() => navItems.find(item => item.key === activeSection.value))

function saveSettings() {
  // 通用设置交由 settingsStore 统一持久化（含主题应用）
  settingsStore.updateSettings({ ...settings })
  // 模型 / API / Agent 设置不在 store 中，单独持久化
  storage.set(MODEL_SETTINGS_KEY, { ...modelSettings })
  storage.set(API_SETTINGS_KEY, { ...apiSettings })
  storage.set(AGENT_SETTINGS_KEY, { ...agentSettings })
  // 更新快照，hasChanges 随即变为 false
  savedSnapshot.value = snapshot()
  ElMessage.success('设置已保存')
}

function resetChanges() {
  const saved = JSON.parse(savedSnapshot.value)
  Object.assign(settings, saved.settings)
  Object.assign(modelSettings, saved.modelSettings)
  Object.assign(apiSettings, saved.apiSettings)
  Object.assign(agentSettings, saved.agentSettings)
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

.slider-with-value {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 240px;
}

.slider-value {
  min-width: 40px;
  text-align: right;
  font-size: var(--text-sm);
  color: var(--text-secondary);
  font-variant-numeric: tabular-nums;
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

  .slider-with-value {
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

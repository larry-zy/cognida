import pluginVue from 'eslint-plugin-vue'
import { defineConfigWithVueTs, vueTsConfigs } from '@vue/eslint-config-typescript'

// Flat config (ESLint 9). Vue 3 + TypeScript 官方推荐组合。
export default defineConfigWithVueTs(
  {
    ignores: ['dist/**', 'node_modules/**', 'public/**', '*.config.*', 'chat-demo.html'],
  },
  pluginVue.configs['flat/essential'],
  vueTsConfigs.recommended,
  {
    rules: {
      // 棕地项目: 存量代码大量使用 any / 未用变量, 先降级为 warn, 避免一次性红全屏。
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
      'vue/multi-word-component-names': 'warn',
    },
  },
)

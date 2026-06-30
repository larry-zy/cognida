<template>
  <div class="login-container">
    <div class="login-box">
      <div class="login-header">
        <div class="logo-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
          </svg>
        </div>
        <h1>{{ isLogin ? t('auth.login') : t('auth.register') }}</h1>
        <p class="subtitle">Link - AI Knowledge Platform</p>
      </div>

      <form class="login-form" @submit.prevent="handleSubmit">
        <!-- 用户名 (注册时显示) -->
        <div v-if="!isLogin" class="form-group">
          <div class="input-wrapper">
            <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
              <circle cx="12" cy="7" r="4"/>
            </svg>
            <input
              v-model="formData.username"
              type="text"
              :placeholder="t('auth.usernamePlaceholder')"
              class="form-input"
              :class="{ 'has-error': formErrors.username }"
            >
          </div>
          <span v-if="formErrors.username" class="form-error">{{ formErrors.username }}</span>
        </div>

        <!-- 邮箱 -->
        <div class="form-group">
          <div class="input-wrapper">
            <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z"/>
              <polyline points="22,6 12,13 2,6"/>
            </svg>
            <input
              v-model="formData.email"
              type="email"
              :placeholder="t('auth.emailPlaceholder')"
              class="form-input"
              :class="{ 'has-error': formErrors.email }"
            >
          </div>
          <span v-if="formErrors.email" class="form-error">{{ formErrors.email }}</span>
        </div>

        <!-- 密码 -->
        <div class="form-group">
          <div class="input-wrapper">
            <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
              <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
            </svg>
            <input
              v-model="formData.password"
              :type="showPassword ? 'text' : 'password'"
              :placeholder="t('auth.passwordPlaceholder')"
              class="form-input"
              :class="{ 'has-error': formErrors.password }"
            >
            <button
              type="button"
              class="toggle-password"
              @click="showPassword = !showPassword"
            >
              <svg v-if="showPassword" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                <circle cx="12" cy="12" r="3"/>
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-5.06 5.94M1 1l22 22"/>
              </svg>
            </button>
          </div>
          <span v-if="formErrors.password" class="form-error">{{ formErrors.password }}</span>
        </div>

        <!-- 确认密码 (注册时显示) -->
        <div v-if="!isLogin" class="form-group">
          <div class="input-wrapper">
            <svg class="input-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
              <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
            </svg>
            <input
              v-model="formData.confirmPassword"
              :type="showConfirmPassword ? 'text' : 'password'"
              :placeholder="t('auth.confirmPasswordPlaceholder')"
              class="form-input"
              :class="{ 'has-error': formErrors.confirmPassword }"
            >
            <button
              type="button"
              class="toggle-password"
              @click="showConfirmPassword = !showConfirmPassword"
            >
              <svg v-if="showConfirmPassword" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                <circle cx="12" cy="12" r="3"/>
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-5.06 5.94M1 1l22 22"/>
              </svg>
            </button>
          </div>
          <span v-if="formErrors.confirmPassword" class="form-error">{{ formErrors.confirmPassword }}</span>
        </div>

        <!-- 记住我 (登录时显示) -->
        <div v-if="isLogin" class="form-group checkbox-group">
          <label class="checkbox-label">
            <input v-model="formData.rememberMe" type="checkbox" class="checkbox">
            <span>{{ t('auth.rememberMe') }}</span>
          </label>
        </div>

        <!-- 提交按钮 -->
        <button
          type="submit"
          class="submit-btn"
          :disabled="loading"
          :class="{ loading }"
        >
          <span v-if="!loading">{{ isLogin ? t('auth.login') : t('auth.register') }}</span>
          <span v-else class="loading-text">{{ t('common.loading') }}</span>
        </button>

        <!-- 切换登录/注册 -->
        <div class="switch-mode">
          <span v-if="isLogin">
            {{ t('auth.noAccount') }}
            <button type="button" class="link-btn" @click="toggleMode">{{ t('auth.goToRegister') }}</button>
          </span>
          <span v-else>
            {{ t('auth.hasAccount') }}
            <button type="button" class="link-btn" @click="toggleMode">{{ t('auth.goToLogin') }}</button>
          </span>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from '@/utils/element'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

// 是否登录模式
const isLogin = ref(true)
const loading = ref(false)
const showPassword = ref(false)
const showConfirmPassword = ref(false)

// 表单数据
const formData = reactive({
  username: '',
  email: '',
  password: '',
  confirmPassword: '',
  rememberMe: false
})

// 表单错误
const formErrors = reactive<Record<string, string>>({})

// 验证表单
function validateForm(): boolean {
  // 清空错误
  Object.keys(formErrors).forEach(key => delete formErrors[key])

  let isValid = true

  // 验证用户名
  if (!isLogin.value && !formData.username.trim()) {
    formErrors.username = t('auth.usernameRequired')
    isValid = false
  } else if (!isLogin.value && formData.username.length < 2) {
    formErrors.username = '用户名长度在2到20个字符'
    isValid = false
  } else if (!isLogin.value && formData.username.length > 20) {
    formErrors.username = '用户名长度在2到20个字符'
    isValid = false
  }

  // 验证邮箱
  if (!formData.email.trim()) {
    formErrors.email = t('auth.emailRequired')
    isValid = false
  } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email)) {
    formErrors.email = t('auth.invalidEmail')
    isValid = false
  }

  // 验证密码
  if (!formData.password) {
    formErrors.password = t('auth.passwordRequired')
    isValid = false
  } else if (formData.password.length < 6) {
    formErrors.password = t('auth.passwordTooShort')
    isValid = false
  }

  // 验证确认密码
  if (!isLogin.value) {
    if (!formData.confirmPassword) {
      formErrors.confirmPassword = t('auth.passwordRequired')
      isValid = false
    } else if (formData.confirmPassword !== formData.password) {
      formErrors.confirmPassword = t('auth.passwordNotMatch')
      isValid = false
    }
  }

  return isValid
}

// 切换登录/注册模式
function toggleMode() {
  isLogin.value = !isLogin.value
  // 清空确认密码
  formData.confirmPassword = ''
  // 清空错误
  Object.keys(formErrors).forEach(key => delete formErrors[key])
}

// 处理提交
async function handleSubmit() {
  if (!validateForm()) return

  loading.value = true
  try {
    if (isLogin.value) {
      // 登录
      const success = await authStore.login({
        email: formData.email,
        password: formData.password
      })

      if (success) {
        ElMessage.success(t('auth.loginSuccess'))

        // 登录成功，跳转到原来的页面或平台首页
        const redirect = (route.query.redirect as string) || '/'
        await router.push(redirect)
      } else {
        ElMessage.error('登录失败，请检查用户名和密码')
      }
    } else {
      // 注册
      const success = await authStore.register({
        username: formData.username,
        email: formData.email,
        password: formData.password
      })

      if (success) {
        ElMessage.success(t('auth.registerSuccess'))

        // 注册成功，切换到登录模式
        isLogin.value = true
        formData.confirmPassword = ''
        Object.keys(formErrors).forEach(key => delete formErrors[key])
      } else {
        ElMessage.error('注册失败，请稍后重试')
      }
    }
  } catch (error: any) {
    // 错误已经在 request.ts 中处理并显示，这里只需要打印日志
    console.error('Auth error:', error)
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: #0f0f12;
  position: relative;
  overflow: hidden;
}

.login-container::before {
  content: '';
  position: absolute;
  top: -50%;
  left: -50%;
  width: 200%;
  height: 200%;
  background: radial-gradient(ellipse at 20% 20%, rgba(34, 211, 238, 0.08) 0%, transparent 50%),
              radial-gradient(ellipse at 80% 80%, rgba(129, 140, 248, 0.06) 0%, transparent 50%);
  pointer-events: none;
}

.login-box {
  width: 400px;
  padding: 48px 40px;
  background: rgba(24, 24, 27, 0.8);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 18px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.6);
  position: relative;
  z-index: 1;
}

.login-header {
  text-align: center;
  margin-bottom: 32px;
}

.logo-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  background: linear-gradient(135deg, #22d3ee 0%, #14b8a6 100%);
  border-radius: 14px;
  margin-bottom: 16px;
  color: #0f0f12;
}

.logo-icon svg {
  width: 28px;
  height: 28px;
}

.login-header h1 {
  font-size: 1.75rem;
  font-weight: 600;
  color: #f0f0f5;
  margin-bottom: 8px;
}

.subtitle {
  color: #606070;
  font-size: 0.875rem;
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.input-wrapper {
  position: relative;
  display: flex;
  align-items: center;
}

.input-icon {
  position: absolute;
  left: 16px;
  width: 18px;
  height: 18px;
  color: #606070;
  pointer-events: none;
}

.form-input {
  width: 100%;
  padding: 12px 16px 12px 44px;
  background: rgba(31, 31, 35, 0.8);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  color: #f0f0f5;
  font-size: 14px;
  transition: all 150ms ease;
  outline: none;
}

.form-input:hover {
  border-color: rgba(255, 255, 255, 0.15);
}

.form-input:focus {
  border-color: #22d3ee;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.1);
}

.form-input.has-error {
  border-color: #ef4444;
}

.form-input.has-error:focus {
  box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.1);
}

.form-input::placeholder {
  color: #606070;
}

.toggle-password {
  position: absolute;
  right: 12px;
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  color: #606070;
  cursor: pointer;
  border-radius: 6px;
  transition: all 150ms ease;
}

.toggle-password:hover {
  background: rgba(255, 255, 255, 0.05);
  color: #a0a0b0;
}

.toggle-password svg {
  width: 18px;
  height: 18px;
}

.form-error {
  font-size: 12px;
  color: #ef4444;
  padding-left: 4px;
}

.checkbox-group {
  margin-top: -8px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  align-self: flex-start;
}

.checkbox {
  width: 16px;
  height: 16px;
  accent-color: #22d3ee;
  cursor: pointer;
}

.checkbox-label span {
  font-size: 14px;
  color: #a0a0b0;
}

.submit-btn {
  width: 100%;
  margin-top: 8px;
  height: 44px;
  font-size: 1rem;
  border-radius: 10px;
  border: none;
  background: linear-gradient(135deg, #22d3ee 0%, #14b8a6 100%);
  color: #0f0f12;
  font-weight: 600;
  cursor: pointer;
  transition: all 150ms ease;
}

.submit-btn:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(34, 211, 238, 0.3);
}

.submit-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.submit-btn.loading {
  position: relative;
  color: transparent;
}

.submit-btn.loading::after {
  content: '';
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 20px;
  height: 20px;
  border: 2px solid rgba(15, 15, 18, 0.3);
  border-top-color: #0f0f12;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: translate(-50%, -50%) rotate(360deg); }
}

.switch-mode {
  text-align: center;
  margin-top: 20px;
  font-size: 0.875rem;
  color: #606070;
}

.link-btn {
  background: none;
  border: none;
  padding: 0;
  margin-left: 4px;
  font-size: 0.875rem;
  color: #22d3ee;
  cursor: pointer;
  text-decoration: none;
  transition: color 150ms ease;
}

.link-btn:hover {
  color: #67e8f9;
  text-decoration: underline;
}

/* 响应式 */
@media (max-width: 640px) {
  .login-box {
    width: 100%;
    max-width: 360px;
    padding: 32px 24px;
    margin: 16px;
  }
}
</style>

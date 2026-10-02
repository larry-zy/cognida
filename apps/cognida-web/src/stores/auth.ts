import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { http, resetAuthFailure } from '@/utils/request'
import type { LoginRequest, RegisterRequest, AuthResponse, UserInfo, Tenant } from '@/types'
import { storage } from '@/utils/security'

export const useAuthStore = defineStore('auth', () => {
  // 状态
  const token = ref<string>('')
  const refreshToken = ref<string>('')
  const user = ref<UserInfo | null>(null)
  const tenants = ref<Tenant[]>([])
  const currentTenant = ref<Tenant | null>(null)

  // 计算属性
  const isAuthenticated = computed(() => !!token.value)
  const username = computed(() => user.value?.username || '')
  const email = computed(() => user.value?.email || '')

  // 初始化：从本地存储恢复
  function initFromStorage() {
    const savedToken = storage.get<string>('token')
    const savedRefreshToken = storage.get<string>('refresh_token')
    const savedUser = storage.get<UserInfo>('user')
    const savedTenants = storage.get<Tenant[]>('tenants')
    const savedCurrentTenant = storage.get<Tenant>('current_tenant')

    if (savedToken) token.value = savedToken
    if (savedRefreshToken) refreshToken.value = savedRefreshToken
    if (savedUser) user.value = savedUser
    if (savedTenants) tenants.value = savedTenants
    if (savedCurrentTenant) currentTenant.value = savedCurrentTenant
  }

  // 检查认证状态
  async function checkAuth() {
    initFromStorage()
    if (!token.value) return false

    try {
      // 验证Token是否有效
      await http.get<UserInfo>('/user/profile')
      return true
    } catch {
      // Token无效，清除本地存储
      clearAuth()
      return false
    }
  }

  // 登录
  async function login(credentials: LoginRequest) {
    try {
      const res = await http.post<AuthResponse>('/auth/login', credentials)

      if (res.data) {
        const { access_token: accessToken, refresh_token, user: userData, tenant_id } = res.data
        clearAuth()
        token.value = accessToken
        refreshToken.value = refresh_token
        user.value = userData

        // 重新登录成功：复位「登录已过期」一次性提示，后续过期仍能提示并跳转
        resetAuthFailure()

        // 保存到本地存储
        storage.set('token', accessToken)
        storage.set('refresh_token', refresh_token)
        storage.set('user', userData)

        // 如果有 tenant_id，保存为当前租户
        if (tenant_id) {
          const tenant: Tenant = {
            id: tenant_id,
            name: '',
            created_at: '',
            updated_at: ''
          }
          currentTenant.value = tenant
          storage.set('current_tenant', tenant)
        }

        return true
      }
      return false
    } catch (error) {
      console.error('Login error:', error)
      throw error
    }
  }

  // 注册
  async function register(data: RegisterRequest) {
    try {
      await http.post<AuthResponse>('/auth/register', data)
      // 注册成功不自动登录，不保存 token
      // 用户需要手动登录
      return true
    } catch (error) {
      console.error('Register error:', error)
      throw error
    }
  }

  // 登出
  async function logout() {
    try {
      await http.post('/auth/logout')
    } catch (error) {
      console.error('Logout error:', error)
    } finally {
      clearAuth()
    }
  }

  // 并发 401 单飞：多个请求同时收到 401 时共享同一次 refresh，
  // 避免并发刷新互相顶掉对方的新 refresh_token
  let refreshInflight: Promise<boolean> | null = null

  // 刷新Token（单飞入口）
  function refreshAccessToken(): Promise<boolean> {
    if (refreshInflight) return refreshInflight
    refreshInflight = doRefreshAccessToken().finally(() => {
      refreshInflight = null
    })
    return refreshInflight
  }

  // 实际执行刷新
  async function doRefreshAccessToken() {
    if (!refreshToken.value) return false
    const originalRefresh = refreshToken.value

    try {
      const res = await http.post<AuthResponse>('/auth/refresh', {
        refresh_token: originalRefresh
      })
      if (refreshToken.value !== originalRefresh) return false
      if (res.data) {
        const { access_token: accessToken, refresh_token } = res.data
        token.value = accessToken
        refreshToken.value = refresh_token

        // 更新本地存储
        storage.set('token', accessToken)
        storage.set('refresh_token', refresh_token)

        return true
      }
      return false
    } catch (error) {
      console.error('Refresh token error:', error)
      if (refreshToken.value === originalRefresh) clearAuth()
      return false
    }
  }

  // 获取用户信息
  async function fetchUserInfo() {
    try {
      const res = await http.get<UserInfo>('/user/profile')
      if (res.data) {
        user.value = res.data
        storage.set('user', res.data)
      }
    } catch (error) {
      console.error('Fetch user info error:', error)
    }
  }

  // Only memberships verified by the server belong in the switcher.
  async function fetchTenants() {
    const identity = user.value?.id
    const items: Tenant[] = []
    let cursor = ''
    do {
      const res = await http.get<{items: Tenant[]; next_cursor: string}>('/user/tenants', { params: {cursor, page_size: 100} })
      if (identity !== user.value?.id) return
      items.push(...(res.data?.items || []))
      cursor = res.data?.next_cursor || ''
    } while (cursor)
    tenants.value = items
    const current = items.find(item => item.id === currentTenant.value?.id)
    if (current) currentTenant.value = current
    storage.set('tenants', items)
  }

  async function setCurrentTenant(tenant: Tenant) {
    if (tenant.id === currentTenant.value?.id) return
    const identity = token.value
    const res = await http.post<AuthResponse>('/auth/switch-tenant', {tenant_id: tenant.id})
    if (identity !== token.value || !res.data) throw new Error('登录状态已改变，请重试')
    const data = res.data
    if (data.tenant_id !== tenant.id) throw new Error('租户切换结果不匹配')
    token.value = data.access_token
    refreshToken.value = data.refresh_token
    user.value = data.user
    currentTenant.value = tenant
    storage.set('token', token.value)
    storage.set('refresh_token', refreshToken.value)
    storage.set('user', user.value)
    storage.set('current_tenant', tenant)
    resetAuthFailure()
  }

  // 清除认证信息
  function clearAuth() {
    token.value = ''
    refreshToken.value = ''
    user.value = null
    tenants.value = []
    currentTenant.value = null
    storage.remove('token')
    storage.remove('refresh_token')
    storage.remove('user')
    storage.remove('tenants')
    storage.remove('current_tenant')
  }

  return {
    // 状态
    token,
    refreshToken,
    user,
    tenants,
    currentTenant,

    // 计算属性
    isAuthenticated,
    username,
    email,

    // 方法
    checkAuth,
    login,
    register,
    logout,
    refreshAccessToken,
    fetchUserInfo,
    fetchTenants,
    setCurrentTenant,
    clearAuth
  }
})

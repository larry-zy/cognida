import { http } from '@/utils/request'
import type { LoginRequest, RegisterRequest, AuthResponse, UserInfo } from '@/types'

export const authApi = {
  /**
   * 用户登录
   */
  login(data: LoginRequest) {
    return http.post<AuthResponse>('/auth/login', data)
  },

  /**
   * 用户注册
   */
  register(data: RegisterRequest) {
    return http.post<AuthResponse>('/auth/register', data)
  },

  /**
   * 用户登出
   */
  logout() {
    return http.post('/auth/logout')
  },

  /**
   * 刷新Token
   */
  refreshToken(refreshToken: string) {
    return http.post<AuthResponse>('/auth/refresh', { refresh_token: refreshToken })
  },

  /**
   * 获取当前用户信息
   */
  getProfile() {
    return http.get<UserInfo>('/user/profile')
  }
}

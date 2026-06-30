import axios, { AxiosInstance, AxiosRequestConfig, AxiosResponse, AxiosError } from 'axios'
import { ElMessage } from '@/utils/element'
import { useAuthStore } from '@/stores/auth'

// 响应数据接口
export interface ApiResponse<T = any> {
  code: number
  message: string
  data?: T
  error?: string
}

// 获取 API 基础 URL
const getBaseURL = () => {
  // 开发环境：使用 Vite proxy（通过相对路径访问）
  if (import.meta.env.DEV) {
    return '/api/v1'
  }
  // 生产环境：使用相对路径（Go 静态文件服务）
  return '/api/v1'
}

// 创建axios实例
const service: AxiosInstance = axios.create({
  baseURL: getBaseURL(),
  timeout: 60000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// 请求拦截器
service.interceptors.request.use(
  (config) => {
    const authStore = useAuthStore()
    // 添加Token到请求头
    if (authStore.token) {
      config.headers.Authorization = `Bearer ${authStore.token}`
    }
    // 添加租户ID（如果有）
    if (authStore.currentTenant) {
      config.headers['X-Tenant-ID'] = authStore.currentTenant.id
    }
    return config
  },
  (error) => {
    console.error('Request error:', error)
    return Promise.reject(error)
  }
)

// 响应拦截器
service.interceptors.response.use(
  (response: AxiosResponse): any => {
    const res = response.data as ApiResponse

    // 如果存在code字段且不为0，表示有错误
    if (res.code !== undefined && res.code !== 0) {
      const errorMsg = res.message || '请求失败'
      ElMessage.error(errorMsg)
      return Promise.reject(new Error(errorMsg))
    }

    // 成功响应，直接返回（message 可能是 "success" 等提示信息）
    return res
  },
  (error: AxiosError) => {
    const authStore = useAuthStore()

    // 处理401未授权错误
    if (error.response?.status === 401) {
      authStore.logout()
      ElMessage.error('登录已过期，请重新登录')
      return Promise.reject(error)
    }

    // 处理403禁止访问错误
    if (error.response?.status === 403) {
      ElMessage.error('没有权限访问')
      return Promise.reject(error)
    }

    // 处理404错误
    if (error.response?.status === 404) {
      ElMessage.error('请求的资源不存在')
      return Promise.reject(error)
    }

    // 处理500错误
    if (error.response?.status === 500) {
      ElMessage.error('服务器错误')
      return Promise.reject(error)
    }

    // 处理网络错误
    if (!error.response) {
      ElMessage.error('网络错误，请检查网络连接')
      return Promise.reject(error)
    }

    // 其他错误（不重复显示错误消息，由调用方处理）
    return Promise.reject(error)
  }
)

// 封装请求方法
export const http = {
  get<T = any>(url: string, config?: AxiosRequestConfig): Promise<ApiResponse<T>> {
    return service.get(url, config)
  },

  post<T = any>(url: string, data?: any, config?: AxiosRequestConfig): Promise<ApiResponse<T>> {
    return service.post(url, data, config)
  },

  put<T = any>(url: string, data?: any, config?: AxiosRequestConfig): Promise<ApiResponse<T>> {
    return service.put(url, data, config)
  },

  delete<T = any>(url: string, config?: AxiosRequestConfig): Promise<ApiResponse<T>> {
    return service.delete(url, config)
  },

  patch<T = any>(url: string, data?: any, config?: AxiosRequestConfig): Promise<ApiResponse<T>> {
    return service.patch(url, data, config)
  }
}

export default service

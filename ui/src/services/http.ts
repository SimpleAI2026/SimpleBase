import axios from 'axios'

const baseURL = import.meta.env.VITE_API_BASE_URL || ''

export { baseURL }

export const http = axios.create({ baseURL, timeout: 15000 })

// 默认 API key（DevMode 种子数据），可被 localStorage 覆盖
const DEFAULT_API_KEY = 'sb_live_dev_key_12345'
export function getApiKey(): string {
  return localStorage.getItem('sb_api_key') || DEFAULT_API_KEY
}
export function setApiKey(key: string) {
  localStorage.setItem('sb_api_key', key)
}

// 自动注入 Authorization: Bearer <key>
http.interceptors.request.use((config) => {
  config.headers = config.headers || {}
  config.headers.Authorization = `Bearer ${getApiKey()}`
  return config
})

// SPA fallback 防御：后端对不存在的路由返回 HTML，axios 按 text/html 解析后 r.data 为字符串
// 此类响应一律转为错误，避免调用方拿到非预期类型导致 .map() 崩溃
http.interceptors.response.use((r) => {
  const ct = r.headers?.['content-type'] || ''
  if (ct.includes('text/html')) {
    return Promise.reject(new Error('接口不存在或返回了 HTML 页面'))
  }
  return r
})

// 统一错误信息：适配后端 {error:{message}} 结构（501 响应无 request_id，解析容忍缺失）
let onUnauthorized: (() => void) | null = null

/** 注册 401 回调（auth store 注入，避免 http ↔ store 循环依赖） */
export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler
}

http.interceptors.response.use(
  (r) => r,
  (e) => {
    const status = e?.response?.status
    const data = e?.response?.data
    const msg = data?.error?.message || data?.message || e?.message || '网络请求失败'
    if (status === 401) {
      // 触发 auth store 打开 key 配置抽屉（延迟导入避免循环依赖）
      import('../stores/auth').then(({ useAuthStore }) => {
        useAuthStore().markUnauthorized()
      })
      onUnauthorized?.()
    }
    return Promise.reject(new Error(msg))
  }
)

export const wsBase =
  import.meta.env.VITE_WS_BASE_URL ||
  (location.origin.startsWith('https')
    ? location.origin.replace('https', 'wss')
    : location.origin.replace('http', 'ws'))

import axios from 'axios'

const baseURL = import.meta.env.VITE_API_BASE_URL || ''

export { baseURL }

export const http = axios.create({ baseURL, timeout: 15000 })

/* ---------- 登录态 token（login-auth-plan）：access JWT + refresh ---------- */

const ACCESS_KEY = 'sb_access_token'
const REFRESH_KEY = 'sb_refresh_token'
// 兼容旧 API Key 通道（SDK / DevMode）；登录态优先。
const API_KEY = 'sb_api_key'
const DEFAULT_API_KEY = 'sb_live_dev_key_12345'

export function getAccessToken(): string {
  return localStorage.getItem(ACCESS_KEY) || ''
}
export function getRefreshToken(): string {
  return localStorage.getItem(REFRESH_KEY) || ''
}
export function setTokens(access: string, refresh: string) {
  localStorage.setItem(ACCESS_KEY, access)
  localStorage.setItem(REFRESH_KEY, refresh)
}
export function clearTokens() {
  localStorage.removeItem(ACCESS_KEY)
  localStorage.removeItem(REFRESH_KEY)
}

export function getApiKey(): string {
  return localStorage.getItem(API_KEY) || DEFAULT_API_KEY
}
export function setApiKey(key: string) {
  localStorage.setItem(API_KEY, key)
}

// 自动注入 Authorization: Bearer <access_token | api-key>
http.interceptors.request.use((config) => {
  config.headers = config.headers || {}
  const token = getAccessToken() || getApiKey()
  config.headers.Authorization = `Bearer ${token}`
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

// 统一错误信息 + 401 自动 refresh 一次
let onUnauthorized: (() => void) | null = null
let refreshing: Promise<string | null> | null = null

/** 注册 401 回调（auth store 注入，避免 http ↔ store 循环依赖） */
export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler
}

async function tryRefresh(): Promise<string | null> {
  const rt = getRefreshToken()
  if (!rt) return null
  if (refreshing) return refreshing
  refreshing = axios
    .post(`${baseURL}/v1/auth/refresh`, { refresh_token: rt }, { timeout: 10000 })
    .then((r) => {
      const access = r.data?.access_token || ''
      const refresh = r.data?.refresh_token || rt
      if (access) setTokens(access, refresh)
      return access || null
    })
    .catch(() => {
      clearTokens()
      return null
    })
    .finally(() => {
      refreshing = null
    })
  return refreshing
}

http.interceptors.response.use(
  (r) => r,
  async (e) => {
    const status = e?.response?.status
    const data = e?.response?.data
    const msg = data?.error?.message || data?.message || e?.message || '网络请求失败'
    const cfg = e?.config || {}
    if (status === 401 && !cfg.__retried && getAccessToken()) {
      cfg.__retried = true
      const access = await tryRefresh()
      if (access) {
        cfg.headers = cfg.headers || {}
        cfg.headers.Authorization = `Bearer ${access}`
        return http.request(cfg)
      }
      import('../stores/auth').then(({ useAuthStore }) => {
        useAuthStore().markUnauthorized()
      })
      onUnauthorized?.()
      return Promise.reject(new Error(msg))
    }
    if (status === 401) {
      import('../stores/auth').then(({ useAuthStore }) => {
        useAuthStore().markUnauthorized()
      })
      onUnauthorized?.()
    }
    return Promise.reject(new Error(msg))
  }
)

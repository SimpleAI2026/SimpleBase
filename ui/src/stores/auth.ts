import { defineStore } from 'pinia'
import { getApiKey, setApiKey } from '../services/http'

/** API key 管理：localStorage 持久化 + 401 状态 + 配置抽屉开关。 */
export const useAuthStore = defineStore('auth', {
  state: () => ({
    apiKey: getApiKey(),
    /** 最近一次 401（invalid_api_key / unauthenticated）时间戳；用于 Header 提示 */
    lastUnauthorizedAt: 0,
    /** key 配置抽屉开关（401 时由 http 拦截器触发打开） */
    keyDrawerOpen: false
  }),
  actions: {
    updateKey(key: string) {
      const v = key.trim()
      if (!v) return
      setApiKey(v)
      this.apiKey = v
      this.lastUnauthorizedAt = 0
    },
    markUnauthorized() {
      this.lastUnauthorizedAt = Date.now()
      this.keyDrawerOpen = true
    },
    openDrawer() {
      this.keyDrawerOpen = true
    },
    closeDrawer() {
      this.keyDrawerOpen = false
    }
  }
})

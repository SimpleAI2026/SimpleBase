import { defineStore } from 'pinia'
import { getApiKey, setApiKey } from '../services/http'

export type SettingsTab = 'connection' | 'appearance' | 'models' | 'providers'

/** API key 管理：localStorage 持久化 + 401 状态 + 全局设置弹窗开关。 */
export const useAuthStore = defineStore('auth', {
  state: () => ({
    apiKey: getApiKey(),
    /** 最近一次 401（invalid_api_key / unauthenticated）时间戳；用于 Header 提示 */
    lastUnauthorizedAt: 0,
    /** 全局设置 SbModal 开关（401 时由 http 拦截器触发打开） */
    settingsOpen: false,
    settingsTab: 'connection' as SettingsTab
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
      this.openSettings({ tab: 'connection' })
    },
    openSettings(opts?: { tab?: SettingsTab }) {
      if (opts?.tab) this.settingsTab = opts.tab
      this.settingsOpen = true
    },
    closeSettings() {
      this.settingsOpen = false
    }
  }
})

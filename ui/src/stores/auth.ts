import { defineStore } from 'pinia'
import { api } from '../services/api'
import {
  clearTokens,
  getAccessToken,
  getApiKey,
  getRefreshToken,
  setApiKey,
  setTokens
} from '../services/http'
import type { AuthUser, UserRole } from '../services/types'

export type SettingsTab = 'connection' | 'models' | 'providers'

/** 登录态 + 角色（login-auth-plan）。API Key 设置仍保留在 SettingsModal。 */
export const useAuthStore = defineStore('auth', {
  state: () => ({
    accessToken: getAccessToken(),
    refreshToken: getRefreshToken(),
    apiKey: getApiKey(),
    user: null as AuthUser | null,
    /** 最近一次 401 时间戳；Header 提示 / 打开登录 */
    lastUnauthorizedAt: 0,
    /** 登录弹窗开关 */
    loginOpen: false,
    loginReason: '' as string,
    /** 强制改密（must_change_password） */
    mustChangePassword: false,
    /** 全局设置 SbModal 开关 */
    settingsOpen: false,
    settingsTab: 'connection' as SettingsTab
  }),
  getters: {
    isAuthenticated: (s) => Boolean(s.accessToken && s.user),
    role: (s): UserRole | '' => (s.user?.role as UserRole) || '',
    isSuper(): boolean {
      return this.role === 'superadminl1'
    },
    /** 角色 admin（只读管理员）。与 project.isAdmin（admin 项目）语义不同。 */
    isAdminRole(): boolean {
      return this.role === 'admin'
    },
    isUser(): boolean {
      return this.role === 'user'
    },
    canManageUsers(): boolean {
      return this.isSuper
    },
    canViewUsers(): boolean {
      return this.isSuper || this.isAdminRole
    },
    canWrite(): boolean {
      // 未登录（API Key 通道）沿旧行为可写；登录态下 admin 只读。
      if (!this.user) return true
      return this.isSuper || this.isUser
    },
    canCreateProject(): boolean {
      if (!this.user) return true
      return this.isSuper || this.isUser
    },
    displayName(): string {
      if (!this.user) return ''
      return this.user.displayName || this.user.username
    }
  },
  actions: {
    openLogin(reason?: string) {
      this.loginReason = reason || ''
      this.loginOpen = true
    },
    closeLogin() {
      this.loginOpen = false
      this.loginReason = ''
    },
    markUnauthorized() {
      this.lastUnauthorizedAt = Date.now()
      this.user = null
      clearTokens()
      this.accessToken = ''
      this.refreshToken = ''
      // 会话失效只弹登录框；设置弹窗由用户从顶栏齿轮自行打开
      // （planv5.0 §2.2：此前同时开两个弹窗，输错密码就会被告知去配 API Key）。
      this.openLogin('登录态已失效，请重新登录')
    },
    /** API Key 通道（Settings 连接页 / DevMode） */
    updateKey(key: string) {
      const v = key.trim()
      if (!v) return
      setApiKey(v)
      this.apiKey = v
      this.lastUnauthorizedAt = 0
    },
    applyTokens(pair: {
      accessToken: string
      refreshToken: string
      user: AuthUser
    }) {
      setTokens(pair.accessToken, pair.refreshToken)
      this.accessToken = pair.accessToken
      this.refreshToken = pair.refreshToken
      this.user = pair.user
      this.mustChangePassword = Boolean(pair.user?.mustChangePassword)
      this.lastUnauthorizedAt = 0
      this.closeLogin()
    },
    async login(username: string, password: string) {
      const pair = await api.auth.login({ username, password })
      this.applyTokens(pair)
      return pair.user
    },
    async logout() {
      try {
        await api.auth.logout(this.refreshToken || undefined)
      } catch {
        // 忽略登出网络错误
      }
      this.user = null
      this.accessToken = ''
      this.refreshToken = ''
      clearTokens()
      this.mustChangePassword = false
      this.openLogin()
    },
    async bootstrap() {
      if (!this.accessToken) {
        this.openLogin()
        return false
      }
      try {
        const me = await api.auth.me()
        this.user = {
          id: me.id,
          username: me.username,
          role: me.role,
          displayName: me.displayName,
          email: me.email,
          status: me.status,
          mustChangePassword: me.mustChangePassword,
          createdAt: me.createdAt,
          lastLoginAt: me.lastLoginAt
        }
        this.mustChangePassword = Boolean(me.mustChangePassword)
        this.closeLogin()
        return true
      } catch {
        this.user = null
        clearTokens()
        this.accessToken = ''
        this.refreshToken = ''
        this.openLogin()
        return false
      }
    },
    async changePassword(oldPassword: string, newPassword: string) {
      await api.auth.changePassword(oldPassword, newPassword)
      this.mustChangePassword = false
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

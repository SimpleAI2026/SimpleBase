import { defineStore } from 'pinia'
import { isLocaleCode, setI18nLocale, type LocaleCode } from '../locales'

export type ThemeMode = 'light' | 'dark' | 'system'

/**
 * 项目级模型配置缓存（服务端 sys_llm_settings 为准，这里只做首屏与离线兜底）。
 *
 * `apiKeyMasked` 是服务端回的脱敏串（如 sk-...abcd），Key 原文只存在服务端，
 * 浏览器侧永远拿不到，所以本地缓存也没有敏感数据。
 */
export interface ProjectLlmDefaults {
  defaultProvider?: string
  defaultModel?: string
  baseUrl?: string
  apiKeyMasked?: string
  hasApiKey?: boolean
  temperature?: number
  maxTokens?: number
}

interface SettingsState {
  theme: ThemeMode
  /** 界面语言（默认中文，不做浏览器探测，见 plan/planv5.0/ui-optimization-v3-plan.md） */
  locale: LocaleCode
  /** projectId → 默认模型与连接信息 */
  projectDefaults: Record<string, ProjectLlmDefaults>
}

const STORAGE_KEY = 'sb_settings_v1'

function load(): SettingsState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return emptyState()
    const parsed = JSON.parse(raw) as Partial<SettingsState>
    return {
      theme: parsed.theme === 'dark' || parsed.theme === 'system' || parsed.theme === 'light' ? parsed.theme : 'light',
      // 旧本地数据没有 locale 字段：必须补默认值，否则开关读到 undefined
      locale: isLocaleCode(parsed.locale) ? parsed.locale : 'zh-CN',
      projectDefaults: parsed.projectDefaults || {}
    }
  } catch {
    return emptyState()
  }
}

function emptyState(): SettingsState {
  return { theme: 'light', locale: 'zh-CN', projectDefaults: {} }
}

function resolveSystemDark(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches
}

export function resolvedTheme(mode: ThemeMode): 'light' | 'dark' {
  if (mode === 'system') return resolveSystemDark() ? 'dark' : 'light'
  return mode
}

export const useSettingsStore = defineStore('settings', {
  state: (): SettingsState => load(),
  getters: {
    effectiveTheme(state): 'light' | 'dark' {
      return resolvedTheme(state.theme)
    },
    defaultsFor:
      (state) =>
      (projectId: string): ProjectLlmDefaults =>
        state.projectDefaults[projectId] || {}
  },
  actions: {
    persist() {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          theme: this.theme,
          locale: this.locale,
          projectDefaults: this.projectDefaults
        })
      )
      this.applyThemeToDom()
    },
    setTheme(mode: ThemeMode) {
      this.theme = mode
      this.persist()
    },
    /** 切换界面语言：store、i18n 实例、<html lang> 三处同步，避免漂移。 */
    setLocale(next: LocaleCode) {
      this.locale = next
      setI18nLocale(next)
      this.persist()
    },
    /** 启动时把已持久化的语言应用到 i18n 实例与 <html lang>。 */
    applyLocaleToI18n() {
      setI18nLocale(this.locale)
    },
    applyThemeToDom() {
      const dark = this.effectiveTheme === 'dark'
      document.documentElement.classList.toggle('dark', dark)
      document.documentElement.removeAttribute('data-theme')
    },
    setProjectDefaults(projectId: string, patch: ProjectLlmDefaults) {
      const cur = { ...(this.projectDefaults[projectId] || {}), ...patch }
      this.projectDefaults = { ...this.projectDefaults, [projectId]: cur }
      this.persist()
    }
  }
})

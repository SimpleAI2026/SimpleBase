import { defineStore } from 'pinia'
import { LLM_PROVIDER_PRESETS, type LlmProviderFieldKey } from '../constants/llmProviders'

export type ThemeMode = 'light' | 'dark' | 'system'

export type ProviderCredentials = Partial<Record<LlmProviderFieldKey, string>>

export interface ProviderLocalConfig {
  enabled: boolean
  defaultModel?: string
  credentials: ProviderCredentials
  updatedAt: string
}

export interface ProjectLlmDefaults {
  defaultProvider?: string
  defaultModel?: string
  temperature?: number
  maxTokens?: number
}

interface SettingsState {
  theme: ThemeMode
  /** projectId → 默认模型/供应商 */
  projectDefaults: Record<string, ProjectLlmDefaults>
  /** projectId → providerId → 本地配置（含 key，仅浏览器） */
  providerConfigs: Record<string, Record<string, ProviderLocalConfig>>
}

const STORAGE_KEY = 'sb_settings_v1'

function load(): SettingsState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return emptyState()
    const parsed = JSON.parse(raw) as Partial<SettingsState>
    return {
      theme: parsed.theme === 'dark' || parsed.theme === 'system' || parsed.theme === 'light' ? parsed.theme : 'light',
      projectDefaults: parsed.projectDefaults || {},
      providerConfigs: parsed.providerConfigs || {}
    }
  } catch {
    return emptyState()
  }
}

function emptyState(): SettingsState {
  return { theme: 'light', projectDefaults: {}, providerConfigs: {} }
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
        state.projectDefaults[projectId] || {},
    configsFor:
      (state) =>
      (projectId: string): Record<string, ProviderLocalConfig> =>
        state.providerConfigs[projectId] || {}
  },
  actions: {
    persist() {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          theme: this.theme,
          projectDefaults: this.projectDefaults,
          providerConfigs: this.providerConfigs
        })
      )
      this.applyThemeToDom()
    },
    setTheme(mode: ThemeMode) {
      this.theme = mode
      this.persist()
    },
    applyThemeToDom() {
      const effective = this.effectiveTheme
      document.documentElement.setAttribute('data-theme', effective)
    },
    setProjectDefaults(projectId: string, patch: ProjectLlmDefaults) {
      const cur = { ...(this.projectDefaults[projectId] || {}), ...patch }
      this.projectDefaults = { ...this.projectDefaults, [projectId]: cur }
      this.persist()
    },
    upsertProviderConfig(projectId: string, providerId: string, patch: Partial<ProviderLocalConfig>) {
      const bucket = { ...(this.providerConfigs[projectId] || {}) }
      const prev = bucket[providerId] || {
        enabled: true,
        credentials: {},
        updatedAt: new Date().toISOString()
      }
      bucket[providerId] = {
        enabled: patch.enabled ?? prev.enabled,
        defaultModel: patch.defaultModel !== undefined ? patch.defaultModel : prev.defaultModel,
        credentials: { ...prev.credentials, ...(patch.credentials || {}) },
        updatedAt: new Date().toISOString()
      }
      this.providerConfigs = { ...this.providerConfigs, [projectId]: bucket }
      this.persist()
    },
    clearProviderCredentials(projectId: string, providerId: string) {
      const bucket = { ...(this.providerConfigs[projectId] || {}) }
      if (!bucket[providerId]) return
      bucket[providerId] = {
        ...bucket[providerId],
        credentials: {},
        updatedAt: new Date().toISOString()
      }
      this.providerConfigs = { ...this.providerConfigs, [projectId]: bucket }
      this.persist()
    },
    setDefaultProvider(projectId: string, providerId: string) {
      const preset = LLM_PROVIDER_PRESETS.find((p) => p.id === providerId)
      const cfg = (this.providerConfigs[projectId] || {})[providerId]
      const model =
        cfg?.defaultModel ||
        preset?.suggestedModels[0] ||
        cfg?.credentials.default_model ||
        undefined
      this.setProjectDefaults(projectId, { defaultProvider: providerId, defaultModel: model })
    },
    isProviderConfigured(projectId: string, providerId: string): boolean {
      const cfg = (this.providerConfigs[projectId] || {})[providerId]
      if (!cfg) return false
      const preset = LLM_PROVIDER_PRESETS.find((p) => p.id === providerId)
      if (!preset) return Boolean(cfg.credentials.api_key)
      return preset.fields
        .filter((f) => f.required)
        .every((f) => Boolean((cfg.credentials[f.key] || '').trim()))
    }
  }
})

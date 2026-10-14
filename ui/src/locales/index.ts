/**
 * i18n 实例与运行时语言切换。
 *
 * - 语言只有 zh-CN / en-US 两种，默认中文，不做浏览器探测（planv5.0 §UI-v3）。
 * - 语言包静态导入：控制台是单实例应用，语言包为数百条短字符串，
 *   懒加载只会引入额外 chunk 与切换闪烁。
 * - 组件内用 useI18n()；组件外（services / utils / composables / store getter）
 *   一律用导出的 i18n.global.t —— 那里没有组件注入上下文。
 */
import { createI18n } from 'vue-i18n'
import zhCN from './zh-CN'
import enUS from './en-US'
import type { LocaleCode } from './types'

export type { LocaleCode, MessageTree } from './types'

/** 支持的语言；顺序即设置面板展示顺序。 */
export const LOCALES: LocaleCode[] = ['zh-CN', 'en-US']

/** 语言展示名（自身语言书写，不随当前语言变化）。 */
export const LOCALE_LABELS: Record<LocaleCode, string> = {
  'zh-CN': '简体中文',
  'en-US': 'English'
}

/** <html lang> 用的值：zh-CN 原样，en-US 取主语言段。 */
export function htmlLang(locale: LocaleCode): string {
  return locale === 'zh-CN' ? 'zh-CN' : 'en'
}

export function isLocaleCode(value: unknown): value is LocaleCode {
  return value === 'zh-CN' || value === 'en-US'
}

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: 'zh-CN',
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS
  }
})

/**
 * 切换语言并同步 <html lang>。
 * 由 stores/settings.ts 的 setLocale 调用，保证 store 与 i18n 实例始终一致。
 */
export function setI18nLocale(locale: LocaleCode) {
  i18n.global.locale.value = locale
  if (typeof document !== 'undefined') {
    document.documentElement.lang = htmlLang(locale)
  }
}

/** 组件外的翻译入口（services / utils / composables / store getter）。 */
export function t(key: string, named?: Record<string, unknown>): string {
  return named ? i18n.global.t(key, named) : i18n.global.t(key)
}

/**
 * 「key 或已是文案」的统一解析，用于兜底文案入口。
 *
 * `useLoadState({ fallback })`、`errorMessage(e, fallback)` 这类位置可能收到三种值：
 *   1. i18n key（`components.documentList.loadFailed`）—— 需要翻译；
 *   2. 后端返回的裸文案或尚未迁移的历史字面量 —— 必须原样保留，不能翻成 key；
 *   3. 空串 —— 直接用 key 自身兜底。
 * 先查 key 表，命中即翻译，否则原样返回。
 */
export function resolveText(value: string): string {
  if (!value) return value
  try {
    return i18n.global.te(value) ? i18n.global.t(value) : value
  } catch {
    return value
  }
}

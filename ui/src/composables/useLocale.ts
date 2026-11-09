/**
 * 界面语言的读写封装。
 *
 * 职责：把「settings store 的 locale」与「i18n 实例 + <html lang>」绑在一起，
 * 页面与组件只调这里，不直接碰 i18n 实例，避免两处状态漂移。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSettingsStore } from '../stores/settings'
import { LOCALES, LOCALE_LABELS, type LocaleCode } from '../locales'

export interface LocaleOption {
  value: LocaleCode
  label: string
}

/** 可选项列表，供设置面板渲染。 */
export const localeOptions: LocaleOption[] = LOCALES.map((value) => ({
  value,
  label: LOCALE_LABELS[value]
}))

export function useLocale() {
  const settings = useSettingsStore()
  const { t, locale } = useI18n()

  const current = computed<LocaleCode>(() => settings.locale)

  function setLocale(next: LocaleCode) {
    settings.setLocale(next)
  }

  return {
    /** 当前语言（响应式） */
    locale,
    current,
    options: localeOptions,
    setLocale,
    t
  }
}

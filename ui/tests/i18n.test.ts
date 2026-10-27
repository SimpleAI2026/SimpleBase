import { describe, expect, it } from 'vitest'
import { effectScope } from 'vue'
import { createI18n } from 'vue-i18n'
import zhCN from '@/locales/zh-CN'
import enUS from '@/locales/en-US'
import { LOCALES, LOCALE_LABELS, htmlLang, i18n, isLocaleCode, setI18nLocale, t } from '@/locales'
import { agentErrorText } from '@/utils/agent-errors'
import { cronStatusText, logLevelText, statusText } from '@/lib/status'
import { currentBcp47, formatCount } from '@/utils/format'
import { relativeTime } from '@/utils/relative-time'

type Tree = { [key: string]: string | Tree }

/** 展平成 `a.b.c` → 文案，便于比对两棵树的 key 集合。 */
function flatten(tree: Tree, prefix = ''): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(tree)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (typeof value === 'string') out[path] = value
    else Object.assign(out, flatten(value, path))
  }
  return out
}

const zhFlat = flatten(zhCN as Tree)
const enFlat = flatten(enUS as Tree)

describe('i18n 语言包', () => {
  it('zh-CN 与 en-US 的 key 集合完全一致', () => {
    const zhKeys = Object.keys(zhFlat).sort()
    const enKeys = Object.keys(enFlat).sort()
    expect(enKeys).toEqual(zhKeys)
    expect(zhKeys.length).toBeGreaterThan(300)
  })

  it('没有空文案', () => {
    for (const [key, value] of Object.entries(zhFlat)) {
      expect(value.trim(), `zh-CN ${key} 为空`).not.toBe('')
    }
    for (const [key, value] of Object.entries(enFlat)) {
      expect(value.trim(), `en-US ${key} 为空`).not.toBe('')
    }
  })

  it('英文侧没有残留中文', () => {
    const offenders = Object.entries(enFlat)
      .filter(([, value]) => /[\u4e00-\u9fff]/.test(value))
      .map(([key, value]) => `${key}: ${value}`)
    expect(offenders).toEqual([])
  })

  it('两棵树使用相同的插值参数', () => {
    // 只比插值名，不比顺序：顺序差异不影响渲染。
    const names = (v: string) => (v.match(/\{(\w+)\}/g) || []).sort()
    for (const key of Object.keys(zhFlat)) {
      expect(names(enFlat[key]), `插值参数不一致：${key}`).toEqual(names(zhFlat[key]))
    }
  })

  it('英文侧没有遗留的转义花括号写法', () => {
    // 早期实现用过 {'{'} 形式，esbuild 会在单引号串里直接报错；
    // 这里额外守住「不要出现这类片段」的约定。
    for (const [key, value] of Object.entries(enFlat)) {
      expect(value, `${key} 不应含花括号转义片段`).not.toContain("{'{'}")
    }
  })

  it('每条消息都能被 vue-i18n 编译', () => {
    // vue-i18n 的 message compiler 把 `@` 当 linked message 起始符，
    // 裸写 `输入 @ 点名` 会在渲染时抛 "Invalid linked format" 且只在
    // 组件真正渲染到那一句时才炸 —— 这条用例把编译期前移到这里。
    //
    // t() 对编译失败只走 onError 回调（默认 console.warn），不抛；
    // 因此用一个把 onError 重新抛出的独立实例来做硬校验。
    const strict = createI18n({
      legacy: false,
      locale: 'zh-CN',
      messages: { 'zh-CN': zhCN } as never,
      onError: (err: unknown) => {
        throw err
      }
    })
    const bad: string[] = []
    for (const key of Object.keys(zhFlat)) {
      try {
        ;(strict.global as unknown as { t: (k: string) => string }).t(key)
      } catch (e) {
        bad.push(`${key}: ${(e as Error).message}`)
      }
    }
    expect(bad).toEqual([])
  })

  it('语言常量与辅助函数', () => {
    expect(LOCALES).toEqual(['zh-CN', 'en-US'])
    expect(LOCALE_LABELS['zh-CN']).toBe('简体中文')
    expect(LOCALE_LABELS['en-US']).toBe('English')
    expect(htmlLang('zh-CN')).toBe('zh-CN')
    expect(htmlLang('en-US')).toBe('en')
    expect(isLocaleCode('zh-CN')).toBe(true)
    expect(isLocaleCode('en-US')).toBe(true)
    expect(isLocaleCode('fr-FR')).toBe(false)
    expect(isLocaleCode(undefined)).toBe(false)
  })
})

describe('setI18nLocale', () => {
  it('切换 locale 并同步 <html lang>', () => {
    setI18nLocale('en-US')
    expect(i18n.global.locale.value).toBe('en-US')
    expect(document.documentElement.lang).toBe('en')
    expect(t('common.ok')).toBe('OK')
    expect(t('pages.databases.subtitle')).toContain('DuckLake')

    setI18nLocale('zh-CN')
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(t('common.ok')).toBe('确定')
  })

  it('支持具名插值', () => {
    setI18nLocale('zh-CN')
    expect(t('common.totalCount', { n: 12 })).toBe('共 12 条')
    setI18nLocale('en-US')
    expect(t('common.totalCount', { n: 12 })).toBe('12 total')
  })
})

describe('文案出口随语言变化', () => {
  it('status / logLevel / cron 三组映射', () => {
    setI18nLocale('zh-CN')
    expect(statusText('ready')).toBe('就绪')
    expect(logLevelText('warn')).toBe('警告')
    expect(cronStatusText('completed')).toBe('成功')

    setI18nLocale('en-US')
    expect(statusText('ready')).toBe('Ready')
    expect(statusText('closed')).toBe('closed')
    expect(logLevelText('error')).toBe('Error')
    expect(logLevelText('')).toBe('-')
    expect(cronStatusText('')).toBe('Not run')
  })

  it('agentErrorText 已知码走 i18n、未知码回退后端原文', () => {
    setI18nLocale('zh-CN')
    expect(agentErrorText({ code: 'llm_timeout' })).toContain('超时')
    expect(agentErrorText({ code: 'nope', message: 'boom' })).toBe('boom')

    setI18nLocale('en-US')
    expect(agentErrorText({ code: 'llm_timeout' })).toContain('timed out')
    expect(agentErrorText('llm_rate_limited')).toContain('rate limited')
    expect(agentErrorText(null)).toBe('Run failed')
  })

  it('formatCount 用当前 locale 排版数字并翻译单位', () => {
    setI18nLocale('zh-CN')
    expect(currentBcp47()).toBe('zh-CN')
    expect(formatCount(1234)).toBe(`1,234 条`)

    setI18nLocale('en-US')
    expect(currentBcp47()).toBe('en-US')
    expect(formatCount(1234)).toBe('1,234')
  })

  it('relativeTime 随语言变化', () => {
    const now = new Date()
    setI18nLocale('zh-CN')
    expect(relativeTime(now.toISOString())).toBe('刚刚')
    setI18nLocale('en-US')
    expect(relativeTime(now.toISOString())).toBe('just now')
    expect(relativeTime(undefined)).toBe('')
    expect(relativeTime('bad-date')).toBe('')
  })
})

describe('语言状态与渲染的联动', () => {
  it('切换后同一个 key 重新求值', () => {
    const scope = effectScope()
    const seen: string[] = []
    scope.run(() => {
      // 直接用 t() 读取，切换后再读一次，验证 global 实例已生效
      seen.push(t('nav.agents'))
      setI18nLocale('en-US')
      seen.push(t('nav.agents'))
      setI18nLocale('zh-CN')
      seen.push(t('nav.agents'))
    })
    scope.stop()
    expect(seen).toEqual(['云助手', 'Cloud Assistant', '云助手'])
  })
})

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { i18n, setI18nLocale } from '@/locales'
import { uiStubs } from '@/test/helpers'
import AgentWelcome from '@/components/agent/AgentWelcome.vue'
import AgentConfirmCard from '@/components/agent/AgentConfirmCard.vue'
import { isEmptyResourceError, EMPTY_RESOURCE_CODES } from '@/utils/api-errors'
import { promptI18nKeys, promptKeysForModule, promptSetForModule } from '@/constants/agent-prompts'

/**
 * planv6.0 §2.2/§3.5/§3.6 新增的两个组件的自身契约，以及空资源错误判定。
 * 这些此前只被宿主测试间接覆盖，这里直接验证。
 */

beforeEach(() => {
  setActivePinia(createPinia())
  setI18nLocale('zh-CN')
})

describe('AgentWelcome', () => {
  const mountWelcome = (props: Record<string, unknown> = {}) =>
    mount(AgentWelcome, { props, global: { stubs: uiStubs, plugins: [i18n] } })

  it('按模块给出提示词，点一下抛 pick 且不自动发送', async () => {
    const w = mountWelcome({ agentName: 'Database', module: 'database' })
    expect(w.text()).toContain('你好，我是Database')
    // database 模块 4 条
    const buttons = w.findAll('button')
    expect(buttons).toHaveLength(4)
    expect(w.text()).toContain('列出每个库的表和字段')
    await buttons[0].trigger('click')
    expect(w.emitted('pick')).toEqual([['列出每个库的表和字段']])
    // 只抛事件，不产生发送
    expect(w.emitted('send')).toBeUndefined()
    w.unmount()
  })

  it('未登记的 module 回落通用助手（5 条）', () => {
    const w = mountWelcome({ agentName: 'X', module: 'nope' })
    expect(w.findAll('button')).toHaveLength(5)
    expect(w.text()).toContain('列出项目里所有数据库和集合')
    w.unmount()
  })

  it('没有 agentName / module 时用默认名与通用提示词', () => {
    const w = mountWelcome()
    expect(w.text()).toContain('你好，我是云助手')
    expect(w.findAll('button')).toHaveLength(5)
    w.unmount()
  })

  it('预设的 i18n key 在两个语言树下都有文案', () => {
    for (const locale of ['zh-CN', 'en-US'] as const) {
      setI18nLocale(locale)
      for (const module of ['general', 'database', 'kv', 'gofunction', 'cron', 's3', 'logs', 'sandbox']) {
        for (const key of promptI18nKeys(module)) {
          expect(i18n.global.t(key)).not.toBe(key)
        }
      }
    }
  })
})

describe('agent-prompts 常量', () => {
  it('按 module 取 key，未知 module 回落 general', () => {
    expect(promptKeysForModule('DATABASE')).toEqual(['listTables', 'rowCounts', 'emptyTables', 'previewRows'])
    expect(promptKeysForModule('')).toEqual(promptKeysForModule('general'))
    expect(promptKeysForModule('unknown')).toEqual(promptKeysForModule('general'))
    expect(promptSetForModule('unknown')).toBe('general')
    expect(promptSetForModule('s3')).toBe('s3')
  })
})

describe('isEmptyResourceError', () => {
  it('只有登记过的 404 码算「空资源」', () => {
    expect(EMPTY_RESOURCE_CODES).toContain('kv_store_not_found')
    expect(isEmptyResourceError(Object.assign(new Error('x'), { status: 404, code: 'kv_store_not_found' }))).toBe(true)
  })

  it('其他 404 / 5xx / 无 code / 非对象一律不当作空', () => {
    expect(isEmptyResourceError(Object.assign(new Error('x'), { status: 404, code: 'kv_not_found' }))).toBe(false)
    expect(isEmptyResourceError(Object.assign(new Error('x'), { status: 404 }))).toBe(false)
    expect(isEmptyResourceError(Object.assign(new Error('x'), { status: 500, code: 'kv_store_not_found' }))).toBe(false)
    expect(isEmptyResourceError(new Error('boom'))).toBe(false)
    expect(isEmptyResourceError(null)).toBe(false)
    expect(isEmptyResourceError(undefined)).toBe(false)
    expect(isEmptyResourceError('404')).toBe(false)
  })
})

describe('AgentConfirmCard', () => {
  const mountCard = (props: Record<string, unknown> = {}) =>
    mount(AgentConfirmCard, {
      props: { message: '将执行', argv: ['database', 'delete', '--id', 'shop'], ...props },
      global: { stubs: uiStubs, plugins: [i18n] },
      attachTo: document.body
    })

  it('展示 argv 与说明，确认/取消各自抛事件', async () => {
    const w = mountCard()
    await flushPromises()
    expect(w.text()).toContain('将执行')
    expect(w.text()).toContain('database delete --id shop')
    const buttons = w.findAll('button')
    const confirm = buttons.find((b) => b.text() === '确认执行')!
    const cancel = buttons.find((b) => b.text() === '取消')!
    await confirm.trigger('click')
    await cancel.trigger('click')
    expect(w.emitted('confirm')).toHaveLength(1)
    expect(w.emitted('deny')).toHaveLength(1)
    w.unmount()
  })

  it('argv 为空时不渲染代码块', () => {
    const w = mountCard({ argv: [] })
    expect(w.find('pre').exists()).toBe(false)
    w.unmount()
  })

  it('Esc 等于取消', async () => {
    const w = mountCard()
    await w.get('[data-slot="agent-confirm-card"]').trigger('keydown', { key: 'Escape' })
    expect(w.emitted('deny')).toHaveLength(1)
    w.unmount()
  })
})

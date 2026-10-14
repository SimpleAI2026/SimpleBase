import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { setI18nLocale } from '@/locales'
import { uiStubs } from '@/test/helpers'
import SbErrorState from '@/components/SbErrorState.vue'

/**
 * SbErrorState：统一报错框（plan v3 §3.4）。
 * 收敛前 SbAsyncRegion 在 tbody 与 div 两个分支各写了一份相同标记，
 * 这里验证唯一实现的两个变体、默认文案与重试事件。
 */
describe('SbErrorState', () => {
  it('block 变体渲染默认标题与重试按钮并抛 retry', async () => {
    setI18nLocale('zh-CN')
    const w = mount(SbErrorState, { global: { stubs: uiStubs } })
    expect(w.text()).toContain('加载失败')
    expect(w.text()).toContain('重试')
    expect(w.attributes('data-slot')).toBe('sb-error-state')
    expect(w.classes()).toContain('flex-col')
    await w.get('button').trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
    w.unmount()
  })

  it('inline 变体限制宽度并居中（表格内嵌场景）', () => {
    const w = mount(SbErrorState, { props: { variant: 'inline' }, global: { stubs: uiStubs } })
    expect(w.classes()).toContain('max-w-lg')
    expect(w.classes()).toContain('items-center')
    w.unmount()
  })

  it('显式传入标题、描述与重试文案时覆盖默认值', () => {
    setI18nLocale('zh-CN')
    const w = mount(SbErrorState, {
      props: { title: '同步失败', description: '第 3 条记录冲突', retryText: '再试一次' },
      global: { stubs: uiStubs }
    })
    expect(w.text()).toContain('同步失败')
    expect(w.text()).toContain('第 3 条记录冲突')
    expect(w.text()).toContain('再试一次')
    w.unmount()
  })

  it('切换语言后默认文案随之变化', () => {
    setI18nLocale('en-US')
    const w = mount(SbErrorState, { global: { stubs: uiStubs } })
    expect(w.text()).toContain('Retry')
    expect(w.text()).not.toContain('重试')
    setI18nLocale('zh-CN')
    w.unmount()
  })
})

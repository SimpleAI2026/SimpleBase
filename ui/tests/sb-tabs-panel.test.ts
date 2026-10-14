import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SbTabsPanel from '@/components/SbTabsPanel.vue'

/**
 * SbTabsPanel：统一的多页签面板骨架（plan v3 §3.5）。
 * SettingsModal 与 KV 面板此前各自复制了「Tabs + TabsList + 内容容器」结构。
 */
const items = [
  { value: 'a', label: '甲' },
  { value: 'b', label: '乙' }
]

describe('SbTabsPanel', () => {
  it('渲染 items 并透传当前值', () => {
    const w = mount(SbTabsPanel, { props: { items, modelValue: 'a' } })
    const triggers = w.findAll('[data-slot="tabs-trigger"]')
    expect(triggers).toHaveLength(2)
    expect(w.text()).toContain('甲')
    expect(w.text()).toContain('乙')
    expect(triggers[0].attributes('data-state')).toBe('active')
    expect(triggers[1].attributes('data-state')).toBe('inactive')
    w.unmount()
  })

  it('具名 slot 命中时用它，未命中时回退到默认 slot', () => {
    // 只激活 a：a 有具名 slot → 用具名的，且不再渲染默认 slot。
    const withNamed = mount(SbTabsPanel, {
      props: { items, modelValue: 'a' },
      slots: {
        a: '<p class="named-a">具名 A</p>',
        default: '<p class="fallback">默认内容</p>'
      }
    })
    expect(withNamed.find('.named-a').exists()).toBe(true)
    expect(withNamed.find('.fallback').exists()).toBe(false)
    withNamed.unmount()

    // 激活 b：b 没有具名 slot → 回退到默认 slot。
    const fallbackOnly = mount(SbTabsPanel, {
      props: { items, modelValue: 'b' },
      slots: {
        a: '<p class="named-a">具名 A</p>',
        default: '<p class="fallback">默认内容</p>'
      }
    })
    expect(fallbackOnly.find('.fallback').exists()).toBe(true)
    expect(fallbackOnly.find('.named-a').exists()).toBe(false)
    fallbackOnly.unmount()
  })

  it('受控切换向上抛出 update:modelValue', async () => {
    const w = mount(SbTabsPanel, { props: { items, modelValue: 'a' } })
    await w.findAll('[data-slot="tabs-trigger"]')[1].trigger('mousedown')
    await w.findAll('[data-slot="tabs-trigger"]')[1].trigger('click')
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['b'])
    w.unmount()
  })

  it('line 变体使用下划线样式，contentMaxHeight 落到内容容器', () => {
    const w = mount(SbTabsPanel, {
      props: { items, modelValue: 'a', variant: 'line', contentMaxHeight: '320px' }
    })
    expect(w.get('[data-slot="tabs-list"]').attributes('data-variant')).toBe('line')
    const content = w.get('[data-slot="tabs-content"]').element.parentElement as HTMLElement
    expect(content.style.maxHeight).toBe('320px')
    expect(content.className).toContain('overflow-y-auto')
    w.unmount()
  })
})

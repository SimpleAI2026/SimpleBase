import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { uiStubs } from '../../test/helpers'
import MessageScroller from './MessageScroller.vue'

describe('chat primitives', () => {
  it('sticks to the bottom until the user scrolls away', async () => {
    const proto = HTMLElement.prototype as HTMLElement & { _st?: number }
    Object.defineProperty(proto, 'scrollHeight', { configurable: true, get: () => 500 })
    Object.defineProperty(proto, 'clientHeight', { configurable: true, get: () => 200 })
    Object.defineProperty(proto, 'scrollTop', {
      configurable: true,
      get() {
        return (this as { _st?: number })._st || 0
      },
      set(v: number) {
        ;(this as { _st?: number })._st = v
      }
    })
    const w = mount(MessageScroller, {
      props: { followKey: '1' },
      slots: { default: '<p>msg</p>' },
      global: { stubs: uiStubs }
    })
    const vp = w.get('div.max-h-\\[480px\\]')
    ;(vp.element as HTMLElement & { _st?: number })._st = 0
    await vp.trigger('scroll')
    expect(w.text()).toContain('跳到最新')
    await w.get('button').trigger('click')
    expect(w.find('button').exists()).toBe(false)
    await w.setProps({ followKey: '2' })
  })
})

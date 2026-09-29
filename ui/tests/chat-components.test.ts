import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { uiStubs } from '@/test/helpers'
import MessageScroller from '@/components/chat/MessageScroller.vue'

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
    // stick=false 时 followKey 变化不滚动；再触发 scroll 使 stick 保持 true 的分支
    await vp.trigger('scroll')
    await w.setProps({ followKey: '2' })
    // scrollToEnd 直接调用（stick=true 路径）
    const vm = w.vm as any
    vm.stick = true
    await vm.scrollToEnd()
    expect(vm.stick).toBe(true)
    // stick=false：followKey 变化走 early-return 分支
    vm.stick = false
    await w.setProps({ followKey: '3' })
    expect(vm.stick).toBe(false)
    w.unmount()
  })
})

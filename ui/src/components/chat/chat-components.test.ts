import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { uiStubs } from '../../test/helpers'
import Bubble from './Bubble.vue'
import Marker from './Marker.vue'
import Message from './Message.vue'
import MessageAvatar from './MessageAvatar.vue'
import MessageContent from './MessageContent.vue'
import MessageScroller from './MessageScroller.vue'

describe('chat primitives', () => {
  it('applies alignment and variant classes', () => {
    expect(mount(Message, { props: { align: 'end' }, slots: { default: 'm' } }).classes().join(' ')).toContain('flex-row-reverse')
    expect(mount(Bubble, { props: { variant: 'default' }, slots: { default: 'b' } }).text()).toContain('b')
    expect(mount(MessageAvatar, { props: { align: 'end' }, slots: { default: 'a' } }).text()).toContain('a')
    expect(mount(MessageContent, { slots: { default: 'c' } }).text()).toContain('c')
    expect(mount(Marker, { props: { variant: 'separator' }, slots: { default: 'today' }, global: { stubs: uiStubs } }).text()).toContain('today')
    expect(mount(Marker, { slots: { default: 'plain' }, global: { stubs: uiStubs } }).text()).toContain('plain')
  })

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

import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import DocsToc from '@/components/docs/DocsToc.vue'

const headings = [
  { id: 'a', text: '概述', level: 2 },
  { id: 'b', text: '步骤', level: 3 },
  { id: 'c', text: '示例', level: 2 }
]

describe('DocsToc', () => {
  it('only shows long articles and updates active state', async () => {
    const observed: Element[] = []
    const disconnect = vi.fn()
    let onIntersect: IntersectionObserverCallback = () => {}
    vi.stubGlobal('IntersectionObserver', class {
      constructor(callback: IntersectionObserverCallback) { onIntersect = callback }
      observe(element: Element) { observed.push(element) }
      disconnect = disconnect
    })
    const refs = headings.map((h) => {
      const element = document.createElement('h2'); element.id = h.id
      document.body.appendChild(element)
      return element
    })
    const wrapper = mount(DocsToc, { props: { headings: headings.slice(0, 2) } })
    expect(wrapper.find('aside').exists()).toBe(false)
    await wrapper.setProps({ headings })
    await flushPromises()
    expect(wrapper.text()).toContain('本页目录')
    expect(wrapper.findAll('a')).toHaveLength(3)
    expect(observed).toHaveLength(3)
    onIntersect([{ isIntersecting: true, target: refs[1] } as unknown as IntersectionObserverEntry], {} as IntersectionObserver)
    await flushPromises()
    expect(wrapper.find('a[href="#b"]').classes()).toContain('text-primary')
    await wrapper.find('a[href="#c"]').trigger('click')
    expect(wrapper.find('a[href="#c"]').classes()).toContain('text-primary')
    wrapper.unmount()
    expect(disconnect).toHaveBeenCalled()
    refs.forEach((element) => element.remove())
    vi.unstubAllGlobals()
  })
})

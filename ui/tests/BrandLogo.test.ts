import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import BrandLogo from '@/components/BrandLogo.vue'

describe('BrandLogo', () => {
  it('renders a flat mark and the product wordmark without serif, bold or shadow', () => {
    const w = mount(BrandLogo)
    expect(w.text()).toContain('SimpleBase')
    const html = w.html()
    expect(html).not.toContain('font-serif')
    expect(html).not.toContain('font-bold')
    expect(html).not.toContain('shadow')
    expect(w.get('[aria-hidden="true"]').text()).toBe('S')
  })

  it('uses the large size on the public pages', () => {
    const w = mount(BrandLogo, { props: { size: 'lg' } })
    expect(w.get('[aria-hidden="true"]').classes()).toContain('size-9')
    expect(w.findAll('span').at(-1)?.classes()).toContain('text-lg')
  })

  it('can show the mark only and accepts extra wordmark classes', () => {
    expect(mount(BrandLogo, { props: { markOnly: true } }).text()).toBe('S')
    const w = mount(BrandLogo, { props: { wordmarkClass: 'hidden sm:inline' } })
    expect(w.findAll('span').at(-1)?.classes()).toContain('hidden')
  })
})

import { describe, expect, it } from 'vitest'
import { mountWithApp } from '../test/helpers'
import DocsLayout from './DocsLayout.vue'

describe('DocsLayout', () => {
  it('renders the docs chrome and github link', async () => {
    const { wrapper } = await mountWithApp(DocsLayout, { path: '/docs' })
    expect(wrapper.text()).toContain('使用文档')
    expect(wrapper.text()).toContain('返回控制台')
    expect(wrapper.find('a[href="https://github.com/linkxzhou/SimpleBase"]').exists()).toBe(true)
  })
})

import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'
import Home from '@/pages/Home.vue'

async function mountHome() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: Home },
      { path: '/console/agents', name: 'agents', component: { template: '<div>agents</div>' } },
      { path: '/docs', name: 'docs', component: { template: '<div>docs</div>' } }
    ]
  })
  await router.push('/')
  await router.isReady()
  const wrapper = mount(Home, { global: { plugins: [router], stubs: { RouterLink: false } } })
  return { wrapper, router }
}

describe('Home', () => {
  it('shows the product and keeps the console action in the header', async () => {
    const { wrapper } = await mountHome()
    expect(wrapper.find('h1').text()).toContain('为 SaaS 构建的 AI 工作台')
    expect(wrapper.text()).toContain('只需承担对象存储成本')
    expect(wrapper.text()).toContain('云助手统一管理 SaaS 模块')
    expect(wrapper.findAll('article').map((item) => item.find('h3').text())).toEqual([
      'AI OS · 云助手', 'SaaS 模块协同', '低成本存储'
    ])
    expect(wrapper.find('h1 span').classes()).toContain('sm:whitespace-nowrap')
    expect(wrapper.findAll('aside .grid > div').map((item) => item.text())).toEqual([
      'AGENTS云助手', 'DATABASE数据库', 'STORAGE对象存储', 'FUNCTIONS云函数'
    ])
    expect(wrapper.findAll('aside .space-y-3 > div').map((item) => item.text())).toEqual([
      '01云助手READY', '02数据库READY', '03对象存储READY', '04云函数READY'
    ])
    expect(wrapper.find('header a[href="/console/agents"]').text()).toContain('控制台')
    expect(wrapper.find('main a[href="/console/agents"]').text()).toContain('进入控制台')
    expect(wrapper.find('a[href="/docs"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('数据库、对象存储与云函数')
    expect(wrapper.text()).toContain('云助手')
    expect(wrapper.find('aside[aria-label="产品能力示意"]').exists()).toBe(true)
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('navigates from the public homepage into cloud agents', async () => {
    const { wrapper, router } = await mountHome()
    await wrapper.find('header a[href="/console/agents"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('agents')
    await wrapper.find('main a[href="/console/agents"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/console/agents')
  })
})

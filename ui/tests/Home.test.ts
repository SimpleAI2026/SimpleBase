import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'
import Home from '@/pages/Home.vue'

async function mountHome() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: Home },
      { path: '/console', name: 'dashboard', component: { template: '<div>dashboard</div>' } },
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
    expect(wrapper.find('h1').text()).toContain('为 AI 数据和应用构建的一体化工作台')
    expect(wrapper.text()).toContain('从数据管理到 AI 应用构建')
    expect(wrapper.find('header a[href="/console"]').text()).toContain('控制台')
    expect(wrapper.find('main a[href="/console"]').text()).toContain('进入控制台')
    expect(wrapper.find('a[href="/docs"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('数据库管理')
    expect(wrapper.text()).toContain('云 Agent')
    expect(wrapper.find('aside[aria-label="产品能力示意"]').exists()).toBe(true)
    expect(wrapper.find('form').exists()).toBe(false)
  })

  it('navigates from the public homepage into the existing dashboard', async () => {
    const { wrapper, router } = await mountHome()
    await wrapper.find('header a[href="/console"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('dashboard')
  })
})

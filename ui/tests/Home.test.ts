import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import Home from '@/pages/Home.vue'
import HomeDemo from '@/components/home/HomeDemo.vue'

async function mountHome() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: Home },
      { path: '/console/agents', name: 'agents', component: { template: '<div>agents</div>' } },
      { path: '/docs/:pathMatch(.*)*', name: 'docs', component: { template: '<div>docs</div>' } }
    ]
  })
  await router.push('/')
  await router.isReady()
  const wrapper = mount(Home, { global: { plugins: [router], stubs: { RouterLink: false } } })
  return { wrapper, router }
}

interface DemoVm {
  stage: number
  beat: number
  state: string
}

function mockReducedMotion(reduce: boolean) {
  vi.spyOn(window, 'matchMedia').mockImplementation((q: string) => ({
    matches: reduce && q.includes('reduce'),
    media: q,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false
  }) as MediaQueryList)
}

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('Home', () => {
  it('uses the shared console theme, one headline and no duplicate header actions', async () => {
    const { wrapper } = await mountHome()
    expect(wrapper.classes()).not.toContain('sb-landing')
    expect(wrapper.find('header a[href="/docs"]').exists()).toBe(false)
    expect(wrapper.find('header a[href="/console/agents"]').exists()).toBe(false)
    expect(wrapper.find('h1').text()).toContain('为 SaaS 构建的 AI 工作台')
    // 标题不再整行染色
    expect(wrapper.find('h1 span').classes()).not.toContain('text-primary')
    expect(wrapper.find('main a[href="/console/agents"]').text()).toContain('进入工作台')
    expect(wrapper.find('main a[href="/docs/getting-started/local-cloud-collab"]').text()).toContain('查看上线指南')
    // 首屏不再重复三条卖点
    expect(wrapper.find('dl').exists()).toBe(false)
    expect(wrapper.find('[data-baseline]').exists()).toBe(false)
    expect(wrapper.get('[data-foundation-diagram]').text()).toContain('事件触发器')
  })

  it('tells three different capabilities with evidence and keeps the cost caveat next to the claim', async () => {
    const { wrapper } = await mountHome()
    const caps = wrapper.findAll('[data-capability]').map((a) => a.attributes('data-capability'))
    expect(caps).toEqual(['agent', 'ship', 'cost'])
    expect(wrapper.get('[data-capability="ship"]').text()).toContain('simplebase dev apply --dir . --yes')
    const cost = wrapper.get('[data-capability="cost"]').text()
    expect(cost).toContain('不必为每类模块分别购买托管存储')
    expect(cost).not.toContain('计算资源与模型调用费用另计')
    const html = wrapper.html()
    for (const bad of ['uppercase', 'blur-3xl', 'radial-gradient', 'backdrop-blur', 'animate-pulse', 'bg-gradient']) {
      expect(html).not.toContain(bad)
    }
  })

  it('navigates into the workbench', async () => {
    const { wrapper, router } = await mountHome()
    await wrapper.find('main a[href="/console/agents"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('agents')
  })
})

describe('HomeDemo', () => {
  it('plays idea → local → cloud, holds on the cloud result, then loops', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const wrapper = mount(HomeDemo)
    const vm = wrapper.vm as unknown as DemoVm
    expect(vm.state).toBe('playing')
    expect(wrapper.find('[data-ask]').exists()).toBe(true)
    expect(wrapper.text()).toContain('等待建表')

    await vi.advanceTimersByTimeAsync(600)
    expect(wrapper.text()).toContain('表已建好')
    await vi.advanceTimersByTimeAsync(600 * 2 + 1400)
    expect(vm.stage).toBe(1)
    expect(wrapper.find('[data-workspace]').text()).toContain('001_orders.sql')
    expect(wrapper.text()).toContain('simplebase dev apply')

    await vi.advanceTimersByTimeAsync(600 * 4 + 1400)
    expect(vm.stage).toBe(2)
    expect(wrapper.find('[data-foundation]').text()).toContain('对象存储')
    await vi.advanceTimersByTimeAsync(600)
    expect(vm.beat).toBe(1)
    // 云端结果停留，不提前切走
    await vi.advanceTimersByTimeAsync(4900)
    expect(vm.stage).toBe(2)
    await vi.advanceTimersByTimeAsync(100)
    expect(vm.stage).toBe(0)
    expect(vm.beat).toBe(0)
    expect(vm.state).toBe('playing')
    // 订单数据位置不变，始终在
    expect(wrapper.find('[data-app]').text()).toContain('#1042')
    wrapper.unmount()
  })

  it('keeps playing while the pointer rests on the demo (no hover pause)', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const wrapper = mount(HomeDemo)
    const vm = wrapper.vm as unknown as DemoVm
    await wrapper.trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(600 * 3)
    expect(vm.beat).toBe(3)
    expect(vm.state).toBe('playing')
    wrapper.unmount()
  })

  it('pauses and resumes with a real button, replays from the start, and jumps to a stage', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const wrapper = mount(HomeDemo)
    const vm = wrapper.vm as unknown as DemoVm
    await wrapper.get('[data-demo-toggle]').trigger('click')
    expect(vm.state).toBe('paused')
    expect(wrapper.get('[data-demo-toggle]').attributes('aria-label')).toBe('继续演示')
    await vi.advanceTimersByTimeAsync(5000)
    expect(vm.beat).toBe(0)

    // 暂停中跳阶段：展示完整状态，不自动推进
    await wrapper.get('[data-stage="2"]').trigger('click')
    expect(vm.stage).toBe(2)
    expect(vm.beat).toBe(1)
    expect(wrapper.get('[data-stage="2"]').attributes('aria-current')).toBe('step')
    await vi.advanceTimersByTimeAsync(10000)
    expect(vm.stage).toBe(2)

    await wrapper.get('[data-demo-toggle]').trigger('click')
    expect(vm.state).toBe('playing')
    // 播放中跳阶段：从该阶段开头播
    await wrapper.get('[data-stage="1"]').trigger('click')
    expect(vm.stage).toBe(1)
    expect(vm.beat).toBe(0)
    await vi.advanceTimersByTimeAsync(600)
    expect(vm.beat).toBe(1)

    await wrapper.get('[data-demo-toggle]').trigger('click')
    await wrapper.get('[data-demo-replay]').trigger('click')
    expect(vm.stage).toBe(0)
    expect(vm.beat).toBe(0)
    expect(vm.state).toBe('playing')
    await vi.advanceTimersByTimeAsync(600)
    expect(vm.beat).toBe(1)
    wrapper.unmount()
  })

  it('never stacks timers when stages are clicked rapidly', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const wrapper = mount(HomeDemo)
    const vm = wrapper.vm as unknown as DemoVm
    for (const i of [1, 0, 2, 1, 0]) await wrapper.get(`[data-stage="${i}"]`).trigger('click')
    expect(vi.getTimerCount()).toBe(1)
    await vi.advanceTimersByTimeAsync(600)
    expect(vm.stage).toBe(0)
    expect(vm.beat).toBe(1)
    wrapper.unmount()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('stops while the page is hidden', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const wrapper = mount(HomeDemo)
    const vm = wrapper.vm as unknown as DemoVm
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' })
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(5000)
    expect(vm.beat).toBe(0)
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'visible' })
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(600)
    expect(vm.beat).toBe(1)
    wrapper.unmount()
  })

  it('waits until it scrolls into view when IntersectionObserver exists', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    let cb: IntersectionObserverCallback = () => {}
    const disconnect = vi.fn()
    vi.stubGlobal('IntersectionObserver', class {
      constructor(c: IntersectionObserverCallback) { cb = c }
      observe() {}
      disconnect = disconnect
    })
    const wrapper = mount(HomeDemo)
    const vm = wrapper.vm as unknown as DemoVm
    await vi.advanceTimersByTimeAsync(3000)
    expect(vm.beat).toBe(0)
    cb([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver)
    await vi.advanceTimersByTimeAsync(600)
    expect(vm.beat).toBe(1)
    wrapper.unmount()
    expect(disconnect).toHaveBeenCalled()
    vi.unstubAllGlobals()
  })

  it('still autoplays when the system asks for reduced motion', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    mockReducedMotion(true)
    const wrapper = mount(HomeDemo)
    const vm = wrapper.vm as unknown as DemoVm
    expect(vm.stage).toBe(0)
    expect(vm.state).toBe('playing')
    await vi.advanceTimersByTimeAsync(600)
    expect(vm.beat).toBe(1)
    expect(wrapper.html()).toContain('motion-reduce:transition-none')
    wrapper.unmount()
  })
})

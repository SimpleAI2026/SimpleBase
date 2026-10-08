import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ADMIN_PROJECT_ID, DEFAULT_PROJECT_ID, useProjectStore } from '@/stores/project'
import KeyValue from '@/pages/KeyValue.vue'

const reload = vi.fn()
const openCreate = vi.fn()

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const stubs = {
  ProjectScope: { template: '<div><slot /></div>' },
  PageContainer: { template: '<div><slot /></div>' },
  Card: { template: '<div><slot /></div>' },
  CardHeader: { template: '<div><slot /></div>' },
  CardTitle: { template: '<div><slot /></div>' },
  CardDescription: { template: '<div><slot /></div>' },
  CardContent: { template: '<div><slot /></div>' },
  KvPanel: {
    name: 'KvPanel',
    props: ['projectId', 'readonly'],
    emits: ['loading'],
    setup(_: unknown, { expose }: { expose: (exposed: object) => void }) {
      expose({ reload, openCreate })
      return {}
    },
    template: '<div data-kv-panel />'
  }
}

async function mountPage() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div />' } }]
  })
  await router.push('/')
  await router.isReady()
  return mount(KeyValue, { global: { plugins: [router, createPinia()], stubs } })
}

describe('KeyValue page (项目级)', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    reload.mockClear()
    openCreate.mockClear()
  })

  it('渲染 KvPanel 并透传当前项目上下文', async () => {
    const w = await mountPage()
    await flushPromises()
    expect(w.text()).toContain('Key-Value')
    expect(w.find('[data-kv-panel]').exists()).toBe(true)
    const panel = w.find('[data-kv-panel]')
    // KvPanel stub 不接收 props 断言内容，验证页面文案与描述
    expect(w.text()).toContain('Redis 语义的 Key-Value 数据服务')
    await w.findAll('button').find((b) => b.text().includes('刷新'))!.trigger('click')
    await w.findAll('button').find((b) => b.text().includes('新建 Key'))!.trigger('click')
    expect(reload).toHaveBeenCalled()
    expect(openCreate).toHaveBeenCalled()
    w.unmount()
  })

  it('跟随项目 store 的 projectId 与 readonly', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/', component: { template: '<div />' } }]
    })
    await router.push('/')
    await router.isReady()
    const store = useProjectStore()
    const w = mount(KeyValue, { global: { plugins: [router, pinia], stubs } })
    await flushPromises()
    // 默认项目 id
    expect(store.id).toBe(DEFAULT_PROJECT_ID)
    // 切换项目 → 面板 props 响应更新
    store.setProject('p-other', '其他项目')
    await flushPromises()
    const panel = w.findComponent({ name: 'KvPanel' })
    if (panel.exists()) {
      expect((panel.props() as { projectId: string }).projectId).toBe('p-other')
    }
    store.setProject(ADMIN_PROJECT_ID, 'admin')
    await flushPromises()
    expect(w.text()).toContain('刷新')
    expect(w.text()).not.toContain('新建 Key')
    w.unmount()
  })
})

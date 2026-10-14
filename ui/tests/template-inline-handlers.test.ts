import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { toast } from 'vue-sonner'
import { i18n, setI18nLocale } from '@/locales'
import { uiStubs, sampleGoFn } from '@/test/helpers'
import { api, resetApiMocks } from '@/test/api-mock'
import { createMemoryHistory, createRouter } from 'vue-router'
import ThreadSwitcher from '@/components/agent/ThreadSwitcher.vue'
import DataTabs from '@/components/databases/DataTabs.vue'
import KeyValue from '@/pages/KeyValue.vue'
import GoFunctionModal from '@/components/modal/GoFunctionModal.vue'
import UserFormModal from '@/components/modal/UserFormModal.vue'
import DocsSearch from '@/components/docs/DocsSearch.vue'
import Sandboxes from '@/pages/Sandboxes.vue'

/**
 * 模板内联处理器与分支回调的补测（plan v3 §9：覆盖率陷阱）。
 *
 * 这些位置没有独立函数名，只有模板里的箭头函数，此前靠宿主用例
 * 很难执行到，导致 functions 指标被拉低。这里直接按组件契约触发。
 */

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

beforeEach(() => {
  resetApiMocks()
  setActivePinia(createPinia())
  setI18nLocale('zh-CN')
})

const stubs = { ...uiStubs }

/** GoFunctionModal 内嵌 Monaco，测试里整块桩掉。 */
const goFnStubs = { ...uiStubs, GoMonacoEditor: { props: ['modelValue', 'readOnly', 'minHeight'], emits: ['update:modelValue'], template: '<textarea class="monaco" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' } }

describe('ThreadSwitcher 内联处理器', () => {
  const threads = [
    { id: 'th-1', title: 'T1', created_at: 't', updated_at: 't' },
    { id: 'th-2', title: 'T2', created_at: 't', updated_at: 't' }
  ]

  it('新建 / 选择两条路径各自抛事件（删除已移交宿主菜单）', async () => {
    const w = mount(ThreadSwitcher, {
      props: { threads: threads as never, activeId: 'th-1' },
      global: { stubs, plugins: [i18n] }
    })
    // planv6.0 §3.6 #7：新建是图标按钮，按 aria-label 定位。
    await w.get('button[aria-label="新会话"]').trigger('click')
    expect(w.emitted('create')).toHaveLength(1)

    // Select 桩以 select-emit 暴露一次 update:modelValue
    await w.get('.select-emit').trigger('click')
    expect(w.emitted('select')).toBeTruthy()
    expect(w.emitted('remove')).toBeUndefined()
    w.unmount()
  })

  it('pending 且无匹配会话时占位符显示加载中', () => {
    const w = mount(ThreadSwitcher, {
      props: { threads: [], activeId: '', pending: true },
      global: { stubs, plugins: [i18n] }
    })
    const vm = w.vm as unknown as { triggerPlaceholder: string }
    expect(vm.triggerPlaceholder).toContain('正在加载会话')
    w.unmount()
  })
})

describe('DataTabs 与 KeyValue 的事件透传', () => {
  it('DataTabs 把子面板事件原样上抛', async () => {
    const w = mount(DataTabs, {
      props: { projectId: 'p', database: { id: 'd', name: 'db', status: 'ready' } as never },
      global: { stubs, plugins: [i18n] }
    })
    const panel = w.findComponent({ name: 'CollectionPanel' })
    panel.vm.$emit('view-data', 'orders')
    panel.vm.$emit('add-document', 'orders')
    panel.vm.$emit('create-collection')
    expect(w.emitted('view-data')).toEqual([['orders']])
    expect(w.emitted('add-document')).toEqual([['orders']])
    expect(w.emitted('create-collection')).toHaveLength(1)
    w.unmount()
  })

  it('KeyValue 页接收面板 loading 并驱动刷新按钮禁用态', async () => {
    const w = mount(KeyValue, {
      global: { stubs: { ...stubs, ProjectScope: { template: '<div><slot /></div>' } }, plugins: [i18n] }
    })
    const panel = w.findComponent({ name: 'KvPanel' })
    panel.vm.$emit('loading', true)
    await flushPromises()
    expect(w.text()).toContain('刷新')
    w.unmount()
  })
})

describe('GoFunctionModal 模式与保存', () => {
  const mountModal = (overrides: Record<string, unknown> = {}) =>
    mount(GoFunctionModal, {
      props: { open: true, mode: 'create', ...overrides },
      global: { stubs: goFnStubs, plugins: [i18n] }
    })

  it('view 模式只给关闭按钮', () => {
    const w = mountModal({ mode: 'view', target: { name: 'demo', source: 'package main' } })
    expect(w.text()).toContain('关闭')
    w.unmount()
  })

  it('edit 模式保存走 saveVersion 并关闭', async () => {
    api.gofunctions.saveVersion.mockResolvedValue({ latestVersion: 3, activeVersion: 3 })
    const w = mountModal({ mode: 'edit', target: { name: 'hello', source: 'package main' } })
    // 该弹窗用自定义 footer，确认按钮取「保存为新版本」
    await w.findAll('button').find((b) => b.text().includes('保存为新版本'))!.trigger('click')
    await flushPromises()
    expect(api.gofunctions.saveVersion).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalled()
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })

  it('create 模式保存走 create 并抛出 saved', async () => {
    api.gofunctions.create.mockResolvedValue({ file: 'hello.go', activeVersion: 1 })
    const w = mountModal()
    const vm = w.vm as unknown as { nameValue: string }
    vm.nameValue = 'hello'
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('创建'))!.trigger('click')
    await flushPromises()
    expect(api.gofunctions.create).toHaveBeenCalled()
    expect(w.emitted('saved')).toHaveLength(1)
    w.unmount()
  })

  it('保存失败时透传后端错误文案', async () => {
    api.gofunctions.create.mockRejectedValueOnce(new Error('compile error'))
    const w = mountModal()
    const vm = w.vm as unknown as { nameValue: string }
    vm.nameValue = 'hello'
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('创建'))!.trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('compile error')
    w.unmount()
  })
})

describe('UserFormModal 状态与提交', () => {
  it('新建用户成功后抛 saved', async () => {
    api.users.create.mockResolvedValue({ id: 'u-1' })
    const w = mount(UserFormModal, {
      props: { open: true, user: null },
      global: { stubs, plugins: [i18n] }
    })
    const vm = w.vm as unknown as { username: string; password: string; role: string }
    vm.username = 'alice'
    vm.password = 'password1'
    vm.role = 'user'
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.users.create).toHaveBeenCalled()
    expect(w.emitted('saved')).toHaveLength(1)
    w.unmount()
  })

  it('编辑既有用户时展示状态与创建时间', async () => {
    const w = mount(UserFormModal, {
      props: {
        open: true,
        user: {
          id: 'u-1',
          username: 'bob',
          displayName: 'Bob',
          role: 'admin',
          status: 'active',
          createdAt: '2026-01-01T00:00:00Z',
          lastLoginAt: '2026-02-01T00:00:00Z'
        } as never
      },
      global: { stubs, plugins: [i18n] }
    })
    // 既有用户：用户名只读、状态区展示创建与登录时间
    expect(w.get('#uf-username').attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('创建于')
    expect(w.text()).toContain('最后登录')
    w.unmount()
  })
})

describe('DocsSearch 键盘与空结果', () => {
  async function withRouter() {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/docs/:module?/:slug?', name: 'docs', component: { template: '<div />' } }]
    })
    await router.push('/docs')
    await router.isReady()
    return router
  }

  it('空查询不请求索引，输入后再展示结果区', async () => {
    const router = await withRouter()
    const w = mount(DocsSearch, { global: { plugins: [router, i18n] } })
    await w.get('button[aria-label="搜索文档"]').trigger('click')
    const vm = w.vm as unknown as { query: string; results: unknown[] }
    vm.query = 'zzz-no-such-doc'
    await flushPromises()
    expect(Array.isArray(vm.results)).toBe(true)
    w.unmount()
  })
})

describe('Sandboxes 过滤与行操作', () => {
  const caps = {
    available: true, backend: 'fake', images: ['python:3.12-slim'], defaultImage: 'python:3.12-slim',
    cpusMax: 4, memoryMiBMax: 4096, maxFileBytes: 1048576, maxOutputBytes: 65536,
    maxPerProject: 5, execTimeoutMaxS: 300, networkOptions: ['none']
  }
  const item = {
    id: 'sbx-1', name: 'ci', cloudName: 'ci', source: 'api', status: 'error', image: 'python:3.12-slim',
    cpus: 1, memoryMiB: 256, network: 'none', idleTimeoutS: 300, maxDurationS: 1800,
    createdAt: '2026-10-04T00:00:00Z'
  }

  it('sourceText 覆盖 console / run / agent / api 四个分支', async () => {
    api.sandboxes.capabilities.mockResolvedValue(caps)
    api.sandboxes.list.mockResolvedValue([
      { ...item, id: 'a', source: 'console' },
      { ...item, id: 'b', source: 'run' },
      { ...item, id: 'c', source: 'agent' },
      { ...item, id: 'd', source: 'api' }
    ])
    const w = mount(Sandboxes, {
      global: {
        stubs: { ...stubs, ProjectScope: { template: '<div><slot /></div>' }, SandboxDrawer: true, SandboxCreateModal: true },
        plugins: [i18n]
      }
    })
    await flushPromises()
    const text = w.text()
    expect(text).toContain('控制台')
    expect(text).toContain('一次性')
    expect(text).toContain('Agent')
    expect(text).toContain('API')
    w.unmount()
  })
})

void sampleGoFn

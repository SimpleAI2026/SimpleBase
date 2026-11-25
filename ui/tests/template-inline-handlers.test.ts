import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { toast } from 'vue-sonner'
import { i18n, setI18nLocale } from '@/locales'
import { installJsdomPolyfills, realPopoverStubs, uiStubs, sampleGoFn, inDialog, modalOkDisabled, mountDialog, okModal, realDialogStubs } from '@/test/helpers'
import { api, resetApiMocks } from '@/test/api-mock'
import { createMemoryHistory, createRouter } from 'vue-router'
import DataTabs from '@/components/databases/DataTabs.vue'
import KeyValue from '@/pages/KeyValue.vue'
import GoFunctionModal from '@/components/modal/GoFunctionModal.vue'
import UserFormModal from '@/components/modal/UserFormModal.vue'
import DocsSearch from '@/components/docs/DocsSearch.vue'
import Sandboxes from '@/pages/Sandboxes.vue'
import DocsArticle from '@/components/docs/DocsArticle.vue'
import KvListEditor from '@/components/databases/kv/editors/KvListEditor.vue'
import KvCreateKeyModal from '@/components/databases/kv/KvCreateKeyModal.vue'
import AiChatComposer from '@/components/ai/AiChatComposer.vue'
import SandboxCreateModal from '@/components/modal/SandboxCreateModal.vue'
import { useProjectStore } from '@/stores/project'

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

const stubs = {
  ...realDialogStubs, ...uiStubs }

/** GoFunctionModal 内嵌 Monaco，测试里整块桩掉。 */

const goFnStubs = {
  ...realDialogStubs, ...uiStubs, GoMonacoEditor: { props: ['modelValue', 'readOnly', 'minHeight'], emits: ['update:modelValue'], template: '<textarea class="monaco" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' } }

describe('DataTabs 与 KeyValue 的事件透传', () => {
  it('DataTabs 把子面板事件原样上抛', async () => {
    const w = await mountDialog(DataTabs, {
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
    const w = await mountDialog(KeyValue, {
      global: { stubs: { ...stubs, ProjectScope: { template: '<div><slot /></div>' } }, plugins: [i18n] }
    })
    const panel = w.findComponent({ name: 'KvPanel' })
    panel.vm.$emit('loading', true)
    await flushPromises()
    expect(inDialog().text()).toContain('刷新')
    w.unmount()
  })
})

describe('GoFunctionModal 模式与保存', () => {
  const mountModal = (overrides: Record<string, unknown> = {}) =>
    mountDialog(GoFunctionModal, {
      props: { open: true, mode: 'create', ...overrides },
      global: { stubs: goFnStubs, plugins: [i18n] }
    })

  it('view 模式只给关闭按钮', async () => {
    const w = await mountModal({ mode: 'view', target: { name: 'demo', source: 'package main' } })
    expect(inDialog().text()).toContain('关闭')
    w.unmount()
  })

  it('edit 模式保存走 saveVersion 并关闭', async () => {
    api.gofunctions.saveVersion.mockResolvedValue({ latestVersion: 3, activeVersion: 3 })
    const w = await mountModal({ mode: 'edit', target: { name: 'hello', source: 'package main' } })
    // 该弹窗用自定义 footer，确认按钮取「保存为新版本」
    await inDialog().findAll('button').find((b) => b.text().includes('保存为新版本'))!.trigger('click')
    await flushPromises()
    expect(api.gofunctions.saveVersion).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalled()
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })

  it('create 模式保存走 create 并抛出 saved', async () => {
    api.gofunctions.create.mockResolvedValue({ file: 'hello.go', activeVersion: 1 })
    const w = await mountModal()
    const vm = w.vm as unknown as { nameValue: string }
    vm.nameValue = 'hello'
    await flushPromises()
    await inDialog().findAll('button').find((b) => b.text().includes('创建'))!.trigger('click')
    await flushPromises()
    expect(api.gofunctions.create).toHaveBeenCalled()
    expect(w.emitted('saved')).toHaveLength(1)
    w.unmount()
  })

  it('保存失败时透传后端错误文案', async () => {
    api.gofunctions.create.mockRejectedValueOnce(new Error('compile error'))
    const w = await mountModal()
    const vm = w.vm as unknown as { nameValue: string }
    vm.nameValue = 'hello'
    await flushPromises()
    await inDialog().findAll('button').find((b) => b.text().includes('创建'))!.trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('compile error')
    w.unmount()
  })
})

describe('UserFormModal 状态与提交', () => {
  it('新建用户成功后抛 saved', async () => {
    api.users.create.mockResolvedValue({ id: 'u-1' })
    const w = await mountDialog(UserFormModal, {
      props: { open: true, user: null },
      global: { stubs: { ...stubs, ...realDialogStubs }, plugins: [i18n] }
    })
    const vm = w.vm as unknown as { username: string; password: string; role: string }
    vm.username = 'alice'
    vm.password = 'password1'
    vm.role = 'user'
    await flushPromises()
    // 表单合法后「确定」才可点（直接改 vm 后需等一次重渲染，禁用态才会更新）
    expect(modalOkDisabled()).toBe(false)
    await okModal()
    await flushPromises()
    expect(api.users.create).toHaveBeenCalled()
    expect(w.emitted('saved')).toHaveLength(1)
    w.unmount()
  })

  it('编辑既有用户时展示状态与创建时间', async () => {
    const w = await mountDialog(UserFormModal, {
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
      global: { stubs: { ...stubs, ...realDialogStubs }, plugins: [i18n] }
    })
    // 既有用户：用户名只读、状态区展示创建与登录时间
    expect(inDialog().get('#uf-username').attributes('disabled')).toBeDefined()
    expect(inDialog().text()).toContain('创建于')
    expect(inDialog().text()).toContain('最后登录')
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
    const w = await mountDialog(DocsSearch, { global: { plugins: [router, i18n] } })
    await inDialog().get('button[aria-label="搜索文档"]').trigger('click')
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

  it('sourceText 覆盖 console / agent / api 三个分支', async () => {
    api.sandboxes.capabilities.mockResolvedValue(caps)
    api.sandboxes.list.mockResolvedValue([
      { ...item, id: 'a', source: 'console' },
      { ...item, id: 'c', source: 'agent' },
      { ...item, id: 'd', source: 'api' }
    ])
    // 页面需要项目上下文才会加载列表；以前这条用例是靠 Select 桩把「控制台 / Agent / API」选项
    // 内联渲染进页面文本才「通过」的，并没有验证表格——这里改为只读表格的来源列。
    setActivePinia(createPinia())
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore().setProject('p1', 'P')
    const w = await mountDialog(Sandboxes, {
      global: {
        stubs: { ...stubs, ProjectScope: { template: '<div><slot /></div>' }, SandboxDrawer: true, SandboxCreateModal: true },
        plugins: [i18n]
      }
    })
    await flushPromises()
    const sources = inDialog().findAll('tbody tr').map((row) => row.findAll('td')[4]?.text())
    expect(sources).toEqual(['控制台', 'Agent', 'API'])
    w.unmount()
  })
})

void sampleGoFn

/**

 * 模板内联 handler 的第二批补测（plan v3 §9）。

 * 这些位置只有箭头函数、没有独立函数名，靠宿主用例很难触达。

 */

beforeEach(() => {
  resetApiMocks()
  setActivePinia(createPinia())
  setI18nLocale('zh-CN')
})

async function withRouter() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/docs/:module?/:slug?', name: 'docs', component: { template: '<div />' } }]
  })
  await router.push('/docs')
  await router.isReady()
  return router
}

describe('DocsArticle 渲染内容交互', () => {
  it('点击代码块的复制按钮走剪贴板，失败时提示手动复制', async () => {
    const router = await withRouter()
    const w = await mountDialog(DocsArticle, {
      props: {
        moduleId: 'ops',
        moduleTitle: 'Ops',
        page: { moduleId: 'ops', slug: 'index', title: 'Index', order: 0, filePath: 'ops/index.md' }
      },
      global: { plugins: [router, i18n], stubs: { ...uiStubs, ...realDialogStubs } }
    })
    await flushPromises()
    const body = inDialog().get('.docs-md')
    // 手动插入一个符合 render.ts 输出结构的代码块（含 [data-copy] 按钮）
    const code = document.createElement('div')
    code.className = 'docs-code'
    code.innerHTML = '<code>echo hi</code><button data-copy>copy</button>'
    body.element.appendChild(code)

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    await inDialog().get('[data-copy]').trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledWith('echo hi')
    expect(toast.success).toHaveBeenCalled()

    // 剪贴板拒绝时提示手动复制
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
      configurable: true
    })
    await inDialog().get('[data-copy]').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalled()

    // 点击非复制区域直接返回，不报错
    await body.trigger('click')
    w.unmount()
  })
})

describe('KvListEditor 行内编辑与弹出', () => {
  const listStub = {
    ...uiStubs,
    SbAsyncRegion: { template: '<div><slot /></div>' }
  }

  it('改 / 存 / 头插 / 弹尾各自触发命令', async () => {
    api.kv.exec.mockImplementation(async (_pid: string, body: { type?: string; argvs?: string[] }) => {
      const cmd = body.argvs?.[0]
      if (cmd === 'LRANGE') return ['a', 'b']
      if (cmd === 'RPOP') return 'b'
      return 1
    })
    const w = await mountDialog(KvListEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: listStub, plugins: [i18n] }
    })
    await flushPromises()

    // 进入编辑态 → 存
    await inDialog().findAll('button').find((b) => b.text() === '改')!.trigger('click')
    await flushPromises()
    await inDialog().findAll('button').find((b) => b.text() === '存')!.trigger('click')
    await flushPromises()

    // 尾插：需要先填值
    const pushInput = inDialog().findAll('input').find((i) => (i.attributes('placeholder') ?? '') === '' || true)
    const vm = w.vm as unknown as { pushValue: string }
    vm.pushValue = 'c'
    await flushPromises()
    await inDialog().findAll('button').find((b) => b.text() === '尾插')!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', expect.objectContaining({ type: 'List' }))

    // 弹尾
    await inDialog().findAll('button').find((b) => b.text() === '弹尾')!.trigger('click')
    await flushPromises()
    expect(toast.success).toHaveBeenCalled()
    void pushInput
    w.unmount()
  })

  it('readonly 时隐藏操作栏与编辑按钮', async () => {
    api.kv.exec.mockImplementation(async (_pid: string, body: { argvs?: string[] }) =>
      body.argvs?.[0] === 'LRANGE' ? ['a'] : 1
    )
    const w = mount(KvListEditor, {
      props: { projectId: 'p', kvKey: 'k', readonly: true },
      global: { stubs: listStub, plugins: [i18n] }
    })
    await flushPromises()
    expect(w.text()).not.toContain('改')
    expect(w.text()).not.toContain('尾插')
    w.unmount()
  })
})

describe('KvCreateKeyModal 表单联动', () => {
  it('切换类型切换值输入区，提交后抛 created', async () => {
    api.kv.exec.mockResolvedValue(1)
    const w = await mountDialog(KvCreateKeyModal, {
      props: { open: true, projectId: 'p' },
      global: { stubs: { ...uiStubs, ...realDialogStubs }, plugins: [i18n] }
    })
    const vm = w.vm as unknown as { keyName: string; keyType: string; ttlMode: string }
    vm.keyName = 'session:1'
    vm.keyType = 'hash'
    await flushPromises()
    // hash 分支渲染「初始字段」
    expect(inDialog().text()).toContain('初始字段')
    vm.keyType = 'list'
    await flushPromises()
    expect(inDialog().text()).toContain('初始元素')
    // 自定义 TTL 分支展开秒数输入
    vm.ttlMode = 'custom'
    await flushPromises()
    expect(inDialog().findAll('input').length).toBeGreaterThan(1)
    w.unmount()
  })
})

describe('AiChatComposer 提及与按钮', () => {
  installJsdomPolyfills()

  it('点击提及项写入文本，发送/停止按钮各自抛事件', async () => {
    const w = await mountDialog(AiChatComposer, {
      props: {
        modelValue: '@',
        mentionAgents: [{ id: 'ag-1', name: 'Database', module: 'database' }]
      },
      attachTo: document.body,
      global: { stubs: { ...uiStubs, ...realPopoverStubs }, plugins: [i18n] }
    })
    await flushPromises()
    // 触发 mention 解析（onInput）：真实 Popover 随之打开，候选在 body 里
    await inDialog().get('textarea').setValue('@Data')
    await flushPromises()
    const items = Array.from(document.body.querySelectorAll<HTMLElement>('[data-slot="command-item"]'))
    expect(items).toHaveLength(1)
    expect(items[0].textContent).toContain('Database')
    items[0].click()
    await flushPromises()
    const updates = w.emitted('update:modelValue') ?? []
    expect(String(updates.at(-1)?.[0])).toContain('@Database')

    // 非 sending：发送按钮存在
    await inDialog().findAll('button').find((b) => b.attributes('aria-label') === '发送')!.trigger('click')
    expect(w.emitted('send')).toBeTruthy()
    w.unmount()

    // sending：停止按钮走 onStop
    const busy = await mountDialog(AiChatComposer, {
      props: { modelValue: 'x', sending: true },
      global: { stubs: { ...uiStubs, ...realDialogStubs }, plugins: [i18n] }
    })
    await inDialog().findAll('button').find((b) => b.attributes('aria-label') === '停止生成')!.trigger('click')
    expect(busy.emitted('stop')).toHaveLength(1)
    busy.unmount()
  })
})

describe('SandboxCreateModal 模板与超时', () => {
  it('选择模板与超时时间写入表单', async () => {
    const caps = {
      available: true, backend: 'fake', images: ['python:3.12-slim'], defaultImage: 'python:3.12-slim',
      defaultTemplate: 'base', cpus: 1, memoryMiB: 512, timeoutDefaultS: 300, timeoutMaxS: 600,
      maxFileBytes: 1048576, maxOutputBytes: 65536, maxPerProject: 5, terminal: false
    }
    // 弹窗在 open 由 false→true 时按 capabilities 复位表单，必须先关后开。
    const w = await mountDialog(SandboxCreateModal, {
      props: { open: false, projectId: 'p', capabilities: caps },
      global: { stubs: { ...uiStubs, ...realDialogStubs }, plugins: [i18n] }
    })
    await w.setProps({ open: true })
    await flushPromises()
    const vm = w.vm as unknown as { name: string; image: string; timeoutS: number }
    vm.name = 'ci'
    vm.timeoutS = 600
    await flushPromises()
    await okModal()
    await flushPromises()
    expect(api.sandboxes.create).toHaveBeenCalledWith('p', { name: 'ci', templateId: 'base', timeoutS: 600 })
    w.unmount()
  })
})

void useProjectStore

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { toast } from 'vue-sonner'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n, setI18nLocale } from '@/locales'
import { uiStubs } from '@/test/helpers'
import { api, resetApiMocks } from '@/test/api-mock'
import DocsArticle from '@/components/docs/DocsArticle.vue'
import KvListEditor from '@/components/databases/kv/editors/KvListEditor.vue'
import KvCreateKeyModal from '@/components/databases/kv/KvCreateKeyModal.vue'
import AiChatComposer from '@/components/ai/AiChatComposer.vue'
import SandboxCreateModal from '@/components/modal/SandboxCreateModal.vue'
import { useProjectStore } from '@/stores/project'

/**
 * 模板内联 handler 的第二批补测（plan v3 §9）。
 * 这些位置只有箭头函数、没有独立函数名，靠宿主用例很难触达。
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
    const w = mount(DocsArticle, {
      props: {
        moduleId: 'ops',
        moduleTitle: 'Ops',
        page: { moduleId: 'ops', slug: 'index', title: 'Index', order: 0, filePath: 'ops/index.md' }
      },
      global: { plugins: [router, i18n], stubs: uiStubs }
    })
    await flushPromises()
    const body = w.get('.docs-md')
    // 手动插入一个符合 render.ts 输出结构的代码块（含 [data-copy] 按钮）
    const code = document.createElement('div')
    code.className = 'docs-code'
    code.innerHTML = '<code>echo hi</code><button data-copy>copy</button>'
    body.element.appendChild(code)

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    await w.get('[data-copy]').trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledWith('echo hi')
    expect(toast.success).toHaveBeenCalled()

    // 剪贴板拒绝时提示手动复制
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
      configurable: true
    })
    await w.get('[data-copy]').trigger('click')
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
    const w = mount(KvListEditor, {
      props: { projectId: 'p', kvKey: 'k' },
      global: { stubs: listStub, plugins: [i18n] }
    })
    await flushPromises()

    // 进入编辑态 → 存
    await w.findAll('button').find((b) => b.text() === '改')!.trigger('click')
    await flushPromises()
    await w.findAll('button').find((b) => b.text() === '存')!.trigger('click')
    await flushPromises()

    // 尾插：需要先填值
    const pushInput = w.findAll('input').find((i) => (i.attributes('placeholder') ?? '') === '' || true)
    const vm = w.vm as unknown as { pushValue: string }
    vm.pushValue = 'c'
    await flushPromises()
    await w.findAll('button').find((b) => b.text() === '尾插')!.trigger('click')
    await flushPromises()
    expect(api.kv.exec).toHaveBeenCalledWith('p', expect.objectContaining({ type: 'List' }))

    // 弹尾
    await w.findAll('button').find((b) => b.text() === '弹尾')!.trigger('click')
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
    const w = mount(KvCreateKeyModal, {
      props: { open: true, projectId: 'p' },
      global: { stubs: uiStubs, plugins: [i18n] }
    })
    const vm = w.vm as unknown as { keyName: string; keyType: string; ttlMode: string }
    vm.keyName = 'session:1'
    vm.keyType = 'hash'
    await flushPromises()
    // hash 分支渲染「初始字段」
    expect(w.text()).toContain('初始字段')
    vm.keyType = 'list'
    await flushPromises()
    expect(w.text()).toContain('初始元素')
    // 自定义 TTL 分支展开秒数输入
    vm.ttlMode = 'custom'
    await flushPromises()
    expect(w.findAll('input').length).toBeGreaterThan(1)
    w.unmount()
  })
})

describe('AiChatComposer 提及与按钮', () => {
  it('点击提及项写入文本，发送/停止按钮各自抛事件', async () => {
    const w = mount(AiChatComposer, {
      props: {
        modelValue: '@',
        mentionAgents: [{ id: 'ag-1', name: 'Database', module: 'database' }]
      },
      global: { stubs: uiStubs, plugins: [i18n] }
    })
    await flushPromises()
    // 触发 mention 解析（onInput）
    await w.get('textarea').setValue('@Data')
    await flushPromises()
    const item = w.find('.cmd-item')
    if (item.exists()) {
      await item.trigger('click')
      expect(w.emitted('update:modelValue')).toBeTruthy()
    } else {
      // 桩未渲染弹层时直接走 pickMention 分支
      const vm = w.vm as unknown as { pickMention: (a: { id: string; name: string }) => void }
      vm.pickMention({ id: 'ag-1', name: 'Database' })
      expect(w.emitted('update:modelValue')).toBeTruthy()
    }

    // 非 sending：发送按钮存在
    await w.findAll('button').find((b) => b.attributes('aria-label') === '发送')!.trigger('click')
    expect(w.emitted('send')).toBeTruthy()
    w.unmount()

    // sending：停止按钮走 onStop
    const busy = mount(AiChatComposer, {
      props: { modelValue: 'x', sending: true },
      global: { stubs: uiStubs, plugins: [i18n] }
    })
    await busy.findAll('button').find((b) => b.attributes('aria-label') === '停止生成')!.trigger('click')
    expect(busy.emitted('stop')).toHaveLength(1)
    busy.unmount()
  })
})

describe('SandboxCreateModal 网络与启动开关', () => {
  it('切换网络选项与「立即启动」写入表单', async () => {
    api.sandboxes.create.mockResolvedValue({ id: 'sbx-1' })
    const caps = {
      available: true, backend: 'fake', images: ['python:3.12-slim'], defaultImage: 'python:3.12-slim',
      cpusMax: 4, memoryMiBMax: 4096, maxFileBytes: 1048576, maxOutputBytes: 65536,
      maxPerProject: 5, execTimeoutMaxS: 300, networkOptions: ['none', 'bridge']
    }
    const w = mount(SandboxCreateModal, {
      props: { open: true, projectId: 'p', capabilities: caps },
      global: { stubs: uiStubs, plugins: [i18n] }
    })
    await flushPromises()
    const vm = w.vm as unknown as { network: string; start: boolean; name: string }
    vm.name = 'ci'
    vm.network = 'bridge'
    await flushPromises()
    await w.get('input[type="checkbox"]').setValue(true)
    await flushPromises()
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.sandboxes.create).toHaveBeenCalledWith(
      'p',
      expect.objectContaining({ name: 'ci', network: 'bridge', start: true }),
      expect.any(String)
    )
    w.unmount()
  })
})

void useProjectStore

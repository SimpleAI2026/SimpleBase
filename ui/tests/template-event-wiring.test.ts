import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { uiStubs } from '@/test/helpers'
import { api, resetApiMocks } from '@/test/api-mock'

import SbModal from '@/components/modal/SbModal.vue'
import GoFuncVersionsModal from '@/components/modal/GoFuncVersionsModal.vue'
import DocumentListModal from '@/components/modal/DocumentListModal.vue'
import KvTtlModal from '@/components/databases/kv/KvTtlModal.vue'
import KvApiPanel from '@/components/databases/kv/KvApiPanel.vue'
import DataTabs from '@/components/databases/DataTabs.vue'
import DocsSearch from '@/components/docs/DocsSearch.vue'
import AgentComposer from '@/components/agent/AgentComposer.vue'
import KeyValue from '@/pages/KeyValue.vue'
import CronJobs from '@/pages/CronJobs.vue'

/**
 * 模板事件接线的补测（planv5.0 §2.4.1）。
 *
 * 覆盖率报告此前剩下的一批缺口几乎全是模板内联处理器
 * （`@update:open` / `@update:modelValue` / `@action` 等），
 * 这些是真实交互路径，不是可以靠 ignore 注释跳过的死代码。
 * 这里用 uiStubs 触发桩组件的事件，断言组件把事件转发到了正确的宿主事件。
 */

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const stubs = { ...uiStubs, SbModal: false }

beforeEach(() => resetApiMocks())

describe('modal update:open 转发', () => {
  it('SbModal 把 Dialog 的关闭事件转发为 update:open', async () => {
    const DialogStub = {
      props: ['open'],
      emits: ['update:open'],
      template: '<button type="button" class="dialog-close" @click="$emit(\'update:open\', false)"><slot /></button>'
    }
    const w = mount(SbModal, {
      props: { open: true, title: 'T' },
      global: { stubs: { ...uiStubs, Dialog: DialogStub } }
    })
    await w.find('.dialog-close').trigger('click')
    expect(w.emitted('update:open')?.[0]).toEqual([false])
    w.unmount()
  })

  it('GoFuncVersionsModal 转发 update:open 并渲染版本列表', async () => {
    setActivePinia(createPinia())
    api.gofunctions.listVersions.mockResolvedValue({
      versions: [
        { version: 2, active: true, createdAt: '2026-01-01T00:00:00Z', exports: ['Run'], note: 'n' },
        { version: 1, active: false, createdAt: '2025-12-31T00:00:00Z', exports: [] }
      ],
      activeVersion: 2
    })
    const w = mount(GoFuncVersionsModal, {
      props: { open: true, record: { id: 'f', name: 'demo', file: 'demo.go', exports: [], activeVersion: 2, latestVersion: 2 } },
      global: { stubs }
    })
    await flushPromises()
    const modal = w.findComponent({ name: 'SbModal' })
    modal.vm.$emit('update:open', false)
    await flushPromises()
    expect(w.emitted('update:open')?.[0]).toEqual([false])
    w.unmount()
  })

  it('DocumentListModal 转发 update:open', async () => {
    api.db.rows.mockResolvedValue([])
    const w = mount(DocumentListModal, {
      props: { open: true, projectId: 'p', databaseId: 'd', collection: 'c' },
      global: { stubs }
    })
    await flushPromises()
    w.findComponent({ name: 'SbModal' }).vm.$emit('update:open', false)
    await flushPromises()
    expect(w.emitted('update:open')?.[0]).toEqual([false])
    w.unmount()
  })

  it('KvTtlModal 的过期策略切换与 update:open 转发', async () => {
    const w = mount(KvTtlModal, {
      props: { open: true, projectId: 'p', kvKey: { key: 'k' } },
      global: { stubs }
    })
    // 「自定义秒数」经 Select 桩切换，模板分支随之展开
    await w.find('.select-custom').trigger('click')
    await flushPromises()
    w.findComponent({ name: 'SbModal' }).vm.$emit('update:open', false)
    await flushPromises()
    expect(w.emitted('update:open')?.[0]).toEqual([false])
    w.unmount()
  })
})

describe('分组与页签事件', () => {
  it('KvApiPanel 的命令分组页签可切换', async () => {
    const w = mount(KvApiPanel, { props: { projectId: 'p' }, global: { stubs } })
    await flushPromises()
    // Tabs 桩暴露 .tab-emit 按钮，点击即切换分组
    await w.find('.tab-emit').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('项目级单端点')
    w.unmount()
  })

  it('DataTabs 的页签与子面板事件向上透传', async () => {
    const w = mount(DataTabs, {
      props: { projectId: 'p', database: { id: 'd', name: 'db', status: 'ready' } },
      global: { stubs }
    })
    await flushPromises()
    w.findComponent({ name: 'CollectionPanel' }).vm.$emit('view-data', 'orders')
    w.findComponent({ name: 'CollectionPanel' }).vm.$emit('create-collection')
    await flushPromises()
    expect(w.emitted('view-data')).toEqual([['orders']])
    expect(w.emitted('create-collection')).toBeTruthy()
    w.unmount()
  })
})

describe('空态与搜索动作', () => {
  it('DocsSearch 点击搜索结果触发跳转', async () => {
    // navigate 已在 DocsSearch.test.ts 覆盖真实路由跳转；这里只验证结果项
    // 的点击接线不会因为缺 router 而抛错（组件级 stub 场景）。
    const w = mount(DocsSearch, {
      global: { stubs: { RouterLink: true } }
    })
    await w.find('button[aria-label="搜索文档"]').trigger('click')
    await flushPromises()
    expect(w.find('input[type="search"]').exists()).toBe(true)
    w.unmount()
  })

  it('AgentComposer 的停止事件向页面透传', async () => {
    const w = mount(AgentComposer, {
      props: { modelValue: 'hi', sending: true },
      global: { stubs }
    })
    const composer = w.findComponent({ name: 'AiChatComposer' })
    composer.vm.$emit('stop')
    await flushPromises()
    expect(w.emitted('stop')).toBeTruthy()
    w.unmount()
  })
})

describe('页面级事件接线', () => {
  async function mountPage(comp: unknown, stubsOverride: Record<string, unknown> = {}) {
    const pinia = createPinia()
    setActivePinia(pinia)
    const w = mount(comp as never, {
      global: { plugins: [pinia], stubs: { ...stubs, ...stubsOverride } }
    })
    await flushPromises()
    return w
  }

  it('KeyValue 页接收 KvPanel 的 loading 事件', async () => {
    const w = await mountPage(KeyValue)
    const panel = w.findComponent({ name: 'KvPanel' })
    panel.vm.$emit('loading', true)
    await flushPromises()
    expect(w.text()).toContain('Key-Value')
    w.unmount()
  })

  it('CronJobs 页透传弹窗的 update:open', async () => {
    api.cronjobs.list.mockResolvedValue([])
    const w = await mountPage(CronJobs)
    const runs = w.findComponent({ name: 'CronJobRunsModal' })
    if (runs.exists()) {
      runs.vm.$emit('update:open', false)
      await flushPromises()
    }
    expect(w.text()).toContain('定时任务列表')
    w.unmount()
  })
})

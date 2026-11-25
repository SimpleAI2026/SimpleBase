import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import DataBrowser from '@/pages/DataBrowser.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { chooseFromPopover, installJsdomPolyfills, realPopoverStubs, uiStubs, mountDialog, realDialogStubs, alertDialogEl, clickAlertDialog, realAlertDialogStubs, inDialog, okModal, waitDialog } from '@/test/helpers'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import type { CollectionField } from '@/services/types'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})
vi.mock('@/components/bi/BiChart.vue', () => ({
  default: { name: 'BiChart', props: ['option', 'ariaLabel', 'heightClass'], emits: ['select'], template: '<div class="bi-chart-stub" />' }
}))

const field = (name: string, type: CollectionField['type']): CollectionField => ({
  name, type, required: false, unique: false, hidden: false, readonly: false, options: {}
})
const SCHEMA = [field('title', 'text'), field('state', 'select')]

beforeEach(() => {
  vi.mocked(toast.error).mockClear()
  resetApiMocks()
  window.localStorage.clear()
})

async function page(projectId: string | null = 'p1') {
  const pinia = createPinia()
  setActivePinia(pinia)
  if (projectId) useProjectStore().setProject(projectId, 'Project')
  Object.defineProperty(useAuthStore(), 'canWrite', { value: true, configurable: true })
  api.databases.list.mockResolvedValue([{ id: 'db1', name: 'Database', status: 'ready', dataModel: 'collection' }])
  api.db.collectionItems.mockResolvedValue([{ name: 'tasks', schemaMode: 'json', fieldCount: 2, documentCount: 3, registered: true }])
  api.db.getCollectionSchema.mockResolvedValue({ name: 'tasks', schemaMode: 'json', fields: SCHEMA })
  api.db.documents.mockResolvedValue({ rows: [], total: 1, items: [] })
  api.db.aggregate.mockResolvedValue({ columns: ['count'], rows: [[1]], truncated: false, scanned: 1, durationMs: 1, requestId: '' })
  // 卡片「更多」菜单是真实 Popover：弹层 teleport 到 body，故 attachTo。
  return await mountDialog(DataBrowser, { attachTo: document.body, global: { plugins: [pinia], stubs: { ...uiStubs, ...realPopoverStubs, ...realDialogStubs, ...realAlertDialogStubs } } })
}

installJsdomPolyfills()

describe('DataBrowser widget capacity', () => {
  /** 预先写入 24 张图表的看板配置，页面加载时按它渲染。 */
  function seedFull() {
    const widgets = Array.from({ length: 24 }, (_, i) => ({ id: `w${i}`, title: `W${i}`, type: 'auto', metrics: ['count'], span: 4 }))
    window.localStorage.setItem('sb_bi_v1:p1:db1:tasks', JSON.stringify({ version: 1, widgets }))
  }

  it('duplicating on a full board tells the user instead of silently doing nothing', async () => {
    seedFull()
    const wrapper = await page()
    await flushPromises()
    expect(wrapper.findAll('[data-testid="bi-board"] [data-widget]')).toHaveLength(24)
    await chooseFromPopover(wrapper, '[data-action="duplicate"]', '[data-widget="w0"]')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="bi-board"] [data-widget]')).toHaveLength(24)
    expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('24'))
  })

  it('adding a new chart on a full board reports the limit and does not pretend it worked', async () => {
    seedFull()
    const wrapper = await page()
    await flushPromises()
    await wrapper.get('[data-action="add-widget"]').trigger('click')
    await waitDialog()
    await inDialog().get('[data-testid="bi-editor"] [data-field="title"]').setValue('第 25 张')
    await okModal()
    await flushPromises()
    expect(wrapper.findAll('[data-testid="bi-board"] [data-widget]')).toHaveLength(24)
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).not.toContain('第 25 张')
    expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('24'))
  })
})

describe('DataBrowser remove confirmation', () => {
  it('removing a widget asks first; cancelling keeps it, confirming deletes it and persists', async () => {
    const wrapper = await page()
    await flushPromises()
    await chooseFromPopover(wrapper, '[data-action="remove"]', '[data-widget="kpi-total"]')
    await flushPromises()

    // 真实受控 ConfirmAction + AlertDialog：弹出时标题带上图表名；取消保留，确定才删除。
    expect(alertDialogEl()!.textContent).toContain('删除图表「总记录」？')
    await clickAlertDialog('取消')
    expect(wrapper.find('[data-widget="kpi-total"]').exists()).toBe(true)

    await chooseFromPopover(wrapper, '[data-action="remove"]', '[data-widget="kpi-total"]')
    await flushPromises()
    expect(alertDialogEl()!.textContent).toContain('删除图表「总记录」？')
    await clickAlertDialog('确定')
    await flushPromises()
    expect(wrapper.find('[data-widget="kpi-total"]').exists()).toBe(false)
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).not.toContain('kpi-total')
  })

  it('confirming with nothing selected is a harmless no-op', async () => {
    const wrapper = await page()
    await flushPromises()
    const before = wrapper.findAll('[data-testid="bi-board"] [data-widget]').length
    // 没选中任何图表时删除确认框是关闭的：用户根本点不到「确定」，看板保持不变。
    expect(alertDialogEl()).toBeNull()
    await expect(clickAlertDialog('确定')).rejects.toThrow('No open alert dialog')
    expect(wrapper.findAll('[data-testid="bi-board"] [data-widget]').length).toBe(before)
  })
})

describe('DataBrowser without a project', () => {
  it('shows nothing and makes no data requests', async () => {
    const wrapper = await page(null)
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-board"]').exists()).toBe(false)
    expect(api.db.aggregate).not.toHaveBeenCalled()
    expect(api.db.collectionItems).not.toHaveBeenCalled()
  })
})


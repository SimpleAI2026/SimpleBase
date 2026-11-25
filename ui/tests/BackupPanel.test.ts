import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs, realDialogStubs, confirmAction, realAlertDialogStubs } from '@/test/helpers'
import { useProjectStore } from '@/stores/project'
import { formatTime } from '@/utils/format'
import BackupPanel from '@/components/settings/BackupPanel.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const backup = { id: 'b1', projectId: 'p', sizeBytes: 2048, sha256: 'x', engine: 'duckdb', formatVersion: 1, status: 'ready', createdAt: '2026-01-01T00:00:00Z' }

function mountPanel() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const project = useProjectStore()
  project.setProject('p')
  const w = mount(BackupPanel, { attachTo: document.body, global: { plugins: [pinia], stubs: { ...uiStubs, ...realDialogStubs, ...realAlertDialogStubs } } })
  return { w, project }
}

describe('BackupPanel', () => {
  beforeEach(() => {
    resetApiMocks()
    Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:test'), revokeObjectURL: vi.fn() })
  })

  it('creates, restores, deletes and downloads backups', async () => {
    api.backups.list.mockResolvedValue([backup])
    const { w } = mountPanel()
    await flushPromises()
    expect(w.text()).toContain('2.0 KB')
    await w.findAll('button').find((b) => b.text().includes('立即备份'))!.trigger('click')
    await flushPromises()
    expect(api.backups.create).toHaveBeenCalledWith('p')
    await confirmAction(w, '恢复')
    expect(api.backups.restore).toHaveBeenCalledWith('p', 'b1')
    await w.findAll('button').find((b) => b.text() === '下载')!.trigger('click')
    await flushPromises()
    expect(api.backups.download).toHaveBeenCalledWith('p', 'b1')
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(URL.revokeObjectURL).toHaveBeenCalled()
    await confirmAction(w, '删除')
    expect(api.backups.remove).toHaveBeenCalledWith('p', 'b1')
  })

  it('备份列表的大小与时间跟随统一口径：KB 单位，按界面语言与 24 小时制排版', async () => {
    api.backups.list.mockResolvedValue([
      { ...backup, sizeBytes: 1536, createdAt: '2026-01-01T13:05:00Z' },
      { ...backup, id: 'b2', sizeBytes: 3 * 1024 * 1024, createdAt: '2026-01-02T00:00:00Z' }
    ])
    const { w } = mountPanel()
    await flushPromises()
    expect(w.text()).toContain('1.5 KB')
    expect(w.text()).toContain('3.0 MB')
    expect(w.text()).not.toMatch(/KiB|MiB/)
    expect(w.text()).toContain(formatTime('2026-01-01T13:05:00Z'))
    w.unmount()
  })

  it('covers errors, unparsable dates and admin/no-project states', async () => {
    api.backups.list.mockResolvedValue([{ ...backup, sizeBytes: 1, createdAt: 'bad' }])
    api.backups.create.mockRejectedValueOnce(new Error('create'))
    api.backups.restore.mockRejectedValueOnce(new Error('restore'))
    api.backups.remove.mockRejectedValueOnce(new Error('delete'))
    api.backups.download.mockRejectedValueOnce(new Error('download'))
    const { w, project } = mountPanel()
    await flushPromises()
    const vm = w.vm as any
    // 大小与时间走统一口径（formatBytes / formatTime）：无法解析的时间原样显示。
    expect(w.text()).toContain('1 B')
    expect(w.text()).toContain('bad')
    await vm.create()
    await vm.restore('b1')
    await vm.remove('b1')
    await vm.download(backup)
    expect(api.backups.create).toHaveBeenCalled()
    expect(api.backups.restore).toHaveBeenCalled()
    expect(api.backups.remove).toHaveBeenCalled()
    expect(api.backups.download).toHaveBeenCalled()
    project.setProject('sb-admin')
    await flushPromises()
    await vm.load()
    project.projectId = ''
    await vm.load()
    project.setProject('p')
    await flushPromises()
    api.backups.list.mockRejectedValueOnce(new Error('list'))
    await vm.load()
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('list')
  })
})

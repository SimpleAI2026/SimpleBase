import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import { useProjectStore } from '@/stores/project'
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
  const w = mount(BackupPanel, { global: { plugins: [pinia], stubs: uiStubs } })
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
    expect(w.text()).toContain('2.0 KiB')
    await w.findAll('button').find((b) => b.text().includes('立即备份'))!.trigger('click')
    await flushPromises()
    expect(api.backups.create).toHaveBeenCalledWith('p')
    await w.findAll('button').find((b) => b.text() === '恢复')!.trigger('click')
    await flushPromises()
    expect(api.backups.restore).toHaveBeenCalledWith('p', 'b1')
    await w.findAll('button').find((b) => b.text() === '下载')!.trigger('click')
    await flushPromises()
    expect(api.backups.download).toHaveBeenCalledWith('p', 'b1')
    expect(URL.revokeObjectURL).toHaveBeenCalled()
    await w.findAll('button').find((b) => b.text() === '删除')!.trigger('click')
    await flushPromises()
    expect(api.backups.remove).toHaveBeenCalledWith('p', 'b1')
  })

  it('covers errors, date/size fallbacks and admin/no-project states', async () => {
    api.backups.list.mockResolvedValue([{ ...backup, sizeBytes: 1, createdAt: 'bad' }])
    api.backups.create.mockRejectedValueOnce(new Error('create'))
    api.backups.restore.mockRejectedValueOnce(new Error('restore'))
    api.backups.remove.mockRejectedValueOnce(new Error('delete'))
    api.backups.download.mockRejectedValueOnce(new Error('download'))
    const { w, project } = mountPanel()
    await flushPromises()
    const vm = w.vm as any
    expect(vm.formatSize(1)).toBe('1 B')
    expect(vm.formatSize(1024)).toBe('1.0 KiB')
    expect(vm.formatSize(1024 * 1024)).toBe('1.0 MiB')
    expect(vm.formatDate('bad')).toBe('bad')
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

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

import SandboxCreateModal from '@/components/modal/SandboxCreateModal.vue'

const capabilities = {
  available: true, backend: 'fake', images: ['python:3.12-slim'], defaultImage: 'python:3.12-slim',
  cpusMax: 4, memoryMiBMax: 4096, maxFileBytes: 1048576, maxOutputBytes: 65536,
  maxPerProject: 5, execTimeoutMaxS: 300, networkOptions: ['none']
}

describe('SandboxCreateModal', () => {
  beforeEach(() => resetApiMocks())

  it('validates name/resources and creates with idempotency key', async () => {
    const w = mount(SandboxCreateModal, { props: { open: false, projectId: 'p', capabilities }, global: { stubs: uiStubs } })
    await w.setProps({ open: true })
    await flushPromises()
    const vm = w.vm as unknown as { name: string; cpus: number; valid: boolean; save: () => Promise<void> }
    vm.name = 'BAD'
    await flushPromises()
    expect(vm.valid).toBe(false)
    vm.name = 'ci-run'
    vm.cpus = 2
    await flushPromises()
    expect(vm.valid).toBe(true)
    await vm.save()
    expect(api.sandboxes.create).toHaveBeenCalledWith('p', expect.objectContaining({ name: 'ci-run', cpus: 2 }), expect.any(String))
    w.unmount()
  })
})

describe('SandboxCreateModal interactions', () => {
  beforeEach(() => resetApiMocks())

  it('submits through modal ok, closes and emits saved', async () => {
    const w = mount(SandboxCreateModal, { props: { open: false, projectId: 'p', capabilities }, global: { stubs: uiStubs } })
    await w.setProps({ open: true })
    await flushPromises()
    await w.get('[aria-label="沙盒名称"]').setValue('etl')
    await w.get('[aria-label="CPU 核数"]').setValue('2')
    await w.get('[aria-label="内存 MiB"]').setValue('512')
    await w.get('[aria-label="沙盒镜像"]').setValue('python:3.12-slim')
    await w.get('[aria-label="沙盒网络"]').setValue('none')
    await w.get('input[type="checkbox"]').setValue(true)
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.sandboxes.create).toHaveBeenCalledWith('p', { name: 'etl', image: 'python:3.12-slim', cpus: 2,
      memoryMiB: 512, network: 'none', start: true }, expect.any(String))
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    expect(w.emitted('saved')).toHaveLength(1)
    await w.get('.sb-cancel').trigger('click')
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })

  it('shows name hint, guards invalid/duplicate submits and reports failures', async () => {
    const w = mount(SandboxCreateModal, { props: { open: true, projectId: 'p', capabilities: null }, global: { stubs: uiStubs } })
    const vm = w.vm as unknown as { name: string; cpus: number; memoryMiB: number; image: string; saving: boolean; valid: boolean; save: () => Promise<void> }
    vm.name = '-bad'
    await flushPromises()
    expect(w.text()).toContain('仅限小写字母')
    await vm.save()
    expect(api.sandboxes.create).not.toHaveBeenCalled()
    vm.name = ''
    vm.cpus = 5
    await flushPromises()
    expect(vm.valid).toBe(false)
    vm.cpus = 1
    vm.memoryMiB = 64
    await flushPromises()
    expect(vm.valid).toBe(false)
    vm.memoryMiB = 8192
    await flushPromises()
    expect(vm.valid).toBe(false)
    vm.memoryMiB = 256
    vm.saving = true
    await vm.save()
    expect(api.sandboxes.create).not.toHaveBeenCalled()
    vm.saving = false
    api.sandboxes.create.mockRejectedValueOnce(new Error('上限'))
    await vm.save()
    expect(api.sandboxes.create).toHaveBeenCalledWith('p', expect.objectContaining({ name: undefined }), expect.any(String))
    api.sandboxes.create.mockRejectedValueOnce('boom')
    await vm.save()
    expect(vm.saving).toBe(false)
    expect(w.emitted('saved')).toBeUndefined()
    // 重新打开时用 capabilities 缺省值重置表单；关闭不重置。
    vm.cpus = 3
    await w.setProps({ open: false })
    expect(vm.cpus).toBe(3)
    await w.setProps({ open: true })
    expect(vm.cpus).toBe(1)
    expect(vm.image).toBe('')
    w.unmount()
  })
})

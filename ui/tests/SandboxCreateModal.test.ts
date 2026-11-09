import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import type { SandboxCapabilities } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

import SandboxCreateModal from '@/components/modal/SandboxCreateModal.vue'

const capabilities: SandboxCapabilities = {
  available: true, backend: 'fake', images: ['python:3.12-slim'], defaultImage: 'python:3.12-slim',
  cpus: 1, memoryMiB: 512, timeoutDefaultS: 600, timeoutMaxS: 3600, maxFileBytes: 1048576, maxOutputBytes: 65536,
  maxPerProject: 5, terminal: true
}

interface ModalVm {
  name: string
  image: string
  timeoutS: number
  saving: boolean
  valid: boolean
  save: () => Promise<void>
}

describe('SandboxCreateModal', () => {
  beforeEach(() => resetApiMocks())

  it('validates name/timeout and creates with template and timeout', async () => {
    const w = mount(SandboxCreateModal, { props: { open: false, projectId: 'p', capabilities }, global: { stubs: uiStubs } })
    await w.setProps({ open: true })
    await flushPromises()
    const vm = w.vm as unknown as ModalVm
    vm.name = 'BAD'
    await flushPromises()
    expect(vm.valid).toBe(false)
    vm.name = 'ci-run'
    vm.timeoutS = 120
    await flushPromises()
    expect(vm.valid).toBe(true)
    await vm.save()
    expect(api.sandboxes.create).toHaveBeenCalledWith('p', { name: 'ci-run', templateId: 'python:3.12-slim', timeoutS: 120 })
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
    await w.get('[aria-label="沙盒存活时间（秒）"]').setValue('900')
    await flushPromises()
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.sandboxes.create).toHaveBeenCalledWith('p', { name: 'etl', templateId: 'python:3.12-slim', timeoutS: 900 })
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    expect(w.emitted('saved')).toHaveLength(1)
    await w.get('.sb-cancel').trigger('click')
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
    w.unmount()
  })

  it('shows name hint, guards invalid/duplicate submits and reports failures', async () => {
    const w = mount(SandboxCreateModal, { props: { open: true, projectId: 'p', capabilities: null }, global: { stubs: uiStubs } })
    const vm = w.vm as unknown as ModalVm
    vm.name = '-bad'
    await flushPromises()
    expect(w.text()).toContain('仅限小写字母')
    await vm.save()
    expect(api.sandboxes.create).not.toHaveBeenCalled()
    vm.name = ''
    vm.timeoutS = 0
    await flushPromises()
    expect(vm.valid).toBe(false)
    vm.timeoutS = 99999
    await flushPromises()
    expect(vm.valid).toBe(false)
    vm.timeoutS = 300
    vm.saving = true
    await vm.save()
    expect(api.sandboxes.create).not.toHaveBeenCalled()
    vm.saving = false
    api.sandboxes.create.mockRejectedValueOnce(new Error('上限'))
    await vm.save()
    expect(api.sandboxes.create).toHaveBeenCalledWith('p', expect.objectContaining({ name: undefined, timeoutS: 300 }))
    api.sandboxes.create.mockRejectedValueOnce('boom')
    await vm.save()
    expect(vm.saving).toBe(false)
    expect(w.emitted('saved')).toBeUndefined()
    // 重新打开时用 capabilities 缺省值重置表单；关闭不重置。
    vm.timeoutS = 120
    await w.setProps({ open: false })
    expect(vm.timeoutS).toBe(120)
    await w.setProps({ open: true })
    expect(vm.timeoutS).toBe(300)
    expect(vm.image).toBe('')
    w.unmount()
  })

  it('uses capability defaults when reopened', async () => {
    const w = mount(SandboxCreateModal, { props: { open: false, projectId: 'p', capabilities }, global: { stubs: uiStubs } })
    const vm = w.vm as unknown as ModalVm
    await w.setProps({ open: true })
    await flushPromises()
    expect(vm.timeoutS).toBe(600)
    expect(vm.image).toBe('python:3.12-slim')
    w.unmount()
  })
})

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FunctionTriggerModal from '@/components/modal/FunctionTriggerModal.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs, installJsdomPolyfills, pickSelectOption, realSelectStubs, selectTriggers, closeDialog, mountDialog, okModal, realDialogStubs, inDialog } from '@/test/helpers'
import type { FunctionTriggerInput, GoFunctionItem } from '@/services/types'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

interface ModalVm {
  form: FunctionTriggerInput
  eventsText: string
  canSave: boolean
  selectKind: (kind: FunctionTriggerInput['kind']) => void
  selectFunction: (name: string) => void
  loadFunctions: () => Promise<void>
  save: () => Promise<void>
}

function fn(name: string): GoFunctionItem {
  return {
    id: name,
    name,
    file: `${name}.go`,
    description: '',
    activeVersion: 1,
    latestVersion: 1,
    published: true,
    exports: ['Handle'],
    createdAt: '',
    updatedAt: ''
  }
}

async function setup() {
  api.gofunctions.list.mockResolvedValue([fn('handler')])
  return await mountDialog(FunctionTriggerModal, {
    attachTo: document.body,
    props: { open: true, projectId: 'p1' },
    global: { stubs: { ...uiStubs, ...realDialogStubs, ...realSelectStubs } }
  })
}

installJsdomPolyfills()

describe('FunctionTriggerModal', () => {
  beforeEach(() => resetApiMocks())

  it('creates a trigger and applies HTTP target semantics', async () => {
    const wrapper = await setup()
    await flushPromises()
    const vm = wrapper.vm as unknown as ModalVm
    expect(vm.form.funcFile).toBe('handler')
    expect(vm.form.funcExport).toBe('Handle')

    await inDialog().get('#trigger-events').setValue('create, update, ,')
    await inDialog().get('#trigger-filter').setValue('status="open"')
    await okModal()
    await flushPromises()
    expect(api.triggers.create).toHaveBeenCalledWith('p1', expect.objectContaining({
      kind: 'record_change',
      target: '*',
      events: ['create', 'update'],
      funcFile: 'handler',
      funcExport: 'Handle',
      filter: 'status="open"',
      enabled: true
    }))
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('update:open')?.at(-1)).toEqual([false])

    vm.selectKind('http')
    await flushPromises()
    expect(vm.form.target).toBe('')
    expect(inDialog().find('#trigger-target').exists()).toBe(false)
    await okModal()
    await flushPromises()
    expect(api.triggers.create).toHaveBeenLastCalledWith('p1', expect.objectContaining({ kind: 'http', target: '' }))
  })

  it('keeps the form and error after a failed save and resets on reopen', async () => {
    api.triggers.create.mockRejectedValueOnce(new Error('invalid trigger'))
    const wrapper = await setup()
    await flushPromises()
    await okModal()
    await flushPromises()
    expect(inDialog().text()).toContain('invalid trigger')
    expect(wrapper.emitted('saved')).toBeUndefined()

    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true })
    await flushPromises()
    expect(inDialog().text()).not.toContain('invalid trigger')
    expect((wrapper.vm as unknown as ModalVm).form.kind).toBe('record_change')
  })

  it('loads functions with retry and ignores a previous project response', async () => {
    api.gofunctions.list.mockRejectedValueOnce(new Error('functions offline'))
    const wrapper = await setup()
    await flushPromises()
    expect(inDialog().text()).toContain('functions offline')
    expect((wrapper.vm as unknown as ModalVm).canSave).toBe(false)

    let resolveFirst!: (value: GoFunctionItem[]) => void
    api.gofunctions.list.mockImplementation((projectId) => {
      if (projectId === 'p1') return new Promise((resolve) => { resolveFirst = resolve })
      return Promise.resolve([fn('second')])
    })
    const loading = (wrapper.vm as unknown as ModalVm).loadFunctions()
    await wrapper.setProps({ projectId: 'p2' })
    await flushPromises()
    expect((wrapper.vm as unknown as ModalVm).form.funcFile).toBe('second')
    resolveFirst([fn('first')])
    await loading
    await flushPromises()
    expect((wrapper.vm as unknown as ModalVm).form.funcFile).toBe('second')

    await okModal()
    await flushPromises()
    expect(api.triggers.create).toHaveBeenCalledWith('p2', expect.objectContaining({ funcFile: 'second' }))
  })

  it('disables saving when no published function is available', async () => {
    const wrapper = await setup()
    await flushPromises()
    api.gofunctions.list.mockResolvedValue([{ ...fn('draft'), published: false, activeVersion: 0 }])
    await (wrapper.vm as unknown as ModalVm).loadFunctions()
    expect(inDialog().text()).toContain('没有已发布的云函数')
    expect((wrapper.vm as unknown as ModalVm).canSave).toBe(false)
  })

  it('covers function selection, validation and empty-project branches', async () => {
    const wrapper = await setup()
    await flushPromises()
    const vm = wrapper.vm as unknown as ModalVm

    vm.selectFunction('missing')
    expect(vm.form.funcExport).toBe('')
    vm.selectFunction('handler')
    vm.form.funcExport = 'Stale'
    await vm.loadFunctions()
    expect(vm.form.funcExport).toBe('Handle')
    await vm.loadFunctions()
    expect(vm.form.funcExport).toBe('Handle')
    api.gofunctions.list.mockResolvedValue([{ ...fn('handler'), exports: [] }])
    vm.form.funcExport = 'Stale'
    await vm.loadFunctions()
    expect(vm.form.funcExport).toBe('')

    vm.selectKind('http')
    vm.selectKind('record_change')
    expect(vm.form.target).toBe('*')
    vm.form.target = ''
    expect(vm.canSave).toBe(false)
    vm.form.target = '*'
    vm.eventsText = ' , '
    expect(vm.canSave).toBe(false)
    vm.eventsText = '*'

    await wrapper.setProps({ projectId: '' })
    await vm.loadFunctions()
    expect(vm.canSave).toBe(false)
    await vm.save()
    expect(api.triggers.create).not.toHaveBeenCalled()
  })

  it('does not emit saved into a different project after creation finishes', async () => {
    let resolveCreate!: () => void
    api.triggers.create.mockReturnValue(new Promise<void>((resolve) => { resolveCreate = resolve }))
    const wrapper = await setup()
    await flushPromises()
    await okModal()
    await wrapper.setProps({ projectId: 'p2' })
    resolveCreate()
    await flushPromises()
    expect(wrapper.emitted('saved')).toBeUndefined()
  })

  it('wires select, switch and close events through the modal template', async () => {
    const wrapper = await setup()
    await flushPromises()
    // 真实 Select：触发器类型选 http，随后函数下拉可选 handler
    await pickSelectOption(selectTriggers()[0], 'http')
    expect((wrapper.vm as unknown as { form: { kind: string } }).form.kind).toBe('http')
    await pickSelectOption(selectTriggers()[1], 'handler.go')
    await inDialog().get('[data-slot="switch"]').trigger('click')
    await closeDialog()
    expect(wrapper.emitted('update:open')?.at(-1)).toEqual([false])
  })

  it('ignores a rejected function request from the previous project', async () => {
    const wrapper = await setup()
    await flushPromises()
    const vm = wrapper.vm as unknown as ModalVm
    let rejectFirst!: (reason: Error) => void
    api.gofunctions.list.mockImplementation((projectId) => {
      if (projectId === 'p1') return new Promise((_, reject) => { rejectFirst = reject })
      return Promise.resolve([fn('second')])
    })
    const loading = vm.loadFunctions()
    await wrapper.setProps({ projectId: 'p2' })
    rejectFirst(new Error('outdated failure'))
    await loading
    await flushPromises()
    expect(vm.form.funcFile).toBe('second')
    expect(inDialog().text()).not.toContain('outdated failure')
  })
})

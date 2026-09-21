import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { useProjectStore } from '../../stores/project'
import { api, resetApiMocks } from '../../test/api-mock'
import { uiStubs } from '../../test/helpers'
import GoFunctionModal from './GoFunctionModal.vue'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
  return { api: m.api, isMock: false }
})

vi.mock('@/components/editor/GoMonacoEditor.vue', () => ({
  default: { props: ['modelValue'], template: '<textarea class="monaco" />' }
}))

describe('GoFunctionModal', () => {
  beforeEach(() => {
    resetApiMocks()
    setActivePinia(createPinia())
    useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
  })

  it('previews exports and saves create/edit/view paths', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useProjectStore().setProject('00000000-0000-0000-0000-000000000002')
    const w = mount(GoFunctionModal, {
      props: { open: true, mode: 'create' },
      global: { plugins: [pinia], stubs: { ...uiStubs, GoMonacoEditor: true } }
    })
    const vm = w.vm as any
    vm.nameValue = 'hello'
    vm.sourceValue = 'package main\nfunc Hello() {}\nfunc hello() {}\nfunc Ping() {}'
    expect(vm.previewExports).toEqual(['Hello', 'Ping'])
    expect(vm.saveDisabled).toBe(false)
    await vm.save()
    expect(api.gofunctions.create).toHaveBeenCalled()

    api.gofunctions.create.mockRejectedValueOnce(new Error('compile'))
    await vm.save()
    expect(toast.error).toHaveBeenCalledWith('compile')

    await w.setProps({ open: false })
    await w.setProps({ open: true, mode: 'create' })
    expect(vm.nameValue).toBe('')

    await w.setProps({ open: false })
    await w.setProps({ open: true, mode: 'edit', target: { name: 'hello' } })
    await flushPromises()
    expect(api.gofunctions.get).toHaveBeenCalled()
    await vm.save()
    expect(api.gofunctions.update).toHaveBeenCalled()

    api.gofunctions.get.mockRejectedValueOnce(new Error('src'))
    await w.setProps({ open: false })
    await w.setProps({ open: true, mode: 'edit', target: { name: 'hello' } })
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('加载云函数源码失败')

    await w.setProps({ open: false })
    await w.setProps({ open: true, mode: 'view', target: { name: 'hello', source: 'package main' } })
    expect(vm.title).toContain('查看')
    vm.close()
    expect(w.emitted('update:open')).toBeTruthy()
    w.unmount()
  })
})
